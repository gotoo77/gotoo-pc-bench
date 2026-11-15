package bench

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

func fakeSysbench(t *testing.T, script string) (string, string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Linux runner test")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sysbench")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	old := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+old)
	return dir, path
}

func TestRunCPUHonorsLegacyProtocol(t *testing.T) {
	dir, _ := fakeSysbench(t, `if [ "$1" = "--version" ]; then echo "sysbench 1.0.20"; exit 0; fi
 echo "$*" >> "$CALLS"
 case "$*" in
   "cpu --cpu-max-prime=10000 --threads=1 --time=30 run"|"cpu --cpu-max-prime=10000 --threads=4 --time=30 run"|"cpu --cpu-max-prime=10000 --threads=8 --time=30 run") echo "    events per second: 1234.56";;
   *) exit 15;;
 esac`)
	calls := filepath.Join(dir, "calls")
	t.Setenv("CALLS", calls)
	var steps []string
	r, err := RunCPU(context.Background(), "avant_upgrade", func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 || len(r.Metrics) != 3 || r.Sysbench != "1.0.20" || !r.Complete {
		t.Fatalf("unexpected result: %+v", r)
	}
	for i, n := range []string{"cpu.1", "cpu.4", "cpu.8"} {
		if r.Metrics[i].Key != n || r.Metrics[i].Value != 1234.56 || !r.Metrics[i].Higher {
			t.Fatalf("bad metric: %+v", r.Metrics[i])
		}
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	expected := "cpu --cpu-max-prime=10000 --threads=1 --time=30 run\ncpu --cpu-max-prime=10000 --threads=4 --time=30 run\ncpu --cpu-max-prime=10000 --threads=8 --time=30 run\n"
	if string(b) != expected {
		t.Fatalf("unexpected commands: %s", b)
	}
	out, err := WriteResult(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("machine-id")) {
		t.Fatal("sensitive metadata leaked")
	}
	result, err := report.ParseJSON(filepath.Base(out), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metrics) != 3 || result.CPU == "" {
		t.Fatal("broken JSON result")
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0600 {
		t.Fatalf("mode: %v", st.Mode())
	}
	loaded, err := report.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d", len(loaded))
	}
}

func TestRunCPUFailuresDoNotPublishReport(t *testing.T) {
	for _, tc := range []struct{ name, script string }{
		{"nonzero", `if [ "$1" = "--version" ]; then echo "sysbench 1.0.20"; else echo "fail" >&2; exit 21; fi`},
		{"no-metric", `if [ "$1" = "--version" ]; then echo "sysbench 1.0.20"; else echo "no stats"; fi`},
		{"double-metric", `if [ "$1" = "--version" ]; then echo "sysbench 1.0.20"; else echo "events per second: 2"; echo "events per second: 3"; fi`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := fakeSysbench(t, tc.script)
			_, err := RunCPU(context.Background(), "before", nil)
			if err == nil {
				t.Fatal("accepted failed run")
			}
			entries, e := os.ReadDir(dir)
			if e != nil {
				t.Fatal(e)
			}
			for _, x := range entries {
				if strings.HasSuffix(x.Name(), ".json") {
					t.Fatal("unexpected published report")
				}
			}
		})
	}
}

func TestRunCPUValidationCancellationAndPreflight(t *testing.T) {
	fakeSysbench(t, `if [ "$1" = "--version" ]; then echo "sysbench 1.0.20"; else echo "events per second: 55"; fi`)
	for _, label := range []string{"", "../escape", "bad spaces", strings.Repeat("x", 129)} {
		if _, e := RunCPU(context.Background(), label, nil); e == nil {
			t.Fatalf("accepted label %q", label)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := RunCPU(ctx, "ok", nil); e == nil {
		t.Fatal("accepted canceled context")
	}
	p, v, e := CheckCPU()
	if e != nil || p == "" || v != "1.0.20" {
		t.Fatalf("preflight: %q %q %v", p, v, e)
	}
}

func TestRunCPUDeadline(t *testing.T) {
	fakeSysbench(t, `if [ "$1" = "--version" ]; then echo "sysbench 1.0.20"; else sleep 2; fi`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := RunCPU(ctx, "ok", nil)
	if err == nil {
		t.Fatal("expected timeout")
	}
}

func TestCPUCheckRejectsUnrecognizedVersion(t *testing.T) {
	fakeSysbench(t, `echo "unknown version"`)
	if _, _, e := CheckCPU(); e == nil {
		t.Fatal("accepted version")
	}
}

func TestLockCPUPreventsOverlappingRuns(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only lock")
	}
	dir := t.TempDir()
	release, err := LockCPU(dir)
	if err != nil {
		t.Fatal(err)
	}
	if other, e := LockCPU(dir); e == nil {
		other()
		t.Fatal("allowed concurrent lock")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	next, err := LockCPU(dir)
	if err != nil {
		t.Fatal("stale lock after release:", err)
	}
	if err := next(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteResultUniqueNames(t *testing.T) {
	dir := t.TempDir()
	r := report.Run{Complete: true, Label: "quick", OS: "linux", Sysbench: "1.0.20", Metrics: []report.Metric{{Key: "cpu.1", Label: "CPU", Value: 1, Unit: "événements/s", Higher: true}}}
	a, e := WriteResult(dir, r)
	if e != nil {
		t.Fatal(e)
	}
	b, e := WriteResult(dir, r)
	if e != nil {
		t.Fatal(e)
	}
	if a == b {
		t.Fatalf("unexpected filename collision %s", a)
	}
}
