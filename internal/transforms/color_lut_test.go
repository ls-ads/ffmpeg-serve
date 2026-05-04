package transforms

import (
	"strings"
	"testing"
)

func TestBuildLUTFilter_FullIntensity(t *testing.T) {
	got := buildLUTFilter("/tmp/grade.cube", colorLUTParams{})
	want := "lut3d=/tmp/grade.cube"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildLUTFilter_PartialIntensity(t *testing.T) {
	intensity := 0.5
	got := buildLUTFilter("/tmp/grade.cube", colorLUTParams{Intensity: &intensity})
	if !strings.Contains(got, "split=2") {
		t.Errorf("partial intensity should split for blend: %q", got)
	}
	if !strings.Contains(got, "lut3d=/tmp/grade.cube") {
		t.Errorf("partial intensity should still apply lut3d: %q", got)
	}
	if !strings.Contains(got, "all_opacity=0.5") {
		t.Errorf("blend opacity missing: %q", got)
	}
}

func TestBuildLUTFilter_AlmostFullIntensity(t *testing.T) {
	// Anything ≥ 0.999 should snap to the simple lut3d path —
	// the blend filter is wasted overhead in that range.
	intensity := 0.9999
	got := buildLUTFilter("/tmp/grade.cube", colorLUTParams{Intensity: &intensity})
	if strings.Contains(got, "split") {
		t.Errorf("near-full intensity shouldn't trigger blend, got %q", got)
	}
}

func TestParseColorLUTParams_IntensityRange(t *testing.T) {
	for _, v := range []float64{-0.1, 1.1, 5} {
		if _, err := parseColorLUTParams(map[string]any{"intensity": v}); err == nil {
			t.Errorf("intensity=%g should fail", v)
		}
	}
}

func TestParseColorLUTParams_ValidIntensity(t *testing.T) {
	for _, v := range []float64{0, 0.5, 1.0} {
		p, err := parseColorLUTParams(map[string]any{"intensity": v})
		if err != nil {
			t.Errorf("intensity=%g should pass: %v", v, err)
		}
		if p.Intensity == nil || *p.Intensity != v {
			t.Errorf("intensity=%g not parsed: %+v", v, p)
		}
	}
}
