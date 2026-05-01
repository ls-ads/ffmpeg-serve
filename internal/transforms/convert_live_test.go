package transforms

import (
	"context"
	"encoding/base64"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Live: PNG → WebP. Asserts the output reads back as a webp file.
func TestConvert_LiveImage_PngToWebp(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; live test skipped")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; live test skipped")
	}

	gen := exec.Command(ffmpeg, "-y",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=1",
		"-frames:v", "1", "-f", "image2pipe", "-vcodec", "png", "-")
	in, err := gen.Output()
	if err != nil {
		t.Fatalf("generate test PNG: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "convert",
		Params:    map[string]any{"to": "webp"},
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "png"},
	}

	outputs, err := convert(ctx, ffmpeg, ffprobe, req)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0].Format != "webp" {
		t.Errorf("expected format=webp, got %q", outputs[0].Format)
	}
	decoded, _ := base64.StdEncoding.DecodeString(outputs[0].MediaB64)
	// WebP magic: "RIFF....WEBP"
	if len(decoded) < 12 || string(decoded[0:4]) != "RIFF" || string(decoded[8:12]) != "WEBP" {
		t.Errorf("output bytes don't look like a WebP file (head: %x)", decoded[:min(16, len(decoded))])
	}
}

// Live: cross-type rejection. PNG input → "to: mp4" should error
// before any encoding happens.
func TestConvert_LiveImage_RejectedCrossType(t *testing.T) {
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
		Transform: "convert",
		Params:    map[string]any{"to": "mp4"},
		Media:     Media{InputB64: base64.StdEncoding.EncodeToString(in), Format: "png"},
	}
	_, err = convert(ctx, ffmpeg, ffprobe, req)
	if err == nil {
		t.Fatal("expected error converting image to mp4")
	}
	if !strings.Contains(err.Error(), "cannot convert") {
		t.Errorf("error should explain cross-type rejection: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
