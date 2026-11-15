package bench

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

// ParseTests accepts the three implemented runners only. A combined campaign
// always executes CPU, then memory, then FFmpeg, regardless of input order.
func ParseTests(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	seen := map[string]bool{}
	for _, part := range parts {
		if part != "cpu" && part != "memory" && part != "ffmpeg" {
			return nil, fmt.Errorf("unsupported benchmark %q (expected cpu, memory and/or ffmpeg)", part)
		}
		if seen[part] {
			return nil, fmt.Errorf("duplicate benchmark %q", part)
		}
		seen[part] = true
	}
	selected := make([]string, 0, len(seen))
	for _, name := range []string{"cpu", "memory", "ffmpeg"} {
		if seen[name] {
			selected = append(selected, name)
		}
	}
	return selected, nil
}

// RunSelected keeps the original API for CPU/memory callers and tests.
func RunSelected(ctx context.Context, label string, tests []string, status func(string)) (report.Run, error) {
	return RunSelectedWithVideo(ctx, label, tests, "", status)
}

// CheckSelected runs preflights only for requested protocols. A video path is
// mandatory for FFmpeg and rejected for CPU/memory-only campaigns.
func CheckSelected(ctx context.Context, tests []string, video string) (string, error) {
	if len(tests) == 0 || len(tests) > 3 {
		return "", errors.New("expected 1..3 benchmarks")
	}
	hasFFmpeg, hasSysbench := false, false
	for _, name := range tests {
		switch name {
		case "cpu", "memory":
			hasSysbench = true
		case "ffmpeg":
			hasFFmpeg = true
		default:
			return "", fmt.Errorf("unknown benchmark %q", name)
		}
	}
	if hasFFmpeg != (video != "") {
		return "", errors.New("--video is required exactly when ffmpeg is selected")
	}
	var versions []string
	if hasSysbench {
		_, version, err := CheckSysbench()
		if err != nil {
			return "", err
		}
		versions = append(versions, "sysbench "+version)
	}
	if hasFFmpeg {
		check, err := CheckFFmpeg(ctx, video)
		if err != nil {
			return "", err
		}
		versions = append(versions, "ffmpeg "+check.Version+", video sha256 "+check.SHA256)
	}
	return strings.Join(versions, "; "), nil
}

// RunSelectedWithVideo runs in stable CPU -> memory -> FFmpeg order. The
// caller retains one lock and publishes only after all runners succeed.
func RunSelectedWithVideo(ctx context.Context, label string, tests []string, video string, status func(string)) (report.Run, error) {
	if err := validateLabel(label); err != nil {
		return report.Run{}, err
	}
	if len(tests) == 0 || len(tests) > 3 {
		return report.Run{}, errors.New("expected 1..3 benchmarks")
	}
	hasFFmpeg := false
	for _, name := range tests {
		if name == "ffmpeg" {
			hasFFmpeg = true
		} else if name != "cpu" && name != "memory" {
			return report.Run{}, fmt.Errorf("unknown benchmark %q", name)
		}
	}
	if hasFFmpeg != (video != "") {
		return report.Run{}, errors.New("--video is required exactly when ffmpeg is selected")
	}
	// Hard-stop capability preflight before any expensive CPU/RAM workload.
	if _, err := CheckSelected(ctx, tests, video); err != nil {
		return report.Run{}, err
	}
	var merged report.Run
	for i, name := range tests {
		if err := ctx.Err(); err != nil {
			return report.Run{}, err
		}
		var part report.Run
		var err error
		switch name {
		case "cpu":
			part, err = RunCPU(ctx, label, status)
		case "memory":
			part, err = RunMemory(ctx, label, status)
		case "ffmpeg":
			part, err = RunFFmpeg(ctx, label, video, status)
		}
		if err != nil {
			return report.Run{}, fmt.Errorf("%s benchmark: %w", name, err)
		}
		if i == 0 {
			merged = part
		} else {
			if part.Sysbench != "" && merged.Sysbench != "" && merged.Sysbench != part.Sysbench {
				return report.Run{}, errors.New("sysbench version changed during campaign")
			}
			if merged.Sysbench == "" {
				merged.Sysbench = part.Sysbench
			}
			if merged.FFmpeg == "" {
				merged.FFmpeg = part.FFmpeg
			}
			if merged.VideoSHA256 == "" {
				merged.VideoSHA256 = part.VideoSHA256
			}
			if merged.VideoSpec == "" {
				merged.VideoSpec = part.VideoSpec
			}
			merged.Metrics = append(merged.Metrics, part.Metrics...)
		}
	}
	if err := ctx.Err(); err != nil {
		return report.Run{}, err
	}
	return merged, nil
}
