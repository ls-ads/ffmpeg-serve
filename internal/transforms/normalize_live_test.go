package transforms

import (
	"context"
	"encoding/base64"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Live: 1-second sine wave through normalize, default -16 LUFS.
// Asserts the filter actually applied (we look for ffmpeg's
// loudnorm-summary stderr output via a separate verification probe;
// here we just confirm a non-empty output is produced with the
// expected format).
func TestNormalize_LiveAudio_DefaultLufs(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; live test skipped")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; live test skipped")
	}

	gen := exec.Command(ffmpeg, "-y",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:a", "libmp3lame", "-b:a", "128k",
		"-f", "mp3", "-")
	in, err := gen.Output()
	if err != nil {
		t.Fatalf("generate test MP3: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "normalize",
		Params:    map[string]any{},
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "mp3"},
	}
	outputs, err := normalize(ctx, ffmpeg, ffprobe, req)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0].Format == "" {
		t.Errorf("expected non-empty output format")
	}
	decoded, _ := base64.StdEncoding.DecodeString(outputs[0].MediaB64)
	if len(decoded) == 0 {
		t.Errorf("expected non-empty output bytes")
	}
}

// Live: image input → clear "doesn't apply to images" error.
func TestNormalize_LiveImage_RejectedCleanly(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; live test skipped")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; live test skipped")
	}

	gen := exec.Command(ffmpeg, "-y",
		"-f", "lavfi", "-i", "testsrc=size=64x64:rate=1",
		"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-")
	in, err := gen.Output()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "normalize",
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "png"},
	}
	_, err = normalize(ctx, ffmpeg, ffprobe, req)
	if err == nil {
		t.Fatal("expected error on image input")
	}
	if !strings.Contains(err.Error(), "image") {
		t.Errorf("error should mention image: %v", err)
	}
}
