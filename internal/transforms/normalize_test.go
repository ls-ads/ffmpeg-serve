package transforms

import (
	"strings"
	"testing"
)

func TestLoudnormFilter_Defaults(t *testing.T) {
	got := loudnormFilter(normalizeParams{})
	want := "loudnorm=I=-16:LRA=11:TP=-1.5"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLoudnormFilter_Overrides(t *testing.T) {
	target := -23.0
	lra := 7.0
	tp := -2.0
	got := loudnormFilter(normalizeParams{
		TargetLUFS: &target,
		LRA:        &lra,
		TruePeak:   &tp,
	})
	want := "loudnorm=I=-23:LRA=7:TP=-2"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseNormalizeParams_Defaults(t *testing.T) {
	got, err := parseNormalizeParams(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	// Defaults are nil pointers — applied at filter-render time.
	if got.TargetLUFS != nil || got.LRA != nil || got.TruePeak != nil {
		t.Errorf("expected all-nil defaults, got %+v", got)
	}
}

func TestParseNormalizeParams_Valid(t *testing.T) {
	got, err := parseNormalizeParams(map[string]any{
		"target_lufs": float64(-23),
		"lra":         float64(7),
		"true_peak":   float64(-2),
		"format":      "wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.TargetLUFS == nil || *got.TargetLUFS != -23 {
		t.Errorf("target_lufs not parsed: %+v", got)
	}
	if got.LRA == nil || *got.LRA != 7 {
		t.Errorf("lra not parsed: %+v", got)
	}
	if got.TruePeak == nil || *got.TruePeak != -2 {
		t.Errorf("true_peak not parsed: %+v", got)
	}
	if got.Format != "wav" {
		t.Errorf("format = %q, want wav", got.Format)
	}
}

func TestParseNormalizeParams_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"target_lufs positive", map[string]any{"target_lufs": float64(1)}, "[-70, 0)"},
		{"target_lufs too negative", map[string]any{"target_lufs": float64(-100)}, "[-70, 0)"},
		{"lra below 1", map[string]any{"lra": float64(0.5)}, "[1, 50]"},
		{"lra above 50", map[string]any{"lra": float64(51)}, "[1, 50]"},
		{"true_peak positive", map[string]any{"true_peak": float64(1)}, "[-9, 0]"},
		{"true_peak too negative", map[string]any{"true_peak": float64(-10)}, "[-9, 0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseNormalizeParams(tc.in)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestParseNormalizeParams_RejectsWrongTypes(t *testing.T) {
	cases := []map[string]any{
		{"target_lufs": "minus sixteen"},
		{"lra": []int{1}},
		{"true_peak": true},
		{"format": 42},
	}
	for i, c := range cases {
		if _, err := parseNormalizeParams(c); err == nil {
			t.Errorf("case %d: expected error on %+v", i, c)
		}
	}
}

func TestAudioCodecForFormat(t *testing.T) {
	cases := map[string]string{
		"mp3":  "libmp3lame",
		"aac":  "aac",
		"m4a":  "aac",
		"opus": "libopus",
		"ogg":  "libvorbis",
		"flac": "flac",
		"wav":  "pcm_s16le",
		"xyz":  "libmp3lame", // unknown → fall back
	}
	for in, want := range cases {
		if got := audioCodecForFormat(in); got != want {
			t.Errorf("audioCodecForFormat(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatFloat_TrimsTrailingZeros(t *testing.T) {
	cases := map[float64]string{
		-16:    "-16",
		-23.0:  "-23",
		-1.5:   "-1.5",
		11:     "11",
		11.5:   "11.5",
	}
	for in, want := range cases {
		if got := formatFloat(in); got != want {
			t.Errorf("formatFloat(%v) = %q, want %q", in, got, want)
		}
	}
}
