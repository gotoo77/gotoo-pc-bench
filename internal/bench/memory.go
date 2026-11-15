package bench

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

// Fixed Sysbench memory protocol, matching benchmark_pc_v6.sh exactly.
// The 1 TiB setting is a transfer limit, NOT a 1 TiB RAM allocation.
const (
	memoryDuration = "15"
	memoryBlock    = "1K"
	memoryLimit    = "1T"
)

var memorySpeed = regexp.MustCompile(`(?m)^\s*[0-9]+(?:\.[0-9]+)? MiB transferred \(([0-9]+(?:\.[0-9]+)?) MiB/sec\)\s*$`)

// CheckMemory checks for Sysbench and its version without executing a workload.
func CheckMemory() (string, string, error) { return CheckSysbench() }

// RunMemory measures one fixed 1 KiB write workload with 1 and 4 threads.
// An error or cancellation prevents the caller from receiving a partial run.
func RunMemory(ctx context.Context, label string, status func(string)) (report.Run, error) {
	if err := validateLabel(label); err != nil {
		return report.Run{}, err
	}
	path, version, err := CheckMemory()
	if err != nil {
		return report.Run{}, err
	}
	if err := ctx.Err(); err != nil {
		return report.Run{}, err
	}
	result := report.Run{Complete: true, Label: label, Timestamp: time.Now().Format(time.RFC3339), OS: "linux", Sysbench: version, Metrics: []report.Metric{}}
	captureLinux(&result)

	for _, n := range []int{1, 4} {
		if err := ctx.Err(); err != nil {
			return report.Run{}, err
		}
		if status != nil {
			status(fmt.Sprintf("RAM %d thread(s): 15s, write, block=%s, transfer limit=%s", n, memoryBlock, memoryLimit))
		}
		stepCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		cmd := exec.CommandContext(stepCtx, path, "memory", "--memory-oper=write", "--memory-block-size="+memoryBlock, "--memory-total-size="+memoryLimit, "--threads="+strconv.Itoa(n), "--time="+memoryDuration, "run")
		output, runErr := cmd.CombinedOutput()
		ctxErr := stepCtx.Err()
		cancel()
		if runErr != nil {
			return report.Run{}, fmt.Errorf("sysbench memory %d threads failed (%v; context=%v): %s", n, runErr, ctxErr, brief(output))
		}
		matches := memorySpeed.FindAllSubmatch(output, -1)
		if len(matches) != 1 {
			return report.Run{}, fmt.Errorf("invalid sysbench memory %d output (expected one MiB/sec metric): %s", n, brief(output))
		}
		value, err := strconv.ParseFloat(string(matches[0][1]), 64)
		if err != nil || value <= 0 {
			return report.Run{}, fmt.Errorf("invalid memory %d result", n)
		}
		suffix := " threads"
		if n == 1 {
			suffix = " thread"
		}
		result.Metrics = append(result.Metrics, report.Metric{Key: fmt.Sprintf("ram.%d", n), Label: fmt.Sprintf("RAM écriture · %d%s", n, suffix), Value: value, Unit: "MiB/s", Higher: true})
	}
	return result, nil
}
