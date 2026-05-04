package transforms

import (
	"strings"
	"testing"
)

func TestBuildSubtitleFilter_NoStyle(t *testing.T) {
	got := buildSubtitleFilter("/tmp/sub.srt", subtitleBurnParams{})
	want := "subtitles='/tmp/sub.srt'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSubtitleFilter_EscapesColon(t *testing.T) {
	// Windows-style paths shouldn't break libass parsing.
	got := buildSubtitleFilter("C:/tmp/sub.srt", subtitleBurnParams{})
	if !strings.Contains(got, `C\:/tmp/sub.srt`) {
		t.Errorf("colon not escaped: %q", got)
	}
}

func TestBuildSubtitleFilter_StyleOverrides(t *testing.T) {
	size := 36
	outline := 3
	got := buildSubtitleFilter("/tmp/sub.srt", subtitleBurnParams{
		FontSize:  &size,
		FontColor: "ff0000", // red
		Outline:   &outline,
	})
	if !strings.Contains(got, "FontSize=36") {
		t.Errorf("missing FontSize=36: %q", got)
	}
	if !strings.Contains(got, "OutlineWidth=3") {
		t.Errorf("missing OutlineWidth=3: %q", got)
	}
	// libass colour byte order is BBGGRR not RRGGBB.
	if !strings.Contains(got, "PrimaryColour=&H000000ff&") {
		t.Errorf("colour not converted to BGR: %q", got)
	}
}

func TestBgrFromRgb(t *testing.T) {
	cases := map[string]string{
		"ff0000": "0000ff",
		"00ff00": "00ff00",
		"0000ff": "ff0000",
		"ffffff": "ffffff",
		"123456": "563412",
	}
	for in, want := range cases {
		if got := bgrFromRgb(in); got != want {
			t.Errorf("bgrFromRgb(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSubtitleBurnParams_BadFontColor(t *testing.T) {
	for _, c := range []string{"red", "#fff", "12345", "12345g", "1234567"} {
		_, err := parseSubtitleBurnParams(map[string]any{"font_color": c})
		if err == nil {
			t.Errorf("font_color=%q should fail", c)
		}
	}
}

func TestParseSubtitleBurnParams_StripsHash(t *testing.T) {
	p, err := parseSubtitleBurnParams(map[string]any{"font_color": "#FFFFFF"})
	if err != nil {
		t.Fatal(err)
	}
	if p.FontColor != "ffffff" {
		t.Errorf("expected lowercase + stripped #, got %q", p.FontColor)
	}
}

func TestParseSubtitleBurnParams_FontSizeRange(t *testing.T) {
	for _, v := range []float64{0, 7, 201, 1000} {
		if _, err := parseSubtitleBurnParams(map[string]any{"font_size": v}); err == nil {
			t.Errorf("font_size=%g should fail", v)
		}
	}
}
