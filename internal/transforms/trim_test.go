package transforms

import (
	"strings"
	"testing"
)

func TestParseTrimParams_RequiresOneEndpoint(t *testing.T) {
	if _, err := parseTrimParams(map[string]any{}); err == nil {
		t.Errorf("empty params should fail — at least start or end is required")
	}
}

func TestParseTrimParams_StartOnly(t *testing.T) {
	p, err := parseTrimParams(map[string]any{"start_sec": float64(2.5)})
	if err != nil {
		t.Fatal(err)
	}
	if p.StartSec == nil || *p.StartSec != 2.5 {
		t.Errorf("start_sec not parsed: %+v", p)
	}
	if p.EndSec != nil {
		t.Errorf("end_sec should be nil")
	}
}

func TestParseTrimParams_EndOnly(t *testing.T) {
	p, err := parseTrimParams(map[string]any{"end_sec": float64(10)})
	if err != nil {
		t.Fatal(err)
	}
	if p.EndSec == nil || *p.EndSec != 10 {
		t.Errorf("end_sec not parsed: %+v", p)
	}
}

func TestParseTrimParams_NegativeStart(t *testing.T) {
	if _, err := parseTrimParams(map[string]any{"start_sec": float64(-1)}); err == nil {
		t.Errorf("start_sec=-1 should fail")
	}
}

func TestParseTrimParams_ZeroEnd(t *testing.T) {
	if _, err := parseTrimParams(map[string]any{"end_sec": float64(0)}); err == nil {
		t.Errorf("end_sec=0 should fail")
	}
}

func TestParseTrimParams_BadMode(t *testing.T) {
	_, err := parseTrimParams(map[string]any{
		"start_sec": float64(1),
		"mode":      "fast",
	})
	if err == nil {
		t.Errorf("mode=fast should fail")
	}
}

func TestParseTrimParams_ModeCopyAndEncode(t *testing.T) {
	for _, m := range []string{"copy", "encode"} {
		_, err := parseTrimParams(map[string]any{
			"start_sec": float64(1),
			"mode":      m,
		})
		if err != nil {
			t.Errorf("mode=%q should be accepted: %v", m, err)
		}
	}
}

func TestBuildTrimArgs_EncodeMode_PutsSeekAfterInput(t *testing.T) {
	start := 2.0
	end := 7.5
	args := buildTrimArgs("/in.mp4", "/out.mp4", KindVideo, "mp4", trimParams{
		StartSec: &start,
		EndSec:   &end,
	})
	joined := strings.Join(args, " ")
	// In encode mode, -ss must come *after* -i so it operates on
	// decoded frames (frame-accurate).
	iIdx := strings.Index(joined, "-i /in.mp4")
	ssIdx := strings.Index(joined, "-ss 2")
	if iIdx < 0 || ssIdx < 0 {
		t.Fatalf("missing -i or -ss in args: %v", args)
	}
	if ssIdx < iIdx {
		t.Errorf("encode mode: -ss should be AFTER -i, got: %v", args)
	}
}

func TestBuildTrimArgs_CopyMode_PutsSeekBeforeInput(t *testing.T) {
	start := 2.0
	args := buildTrimArgs("/in.mp4", "/out.mp4", KindVideo, "mp4", trimParams{
		StartSec: &start,
		Mode:     "copy",
	})
	joined := strings.Join(args, " ")
	iIdx := strings.Index(joined, "-i /in.mp4")
	ssIdx := strings.Index(joined, "-ss 2")
	if ssIdx >= iIdx {
		t.Errorf("copy mode: -ss should be BEFORE -i, got: %v", args)
	}
	if !strings.Contains(joined, "-c copy") {
		t.Errorf("copy mode: expected -c copy, got: %v", args)
	}
}

func TestBuildTrimArgs_AudioEncode_StripsVideo(t *testing.T) {
	start := 1.0
	args := buildTrimArgs("/in.mp3", "/out.mp3", KindAudio, "mp3", trimParams{StartSec: &start})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-vn") {
		t.Errorf("audio encode mode should -vn, got: %v", args)
	}
	if strings.Contains(joined, "libx264") {
		t.Errorf("audio encode mode shouldn't pull in libx264, got: %v", args)
	}
}
