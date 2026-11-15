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

func TestRunMemoryHistoricalProtocolAndJSON(t *testing.T) {
	dir, _ := fakeSysbench(t, `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; exit 0; fi
 echo "$*" >> "$CALLS"
 case "$*" in
   "memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=1 --time=15 run") echo '1000.00 MiB transferred (6000.25 MiB/sec)';;
   "memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=4 --time=15 run") echo '2000.00 MiB transferred (13000.75 MiB/sec)';;
   *) exit 17;;
 esac`)
	calls := filepath.Join(dir, "calls")
	t.Setenv("CALLS", calls)
	var steps []string
	r, err := RunMemory(context.Background(), "mem_probe", func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || r.Sysbench != "1.0.20" || len(r.Metrics) != 2 || !r.Complete {
		t.Fatalf("unexpected result: %+v", r)
	}
	for i, expected := range []struct {
		key   string
		value float64
	}{{"ram.1", 6000.25}, {"ram.4", 13000.75}} {
		m := r.Metrics[i]
		if m.Key != expected.key || m.Value != expected.value || m.Unit != "MiB/s" || !m.Higher {
			t.Fatalf("unexpected metric: %+v", m)
		}
	}
	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	want := "memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=1 --time=15 run\n" +
		"memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=4 --time=15 run\n"
	if string(data) != want {
		t.Fatalf("command mismatch:\n%s", data)
	}
	saved, err := WriteResult(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(saved)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := report.ParseJSON(filepath.Base(saved), bytes.NewReader(content))
	if err != nil || len(parsed.Metrics) != 2 {
		t.Fatalf("invalid JSON: %v %+v", err, parsed)
	}
	loaded, err := report.Load(dir)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("report load: %v (%d)", err, len(loaded))
	}
	if info, err := os.Stat(saved); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("report permissions: %v %v", info, err)
	}
	// Go memory runner and legacy sysbench files use identical metric keys.
	legacy := report.Run{Complete: true, Sysbench: "1.0.20", Metrics: []report.Metric{{Key: "ram.1", Label: "RAM écriture · 1 threads", Value: 6100, Unit: "MiB/s", Higher: true}, {Key: "ram.4", Label: "RAM écriture · 4 threads", Value: 12000, Unit: "MiB/s", Higher: true}}}
	cmp := report.Compare(legacy, r)
	if len(cmp.Rows) != 2 || !cmp.Rows[0].Comparable || !cmp.Rows[1].Comparable {
		t.Fatalf("not comparable with legacy metrics: %+v", cmp)
	}
}

func TestRunMemoryRejectsInvalidOutputAndDoesNotPersist(t *testing.T) {
	cases := []struct {
		name   string
		script string
	}{
		{"nonzero", `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; else echo fail >&2; exit 15; fi`},
		{"missing", `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; else echo 'No memory result'; fi`},
		{"duplicate", `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; else echo '123 MiB transferred (123 MiB/sec)'; echo '123 MiB transferred (456 MiB/sec)'; fi`},
		{"zero", `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; else echo '123 MiB transferred (0 MiB/sec)'; fi`},
		{"second-fails", `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; elif echo "$*" | grep -q 'threads=4'; then exit 11; else echo '123 MiB transferred (700 MiB/sec)'; fi`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := fakeSysbench(t, tc.script)
			result, err := RunMemory(context.Background(), "probe", nil)
			if err == nil {
				t.Fatalf("accepted invalid run: %+v", result)
			}
			if len(result.Metrics) != 0 {
				t.Fatalf("partial result returned: %+v", result)
			}
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range files {
				if strings.HasSuffix(f.Name(), ".json") {
					t.Fatal("partial report persisted")
				}
			}
		})
	}
}

func TestRunMemoryCancellationAndValidation(t *testing.T) {
	fakeSysbench(t, `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; else echo '100 MiB transferred (100 MiB/sec)'; fi`)
	for _, label := range []string{"", "../bad", "has spaces", strings.Repeat("a", 129)} {
		if _, err := RunMemory(context.Background(), label, nil); err == nil {
			t.Fatalf("bad label %q accepted", label)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunMemory(ctx, "probe", nil); err == nil {
		t.Fatal("canceled run accepted")
	}
	p, version, err := CheckMemory()
	if err != nil || p == "" || version != "1.0.20" {
		t.Fatalf("preflight: %q %q %v", p, version, err)
	}
}

func TestRunMemoryDeadline(t *testing.T) {
	fakeSysbench(t, `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; else sleep 2; fi`)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := RunMemory(ctx, "probe", nil)
	if err == nil {
		t.Fatal("deadline ignored")
	}
}

func TestSharedLockBlocksMemoryAndCPU(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux flock only")
	}
	dir := t.TempDir()
	unlock, err := LockCPU(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := LockBenchmark(dir); err == nil {
		second()
		t.Fatal("memory was permitted while CPU lock is held")
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock2, err := LockBenchmark(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := LockCPU(dir); err == nil {
		second()
		t.Fatal("CPU was permitted while memory lock is held")
	}
	if err := unlock2(); err != nil {
		t.Fatal(err)
	}
}
