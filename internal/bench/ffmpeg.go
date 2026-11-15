package bench

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"time"

	"github.com/gotoo77/gotoo-pc-bench/internal/report"
)

// FFmpeg uses the fixed benchmark_pc_v6.sh encoding workload. The input must
// already exist: the runner never generates, changes or deletes the source.
const maxVideoSize = 512 << 20 // bound input hashing and decoding work

var (
	ffmpegVersion = regexp.MustCompile(`^ffmpeg version (\S+)`)
	x264Encoder   = regexp.MustCompile(`(?m)^\s*V\S*\s+libx264\s`)
	encodedFrame  = regexp.MustCompile(`\bframe=\s*(\d+)\b`)
)

type VideoCheck struct {
	Path    string
	Version string
	SHA256  string
	Spec    string
}

func ffmpegCmd(ctx context.Context, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	return cmd.CombinedOutput()
}

func ffmpegCall(ctx context.Context, binary string, timeout time.Duration, args ...string) ([]byte, error) {
	step, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := ffmpegCmd(step, binary, args...)
	if err != nil {
		return nil, fmt.Errorf("command failed (context=%v, error=%w): %s", step.Err(), err, brief(output))
	}
	return output, nil
}

func videoHash(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	st, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("video source: %w", err)
	}
	if !st.Mode().IsRegular() || st.Size() <= 0 || st.Size() > maxVideoSize {
		return "", errors.New("video must be a nonempty regular file of at most 512 MiB (symlinks refused)")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	openSt, err := f.Stat()
	if err != nil || !openSt.Mode().IsRegular() || !os.SameFile(st, openSt) {
		return "", errors.New("video source changed while opening")
	}
	h := sha256.New()
	buf := make([]byte, 128<<10)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, e := f.Read(buf)
		if n > 0 {
			if _, werr := h.Write(buf[:n]); werr != nil {
				return "", werr
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// CheckFFmpeg verifies the codec, historical source shape and encoder usability.
// It never installs tools or launches the actual 600-frame benchmark workload.
func CheckFFmpeg(ctx context.Context, video string) (VideoCheck, error) {
	if runtime.GOOS != "linux" {
		return VideoCheck{}, fmt.Errorf("FFmpeg runner requires Linux (current OS: %s)", runtime.GOOS)
	}
	if video == "" {
		return VideoCheck{}, errors.New("--video is required when --test includes ffmpeg")
	}
	hash, err := videoHash(ctx, video)
	if err != nil {
		return VideoCheck{}, err
	}
	binary, err := exec.LookPath("ffmpeg")
	if err != nil {
		return VideoCheck{}, errors.New("ffmpeg missing; install ffmpeg with libx264 using your Linux package manager")
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		return VideoCheck{}, errors.New("ffprobe missing; install alongside ffmpeg")
	}
	ver, err := ffmpegCall(ctx, binary, 5*time.Second, "-version")
	if err != nil {
		return VideoCheck{}, fmt.Errorf("ffmpeg -version: %w", err)
	}
	m := ffmpegVersion.FindStringSubmatch(string(ver))
	if len(m) != 2 {
		return VideoCheck{}, errors.New("unrecognized ffmpeg version")
	}
	encoders, err := ffmpegCall(ctx, binary, 5*time.Second, "-hide_banner", "-encoders")
	if err != nil || !x264Encoder.Match(encoders) {
		return VideoCheck{}, errors.New("libx264 encoder unavailable in ffmpeg")
	}
	info, err := ffmpegCall(ctx, probe, 10*time.Second, "-v", "error", "-select_streams", "v:0", "-show_entries", "format=duration,size:stream=codec_name,width,height,r_frame_rate,pix_fmt", "-of", "default=noprint_wrappers=1", video)
	if err != nil {
		return VideoCheck{}, fmt.Errorf("ffprobe video: %w", err)
	}
	fields := map[string]string{}
	for _, line := range bytes.Split(info, []byte{'\n'}) {
		pair := strings.SplitN(strings.TrimSpace(string(line)), "=", 2)
		if len(pair) == 2 {
			if _, exists := fields[pair[0]]; exists {
				return VideoCheck{}, errors.New("duplicate video metadata field")
			}
			fields[pair[0]] = pair[1]
		}
	}
	duration, derr := strconv.ParseFloat(fields["duration"], 64)
	if fields["codec_name"] != "ffv1" || fields["width"] != "640" || fields["height"] != "360" || fields["pix_fmt"] != "yuv420p" || fields["r_frame_rate"] != "30/1" || derr != nil || math.Abs(duration-20) > 0.001 {
		return VideoCheck{}, fmt.Errorf("video does not match historical FFV1 640x360 30fps 20s yuv420p protocol: %s", brief(info))
	}
	// A listed encoder may still fail in practice. Mirror the v6 preflight.
	_, err = ffmpegCall(ctx, binary, 20*time.Second, "-nostdin", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=1:duration=1", "-frames:v", "1", "-an", "-c:v", "libx264", "-threads", "1", "-f", "null", "-")
	if err != nil {
		return VideoCheck{}, fmt.Errorf("libx264 encoder failed smoke preflight: %w", err)
	}
	return VideoCheck{Path: binary, Version: m[1], SHA256: hash, Spec: "ffv1 640x360 20.000000s"}, nil
}

// RunFFmpeg measures wall-clock time (including process startup) exactly as
// benchmark_pc_v6.sh did, with 1/4/8 encoder threads and a warmed source.
func RunFFmpeg(ctx context.Context, label, video string, status func(string)) (report.Run, error) {
	if err := validateLabel(label); err != nil {
		return report.Run{}, err
	}
	check, err := CheckFFmpeg(ctx, video)
	if err != nil {
		return report.Run{}, err
	}
	result := report.Run{Complete: true, Label: label, Timestamp: time.Now().Format(time.RFC3339), OS: "linux", FFmpeg: check.Version, VideoSHA256: check.SHA256, VideoSpec: check.Spec, Metrics: []report.Metric{}}
	captureLinux(&result)
	// v6: `cat "$video" > /dev/null` to warm file cache, outside timing.
	if status != nil {
		status("FFmpeg: warming source cache (not timed)")
	}
	if err := warmVideo(ctx, video); err != nil {
		return report.Run{}, err
	}
	for _, n := range []int{1, 4, 8} {
		if err := ctx.Err(); err != nil {
			return report.Run{}, err
		}
		if status != nil {
			status(fmt.Sprintf("FFmpeg H.264 libx264 %d thread(s): medium, CRF 23, output null", n))
		}
		stepCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		start := time.Now()
		output, runErr := ffmpegCmd(stepCtx, check.Path, "-nostdin", "-hide_banner", "-loglevel", "info", "-nostats", "-benchmark", "-i", video, "-map", "0:v:0", "-an", "-sn", "-dn", "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p", "-threads", strconv.Itoa(n), "-f", "null", "-")
		elapsed := time.Since(start)
		ctxErr := stepCtx.Err()
		cancel()
		if runErr != nil {
			return report.Run{}, fmt.Errorf("ffmpeg %d threads failed (%v; context=%v): %s", n, runErr, ctxErr, brief(output))
		}
		// Even a zero-exit encode must demonstrate the full 600-frame workload.
		frames := encodedFrame.FindAllSubmatch(output, -1)
		if len(frames) == 0 || string(frames[len(frames)-1][1]) != "600" {
			return report.Run{}, fmt.Errorf("ffmpeg %d threads did not confirm 600 encoded frames", n)
		}
		seconds := math.Round(elapsed.Seconds()*1000) / 1000
		if seconds <= 0 {
			return report.Run{}, errors.New("FFmpeg duration too short for v1 precision")
		}
		suffix := " threads"
		if n == 1 {
			suffix = " thread"
		}
		result.Metrics = append(result.Metrics, report.Metric{Key: fmt.Sprintf("ffmpeg.%d", n), Label: fmt.Sprintf("FFmpeg H.264 · %d%s", n, suffix), Value: seconds, Unit: "s", Higher: false})
	}
	// Do not publish a mixed-source run if the file changed while encoding.
	finalHash, err := videoHash(ctx, video)
	if err != nil || finalHash != check.SHA256 {
		return report.Run{}, errors.New("video source changed during encoding; report discarded")
	}
	return result, nil
}

func warmVideo(ctx context.Context, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 128<<10)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := f.Read(buf)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
