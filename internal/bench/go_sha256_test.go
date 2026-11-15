package bench

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

// Independent, sequential test oracle to detect workload drift: the same
// block contents and 4-byte little-endian index as benchmark_go.go.
func sequentialSHAChecksum(jobs uint32) [sha256.Size]byte {
	buf := make([]byte, shaBlockSize)
	for i := range buf {
		buf[i] = byte(i*131 + 17)
	}
	var sum [sha256.Size]byte
	for i := uint32(0); i < jobs; i++ {
		binary.LittleEndian.PutUint32(buf[:4], i)
		h := sha256.Sum256(buf)
		for j := range sum {
			sum[j] ^= h[j]
		}
	}
	return sum
}

func TestSHAWorkloadExactAcrossWorkers(t *testing.T) {
	for _, jobs := range []uint32{1, 37, 127} {
		want := sequentialSHAChecksum(jobs)
		for _, workers := range []int{1, 4, 8} {
			got, elapsed := runSHAWorkload(workers, jobs)
			if got != want || elapsed <= 0 {
				t.Fatalf("jobs=%d workers=%d got=%x want=%x elapsed=%v", jobs, workers, got, want, elapsed)
			}
		}
	}
}

func fakeSHAOutput(n int, checksum string) []byte {
	return []byte(fmt.Sprintf("RESULT go_workers=%d gomaxprocs=%d jobs=131072 bytes=8589934592 elapsed_seconds=20.000 throughput_mib_s=409.60 checksum=%s\n", n, n, checksum))
}

func TestSHAProtocolStrictOutput(t *testing.T) {
	for _, n := range []int{1, 4, 8} {
		elapsed, checksum, err := parseSHAResult(fakeSHAOutput(n, shaExpectedDigest), n)
		if err != nil || elapsed != 20 || checksum != shaExpectedDigest {
			t.Fatalf("valid result %d rejected: %v", n, err)
		}
	}
	invalid := [][]byte{
		fakeSHAOutput(1, strings.Repeat("0", 64)),
		fakeSHAOutput(4, shaExpectedDigest), // wrong workers for caller 1
		bytes.Replace(fakeSHAOutput(1, shaExpectedDigest), []byte("gomaxprocs=1"), []byte("gomaxprocs=8"), 1),
		bytes.Replace(fakeSHAOutput(1, shaExpectedDigest), []byte("jobs=131072"), []byte("jobs=131071"), 1),
		bytes.Replace(fakeSHAOutput(1, shaExpectedDigest), []byte("bytes=8589934592"), []byte("bytes=100"), 1),
		bytes.Replace(fakeSHAOutput(1, shaExpectedDigest), []byte("throughput_mib_s=409.60"), []byte("throughput_mib_s=800.00"), 1),
		bytes.Replace(fakeSHAOutput(1, shaExpectedDigest), []byte("elapsed_seconds=20.000"), []byte("elapsed_seconds=0.000"), 1),
		append(fakeSHAOutput(1, shaExpectedDigest), []byte("unexpected output\n")...),
	}
	for i, bad := range invalid {
		if _, _, err := parseSHAResult(bad, 1); err == nil {
			t.Fatalf("invalid output %d accepted: %s", i, bad)
		}
	}
}

func TestSHAProfilesThreeMetricsAndJSONRoundtrip(t *testing.T) {
	var calls []int
	r, err := runSHAProfiles(context.Background(), "sha_probe", nil, func(_ context.Context, n int) ([]byte, error) {
		calls = append(calls, n)
		return fakeSHAOutput(n, shaExpectedDigest), nil
	})
	if err != nil || !reflect.DeepEqual(calls, []int{1, 4, 8}) || !r.Complete || len(r.Metrics) != 3 || r.GoChecksum != shaExpectedDigest {
		t.Fatalf("invalid SHA run: %+v err=%v calls=%v", r, err, calls)
	}
	if r.Go != runtime.Version()+" "+runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("unexpected Go version format %q", r.Go)
	}
	for i, key := range []string{"go.1", "go.4", "go.8"} {
		m := r.Metrics[i]
		if m.Key != key || m.Unit != "s" || m.Higher || m.Value != 20 {
			t.Fatalf("wrong metric %+v", m)
		}
	}
	dir := t.TempDir()
	if _, err := WriteResult(dir, r); err != nil {
		t.Fatal(err)
	}
	got, err := report.Load(dir)
	if err != nil || len(got) != 1 || len(got[0].Metrics) != 3 || got[0].GoChecksum != shaExpectedDigest {
		t.Fatalf("roundtrip: %+v, err=%v", got, err)
	}
}

func TestSHAProfilesFailureAndCancellationNoPartialRun(t *testing.T) {
	for _, mode := range []string{"bad_checksum", "exec_error", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls []int
			r, err := runSHAProfiles(ctx, "go_probe", nil, func(_ context.Context, n int) ([]byte, error) {
				calls = append(calls, n)
				if n == 4 {
					switch mode {
					case "bad_checksum":
						return fakeSHAOutput(n, strings.Repeat("f", 64)), nil
					case "exec_error":
						return nil, errors.New("child crashed")
					case "canceled":
						cancel()
						return nil, context.Canceled
					}
				}
				return fakeSHAOutput(n, shaExpectedDigest), nil
			})
			if err == nil || len(r.Metrics) != 0 || !reflect.DeepEqual(calls, []int{1, 4}) {
				t.Fatalf("partial run accepted: %+v err=%v calls=%v", r, err, calls)
			}
		})
	}
}

func TestSHAChildInvalidParameters(t *testing.T) {
	for _, args := range [][]string{nil, {"--workers=2"}, {"--workers=1", "extra"}, {"--workers=0"}, {"--workers=01"}, {"--jobs=1"}} {
		if err := RunSHAChild(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted invalid child arguments: %v", args)
		}
	}
}

func TestGoSelectedPreflightAndInvalid(t *testing.T) {
	for _, name := range []string{"go", "go,cpu,ffmpeg,memory"} {
		selection, err := ParseTests(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(selection) == 0 || selection[len(selection)-1] != "go" {
			t.Fatalf("unexpected order %v", selection)
		}
	}
	for _, name := range []string{"go,go", "gO", "go,", "cpu,go,unknown"} {
		if _, err := ParseTests(name); err == nil {
			t.Fatalf("accepted bad selection %q", name)
		}
	}
	if runtime.GOOS == "linux" {
		if version, err := CheckSelected(context.Background(), []string{"go"}, ""); err != nil || !strings.Contains(version, runtime.Version()) {
			t.Fatalf("Go preflight failed: %q %v", version, err)
		}
	}
	if _, err := CheckSelected(context.Background(), []string{"go"}, "/tmp/unused-video"); err == nil {
		t.Fatal("accepted --video for Go-only")
	}
}
