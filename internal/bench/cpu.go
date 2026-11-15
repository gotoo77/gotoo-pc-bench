// Package bench runs the first integrated Linux benchmark protocol.
// It deliberately does not expose a generic plugin registry until another
// built-in benchmark provides a concrete second consumer.
package bench

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

const cpuPrimeLimit = "10000"
const cpuDuration = "30"

var cpuSpeeds = regexp.MustCompile(`(?m)^\s*events per second:\s*([0-9]+(?:\.[0-9]+)?)\s*$`)
var versionPattern = regexp.MustCompile(`^sysbench\s+(\S+)`)

// CheckCPU fails closed when the requested platform or benchmark executable
// is unsupported. It never installs dependencies or modifies system settings.
func CheckCPU() (string, string, error) {
	if runtime.GOOS != "linux" {
		return "", "", fmt.Errorf("CPU runner not supported on %s; only report viewing is portable today", runtime.GOOS)
	}
	path, err := exec.LookPath("sysbench")
	if err != nil {
		return "", "", errors.New("sysbench missing; install with your Linux package manager (Fedora: sudo dnf install sysbench)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("sysbench --version failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	m := versionPattern.FindStringSubmatch(strings.TrimSpace(string(output)))
	if len(m) != 2 {
		return "", "", errors.New("unrecognized sysbench version output")
	}
	return path, m[1], nil
}

// RunCPU uses exactly the parameters from benchmark_pc_v6.sh for CPU.
// The runner permits only this fixed protocol and only these thread counts.
func RunCPU(ctx context.Context, label string, status func(string)) (report.Run, error) {
	if label == "" || len(label) > 128 || !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(label) {
		return report.Run{}, errors.New("label must contain 1..128 ASCII letters, digits, underscores or hyphens")
	}
	path, version, err := CheckCPU()
	if err != nil {
		return report.Run{}, err
	}
	started := time.Now()
	r := report.Run{Complete: true, Label: label, Timestamp: started.Format(time.RFC3339), OS: "linux", Sysbench: version, Metrics: []report.Metric{}}
	captureLinux(&r)
	for _, n := range []int{1, 4, 8} {
		if err := ctx.Err(); err != nil {
			return report.Run{}, err
		}
		if status != nil {
			status(fmt.Sprintf("CPU %d thread(s): 30s, prime limit %s", n, cpuPrimeLimit))
		}
		testCtx, cancel := context.WithTimeout(ctx, 65*time.Second)
		cmd := exec.CommandContext(testCtx, path, "cpu", "--cpu-max-prime="+cpuPrimeLimit, "--threads="+strconv.Itoa(n), "--time="+cpuDuration, "run")
		output, callErr := cmd.CombinedOutput()
		timedOut := testCtx.Err()
		cancel()
		if callErr != nil {
			return report.Run{}, fmt.Errorf("sysbench cpu %d threads failed (%v; context=%v): %s", n, callErr, timedOut, brief(output))
		}
		matches := cpuSpeeds.FindAllSubmatch(output, -1)
		if len(matches) != 1 {
			return report.Run{}, fmt.Errorf("invalid sysbench CPU %d output (expected one events/s metric): %s", n, brief(output))
		}
		value, err := strconv.ParseFloat(string(matches[0][1]), 64)
		if err != nil || value <= 0 {
			return report.Run{}, fmt.Errorf("invalid CPU %d result", n)
		}
		suffix := " threads"
		if n == 1 {
			suffix = " thread"
		}
		r.Metrics = append(r.Metrics, report.Metric{Key: fmt.Sprintf("cpu.%d", n), Label: fmt.Sprintf("CPU · %d%s", n, suffix), Value: value, Unit: "événements/s", Higher: true})
	}
	return r, nil
}

func brief(output []byte) string {
	const max = 512
	s := strings.TrimSpace(string(output))
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

func captureLinux(r *report.Run) {
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "ID=") {
				r.OS = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
				break
			}
		}
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		r.Kernel = "Linux " + strings.TrimSpace(string(b))
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				f := strings.Fields(line)
				if len(f) >= 2 {
					if kb, e := strconv.ParseUint(f[1], 10, 64); e == nil {
						r.RAM = fmt.Sprintf("%.1f GiB", float64(kb)/1048576)
					}
				}
				break
			}
		}
	}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(b))
		model, cores, siblings := "", 0, 0
		for scanner.Scan() {
			s := strings.SplitN(scanner.Text(), ":", 2)
			if len(s) != 2 {
				continue
			}
			key := strings.TrimSpace(s[0])
			val := strings.TrimSpace(s[1])
			switch key {
			case "model name":
				if model == "" {
					model = val
				}
			case "cpu cores":
				if cores == 0 {
					cores, _ = strconv.Atoi(val)
				}
			case "siblings":
				if siblings == 0 {
					siblings, _ = strconv.Atoi(val)
				}
			}
		}
		if model != "" {
			r.CPU = model
		}
		if cores > 0 {
			r.Cores = strconv.Itoa(cores)
			if siblings >= cores && siblings%cores == 0 {
				r.Threads = strconv.Itoa(siblings / cores)
			}
		}
	}
	if r.CPU == "" {
		r.CPU = runtime.GOARCH
	}
}

// WriteResult commits a private JSON report with no overwrite. The temporary
// file is invisible to report.Load until it has been fully flushed and linked.
func WriteResult(dir string, r report.Run) (string, error) {
	now := time.Now()
	name := "rapport_" + r.Label + "_" + now.Format("20060102_150405") + fmt.Sprintf("_%09d", now.Nanosecond()) + ".json"
	r.ID = name
	if err := report.ValidateRun(r); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".pc-bench-*.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return "", err
	}
	if err := report.WriteJSON(tmp, r); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	target := filepath.Join(dir, name)
	// Linux hardlink is atomic and refuses existing names, including symlinks.
	if err := os.Link(tmpPath, target); err != nil {
		return "", fmt.Errorf("publish result without overwrite: %w", err)
	}
	return target, nil
}
