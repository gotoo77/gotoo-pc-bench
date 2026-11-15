package report

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `
========== CONDITIONS DE MESURE ==========
2026-11-15T13:58:44+01:00
Label: avant_upgrade
OS: fedora
ea7ca7ecd8795138031a703e324952653277928bea91208058dea7a6005c44d2  /private/path/benchmark_source_ffv1.mkv
ffmpeg version 8.1.3 Copyright
sysbench 1.0.20
========== SYSTEME ET MATERIEL ==========
Machine ID: sensitive-do-not-publish
Model name: Intel(R) Core(TM) i3-4130 CPU @ 3.40GHz
Core(s) per socket: 2
Thread(s) per core: 2
Mem: 11Gi  3Gi
board_name: B85-G43 (MS-7816)
========== VIDEO SOURCE ==========
codec_name=ffv1
width=640
height=360
duration=20.000000
========== CPU SYSBENCH ==========
--- CPU 1 threads ---
    events per second:  1117.78
--- CPU 4 threads ---
    events per second:  3435.51
========== RAM SYSBENCH (1K write, 1TiB max, 15s) ==========
--- RAM 1 threads ---
  92384.28 MiB transferred (6158.24 MiB/sec)
========== ENCODAGE H264 LOGICIEL LIBX264 ==========
--- FFmpeg 1 threads ---
RESULT ffmpeg_threads=1 elapsed_seconds=4.352
========== GO SHA256 (CHARGE FIXE, 1/4/8 WORKERS) ==========
go version go1.23.2 linux/amd64
RESULT go_workers=1 gomaxprocs=1 jobs=131072 bytes=8589934592 elapsed_seconds=12.223 throughput_mib_s=670.20 checksum=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
RESULT go_workers=4 gomaxprocs=4 jobs=131072 bytes=8589934592 elapsed_seconds=4.223 throughput_mib_s=1939.90 checksum=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
========== FIN ==========
`

func TestParseKnownMetricsAndPrivacy(t *testing.T) {
	r, err := Parse("rapport_avant_upgrade_20261115_135844.txt", strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if r.CPU != "Intel(R) Core(TM) i3-4130 CPU @ 3.40GHz" || r.RAM != "11Gi" || r.Label != "avant_upgrade" {
		t.Fatalf("matériel: %+v", r)
	}
	if len(r.Metrics) != 6 {
		t.Fatalf("attendu 6 mesures: %+v", r.Metrics)
	}
	if r.GoChecksum != strings.Repeat("a", 64) {
		t.Fatalf("checksum: %s", r.GoChecksum)
	}
	if strings.Contains(r.CPU, "sensitive") {
		t.Fatal("machine ID leaked")
	}
}

func TestCompareDirectionAndInputMismatch(t *testing.T) {
	b, e := Parse("rapport_a.txt", strings.NewReader(sample))
	if e != nil {
		t.Fatal(e)
	}
	a := b
	a.ID = "rapport_b.txt"
	a.Metrics = append([]Metric{}, b.Metrics...)
	for i := range a.Metrics {
		switch a.Metrics[i].Key {
		case "cpu.1":
			a.Metrics[i].Value = 2235.56
		case "ffmpeg.1":
			a.Metrics[i].Value = 2.176
		}
	}
	c := Compare(b, a)
	for _, m := range c.Rows {
		if m.Key == "cpu.1" || m.Key == "ffmpeg.1" {
			if !m.Comparable || math.Abs(m.Improvement-100) > 0.01 {
				t.Fatalf("ratio: %+v", m)
			}
		}
	}
	a.VideoSHA256 = strings.Repeat("f", 64)
	c = Compare(b, a)
	for _, m := range c.Rows {
		if m.Key == "ffmpeg.1" && m.Comparable {
			t.Fatal("FFmpeg different source must not be comparable")
		}
	}
}

func TestCompareGoChecksumMismatch(t *testing.T) {
	b, e := Parse("rapport_a.txt", strings.NewReader(sample))
	if e != nil {
		t.Fatal(e)
	}
	a := b
	a.GoChecksum = strings.Repeat("b", 64)
	for _, m := range Compare(b, a).Rows {
		if strings.HasPrefix(m.Key, "go.") && m.Comparable {
			t.Fatal("Go checksum mismatch accepted")
		}
	}
}

func TestLoadSkipsSymlinkAndMalformed(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "rapport_good.txt"), []byte(sample), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "rapport_broken.txt"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("rapport_good.txt", filepath.Join(d, "rapport_alias.txt")); err != nil {
		t.Fatal(err)
	}
	runs, err := Load(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("attendu 1 rapport, got %d", len(runs))
	}
}

func TestBrokenGoChecksumsWarning(t *testing.T) {
	changed := strings.Replace(sample, strings.Repeat("a", 64), strings.Repeat("b", 64), 1)
	r, err := Parse("rapport_a.txt", strings.NewReader(changed))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Warnings) == 0 {
		t.Fatal("warnings not generated")
	}
}

func TestRejectUnknownReport(t *testing.T) {
	_, err := Parse("rapport_wrong.txt", strings.NewReader("nothing"))
	if err == nil {
		t.Fatal("invalid report accepted")
	}
}

func TestIncompleteReportIsNotComparable(t *testing.T) {
	b, err := Parse("rapport_a.txt", strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	a, err := Parse("rapport_b.txt", strings.NewReader(strings.Replace(sample, "========== FIN ==========", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if a.Complete {
		t.Fatal("partial report marked complete")
	}
	for _, m := range Compare(b, a).Rows {
		if m.Comparable {
			t.Fatalf("partial report comparable: %+v", m)
		}
	}
}
