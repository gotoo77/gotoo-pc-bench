package bench

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

// ParseTests accepts the two implemented runners only. A combined campaign
// always executes CPU then memory, regardless of the order typed by the user.
func ParseTests(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	seen := map[string]bool{}
	for _, part := range parts {
		if part != "cpu" && part != "memory" {
			return nil, fmt.Errorf("unsupported benchmark %q (expected cpu, memory or cpu,memory)", part)
		}
		if seen[part] {
			return nil, fmt.Errorf("duplicate benchmark %q", part)
		}
		seen[part] = true
	}
	selected := make([]string, 0, len(seen))
	for _, name := range []string{"cpu", "memory"} {
		if seen[name] {
			selected = append(selected, name)
		}
	}
	return selected, nil
}

// RunSelected reuses the existing fixed CPU and RAM protocols. A combined run
// is returned only when both complete successfully; the CLI publishes once.
// The caller owns the process-wide benchmark lock and cancellation context.
func RunSelected(ctx context.Context, label string, tests []string, status func(string)) (report.Run, error) {
	if err := validateLabel(label); err != nil {
		return report.Run{}, err
	}
	if len(tests) == 0 || len(tests) > 2 {
		return report.Run{}, errors.New("expected CPU and/or memory benchmarks")
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
		default:
			return report.Run{}, fmt.Errorf("unknown benchmark %q", name)
		}
		if err != nil {
			return report.Run{}, fmt.Errorf("%s benchmark: %w", name, err)
		}
		if i == 0 {
			merged = part
		} else {
			if merged.Sysbench == "" || merged.Sysbench != part.Sysbench {
				return report.Run{}, errors.New("sysbench version changed during campaign")
			}
			merged.Metrics = append(merged.Metrics, part.Metrics...)
		}
	}
	if err := ctx.Err(); err != nil {
		return report.Run{}, err
	}
	return merged, nil
}
