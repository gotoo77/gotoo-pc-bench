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

func TestCompareOnlyWarnsForCommonMetricFamilies(t *testing.T) {
	full := Run{Complete: true, Sysbench: "1.0.20", FFmpeg: "7.0", VideoSHA256: "",
		Metrics: []Metric{
			{Key: "cpu.1", Value: 100, Unit: "événements/s", Higher: true},
			{Key: "ffmpeg.1", Value: 3, Unit: "s"},
		}}
	cpuOnly := Run{Complete: true, Sysbench: "1.0.20", Metrics: []Metric{{Key: "cpu.1", Value: 110, Unit: "événements/s", Higher: true}}}
	cmp := Compare(cpuOnly, full)
	if len(cmp.Rows) != 1 || !cmp.Rows[0].Comparable || len(cmp.Warnings) != 0 {
		t.Fatalf("CPU-only comparison has unrelated warning: %+v", cmp)
	}
	videoOnly := Run{Complete: true, FFmpeg: "7.0", Metrics: []Metric{{Key: "ffmpeg.1", Value: 2, Unit: "s"}}}
	cmp = Compare(videoOnly, full)
	if len(cmp.Warnings) != 1 || !strings.Contains(cmp.Warnings[0], "SHA-256 vidéo") || cmp.Rows[0].Comparable {
		t.Fatalf("FFmpeg warning or guard missing: %+v", cmp)
	}
	goOnly := Run{Complete: true, Go: "go1.23", Metrics: []Metric{{Key: "go.1", Value: 1, Unit: "s"}}}
	cmp = Compare(goOnly, full)
	if len(cmp.Rows) != 0 || len(cmp.Warnings) != 0 {
		t.Fatalf("no shared metric must have no family warnings: %+v", cmp)
	}
}
