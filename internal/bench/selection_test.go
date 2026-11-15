package bench

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

func TestParseTestsStrictAndCanonical(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"cpu", []string{"cpu"}},
		{"memory", []string{"memory"}},
		{"cpu,memory", []string{"cpu", "memory"}},
		{"memory,cpu", []string{"cpu", "memory"}},
	} {
		got, err := ParseTests(tc.input)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("ParseTests(%q) = %v, %v; want %v", tc.input, got, err, tc.want)
		}
	}
	for _, invalid := range []string{"", ",", "cpu,", ",memory", "cpu,cpu", "memory,memory", "disk", "cpu,disk", "CPU", "cpu, memory"} {
		if got, err := ParseTests(invalid); err == nil {
			t.Fatalf("accepted invalid selection %q: %v", invalid, got)
		}
	}
}

func TestRunSelectedCombinedCampaignAndJSON(t *testing.T) {
	dir, _ := fakeSysbench(t, `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; exit 0; fi
 echo "$*" >> "$CALLS"
 case "$*" in
   "cpu --cpu-max-prime=10000 --threads=1 --time=30 run"|"cpu --cpu-max-prime=10000 --threads=4 --time=30 run"|"cpu --cpu-max-prime=10000 --threads=8 --time=30 run") echo 'events per second: 1234.56';;
   "memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=1 --time=15 run"|"memory --memory-oper=write --memory-block-size=1K --memory-total-size=1T --threads=4 --time=15 run") echo '1024.00 MiB transferred (5000.25 MiB/sec)';;
   *) exit 19;;
 esac`)
	t.Setenv("CALLS", filepath.Join(dir, "calls"))
	selection, err := ParseTests("memory,cpu")
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	r, err := RunSelected(context.Background(), "full_probe", selection, func(m string) { messages = append(messages, m) })
	if err != nil {
		t.Fatal(err)
	}
	if !r.Complete || r.Sysbench != "1.0.20" || len(r.Metrics) != 5 || len(messages) != 5 {
		t.Fatalf("invalid combined run: %+v steps=%v", r, messages)
	}
	for i, key := range []string{"cpu.1", "cpu.4", "cpu.8", "ram.1", "ram.4"} {
		if r.Metrics[i].Key != key {
			t.Fatalf("wrong metric at %d: %+v", i, r.Metrics[i])
		}
	}
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	if len(lines) != 5 || !strings.HasPrefix(lines[0], "cpu ") || !strings.HasPrefix(lines[3], "memory ") {
		t.Fatalf("wrong sequence: %q", lines)
	}
	path, err := WriteResult(dir, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, ".json") {
		t.Fatalf("unexpected output: %s", path)
	}
	loaded, err := report.Load(dir)
	if err != nil || len(loaded) != 1 || len(loaded[0].Metrics) != 5 || loaded[0].Label != "full_probe" {
		t.Fatalf("roundtrip failed: %v %v", loaded, err)
	}
}

func TestRunSelectedFailureOrCancellationReturnsNoPartialRun(t *testing.T) {
	dir, _ := fakeSysbench(t, `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; exit 0; fi
 case "$*" in
   cpu*) echo 'events per second: 1234.56';;
   memory*) echo 'failed memory benchmark' >&2; exit 9;;
   *) exit 19;;
 esac`)
	got, err := RunSelected(context.Background(), "full_probe", []string{"cpu", "memory"}, nil)
	if err == nil || len(got.Metrics) != 0 || !strings.Contains(err.Error(), "memory benchmark") {
		t.Fatalf("partial run returned: %+v %v", got, err)
	}
	files, _ := os.ReadDir(dir)
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			t.Fatal("partial result saved")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err = RunSelected(ctx, "full_probe", []string{"cpu", "memory"}, nil)
	if !errors.Is(err, context.Canceled) || len(got.Metrics) != 0 {
		t.Fatalf("cancellation not respected: %+v %v", got, err)
	}
}
