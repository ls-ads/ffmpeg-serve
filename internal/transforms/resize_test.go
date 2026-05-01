package transforms

import (
	"strings"
	"testing"
)

func TestParseResizeParams(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want resizeParams
		err  string
	}{
		{"empty", map[string]any{}, resizeParams{}, ""},
		{"scale + method", map[string]any{"scale": float64(2), "method": "bicubic"},
			resizeParams{Scale: 2, Method: "bicubic"}, ""},
		{"scale int coerces", map[string]any{"scale": 4},
			resizeParams{Scale: 4}, ""},
		{"non-numeric scale", map[string]any{"scale": "two"},
			resizeParams{}, "must be a number"},
		{"non-string method", map[string]any{"method": 4},
			resizeParams{}, "must be a string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseResizeParams(tc.in)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Errorf("expected error containing %q, got %v", tc.err, err)
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

func TestResize_AllowedMethods(t *testing.T) {
	for _, m := range []string{"lanczos", "bicubic", "bilinear", "neighbor"} {
		if !allowedMethods[m] {
			t.Errorf("expected %q in allowedMethods", m)
		}
	}
	if allowedMethods["spline"] {
		t.Error("spline shouldn't be allowed yet (not in iosuite's `--method` surface)")
	}
}
