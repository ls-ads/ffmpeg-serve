package transforms

import (
	"strings"
	"testing"
)

func TestParseAspect(t *testing.T) {
	cases := []struct {
		in     string
		w, h   int
		errSub string
	}{
		{"9:16", 9, 16, ""},
		{"16:9", 16, 9, ""},
		{"1:1", 1, 1, ""},
		{"4:5", 4, 5, ""},
		{" 9 : 16 ", 9, 16, ""},  // whitespace tolerant
		{"9", 0, 0, "must be \"W:H\""},
		{"0:16", 0, 0, "positive integer"},
		{"9:0", 0, 0, "positive integer"},
		{"a:b", 0, 0, "positive integer"},
		{"-1:1", 0, 0, "positive integer"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			w, h, err := parseAspect(tc.in)
			if tc.errSub != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tc.errSub)
				}
				if !strings.Contains(err.Error(), tc.errSub) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if w != tc.w || h != tc.h {
				t.Errorf("got (%d,%d), want (%d,%d)", w, h, tc.w, tc.h)
			}
		})
	}
}

func TestOutputDims(t *testing.T) {
	cases := []struct {
		name                       string
		inW, inH, aspW, aspH       int
		wantW, wantH               int
	}{
		{"1920×1080 → 9:16 portrait", 1920, 1080, 9, 16, 1080, 1920},
		{"1080×1920 → 1:1 square", 1080, 1920, 1, 1, 1920, 1920},
		{"3840×2160 4K → 9:16", 3840, 2160, 9, 16, 2160, 3840},
		{"720×480 → 16:9 already, no upsize", 720, 480, 16, 9, 720, 404},
		{"720×480 → 9:16", 720, 480, 9, 16, 404, 720},
		{"odd input dims → even output", 1921, 1081, 9, 16, 1080, 1920},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotW, gotH := outputDims(tc.inW, tc.inH, tc.aspW, tc.aspH)
			if gotW != tc.wantW || gotH != tc.wantH {
				t.Errorf("got (%d,%d), want (%d,%d)", gotW, gotH, tc.wantW, tc.wantH)
			}
			if gotW%2 != 0 || gotH%2 != 0 {
				t.Errorf("output dims must be even, got (%d,%d)", gotW, gotH)
			}
		})
	}
}

func TestReframeFilter_BlurPad(t *testing.T) {
	got, err := reframeFilter("blur-pad", 1080, 1920)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"split=2",
		"force_original_aspect_ratio=increase",
		"crop=1080:1920",
		"boxblur",
		"force_original_aspect_ratio=decrease",
		"overlay=(W-w)/2:(H-h)/2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("blur-pad chain missing %q. got: %s", want, got)
		}
	}
}

func TestReframeFilter_Letterbox(t *testing.T) {
	got, err := reframeFilter("letterbox", 1080, 1920)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"force_original_aspect_ratio=decrease",
		"pad=1080:1920",
		"color=black",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("letterbox chain missing %q. got: %s", want, got)
		}
	}
}

func TestReframeFilter_Crop(t *testing.T) {
	got, err := reframeFilter("crop", 1080, 1920)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "force_original_aspect_ratio=increase") {
		t.Errorf("crop chain should use ratio=increase. got: %s", got)
	}
	if !strings.Contains(got, "crop=1080:1920") {
		t.Errorf("crop chain missing crop dims. got: %s", got)
	}
}

func TestReframeFilter_Stretch(t *testing.T) {
	got, err := reframeFilter("stretch", 1080, 1920)
	if err != nil {
		t.Fatal(err)
	}
	// Stretch is plain anisotropic scale.
	if !strings.HasPrefix(got, "scale=1080:1920") {
		t.Errorf("stretch chain should be plain scale. got: %s", got)
	}
}

func TestReframeFilter_UnknownFit(t *testing.T) {
	_, err := reframeFilter("imagined", 1080, 1920)
	if err == nil {
		t.Fatal("expected error for unknown fit mode")
	}
	if !strings.Contains(err.Error(), "imagined") {
		t.Errorf("error should name the unknown fit: %v", err)
	}
}

func TestParseReframeParams(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want reframeParams
		err  string
	}{
		{"to + fit", map[string]any{"to": "9:16", "fit": "blur-pad"},
			reframeParams{To: "9:16", Fit: "blur-pad"}, ""},
		{"to only (fit defaults later)", map[string]any{"to": "1:1"},
			reframeParams{To: "1:1"}, ""},
		{"empty", map[string]any{}, reframeParams{}, ""},
		{"non-string to", map[string]any{"to": 9}, reframeParams{}, "`to` must be a string"},
		{"non-string fit", map[string]any{"fit": false}, reframeParams{}, "`fit` must be a string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseReframeParams(tc.in)
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

func TestMakeEven(t *testing.T) {
	cases := map[int]int{0: 0, 1: 0, 2: 2, 3: 2, 1080: 1080, 1081: 1080, 1920: 1920}
	for in, want := range cases {
		if got := makeEven(in); got != want {
			t.Errorf("makeEven(%d) = %d, want %d", in, got, want)
		}
	}
}
