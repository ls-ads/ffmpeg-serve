package transforms

import (
	"testing"
)

func TestSilenceRemoveFilter_Defaults(t *testing.T) {
	got := silenceRemoveFilter(silenceRemoveParams{})
	want := "silenceremove=stop_periods=-1:stop_duration=0.5:stop_threshold=-50dB"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSilenceRemoveFilter_Overrides(t *testing.T) {
	threshold := -45.0
	minSilence := 1.0
	got := silenceRemoveFilter(silenceRemoveParams{
		ThresholdDB:   &threshold,
		MinSilenceSec: &minSilence,
	})
	want := "silenceremove=stop_periods=-1:stop_duration=1:stop_threshold=-45dB"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseSilenceRemoveParams_Valid(t *testing.T) {
	p, err := parseSilenceRemoveParams(map[string]any{
		"threshold_db":    float64(-40),
		"min_silence_sec": float64(0.3),
		"format":          "mp3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ThresholdDB == nil || *p.ThresholdDB != -40 {
		t.Errorf("threshold not parsed: %+v", p)
	}
	if p.MinSilenceSec == nil || *p.MinSilenceSec != 0.3 {
		t.Errorf("min_silence_sec not parsed: %+v", p)
	}
	if p.Format != "mp3" {
		t.Errorf("format not parsed: %+v", p)
	}
}

func TestParseSilenceRemoveParams_InvalidThreshold(t *testing.T) {
	for _, v := range []float64{0, 1, -100} {
		if _, err := parseSilenceRemoveParams(map[string]any{"threshold_db": v}); err == nil {
			t.Errorf("threshold_db=%g should fail", v)
		}
	}
}

func TestParseSilenceRemoveParams_InvalidMinSilence(t *testing.T) {
	for _, v := range []float64{0, -1, 11} {
		if _, err := parseSilenceRemoveParams(map[string]any{"min_silence_sec": v}); err == nil {
			t.Errorf("min_silence_sec=%g should fail", v)
		}
	}
}
