package bench

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

// These constants and the workload below reproduce benchmark_go.go used by
// benchmark_pc_v6.sh: 131072 independent 64-KiB SHA-256 blocks (8 GiB total).
const (
	shaJobs           uint32 = 131072
	shaBlockSize             = 64 * 1024
	SHAWorkerCommand         = "__go-sha256-worker"
	shaExpectedDigest        = "3e1ddd341ade2515c601c25ab72130f92b87670e1fc39663390fa1b126dc2de1"
)

var shaResultLine = regexp.MustCompile(`^RESULT go_workers=(\d+) gomaxprocs=(\d+) jobs=(\d+) bytes=(\d+) elapsed_seconds=([0-9]+\.[0-9]{3}) throughput_mib_s=([0-9]+\.[0-9]{2}) checksum=([0-9a-f]{64})\n?$`)

// CheckGoSHA checks the currently running executable, not a Go toolchain from PATH.
// The Go benchmark runs the same executable as a cancelable child process.
func CheckGoSHA() (string, string, error) {
	if runtime.GOOS != "linux" {
		return "", "", fmt.Errorf("Go SHA-256 runner requires Linux (current OS: %s)", runtime.GOOS)
	}
	binary, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("cannot locate current executable: %w", err)
	}
	st, err := os.Stat(binary)
	if err != nil || !st.Mode().IsRegular() {
		return "", "", fmt.Errorf("cannot execute own binary: %v", err)
	}
	return binary, runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH, nil
}

// runSHAWorkload is a faithful transcription of the historical benchmark_go.go
// computation. The caller runs it in a separate process for cancellation and
// to isolate the process-wide GOMAXPROCS setting from the CLI parent.
// No I/O, context polling or hashing shortcuts are inserted into the timed loop.
func runSHAWorkload(workers int, jobs uint32) ([sha256.Size]byte, time.Duration) {
	previous := runtime.GOMAXPROCS(workers)
	defer runtime.GOMAXPROCS(previous)
	tasks := make(chan uint32, workers*2)
	results := make(chan [sha256.Size]byte, workers)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, shaBlockSize)
			for i := range buf {
				buf[i] = byte(i*131 + 17)
			}
			var accumulator [sha256.Size]byte
			for index := range tasks {
				binary.LittleEndian.PutUint32(buf[:4], index)
				digest := sha256.Sum256(buf)
				for j := range accumulator {
					accumulator[j] ^= digest[j]
				}
			}
			results <- accumulator
		}()
	}
	for i := uint32(0); i < jobs; i++ {
		tasks <- i
	}
	close(tasks)
	wg.Wait()
	close(results)
	var result [sha256.Size]byte
	for sum := range results {
		for j := range result {
			result[j] ^= sum[j]
		}
	}
	return result, time.Since(start)
}

// RunSHAChild is only invoked through the hidden internal CLI subcommand.
// All allowed inputs are fixed, so the benchmark cannot be used to allocate
// arbitrary workloads or alter external files.
func RunSHAChild(args []string, out io.Writer) error {
	if len(args) != 1 || !strings.HasPrefix(args[0], "--workers=") {
		return errors.New("internal SHA-256 worker: expected --workers=1|4|8")
	}
	rawWorkers := strings.TrimPrefix(args[0], "--workers=")
	workers, err := strconv.Atoi(rawWorkers)
	if err != nil || strconv.Itoa(workers) != rawWorkers || (workers != 1 && workers != 4 && workers != 8) {
		return errors.New("internal SHA-256 worker: invalid worker count")
	}
	if runtime.GOOS != "linux" {
		return errors.New("internal SHA-256 worker: Linux required")
	}
	sum, elapsed := runSHAWorkload(workers, shaJobs)
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		return errors.New("invalid elapsed duration")
	}
	_, err = fmt.Fprintf(out, "RESULT go_workers=%d gomaxprocs=%d jobs=%d bytes=%d elapsed_seconds=%.3f throughput_mib_s=%.2f checksum=%x\n",
		workers, workers, shaJobs, uint64(shaJobs)*shaBlockSize, seconds,
		float64(uint64(shaJobs)*shaBlockSize)/1048576/seconds, sum)
	return err
}

func runSHAChildProcess(ctx context.Context, workers int) ([]byte, error) {
	binary, _, err := CheckGoSHA()
	if err != nil {
		return nil, err
	}
	stepCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	output, err := exec.CommandContext(stepCtx, binary, SHAWorkerCommand, "--workers="+strconv.Itoa(workers)).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("SHA-256 %d workers failed (context=%v, error=%w): %s", workers, stepCtx.Err(), err, brief(output))
	}
	return output, nil
}

func parseSHAResult(output []byte, workers int) (float64, string, error) {
	m := shaResultLine.FindSubmatch(output)
	if m == nil {
		return 0, "", fmt.Errorf("invalid SHA-256 worker output: %s", brief(output))
	}
	fields := []struct {
		index int
		want  uint64
	}{
		{1, uint64(workers)}, {2, uint64(workers)}, {3, uint64(shaJobs)}, {4, uint64(shaJobs) * shaBlockSize},
	}
	for _, f := range fields {
		v, err := strconv.ParseUint(string(m[f.index]), 10, 64)
		if err != nil || v != f.want {
			return 0, "", errors.New("SHA-256 worker returned unexpected protocol parameters")
		}
	}
	elapsed, err := strconv.ParseFloat(string(m[5]), 64)
	if err != nil || elapsed <= 0 || math.IsInf(elapsed, 0) || math.IsNaN(elapsed) {
		return 0, "", errors.New("invalid SHA-256 elapsed time")
	}
	throughput, err := strconv.ParseFloat(string(m[6]), 64)
	if err != nil || throughput <= 0 || math.IsInf(throughput, 0) || math.IsNaN(throughput) {
		return 0, "", errors.New("invalid SHA-256 throughput")
	}
	// Throughput uses unrounded elapsed time; tolerate the historical 3-decimal
	// elapsed-time rounding while rejecting obviously inconsistent output.
	if math.Abs(throughput-(8192/elapsed)) > math.Max(1, (8192/elapsed)*0.02) {
		return 0, "", errors.New("SHA-256 throughput inconsistent with duration")
	}
	checksum := string(m[7])
	if checksum != shaExpectedDigest {
		return 0, "", errors.New("SHA-256 checksum differs from verified historical workload")
	}
	return elapsed, checksum, nil
}

// RunGoSHA runs the fixed 1/4/8 profiles, accepting no external input. Each
// profile is a child process so Ctrl+C and timeouts terminate the workload.
func RunGoSHA(ctx context.Context, label string, status func(string)) (report.Run, error) {
	if err := validateLabel(label); err != nil {
		return report.Run{}, err
	}
	if _, _, err := CheckGoSHA(); err != nil {
		return report.Run{}, err
	}
	return runSHAProfiles(ctx, label, status, runSHAChildProcess)
}

type shaExecutor func(context.Context, int) ([]byte, error)

func runSHAProfiles(ctx context.Context, label string, status func(string), execute shaExecutor) (report.Run, error) {
	if err := validateLabel(label); err != nil {
		return report.Run{}, err
	}
	if err := ctx.Err(); err != nil {
		return report.Run{}, err
	}
	result := report.Run{Complete: true, Label: label, Timestamp: time.Now().Format(time.RFC3339), OS: "linux", Go: runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH, Metrics: []report.Metric{}}
	captureLinux(&result)
	for _, workers := range []int{1, 4, 8} {
		if err := ctx.Err(); err != nil {
			return report.Run{}, err
		}
		if status != nil {
			status(fmt.Sprintf("Go SHA-256 %d worker(s): 131072 blocks × 64 KiB, 8 GiB", workers))
		}
		output, err := execute(ctx, workers)
		if err != nil {
			return report.Run{}, fmt.Errorf("Go SHA-256 %d workers: %w", workers, err)
		}
		elapsed, checksum, err := parseSHAResult(output, workers)
		if err != nil {
			return report.Run{}, err
		}
		if result.GoChecksum != "" && result.GoChecksum != checksum {
			return report.Run{}, errors.New("Go SHA-256 checksum differs between worker counts")
		}
		result.GoChecksum = checksum
		result.Metrics = append(result.Metrics, report.Metric{Key: fmt.Sprintf("go.%d", workers), Label: fmt.Sprintf("Go SHA-256 · %d workers", workers), Value: elapsed, Unit: "s", Higher: false})
	}
	if err := ctx.Err(); err != nil {
		return report.Run{}, err
	}
	return result, nil
}
