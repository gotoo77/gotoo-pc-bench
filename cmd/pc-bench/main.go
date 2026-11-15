package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gotoo77/gotoo-pc-bench/internal/bench"
	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

//go:embed web/*
var assets embed.FS

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Erreur:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`pc-bench : benchmarks CPU/mémoire Linux et visualisation locale des résultats

  pc-bench check   --test cpu|memory (préflight sans modification système)
  pc-bench run     --test cpu|memory --label avant_upgrade --reports DIR
  pc-bench serve   --reports ~/Téléchargements [--listen 127.0.0.1:8765]
  pc-bench list    --reports ~/Téléchargements
  pc-bench export  --reports ~/Téléchargements --from rapport_...txt
  pc-bench compare --reports ~/Téléchargements --before rapport_...txt --after rapport_...txt

Serveur HTTP en lecture seule. Runners CPU/mémoire disponibles uniquement via CLI, sans root.
`)
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		usage()
		return nil
	}
	cmd := args[0]
	flags := flag.NewFlagSet(cmd, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	dir := flags.String("reports", ".", "Répertoire contenant les rapports .txt/.json")
	testName := flags.String("test", "cpu", "Protocole de benchmark (cpu ou memory)")
	label := flags.String("label", "", "Label de la campagne à créer (run)")
	listen := flags.String("listen", "127.0.0.1:8765", "Adresse locale HTTP (serve)")
	before := flags.String("before", "", "Fichier rapport avant")
	after := flags.String("after", "", "Fichier rapport après")
	source := flags.String("from", "", "Rapport texte à exporter en JSON")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return errors.New("arguments positionnels non reconnus")
	}
	path, err := report.ValidateDir(*dir)
	if err != nil {
		return err
	}
	switch cmd {
	case "check":
		if *testName != "cpu" && *testName != "memory" {
			return fmt.Errorf("unknown or unsupported benchmark %q", *testName)
		}
		_, v, err := bench.CheckSysbench()
		if err != nil {
			return err
		}
		fmt.Printf("%s benchmark preflight: PASS (sysbench %s)\n", strings.ToUpper(*testName), v)
		return nil
	case "run":
		if *testName != "cpu" && *testName != "memory" {
			return fmt.Errorf("unknown or unsupported benchmark %q", *testName)
		}
		if *label == "" {
			return errors.New("--label requis pour run")
		}
		unlock, err := bench.LockBenchmark(path)
		if err != nil {
			return err
		}
		defer unlock()
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		var result report.Run
		if *testName == "memory" {
			result, err = bench.RunMemory(ctx, *label, func(msg string) { fmt.Println(msg) })
		} else {
			result, err = bench.RunCPU(ctx, *label, func(msg string) { fmt.Println(msg) })
		}
		if err != nil {
			return err
		}
		saved, err := bench.WriteResult(path, result)
		if err != nil {
			return err
		}
		fmt.Printf("%s benchmark completed: %s\n", strings.ToUpper(*testName), saved)
		return nil
	case "serve":
		host, _, err := net.SplitHostPort(*listen)
		if err != nil {
			return fmt.Errorf("adresse d'écoute invalide: %w", err)
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			return errors.New("serveur volontairement limité à localhost/loopback")
		}
		listener, err := net.Listen("tcp", *listen)
		if err != nil {
			return err
		}
		defer listener.Close()
		server := &http.Server{Handler: newHandler(path), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
		fmt.Printf("PC Benchmark v0.1 - lecture seule\nRapports : %s\nOuvrir : http://%s\nCtrl+C pour quitter\n", path, listener.Addr().String())
		return server.Serve(listener)
	case "export":
		if *source == "" {
			return errors.New("--from requis")
		}
		out, err := report.ExportLegacy(path, *source)
		if err != nil {
			return err
		}
		fmt.Println("Rapport JSON créé :", out)
		return nil
	case "list":
		runs, err := report.Load(path)
		if err != nil {
			return err
		}
		for _, r := range runs {
			fmt.Printf("%s\t%s\t%s\t%d mesures\n", r.ID, r.Label, r.CPU, len(r.Metrics))
		}
		return nil
	case "compare":
		if *before == "" || *after == "" {
			return errors.New("--before et --after requis")
		}
		runs, err := report.Load(path)
		if err != nil {
			return err
		}
		b, ok := report.Find(runs, *before)
		if !ok {
			return errors.New("rapport avant non trouvé")
		}
		a, ok := report.Find(runs, *after)
		if !ok {
			return errors.New("rapport après non trouvé")
		}
		cmp := report.Compare(b, a)
		for _, w := range cmp.Warnings {
			fmt.Println("ATTENTION:", w)
		}
		for _, m := range cmp.Rows {
			if !m.Comparable {
				fmt.Printf("%-28s NON COMPARABLE (%s)\n", m.Label, m.Reason)
				continue
			}
			fmt.Printf("%-28s %10.2f -> %10.2f %-15s %+7.2f%%\n", m.Label, m.Before, m.After, m.Unit, m.Improvement)
		}
		return nil
	default:
		return fmt.Errorf("commande inconnue : %s", cmd)
	}
}

func newHandler(dir string) http.Handler {
	mux := http.NewServeMux()
	sub, err := fs.Sub(assets, "web")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Méthode interdite", http.StatusMethodNotAllowed)
			return
		}
		runs, err := report.Load(dir)
		if err != nil {
			http.Error(w, "Lecture des rapports impossible", 500)
			return
		}
		writeJSON(w, runs)
	})
	mux.HandleFunc("/api/compare", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Méthode interdite", http.StatusMethodNotAllowed)
			return
		}
		before, after := r.URL.Query().Get("before"), r.URL.Query().Get("after")
		if before == "" || after == "" || len(before) > 255 || len(after) > 255 || strings.ContainsAny(before, "/\\") || strings.ContainsAny(after, "/\\") {
			http.Error(w, "Identifiants invalides", 400)
			return
		}
		runs, err := report.Load(dir)
		if err != nil {
			http.Error(w, "Lecture impossible", 500)
			return
		}
		b, okB := report.Find(runs, before)
		a, okA := report.Find(runs, after)
		if !okB || !okA {
			http.Error(w, "Rapport introuvable", 404)
			return
		}
		writeJSON(w, report.Compare(b, a))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("JSON: %v", err)
	}
}
