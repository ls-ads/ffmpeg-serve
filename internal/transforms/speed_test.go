package transforms

import (
	"strings"
	"testing"
)

func TestAtempoChain_NoChainNeeded(t *testing.T) {
	cases := map[float64]string{
		1.0:  "atempo=1",
		1.5:  "atempo=1.5",
		0.75: "atempo=0.75",
		2.0:  "atempo=2",
		0.5:  "atempo=0.5",
	}
	for in, want := range cases {
		if got := atempoChain(in); got != want {
			t.Errorf("atempoChain(%g) = %q, want %q", in, got, want)
		}
	}
}

func TestAtempoChain_ChainAbove2(t *testing.T) {
	// 4× = 2 × 2
	got := atempoChain(4.0)
	if got != "atempo=2.0,atempo=2" {
		t.Errorf("atempoChain(4.0) = %q, want atempo=2.0,atempo=2", got)
	}
	// 3× = 2 × 1.5
	got = atempoChain(3.0)
	if got != "atempo=2.0,atempo=1.5" {
		t.Errorf("atempoChain(3.0) = %q, want atempo=2.0,atempo=1.5", got)
	}
}

func TestAtempoChain_ChainBelow0_5(t *testing.T) {
	// 0.25× = 0.5 × 0.5
	got := atempoChain(0.25)
	if got != "atempo=0.5,atempo=0.5" {
		t.Errorf("atempoChain(0.25) = %q, want atempo=0.5,atempo=0.5", got)
	}
}

func TestParseSpeedParams_FactorRequired(t *testing.T) {
	if _, err := parseSpeedParams(map[string]any{}); err == nil {
		t.Errorf("missing factor should fail")
	}
}

func TestParseSpeedParams_OutOfRange(t *testing.T) {
	for _, f := range []float64{0.1, 5.0, -1.0, 0} {
		if _, err := parseSpeedParams(map[string]any{"factor": f}); err == nil {
			t.Errorf("factor=%g should fail", f)
		}
	}
}

func TestParseSpeedParams_ValidRange(t *testing.T) {
	for _, f := range []float64{0.25, 0.5, 1.0, 2.0, 4.0} {
		p, err := parseSpeedParams(map[string]any{"factor": f})
		if err != nil {
			t.Errorf("factor=%g should pass: %v", f, err)
		}
		if p.Factor != f {
			t.Errorf("factor=%g not parsed: %+v", f, p)
		}
	}
}

func TestBuildSpeedArgs_AudioOnly(t *testing.T) {
	args := buildSpeedArgs("/in.mp3", "/out.mp3", KindAudio, "mp3", 1.5)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-filter:a atempo=1.5") {
		t.Errorf("audio speed: expected -filter:a atempo=1.5, got: %v", args)
	}
	if !strings.Contains(joined, "-vn") {
		t.Errorf("audio speed: expected -vn, got: %v", args)
	}
	if strings.Contains(joined, "setpts") {
		t.Errorf("audio speed: shouldn't use setpts, got: %v", args)
	}
}

func TestBuildSpeedArgs_VideoApplyBoth(t *testing.T) {
	args := buildSpeedArgs("/in.mp4", "/out.mp4", KindVideo, "mp4", 2.0)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-filter:v setpts=PTS/2") {
		t.Errorf("video speed: expected setpts=PTS/2, got: %v", args)
	}
	if !strings.Contains(joined, "-filter:a atempo=2") {
		t.Errorf("video speed: expected atempo=2, got: %v", args)
	}
}
