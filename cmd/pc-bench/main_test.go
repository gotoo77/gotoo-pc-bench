package main

import (
	"github.com/gotoo77/gotoo-pc-bench/internal/report"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHTTPIsReadOnlyAndLoopback(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "rapport_x.txt"), []byte("========== CPU SYSBENCH ==========\n--- CPU 1 threads ---\nevents per second:  100\n"), 0600); err != nil {
		t.Fatal(err)
	}
	mux := newHandler(d)
	for _, tt := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/api/runs", 200},
		{"GET", "/api/compare?before=../../etc/passwd&after=x", 400},
		{"GET", "/api/compare?before=bad&after=bad", 404},
		{"POST", "/api/runs", 405},
		{"GET", "/", 200},
		{"GET", "/app.js", 200},
	} {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tt.status {
			t.Errorf("%s %s got %d want %d", tt.method, tt.path, rec.Code, tt.status)
		}
		if rec.Header().Get("Content-Security-Policy") == "" {
			t.Error("CSP absent")
		}
		if strings.Contains(rec.Body.String(), "sensitive") {
			t.Error("data leaked")
		}
	}
}

func TestNonLocalBindRejected(t *testing.T) {
	d := t.TempDir()
	err := run([]string{"serve", "--reports", d, "--listen", "0.0.0.0:8765"})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("bind not rejected: %v", err)
	}
}

func TestExportCommand(t *testing.T) {
	d := t.TempDir()
	content := "========== CONDITIONS DE MESURE ==========\nLabel: test\n========== CPU SYSBENCH ==========\n--- CPU 1 threads ---\nevents per second: 100\n========== FIN ==========\n"
	if err := os.WriteFile(filepath.Join(d, "rapport_test.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"export", "--reports", d, "--from", "rapport_test.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(d, "rapport_test.json")); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"export", "--reports", d, "--from", "../escape.txt"}); err == nil {
		t.Fatal("accepted traversal")
	}
}

// End-to-end CLI dispatch with a fake Sysbench executable. It does not use
// root and must not create partial reports on failures.
func TestMemoryRunnerCLI(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only runner")
	}
	bin := t.TempDir()
	dir := t.TempDir()
	fake := filepath.Join(bin, "sysbench")
	code := `#!/bin/sh
if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; exit 0; fi
case "$*" in
  "memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=1 --time=15 run"|"memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=4 --time=15 run") echo '1024 MiB transferred (5000.25 MiB/sec)';;
  *) exit 23;;
esac
`
	if err := os.WriteFile(fake, []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := run([]string{"check", "--reports", dir, "--test", "memory"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"run", "--reports", dir, "--test", "memory", "--label", "memory_probe"}); err != nil {
		t.Fatal(err)
	}
	runs, err := report.Load(dir)
	if err != nil || len(runs) != 1 || len(runs[0].Metrics) != 2 {
		t.Fatalf("invalid saved memory run: %+v %v", runs, err)
	}
	if runs[0].Metrics[0].Key != "ram.1" || runs[0].Metrics[1].Key != "ram.4" {
		t.Fatalf("wrong metric keys: %+v", runs[0].Metrics)
	}
	if err := run([]string{"run", "--reports", dir, "--test", "disk", "--label", "bad"}); err == nil {
		t.Fatal("unknown runner was accepted")
	}
}

func TestCombinedRunnerCLIAtomicAndStrict(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only runner")
	}
	bin, reports := t.TempDir(), t.TempDir()
	fake := filepath.Join(bin, "sysbench")
	code := `#!/bin/sh
if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; exit 0; fi
case "$*" in
  cpu*) echo 'events per second: 1234.56';;
  memory*) if [ "$FAIL_MEMORY" = "1" ]; then exit 29; fi; echo '1024.00 MiB transferred (5000.25 MiB/sec)';;
  *) exit 19;;
esac
`
	if err := os.WriteFile(fake, []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := run([]string{"check", "--reports", reports, "--test", "cpu,memory"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"run", "--reports", reports, "--test", "cpu,memory", "--label", "combined"}); err != nil {
		t.Fatal(err)
	}
	items, err := report.Load(reports)
	if err != nil || len(items) != 1 || len(items[0].Metrics) != 5 {
		t.Fatalf("expected one complete campaign with 5 metrics: %+v %v", items, err)
	}
	for _, invalid := range []string{"cpu,cpu", "disk,cpu", "cpu,", "CPU"} {
		if err := run([]string{"run", "--reports", reports, "--test", invalid, "--label", "combined"}); err == nil {
			t.Fatalf("accepted test selection %q", invalid)
		}
	}
	t.Setenv("FAIL_MEMORY", "1")
	if err := run([]string{"run", "--reports", reports, "--test", "cpu,memory", "--label", "failing"}); err == nil {
		t.Fatal("accepted campaign with failed second test")
	}
	items, err = report.Load(reports)
	if err != nil || len(items) != 1 {
		t.Fatalf("failing campaign left a report: %+v %v", items, err)
	}
}

func TestFFmpegCLIAtomicAndSourcePrivacy(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux runner")
	}
	bin, reports := t.TempDir(), t.TempDir()
	video := filepath.Join(bin, "video.mkv")
	if err := os.WriteFile(video, []byte("fixed source for fake ffprobe"), 0600); err != nil {
		t.Fatal(err)
	}
	ff := `#!/bin/sh
case "$*" in
  "-version") echo 'ffmpeg version 8.1.3 Copyright fake';;
  "-hide_banner -encoders") echo ' V....D libx264 H264 encoder';;
  "-nostdin -hide_banner -loglevel error -f lavfi -i testsrc2=size=64x64:rate=1:duration=1 -frames:v 1 -an -c:v libx264 -threads 1 -f null -") : ;;
  *) if [ "$BREAK_ENCODE" = 1 ]; then exit 12; fi
     sleep 0.015
     echo 'frame=  600 fps=1000 Lsize=N/A' >&2;;
esac`
	probe := `#!/bin/sh
printf '%s\n' 'codec_name=ffv1' 'width=640' 'height=360' 'pix_fmt=yuv420p' 'r_frame_rate=30/1' 'duration=20.000000' 'size=30'`
	sys := `#!/bin/sh
if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; exit 0; fi
case "$1" in cpu) echo 'events per second: 1200.00';; memory) echo '1024.00 MiB transferred (5000.25 MiB/sec)';; *) exit 18;; esac`
	for name, code := range map[string]string{"ffmpeg": ff, "ffprobe": probe, "sysbench": sys} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(code+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := run([]string{"check", "--test", "cpu,memory,ffmpeg", "--reports", reports, "--video", video}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"run", "--test", "cpu,memory,ffmpeg", "--label", "full", "--reports", reports, "--video", video}); err != nil {
		t.Fatal(err)
	}
	runs, err := report.Load(reports)
	if err != nil || len(runs) != 1 || len(runs[0].Metrics) != 8 || len(runs[0].VideoSHA256) != 64 {
		t.Fatalf("expected 8-metric JSON v1 report, got %+v: %v", runs, err)
	}
	blob, err := os.ReadFile(filepath.Join(reports, runs[0].ID))
	if err != nil || strings.Contains(string(blob), video) {
		t.Fatal("video path leaked in JSON report")
	}
	// A failure after 5 successful Sysbench metrics cannot publish a new run.
	t.Setenv("BREAK_ENCODE", "1")
	if err := run([]string{"run", "--test", "cpu,memory,ffmpeg", "--label", "failed", "--reports", reports, "--video", video}); err == nil {
		t.Fatal("accepted failed video encode")
	}
	runs, err = report.Load(reports)
	if err != nil || len(runs) != 1 {
		t.Fatalf("partial result was published: %+v, %v", runs, err)
	}
	if err := run([]string{"run", "--test", "ffmpeg", "--label", "missing", "--reports", reports}); err == nil {
		t.Fatal("video flag not required")
	}
	if err := run([]string{"check", "--test", "cpu", "--video", video, "--reports", reports}); err == nil {
		t.Fatal("video silently accepted for CPU-only")
	}
}

// Go preflight must use the compiled binary, not rely on a Go compiler or
// a Sysbench/FFmpeg tool from PATH. Full 8-GiB workloads belong to smoke tests.
func TestGoSHACheckCLIWithoutExternalTools(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Go benchmark is Linux only")
	}
	dir := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	if err := run([]string{"check", "--reports", dir, "--test", "go"}); err != nil {
		t.Fatalf("native Go preflight needs external dependencies: %v", err)
	}
	if err := run([]string{"check", "--reports", dir, "--test", "go,go"}); err == nil {
		t.Fatal("duplicate Go runner accepted")
	}
	if err := run([]string{"check", "--reports", dir, "--test", "go", "--video", "/tmp/unused"}); err == nil {
		t.Fatal("Go-only preflight accepted unrelated video")
	}
}
