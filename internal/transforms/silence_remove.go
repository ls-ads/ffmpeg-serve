package transforms

import (
	"context"
	"fmt"
)

func init() {
	Register("silence-remove", silenceRemove)
}

// silenceRemove strips silent gaps from the audio track via ffmpeg's
// `silenceremove` filter. Surprise-popular for podcast prep where
// hosts want every breath-pause cleaned up before the editor sees it.
//
// Params:
//
//	threshold_db    — silence cutoff. Anything quieter is treated as
//	                  silence. Default -50 dBFS — a typical "room
//	                  background" floor. Quieter studios should
//	                  push toward -60; noisy environments toward -40.
//	min_silence_sec — minimum gap duration to remove. Default 0.5 s
//	                  — eliminates breath pauses without chopping
//	                  intentional beats.
//	format          — output format override; defaults to input
//	                  format.
//
// For video inputs the audio track is processed and the video is
// kept stream-copied (no re-encode). The video stream's PTS will be
// unchanged, so video + audio drift if the silence removal is
// substantial — for talking-head video this is fine in short clips
// but a long lecture's gap-trimming would benefit from a re-time
// pass that the current filter doesn't do.
type silenceRemoveParams struct {
	ThresholdDB   *float64 `json:"threshold_db,omitempty"`
	MinSilenceSec *float64 `json:"min_silence_sec,omitempty"`
	Format        string   `json:"format,omitempty"`
}

func silenceRemove(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseSilenceRemoveParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("silence-remove: %w", err)
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("silence-remove: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("silence-remove: %w", err)
	}

	switch kind {
	case KindImage:
		return nil, fmt.Errorf("silence-remove: doesn't apply to images (no audio track)")
	case KindUnknown:
		return nil, fmt.Errorf("silence-remove: could not classify input (format=%q)", probed.Format.FormatName)
	}

	filter := silenceRemoveFilter(params)

	var outFmt string
	var args []string

	if kind == KindAudio {
		outFmt = params.Format
		if outFmt == "" {
			outFmt = audioOutFormat(compressParams{}, probed)
		}
		job.outPath = job.outPath + "." + outFmt
		args = []string{
			"-y", "-i", job.inPath,
			"-af", filter,
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
			"-c:v", "copy",
			"-af", filter,
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

// silenceRemoveFilter renders the silenceremove filter expression.
// stop_periods=-1 means "remove every silence", not just leading/
// trailing. The default leading-only behaviour is rarely what users
// want.
func silenceRemoveFilter(p silenceRemoveParams) string {
	threshold := -50.0
	if p.ThresholdDB != nil {
		threshold = *p.ThresholdDB
	}
	minSilence := 0.5
	if p.MinSilenceSec != nil {
		minSilence = *p.MinSilenceSec
	}
	return fmt.Sprintf(
		"silenceremove=stop_periods=-1:stop_duration=%s:stop_threshold=%sdB",
		formatFloat(minSilence), formatFloat(threshold),
	)
}

func parseSilenceRemoveParams(raw map[string]any) (silenceRemoveParams, error) {
	var p silenceRemoveParams
	if v, ok := raw["threshold_db"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`threshold_db`: %w", err)
		}
		if f >= 0 || f < -90 {
			return p, fmt.Errorf("`threshold_db` must be in [-90, 0), got %g", f)
		}
		p.ThresholdDB = &f
	}
	if v, ok := raw["min_silence_sec"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`min_silence_sec`: %w", err)
		}
		if f <= 0 || f > 10 {
			return p, fmt.Errorf("`min_silence_sec` must be in (0, 10], got %g", f)
		}
		p.MinSilenceSec = &f
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
