package transforms

import (
	"strings"
	"testing"
)

func TestParseCompressParams(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want compressParams
		err  string // substring; "" = no error
	}{
		{
			name: "empty params",
			in:   map[string]any{},
			want: compressParams{},
		},
		{
			name: "preset target",
			in:   map[string]any{"target": "discord"},
			want: compressParams{Target: "discord"},
		},
		{
			name: "size_mb as float",
			in:   map[string]any{"size_mb": float64(15.5)},
			want: compressParams{SizeMB: 15.5},
		},
		{
			name: "size_mb as int (uncommon but accepted)",
			in:   map[string]any{"size_mb": 10},
			want: compressParams{SizeMB: 10},
		},
		{
			name: "quality from JSON (float64) coerces to int",
			in:   map[string]any{"quality": float64(85)},
			want: compressParams{Quality: 85},
		},
		{
			name: "bitrate from JSON",
			in:   map[string]any{"bitrate_kbps": float64(192)},
			want: compressParams{BitrateKbps: 192},
		},
		{
			name: "format passthrough",
			in:   map[string]any{"format": "webp"},
			want: compressParams{Format: "webp"},
		},
		{
			name: "all-fields-set passes through",
			in: map[string]any{
				"target":       "whatsapp",
				"size_mb":      float64(12),
				"quality":      float64(85),
				"bitrate_kbps": float64(192),
				"format":       "mp4",
			},
			want: compressParams{
				Target: "whatsapp", SizeMB: 12, Quality: 85,
				BitrateKbps: 192, Format: "mp4",
			},
		},
		{
			name: "wrong-type target rejected",
			in:   map[string]any{"target": 42},
			err:  "`target` must be a string",
		},
		{
			name: "wrong-type size_mb rejected",
			in:   map[string]any{"size_mb": "ten"},
			err:  "`size_mb` must be a number",
		},
		{
			name: "wrong-type quality rejected",
			in:   map[string]any{"quality": "high"},
			err:  "`quality` must be an integer",
		},
		{
			name: "unknown field ignored (forward-compat)",
			in:   map[string]any{"target": "discord", "future_knob": true},
			want: compressParams{Target: "discord"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCompressParams(tc.in)
			if tc.err != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.err)
				}
				if !strings.Contains(err.Error(), tc.err) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestVideoOutFormat_Defaults(t *testing.T) {
	cases := []struct {
		formatName string
		want       string
	}{
		{"mov,mp4,m4a,3gp,3g2,mj2", "mp4"},
		{"matroska,webm", "mkv"},
		{"webm", "webm"},
		{"avi", "mp4"},
		{"flv", "mp4"},
		{"gif", "mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.formatName, func(t *testing.T) {
			probed := &ffprobeResult{}
			probed.Format.FormatName = tc.formatName
			got := videoOutFormat(compressParams{}, probed)
			if got != tc.want {
				t.Errorf("formatName=%q got=%q, want=%q", tc.formatName, got, tc.want)
			}
		})
	}
}

func TestVideoOutFormat_Override(t *testing.T) {
	probed := &ffprobeResult{}
	probed.Format.FormatName = "mov,mp4,m4a,3gp,3g2,mj2"
	got := videoOutFormat(compressParams{Format: "webm"}, probed)
	if got != "webm" {
		t.Errorf("Format override should win: got %q, want webm", got)
	}
}

func TestImageOutFormat(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		comment string
	}{
		{"png_pipe", "png", "lowercase, strip _pipe"},
		{"jpeg_pipe", "jpg", "jpeg → jpg"},
		{"mjpeg", "jpg", "mjpeg → jpg"},
		{"webp_pipe", "webp", "webp stays webp"},
		{"avif", "avif", "avif stays avif"},
		{"tiff_pipe", "jpg", "uncommon falls back to jpg"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			probed := &ffprobeResult{}
			probed.Format.FormatName = tc.in
			got := imageOutFormat(compressParams{}, probed)
			if got != tc.want {
				t.Errorf("%s: got %q, want %q", tc.comment, got, tc.want)
			}
		})
	}
}

func TestAudioOutFormat(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"mp3", "mp3"},
		{"flac", "mp3"}, // flac → mp3 default for "compress"
		{"wav", "mp3"},
		{"opus", "opus"},
		{"ogg", "ogg"},
		{"matroska,webm,audio", "mp3"}, // unmatched → mp3
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			probed := &ffprobeResult{}
			probed.Format.FormatName = tc.in
			got := audioOutFormat(compressParams{}, probed)
			if got != tc.want {
				t.Errorf("formatName=%q got=%q, want=%q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPresetSizesMB_KnownPresets(t *testing.T) {
	cases := map[string]float64{
		"discord":  10,
		"whatsapp": 16,
		"x":        512,
		"twitter":  512,
	}
	for preset, want := range cases {
		got, ok := presetSizesMB[preset]
		if !ok {
			t.Errorf("preset %q missing from table", preset)
			continue
		}
		if got != want {
			t.Errorf("preset %q = %v, want %v", preset, got, want)
		}
	}
}
