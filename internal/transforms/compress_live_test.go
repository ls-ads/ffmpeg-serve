package transforms

import (
	"context"
	"encoding/base64"
	"os/exec"
	"testing"
	"time"
)

// TestCompress_LiveImage_Roundtrip exercises the full
// stage→ffprobe→ffmpeg→read pipeline against the real ffmpeg binary.
// Skips cleanly when ffmpeg/ffprobe aren't installed (CI without
// ffmpeg in PATH).
//
// Generates a small JPEG test pattern via ffmpeg's `lavfi` source so
// we don't need fixture files in the repo. Asserts the compressed
// output is smaller than the input — quality 25 of a noisy random
// pattern reliably halves it on this fixture, but we use a generous
// margin (any reduction at all) to stay robust across ffmpeg versions.
func TestCompress_LiveImage_Roundtrip(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH; live test skipped")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not on PATH; live test skipped")
	}

	// Generate a 256×256 JPEG via ffmpeg's testsrc filter — easier
	// than checking in fixture bytes. Output to stdout, capture.
	gen := exec.Command(ffmpeg, "-y",
		"-f", "lavfi", "-i", "testsrc=size=256x256:rate=1",
		"-frames:v", "1", "-f", "mjpeg", "-q:v", "2", "-")
	in, err := gen.Output()
	if err != nil {
		t.Fatalf("generate test JPEG: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := Request{
		Transform: "compress",
		Params: map[string]any{
			"quality": float64(25), // aggressive compression
			"format":  "jpg",
		},
		Media: Media{
			InputB64: base64.StdEncoding.EncodeToString(in),
			Format:   "jpg",
		},
	}

	outputs, err := compress(ctx, ffmpeg, ffprobe, req)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	out := outputs[0]
	if out.Format != "jpg" {
		t.Errorf("expected format=jpg, got %q", out.Format)
	}
	decoded, err := base64.StdEncoding.DecodeString(out.MediaB64)
	if err != nil {
		t.Fatalf("decode media_b64: %v", err)
	}
	if len(decoded) >= len(in) {
		t.Errorf("compressed output (%d B) should be smaller than input (%d B) at quality=25",
			len(decoded), len(in))
	}
	if out.ExecMS <= 0 {
		t.Errorf("exec_ms should be > 0, got %d", out.ExecMS)
	}
}
