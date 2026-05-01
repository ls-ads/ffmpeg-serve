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

// Live integration: round-trip a 1920×1080 JPEG through reframe →
// 9:16 blur-pad. Asserts the output is 1080×1920 per outputDims rules.
// Skips when ffmpeg/ffprobe aren't on PATH.
func TestReframe_LiveImage_BlurPadTo9by16(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; live test skipped")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; live test skipped")
	}

	gen := exec.Command(ffmpeg, "-y",
		"-f", "lavfi", "-i", "testsrc=size=1920x1080:rate=1",
		"-frames:v", "1", "-f", "mjpeg", "-q:v", "2", "-")
	in, err := gen.Output()
	if err != nil {
		t.Fatalf("generate test JPEG: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "reframe",
		Params:    map[string]any{"to": "9:16"},
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "jpg"},
	}

	outputs, err := reframe(ctx, ffmpeg, ffprobe, req)
	if err != nil {
		t.Fatalf("reframe: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}

	decoded, _ := base64.StdEncoding.DecodeString(outputs[0].MediaB64)
	tmpf := filepath.Join(t.TempDir(), "out.jpg")
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
	if got != "1080,1920" {
		t.Errorf("output dims = %q, want \"1080,1920\"", got)
	}
}

// Audio inputs error cleanly with a clear message before any ffmpeg
// filter chain is built.
func TestReframe_LiveAudio_RejectedCleanly(t *testing.T) {
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
		t.Fatalf("generate test MP3: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "reframe",
		Params:    map[string]any{"to": "9:16"},
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "mp3"},
	}

	_, err = reframe(ctx, ffmpeg, ffprobe, req)
	if err == nil {
		t.Fatal("expected error on audio input")
	}
	if !strings.Contains(err.Error(), "audio") {
		t.Errorf("error should mention audio: %v", err)
	}
}
