package report

import (
	"strings"
	"testing"
)

func TestCPUOnlyNoFalseVideoWarning(t *testing.T) {
	r := Run{Complete: true, Sysbench: "1.0.20", Metrics: []Metric{{Key: "cpu.1", Label: "CPU", Value: 100, Unit: "événements/s", Higher: true}}}
	c := Compare(r, r)
	if len(c.Rows) != 1 || !c.Rows[0].Comparable {
		t.Fatalf("CPU result not comparable: %+v", c)
	}
	for _, w := range c.Warnings {
		if strings.Contains(w, "vidéo") || strings.Contains(w, "FFmpeg") {
			t.Fatal(w)
		}
	}
}
