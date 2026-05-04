package transforms

import (
	"context"
	"fmt"
	"strconv"
)

func init() {
	Register("trim", trim)
}

// trim cuts a span out of the input — `start_sec` to `end_sec` —
// for video and audio. Image inputs error out (no time axis).
//
// Params:
//
//	start_sec — start timestamp in seconds. Default 0.
//	end_sec   — end timestamp in seconds. Default = end of file.
//	mode      — "encode" (default) re-encodes the trimmed span so
//	            cuts land exactly at the requested timestamps;
//	            "copy" uses -c copy which is near-free but snaps
//	            cuts to the nearest keyframe (can be off by 0–10s
//	            depending on GOP size).
//	format    — output format override; defaults to input format.
//
// Why both modes: a podcast pre-roll trim ("strip the first 12 s")
// is fine in copy mode — keyframe drift is a small fraction of a
// minute-long episode. A film cut to a frame ("end at 3.4s") needs
// encode mode to be exact.
type trimParams struct {
	StartSec *float64 `json:"start_sec,omitempty"`
	EndSec   *float64 `json:"end_sec,omitempty"`
	Mode     string   `json:"mode,omitempty"`
	Format   string   `json:"format,omitempty"`
}

func trim(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseTrimParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("trim: %w", err)
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("trim: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("trim: %w", err)
	}

	switch kind {
	case KindImage:
		return nil, fmt.Errorf("trim: doesn't apply to images (no time axis)")
	case KindUnknown:
		return nil, fmt.Errorf("trim: could not classify input (format=%q)", probed.Format.FormatName)
	}

	// Validate against probed duration when provided. ffmpeg silently
	// clamps out-of-range timestamps, but the user almost certainly
	// wants to know they typed something past the end.
	if params.EndSec != nil && probed.Format.Duration != "" {
		if duration, err := strconv.ParseFloat(probed.Format.Duration, 64); err == nil && duration > 0 {
			if *params.EndSec > duration+0.5 { // 0.5s grace for rounding
				return nil, fmt.Errorf("trim: end_sec %.2f is past input duration %.2fs", *params.EndSec, duration)
			}
		}
	}
	if params.StartSec != nil && params.EndSec != nil && *params.EndSec <= *params.StartSec {
		return nil, fmt.Errorf("trim: end_sec (%.2f) must be greater than start_sec (%.2f)", *params.EndSec, *params.StartSec)
	}

	var outFmt string
	if kind == KindAudio {
		outFmt = params.Format
		if outFmt == "" {
			outFmt = audioOutFormat(compressParams{}, probed)
		}
	} else {
		outFmt = params.Format
		if outFmt == "" {
			outFmt = videoOutFormat(compressParams{}, probed)
		}
	}
	job.outPath = job.outPath + "." + outFmt

	args := buildTrimArgs(job.inPath, job.outPath, kind, outFmt, params)

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

// buildTrimArgs assembles the ffmpeg command for a trim. -ss /
// -to are placed *after* -i in encode mode so they apply to the
// decoded stream (frame-accurate). In copy mode they go before -i
// so ffmpeg seeks via the demuxer at keyframe boundaries (fast,
// approximate).
func buildTrimArgs(inPath, outPath string, kind MediaKind, outFmt string, p trimParams) []string {
	args := []string{"-y"}
	mode := p.Mode
	if mode == "" {
		mode = "encode"
	}

	if mode == "copy" {
		// Demuxer-level seek — fast, snaps to keyframes.
		if p.StartSec != nil {
			args = append(args, "-ss", formatFloat(*p.StartSec))
		}
		if p.EndSec != nil {
			args = append(args, "-to", formatFloat(*p.EndSec))
		}
		args = append(args, "-i", inPath, "-c", "copy")
	} else {
		// Decoder-level seek — frame-accurate, requires re-encode.
		args = append(args, "-i", inPath)
		if p.StartSec != nil {
			args = append(args, "-ss", formatFloat(*p.StartSec))
		}
		if p.EndSec != nil {
			args = append(args, "-to", formatFloat(*p.EndSec))
		}
		// Re-encode with sensible per-kind defaults — the trimmed
		// span lands in the same container with platform-friendly
		// codecs. mp4-family containers also get faststart.
		if kind == KindAudio {
			args = append(args, "-c:a", audioCodecForFormat(outFmt), "-vn")
		} else {
			args = append(args,
				"-c:v", "libx264", "-crf", "20", "-preset", "veryfast",
				"-c:a", "aac", "-b:a", "192k",
				"-movflags", "+faststart",
			)
		}
	}

	args = append(args, outPath)
	return args
}

func parseTrimParams(raw map[string]any) (trimParams, error) {
	var p trimParams
	if v, ok := raw["start_sec"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`start_sec`: %w", err)
		}
		if f < 0 {
			return p, fmt.Errorf("`start_sec` must be ≥ 0, got %g", f)
		}
		p.StartSec = &f
	}
	if v, ok := raw["end_sec"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`end_sec`: %w", err)
		}
		if f <= 0 {
			return p, fmt.Errorf("`end_sec` must be > 0, got %g", f)
		}
		p.EndSec = &f
	}
	if v, ok := raw["mode"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`mode` must be a string")
		}
		if s != "" && s != "copy" && s != "encode" {
			return p, fmt.Errorf("`mode` must be \"copy\" or \"encode\", got %q", s)
		}
		p.Mode = s
	}
	if v, ok := raw["format"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`format` must be a string")
		}
		p.Format = s
	}
	if p.StartSec == nil && p.EndSec == nil {
		return p, fmt.Errorf("trim: at least one of `start_sec` or `end_sec` is required")
	}
	return p, nil
}
