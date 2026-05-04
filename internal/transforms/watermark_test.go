package transforms

import (
	"strings"
	"testing"
)

func TestOverlayPosition_Defaults(t *testing.T) {
	x, y := overlayPosition("", 16)
	if !strings.Contains(x, "main_w") || !strings.Contains(y, "main_h") {
		t.Errorf("default should be bottom-right (uses main_w/main_h), got %s,%s", x, y)
	}
}

func TestOverlayPosition_AllCorners(t *testing.T) {
	cases := []struct {
		pos        string
		xContains  string
		yContains  string
	}{
		{"top-left", "16", "16"},
		{"top-right", "main_w-overlay_w", "16"},
		{"bottom-left", "16", "main_h-overlay_h"},
		{"bottom-right", "main_w-overlay_w", "main_h-overlay_h"},
		{"center", "(main_w-overlay_w)/2", "(main_h-overlay_h)/2"},
	}
	for _, c := range cases {
		x, y := overlayPosition(c.pos, 16)
		if !strings.Contains(x, c.xContains) {
			t.Errorf("%s: x=%q missing %q", c.pos, x, c.xContains)
		}
		if !strings.Contains(y, c.yContains) {
			t.Errorf("%s: y=%q missing %q", c.pos, y, c.yContains)
		}
	}
}

func TestBuildWatermarkFilter_Defaults(t *testing.T) {
	got := buildWatermarkFilter(watermarkParams{})
	if !strings.Contains(got, "scale=main_w*0.2:-1") {
		t.Errorf("default scale missing: %q", got)
	}
	if !strings.Contains(got, "aa=0.85") {
		t.Errorf("default opacity missing: %q", got)
	}
}

func TestBuildWatermarkFilter_AllOverrides(t *testing.T) {
	margin := 32
	opacity := 0.5
	scale := 0.4
	got := buildWatermarkFilter(watermarkParams{
		Position: "top-left",
		Margin:   &margin,
		Opacity:  &opacity,
		Scale:    &scale,
	})
	if !strings.Contains(got, "scale=main_w*0.4:-1") {
		t.Errorf("scale not applied: %q", got)
	}
	if !strings.Contains(got, "aa=0.5") {
		t.Errorf("opacity not applied: %q", got)
	}
	if !strings.Contains(got, "overlay=32:32") {
		t.Errorf("top-left margin not applied: %q", got)
	}
}

func TestParseWatermarkParams_RejectsBadPosition(t *testing.T) {
	if _, err := parseWatermarkParams(map[string]any{"position": "middle-left"}); err == nil {
		t.Errorf("bogus position should fail")
	}
}

func TestParseWatermarkParams_OpacityRange(t *testing.T) {
	for _, v := range []float64{-0.1, 1.1, 5} {
		if _, err := parseWatermarkParams(map[string]any{"opacity": v}); err == nil {
			t.Errorf("opacity=%g should fail", v)
		}
	}
}

func TestParseWatermarkParams_ScaleRange(t *testing.T) {
	for _, v := range []float64{0, 0.005, 1.1} {
		if _, err := parseWatermarkParams(map[string]any{"scale": v}); err == nil {
			t.Errorf("scale=%g should fail", v)
		}
	}
}
