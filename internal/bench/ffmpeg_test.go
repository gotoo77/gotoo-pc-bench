package bench

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

// Fake commands deliberately reject extra/missing args to ensure that we use
// the historical v6 protocol rather than an unverified similar FFmpeg test.
func fakeVideoTools(t *testing.T) (source, log string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only runner")
	}
	bin := t.TempDir()
	source = filepath.Join(bin, "source.mkv")
	if err := os.WriteFile(source, []byte("fixed test source bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	log = filepath.Join(bin, "ffmpeg_calls")
	t.Setenv("FF_CALLS", log)
	ffmpeg := `#!/bin/sh
printf '%s\n' "$*" >> "$FF_CALLS"
case "$*" in
  "-version") echo 'ffmpeg version 8.1.3 Copyright fake';;
  "-hide_banner -encoders") echo ' V....D libx264 H.264';;
  "-nostdin -hide_banner -loglevel error -f lavfi -i testsrc2=size=64x64:rate=1:duration=1 -frames:v 1 -an -c:v libx264 -threads 1 -f null -")
    if [ "$FAIL_SMOKE" = 1 ]; then exit 12; fi ;;
  *)
    case "$*" in
      *" -i "*" -map 0:v:0 -an -sn -dn -c:v libx264 -preset medium -crf 23 -pix_fmt yuv420p -threads "*" -f null -")
        if [ "$FAIL_ENCODE" = 1 ]; then exit 15; fi
        if [ "$INCOMPLETE_ENCODE" = 1 ]; then echo 'frame= 599' >&2; exit 0; fi
        sleep 0.015
        echo 'frame=  600 fps=0 Lsize=N/A' >&2 ;;
      *) echo "unexpected ffmpeg command: $*" >&2; exit 14;;
    esac ;;
esac`
	ffprobe := `#!/bin/sh
case "$*" in
  "-v error -select_streams v:0 -show_entries format=duration,size:stream=codec_name,width,height,r_frame_rate,pix_fmt -of default=noprint_wrappers=1 "*)
    if [ "$INVALID_VIDEO" = 1 ]; then echo 'codec_name=unknown'; exit 0; fi
    printf '%s\n' 'codec_name=ffv1' 'width=640' 'height=360' 'pix_fmt=yuv420p' 'r_frame_rate=30/1' 'duration=20.000000' 'size=23' ;;
  *) exit 17;;
esac`
	for name, script := range map[string]string{"ffmpeg": ffmpeg, "ffprobe": ffprobe} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script+"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return source, log
}

func TestFFmpegV6ProtocolAndJSONRoundtrip(t *testing.T) {
	video, log := fakeVideoTools(t)
	ck, err := CheckFFmpeg(context.Background(), video)
	if err != nil || ck.Version != "8.1.3" || len(ck.SHA256) != 64 || ck.Spec != "ffv1 640x360 20.000000s" {
		t.Fatalf("preflight: %+v, %v", ck, err)
	}
	var updates []string
	run, err := RunFFmpeg(context.Background(), "video_probe", video, func(s string) { updates = append(updates, s) })
	if err != nil {
		t.Fatal(err)
	}
	if !run.Complete || len(updates) != 4 || len(run.Metrics) != 3 || run.VideoSHA256 != ck.SHA256 || run.FFmpeg != ck.Version {
		t.Fatalf("bad FFmpeg result: %+v, %v", run, updates)
	}
	for i, key := range []string{"ffmpeg.1", "ffmpeg.4", "ffmpeg.8"} {
		m := run.Metrics[i]
		if m.Key != key || m.Unit != "s" || m.Higher || m.Value <= 0 {
			t.Fatalf("invalid metric: %+v", m)
		}
	}
	contents, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"1", "4", "8"} {
		want := "-nostdin -hide_banner -loglevel info -nostats -benchmark -i " + video + " -map 0:v:0 -an -sn -dn -c:v libx264 -preset medium -crf 23 -pix_fmt yuv420p -threads " + n + " -f null -"
		if !strings.Contains(string(contents), want+"\n") {
			t.Fatalf("missing exact v6 encode command: %s\n%s", want, contents)
		}
	}
	// Persist using exactly the same schema/loader as historical results.
	out, err := WriteResult(t.TempDir(), run)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	got, err := report.ParseJSON(filepath.Base(out), bytes.NewReader(data))
	if err != nil || len(got.Metrics) != 3 || got.VideoSHA256 != ck.SHA256 || strings.Contains(string(data), video) {
		t.Fatalf("JSON roundtrip/path privacy failed: %+v %v", got, err)
	}
}

func TestFFmpegPreflightRejectsUnsafeSourceAndUnavailableEncoder(t *testing.T) {
	video, _ := fakeVideoTools(t)
	if _, err := CheckFFmpeg(context.Background(), ""); err == nil {
		t.Fatal("empty video accepted")
	}
	if _, err := CheckFFmpeg(context.Background(), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing video accepted")
	}
	symlink := filepath.Join(t.TempDir(), "link.mkv")
	if err := os.Symlink(video, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckFFmpeg(context.Background(), symlink); err == nil {
		t.Fatal("symlink accepted")
	}
	t.Setenv("INVALID_VIDEO", "1")
	if _, err := CheckFFmpeg(context.Background(), video); err == nil {
		t.Fatal("wrong video protocol accepted")
	}
	t.Setenv("INVALID_VIDEO", "0")
	t.Setenv("FAIL_SMOKE", "1")
	if _, err := CheckFFmpeg(context.Background(), video); err == nil {
		t.Fatal("unusable libx264 accepted")
	}
}

func TestFFmpegFailuresNeverExposePartialResults(t *testing.T) {
	video, _ := fakeVideoTools(t)
	for _, name := range []string{"FAIL_ENCODE", "INCOMPLETE_ENCODE"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "1")
			r, err := RunFFmpeg(context.Background(), "x", video, nil)
			if err == nil || len(r.Metrics) != 0 {
				t.Fatalf("bad partial run: %+v %v", r, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r, err := RunFFmpeg(ctx, "x", video, nil); err == nil || len(r.Metrics) != 0 {
		t.Fatal("accepted cancellation")
	}
}

func TestFFmpegSelectionStableAndHardStopBeforeCPU(t *testing.T) {
	video, _ := fakeVideoTools(t)
	_, _ = fakeSysbench(t, `if [ "$1" = "--version" ]; then echo 'sysbench 1.0.20'; exit 0; fi
 echo "$*" >> "$CPU_CALLS"
 case "$1" in cpu) echo 'events per second: 1234.56';; memory) echo '1024.00 MiB transferred (5000.25 MiB/sec)';; *) exit 9;; esac`)
	cpuCalls := filepath.Join(t.TempDir(), "cpu_calls")
	t.Setenv("CPU_CALLS", cpuCalls)
	selection, err := ParseTests("ffmpeg,memory,cpu")
	if err != nil || !reflect.DeepEqual(selection, []string{"cpu", "memory", "ffmpeg"}) {
		t.Fatalf("selection: %v %v", selection, err)
	}
	t.Setenv("FAIL_SMOKE", "1")
	if got, err := RunSelectedWithVideo(context.Background(), "full", selection, video, nil); err == nil || len(got.Metrics) != 0 {
		t.Fatalf("did not hard-stop: %+v %v", got, err)
	}
	if _, err := os.Stat(cpuCalls); !os.IsNotExist(err) {
		t.Fatalf("CPU executed before FFmpeg preflight, stat=%v", err)
	}
	t.Setenv("FAIL_SMOKE", "0")
	got, err := RunSelectedWithVideo(context.Background(), "full", selection, video, nil)
	if err != nil || len(got.Metrics) != 8 || got.Sysbench != "1.0.20" || got.FFmpeg != "8.1.3" || len(got.VideoSHA256) != 64 {
		t.Fatalf("combined run failed: %+v %v", got, err)
	}
	cmp := report.Compare(got, got)
	if len(cmp.Rows) != 8 || len(cmp.Warnings) != 0 {
		t.Fatalf("mixed report was not comparable to itself: %+v", cmp)
	}
}
