// Package report reads the historical benchmark_pc_v5/v6 text reports.
// The parser is deliberately limited to explicit, stable markers. Unknown
// lines are ignored, and it never exposes the original report text over HTTP.
package report

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Metric struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Value  float64 `json:"value"`
	Unit   string  `json:"unit"`
	Higher bool    `json:"higher_is_better"`
}

type Run struct {
	ID          string   `json:"id"`
	Complete    bool     `json:"complete"`
	Label       string   `json:"label"`
	Timestamp   string   `json:"timestamp,omitempty"`
	OS          string   `json:"os,omitempty"`
	Kernel      string   `json:"kernel,omitempty"`
	CPU         string   `json:"cpu,omitempty"`
	Cores       string   `json:"cores,omitempty"`
	Threads     string   `json:"threads,omitempty"`
	RAM         string   `json:"ram,omitempty"`
	Board       string   `json:"board,omitempty"`
	BIOS        string   `json:"bios,omitempty"`
	FFmpeg      string   `json:"ffmpeg_version,omitempty"`
	Sysbench    string   `json:"sysbench_version,omitempty"`
	Go          string   `json:"go_version,omitempty"`
	VideoSHA256 string   `json:"video_sha256,omitempty"`
	VideoSpec   string   `json:"video_spec,omitempty"`
	GoChecksum  string   `json:"go_checksum,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	Metrics     []Metric `json:"metrics"`
}

type Comparison struct {
	Before   Run          `json:"before"`
	After    Run          `json:"after"`
	Warnings []string     `json:"warnings,omitempty"`
	Rows     []MetricDiff `json:"rows"`
}

type MetricDiff struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Before      float64 `json:"before"`
	After       float64 `json:"after"`
	Unit        string  `json:"unit"`
	Improvement float64 `json:"improvement_percent"`
	Comparable  bool    `json:"comparable"`
	Reason      string  `json:"reason,omitempty"`
}

var (
	shaLine     = regexp.MustCompile(`(?i)^([a-f0-9]{64})\s+\S+`)
	sysCPU      = regexp.MustCompile(`events per second:\s*([0-9]+(?:\.[0-9]+)?)`)
	sysRAM      = regexp.MustCompile(`\(([0-9]+(?:\.[0-9]+)?) MiB/sec\)`)
	threadsLine = regexp.MustCompile(`^--- (CPU|RAM|FFmpeg|Go) (\d+) (threads|workers) ---$`)
	resultFF    = regexp.MustCompile(`^RESULT ffmpeg_threads=(\d+) elapsed_seconds=([0-9]+(?:\.[0-9]+)?)$`)
	resultGO    = regexp.MustCompile(`^RESULT go_workers=(\d+)\s+.*?elapsed_seconds=([0-9]+(?:\.[0-9]+)?)\s+throughput_mib_s=([0-9]+(?:\.[0-9]+)?)\s+checksum=([0-9a-f]{64})$`)
	specWidth   = regexp.MustCompile(`^width=(\d+)$`)
	safeLabel   = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

const maxReportSize = 4 << 20
const maxReports = 500

func f64(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0
}

func Parse(id string, r io.Reader) (Run, error) {
	out := Run{ID: id, Label: strings.TrimSuffix(strings.TrimPrefix(id, "rapport_"), ".txt"), Metrics: []Metric{}}
	if !safeLabel.MatchString(out.Label) {
		out.Label = "rapport"
	}
	scanner := bufio.NewScanner(io.LimitReader(r, maxReportSize+1))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	section, kind, workers := "", "", ""
	var width, height, duration, codec string
	var metricKeys = make(map[string]bool)
	var checksums = make(map[string]bool)
	var encounteredGo bool
	add := func(key, label, s, unit string, higher bool) {
		if metricKeys[key] {
			return
		}
		if v, ok := f64(s); ok {
			out.Metrics = append(out.Metrics, Metric{key, label, v, unit, higher})
			metricKeys[key] = true
		}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "========== ") && strings.HasSuffix(line, " ==========") {
			section = strings.TrimSuffix(strings.TrimPrefix(line, "========== "), " ==========")
			if section == "FIN" {
				out.Complete = true
			}
			kind, workers = "", ""
			continue
		}
		if m := threadsLine.FindStringSubmatch(line); m != nil {
			kind, workers = m[1], m[2]
			continue
		}
		// Only parse results in their declared sections. Any partial or
		// malformed report remains visible, but missing metrics stay missing.
		switch section {
		case "CONDITIONS DE MESURE":
			if strings.HasPrefix(line, "Label: ") {
				label := strings.TrimPrefix(line, "Label: ")
				if safeLabel.MatchString(label) {
					out.Label = label
				}
			}
			if strings.HasPrefix(line, "OS: ") {
				out.OS = strings.TrimPrefix(line, "OS: ")
			}
			if strings.HasPrefix(line, "ffmpeg version ") {
				out.FFmpeg = strings.Fields(line)[2]
			}
			if strings.HasPrefix(line, "sysbench ") {
				parts := strings.Fields(line)
				if len(parts) > 1 {
					out.Sysbench = parts[1]
				}
			}
			if m := shaLine.FindStringSubmatch(line); m != nil {
				out.VideoSHA256 = strings.ToLower(m[1])
			}
			if _, err := time.Parse(time.RFC3339, line); err == nil {
				out.Timestamp = line
			}
		case "SYSTEME ET MATERIEL":
			switch {
			case strings.HasPrefix(line, "Kernel: "):
				out.Kernel = strings.TrimPrefix(line, "Kernel: ")
			case strings.HasPrefix(line, "Model name:"):
				out.CPU = strings.TrimSpace(strings.TrimPrefix(line, "Model name:"))
			case strings.HasPrefix(line, "Core(s) per socket:"):
				out.Cores = strings.TrimSpace(strings.TrimPrefix(line, "Core(s) per socket:"))
			case strings.HasPrefix(line, "Thread(s) per core:"):
				out.Threads = strings.TrimSpace(strings.TrimPrefix(line, "Thread(s) per core:"))
			case strings.HasPrefix(line, "Mem:"):
				fields := strings.Fields(line)
				if len(fields) > 1 {
					out.RAM = fields[1]
				}
			case strings.HasPrefix(line, "board_name:"):
				out.Board = strings.TrimSpace(strings.TrimPrefix(line, "board_name:"))
			case strings.HasPrefix(line, "bios_version:"):
				out.BIOS = strings.TrimSpace(strings.TrimPrefix(line, "bios_version:"))
			}
		case "VIDEO SOURCE":
			if m := specWidth.FindStringSubmatch(line); m != nil {
				width = m[1]
			}
			if strings.HasPrefix(line, "height=") {
				height = strings.TrimPrefix(line, "height=")
			}
			if strings.HasPrefix(line, "duration=") {
				duration = strings.TrimPrefix(line, "duration=")
			}
			if strings.HasPrefix(line, "codec_name=") {
				codec = strings.TrimPrefix(line, "codec_name=")
			}
		case "CPU SYSBENCH":
			if kind == "CPU" {
				if m := sysCPU.FindStringSubmatch(line); m != nil {
					add("cpu."+workers, "CPU · "+workers+" threads", m[1], "événements/s", true)
				}
			}
		case "RAM SYSBENCH (1K write, 1TiB max, 15s)":
			if kind == "RAM" {
				if m := sysRAM.FindStringSubmatch(line); m != nil {
					add("ram."+workers, "RAM écriture · "+workers+" threads", m[1], "MiB/s", true)
				}
			}
		case "ENCODAGE H264 LOGICIEL LIBX264":
			if m := resultFF.FindStringSubmatch(line); m != nil {
				add("ffmpeg."+m[1], "FFmpeg H.264 · "+m[1]+" threads", m[2], "s", false)
			}
		case "GO SHA256 (CHARGE FIXE, 1/4/8 WORKERS)":
			if strings.HasPrefix(line, "go version ") {
				out.Go = strings.TrimPrefix(line, "go version ")
			}
			if m := resultGO.FindStringSubmatch(line); m != nil {
				encounteredGo = true
				add("go."+m[1], "Go SHA-256 · "+m[1]+" workers", m[2], "s", false)
				checksums[m[4]] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Run{}, err
	}
	if width != "" && height != "" && codec != "" {
		out.VideoSpec = codec + " " + width + "x" + height + " " + duration + "s"
	}
	if len(checksums) > 1 {
		out.Warnings = append(out.Warnings, "Les checksums Go divergent entre workers : résultats Go invalides")
	}
	if len(checksums) == 1 {
		for c := range checksums {
			out.GoChecksum = c
		}
	}
	if encounteredGo && out.Go == "" {
		out.Warnings = append(out.Warnings, "Version Go non relevée")
	}
	if !out.Complete {
		out.Warnings = append(out.Warnings, "Rapport incomplet : marqueur FIN absent ; comparaison non certifiable")
	}
	if out.VideoSHA256 == "" {
		out.Warnings = append(out.Warnings, "Empreinte vidéo manquante : comparaison FFmpeg non certifiable")
	}
	if len(out.Metrics) == 0 {
		return Run{}, errors.New("aucune métrique reconnue (format de rapport non pris en charge)")
	}
	sort.Slice(out.Metrics, func(i, j int) bool { return order(out.Metrics[i].Key) < order(out.Metrics[j].Key) })
	return out, nil
}

func order(k string) int {
	parts := strings.Split(k, ".")
	bases := map[string]int{"cpu": 0, "ram": 100, "ffmpeg": 200, "go": 300}
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return bases[parts[0]] + n
}

// Load loads bounded regular .txt reports from one flat directory. It never
// opens symlinks or arbitrary request-selected paths.
func Load(dir string) ([]Run, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0)
	for _, entry := range entries {
		if len(runs) >= maxReports {
			break
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "rapport_") || !strings.HasSuffix(name, ".txt") || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxReportSize || info.Size() == 0 {
			continue
		}
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		run, err := Parse(name, file)
		file.Close()
		if err == nil {
			runs = append(runs, run)
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].Timestamp > runs[j].Timestamp })
	return runs, nil
}

// Compare uses the same metric keys in both runs, with explicit guards for
// test input and toolchain changes. It never presents such a difference as
// a verified speedup.
func Compare(before, after Run) Comparison {
	c := Comparison{Before: before, After: after, Rows: []MetricDiff{}}
	if !before.Complete || !after.Complete {
		c.Warnings = append(c.Warnings, "Campagne incomplète : gains désactivés")
	}
	if before.VideoSHA256 == "" || after.VideoSHA256 == "" {
		c.Warnings = append(c.Warnings, "SHA-256 vidéo manquant : FFmpeg non comparable")
	}
	if before.VideoSHA256 != "" && after.VideoSHA256 != "" && before.VideoSHA256 != after.VideoSHA256 {
		c.Warnings = append(c.Warnings, "Vidéos sources différentes : FFmpeg non comparable")
	}
	if before.FFmpeg != "" && after.FFmpeg != "" && before.FFmpeg != after.FFmpeg {
		c.Warnings = append(c.Warnings, "Versions FFmpeg différentes : comparaison FFmpeg indicative uniquement")
	}
	if before.Sysbench != "" && after.Sysbench != "" && before.Sysbench != after.Sysbench {
		c.Warnings = append(c.Warnings, "Versions Sysbench différentes : comparaison synthétique indicative")
	}
	if before.Go != "" && after.Go != "" && before.Go != after.Go {
		c.Warnings = append(c.Warnings, "Versions Go différentes : comparaison Go indicative")
	}
	if before.GoChecksum != "" && after.GoChecksum != "" && before.GoChecksum != after.GoChecksum {
		c.Warnings = append(c.Warnings, "Checksums Go différents : comparaison Go invalide")
	}
	bm := map[string]Metric{}
	for _, m := range before.Metrics {
		bm[m.Key] = m
	}
	for _, a := range after.Metrics {
		b, found := bm[a.Key]
		if !found || b.Value <= 0 || a.Value <= 0 {
			continue
		}
		row := MetricDiff{Key: a.Key, Label: a.Label, Before: b.Value, After: a.Value, Unit: a.Unit, Comparable: true}
		switch {
		case !before.Complete || !after.Complete:
			row.Comparable = false
			row.Reason = "Rapport incomplet"
		case strings.HasPrefix(a.Key, "ffmpeg.") && (before.VideoSHA256 == "" || after.VideoSHA256 == "" || before.VideoSHA256 != after.VideoSHA256):
			row.Comparable = false
			row.Reason = "Source vidéo différente ou non vérifiable"
		case strings.HasPrefix(a.Key, "ffmpeg.") && (before.FFmpeg == "" || after.FFmpeg == "" || before.FFmpeg != after.FFmpeg):
			row.Comparable = false
			row.Reason = "Version FFmpeg différente ou inconnue"
		case strings.HasPrefix(a.Key, "go.") && (before.GoChecksum == "" || after.GoChecksum == "" || before.GoChecksum != after.GoChecksum):
			row.Comparable = false
			row.Reason = "Checksum Go différent ou manquant"
		case strings.HasPrefix(a.Key, "go.") && (before.Go == "" || after.Go == "" || before.Go != after.Go):
			row.Comparable = false
			row.Reason = "Version Go différente ou inconnue"
		case (strings.HasPrefix(a.Key, "cpu.") || strings.HasPrefix(a.Key, "ram.")) && (before.Sysbench == "" || after.Sysbench == "" || before.Sysbench != after.Sysbench):
			row.Comparable = false
			row.Reason = "Version Sysbench différente ou inconnue"
		}
		if a.Higher {
			row.Improvement = (a.Value/b.Value - 1) * 100
		} else {
			row.Improvement = (b.Value/a.Value - 1) * 100
		}
		c.Rows = append(c.Rows, row)
	}
	sort.Slice(c.Rows, func(i, j int) bool { return order(c.Rows[i].Key) < order(c.Rows[j].Key) })
	return c
}

func Find(runs []Run, id string) (Run, bool) {
	for _, r := range runs {
		if r.ID == id {
			return r, true
		}
	}
	return Run{}, false
}

func ValidateDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("pas un répertoire : %s", abs)
	}
	return abs, nil
}
