package transforms

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Live: 64×64 PNG → 4× lanczos resize. Asserts output dims are
// 256×256 and the format round-trips.
func TestResize_LiveImage_4xLanczos(t *testing.T) {
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
		t.Fatalf("generate test PNG: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "resize",
		Params:    map[string]any{"scale": float64(4), "method": "lanczos"},
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "png"},
	}
	outputs, err := resize(ctx, ffmpeg, ffprobe, req)
	if err != nil {
		t.Fatalf("resize: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	decoded, _ := base64.StdEncoding.DecodeString(outputs[0].MediaB64)
	tmpf := filepath.Join(t.TempDir(), "out."+outputs[0].Format)
	if err := os.WriteFile(tmpf, decoded, 0o644); err != nil {
		t.Fatal(err)
	}
	probeCmd := exec.Command(ffprobe, "-v", "error",
		"-show_entries", "stream=width,height", "-of", "csv=p=0", tmpf)
	pout, err := probeCmd.Output()
	if err != nil {
		t.Fatalf("probe output: %v", err)
	}
	got := strings.TrimSpace(string(pout))
	if got != "256,256" {
		t.Errorf("output dims = %q, want \"256,256\"", got)
	}
}

// Live: 2× nearest-neighbor on a small image (the pixel-art preset).
func TestResize_LiveImage_NearestNeighbor(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; live test skipped")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; live test skipped")
	}

	gen := exec.Command(ffmpeg, "-y",
		"-f", "lavfi", "-i", "testsrc=size=32x32:rate=1",
		"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-")
	in, err := gen.Output()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "resize",
		Params:    map[string]any{"scale": float64(2), "method": "neighbor"},
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "png"},
	}
	outputs, err := resize(ctx, ffmpeg, ffprobe, req)
	if err != nil {
		t.Fatalf("resize neighbor: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
}

// Live: audio rejected.
func TestResize_LiveAudio_Rejected(t *testing.T) {
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
		"-f", "mp3", "-")
	in, err := gen.Output()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "resize",
		Params:    map[string]any{"scale": float64(2)},
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "mp3"},
	}
	_, err = resize(ctx, ffmpeg, ffprobe, req)
	if err == nil {
		t.Fatal("expected error on audio")
	}
	if !strings.Contains(err.Error(), "audio") {
		t.Errorf("error should mention audio: %v", err)
	}
}
