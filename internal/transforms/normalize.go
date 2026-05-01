package transforms

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func init() {
	Register("normalize", normalize)
}

// normalize applies EBU R128 loudness normalization to the audio
// stream of the input. Image inputs error out — the concept doesn't
// apply.
//
// Params:
//   target_lufs — integrated loudness target, in LUFS. Defaults to
//                 -16 (podcast / Spotify-ish). Common alternatives:
//                   -14   broadcast TV
//                   -16   Spotify, podcasts
//                   -23   EU EBU R128 standard
//   lra         — loudness range, default 11 (dB).
//   true_peak   — true-peak ceiling, default -1.5 dBTP.
//   format      — output format override; defaults to input format.
//
// For video inputs: only the audio stream is touched (the video
// stream is copied with -c:v copy, near-zero cost).
type normalizeParams struct {
	TargetLUFS *float64 `json:"target_lufs,omitempty"`
	LRA        *float64 `json:"lra,omitempty"`
	TruePeak   *float64 `json:"true_peak,omitempty"`
	Format     string   `json:"format,omitempty"`
}

func normalize(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseNormalizeParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("normalize: %w", err)
	}

	switch kind {
	case KindImage:
		return nil, fmt.Errorf("normalize: doesn't apply to images")
	case KindUnknown:
		return nil, fmt.Errorf("normalize: could not classify input (format=%q)", probed.Format.FormatName)
	}

	loudnorm := loudnormFilter(params)

	var args []string
	var outFmt string

	if kind == KindAudio {
		outFmt = params.Format
		if outFmt == "" {
			outFmt = audioOutFormat(compressParams{}, probed)
		}
		job.outPath = job.outPath + "." + outFmt
		args = []string{
			"-y", "-i", job.inPath,
			"-af", loudnorm,
			"-c:a", audioCodecForFormat(outFmt),
			"-vn",
			job.outPath,
		}
	} else {
		// kind == KindVideo
		outFmt = params.Format
		if outFmt == "" {
			outFmt = videoOutFormat(compressParams{}, probed)
		}
		job.outPath = job.outPath + "." + outFmt
		args = []string{
			"-y", "-i", job.inPath,
			"-c:v", "copy", // video stream untouched
			"-af", loudnorm,
			"-c:a", "aac", "-b:a", "192k",
			"-movflags", "+faststart",
			job.outPath,
		}
	}

	execMS, err := timeIt(func() error { return runFFmpeg(ctx, ffmpegBin, args) })
	if err != nil {
		return nil, err
	}
	b64, err := readOutput(job.outPath)
	if err != nil {
		return nil, err
	}
	return []Output{{MediaB64: b64, Format: outFmt, ExecMS: execMS}}, nil
}

// loudnormFilter renders the -af string. Single-pass linear mode is
// "good enough" for most callers; a future `--accurate` flag can do
// the two-pass measure-then-apply dance for podcasts that need
// strict targets.
func loudnormFilter(p normalizeParams) string {
	target := -16.0
	if p.TargetLUFS != nil {
		target = *p.TargetLUFS
	}
	lra := 11.0
	if p.LRA != nil {
		lra = *p.LRA
	}
	tp := -1.5
	if p.TruePeak != nil {
		tp = *p.TruePeak
	}
	return fmt.Sprintf("loudnorm=I=%s:LRA=%s:TP=%s",
		formatFloat(target), formatFloat(lra), formatFloat(tp))
}

// formatFloat trims trailing zeros from %f output ("16.000000" →
// "16", "-1.5" stays "-1.5") so the filter string looks like a
// human typed it. ffmpeg accepts either; readability matters in
// the rare case the operator has to grep this out of stderr.
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

// audioCodecForFormat picks the right encoder for an output audio
// container. Mirrors the table in compressAudio + buildConvertAudio
// — kept inline here to avoid cross-transform coupling on a tiny
// switch.
func audioCodecForFormat(fmt string) string {
	switch strings.ToLower(fmt) {
	case "mp3":
		return "libmp3lame"
	case "aac", "m4a":
		return "aac"
	case "opus":
		return "libopus"
	case "ogg":
		return "libvorbis"
	case "flac":
		return "flac"
	case "wav":
		return "pcm_s16le"
	}
	return "libmp3lame"
}

func parseNormalizeParams(raw map[string]any) (normalizeParams, error) {
	var p normalizeParams
	if v, ok := raw["target_lufs"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`target_lufs`: %w", err)
		}
		if f >= 0 || f < -70 {
			return p, fmt.Errorf("`target_lufs` must be in [-70, 0), got %g", f)
		}
		p.TargetLUFS = &f
	}
	if v, ok := raw["lra"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`lra`: %w", err)
		}
		if f < 1 || f > 50 {
			return p, fmt.Errorf("`lra` must be in [1, 50], got %g", f)
		}
		p.LRA = &f
	}
	if v, ok := raw["true_peak"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`true_peak`: %w", err)
		}
		if f > 0 || f < -9 {
			return p, fmt.Errorf("`true_peak` must be in [-9, 0], got %g", f)
		}
		p.TruePeak = &f
	}
	if v, ok := raw["format"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`format` must be a string")
		}
		p.Format = s
	}
	return p, nil
}

// toFloat accepts JSON's float64 (the only number type after
// json.Unmarshal into any) plus a few defensive coercions.
func toFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	}
	return 0, fmt.Errorf("must be a number")
}
