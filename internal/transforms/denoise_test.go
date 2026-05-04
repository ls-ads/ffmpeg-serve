package transforms

import (
	"testing"
)

func TestAfftdnFilter_Defaults(t *testing.T) {
	got := afftdnFilter(denoiseParams{})
	want := "afftdn=nf=-25:nr=12"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAfftdnFilter_Overrides(t *testing.T) {
	nf := -35.0
	nr := 20.0
	got := afftdnFilter(denoiseParams{NoiseFloorDB: &nf, NoiseReduction: &nr})
	want := "afftdn=nf=-35:nr=20"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseDenoiseParams_Valid(t *testing.T) {
	p, err := parseDenoiseParams(map[string]any{
		"noise_floor_db":  float64(-30),
		"noise_reduction": float64(15),
		"format":          "wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.NoiseFloorDB == nil || *p.NoiseFloorDB != -30 {
		t.Errorf("noise_floor_db not parsed: %+v", p)
	}
	if p.NoiseReduction == nil || *p.NoiseReduction != 15 {
		t.Errorf("noise_reduction not parsed: %+v", p)
	}
	if p.Format != "wav" {
		t.Errorf("format not parsed: %+v", p)
	}
}

func TestParseDenoiseParams_InvalidNoiseFloor(t *testing.T) {
	for _, v := range []float64{0, 1, -100} {
		if _, err := parseDenoiseParams(map[string]any{"noise_floor_db": v}); err == nil {
			t.Errorf("noise_floor_db=%g should fail", v)
		}
	}
}

func TestParseDenoiseParams_InvalidNoiseReduction(t *testing.T) {
	for _, v := range []float64{0, -1, 100, 0.001} {
		if _, err := parseDenoiseParams(map[string]any{"noise_reduction": v}); err == nil {
			t.Errorf("noise_reduction=%g should fail", v)
		}
	}
}
