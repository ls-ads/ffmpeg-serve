package transforms

import (
	"testing"
)

func TestParseExtractAudioParams_Defaults(t *testing.T) {
	p, err := parseExtractAudioParams(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != "" {
		t.Errorf("default format should be empty (filled by handler), got %q", p.Format)
	}
}

func TestParseExtractAudioParams_ValidFormats(t *testing.T) {
	for _, fmt := range []string{"mp3", "wav", "flac", "m4a", "ogg", "opus"} {
		_, err := parseExtractAudioParams(map[string]any{"format": fmt})
		if err != nil {
			t.Errorf("format=%q should be valid: %v", fmt, err)
		}
	}
}

func TestParseExtractAudioParams_InvalidFormat(t *testing.T) {
	_, err := parseExtractAudioParams(map[string]any{"format": "aiff"})
	if err == nil {
		t.Errorf("format=aiff should fail")
	}
}

func TestExtractAudioBitrate(t *testing.T) {
	cases := map[string]string{
		"mp3":  "192k",
		"m4a":  "192k",
		"ogg":  "192k",
		"opus": "96k",
		"flac": "",
		"wav":  "",
	}
	for in, want := range cases {
		if got := extractAudioBitrate(in); got != want {
			t.Errorf("extractAudioBitrate(%q) = %q, want %q", in, got, want)
		}
	}
}
