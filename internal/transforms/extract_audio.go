package transforms

import (
	"context"
	"fmt"
)

func init() {
	Register("extract-audio", extractAudio)
}

// extractAudio pulls the audio track out of a video as a standalone
// audio file. Sugar for the common "give me just the audio" workflow
// that callers used to do via convert with a deliberate cross-kind
// target.
//
// Params:
//
//	format — target audio container. Default "mp3". Known: mp3, wav,
//	         flac, m4a, ogg, opus.
//
// Image inputs error out. Audio inputs error out (use convert
// instead — extracting from audio is a no-op rename, the wrong
// shape for this transform).
type extractAudioParams struct {
	Format string `json:"format,omitempty"`
}

var extractAudioDefaults = extractAudioParams{Format: "mp3"}

func extractAudio(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseExtractAudioParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("extract-audio: %w", err)
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("extract-audio: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("extract-audio: %w", err)
	}

	switch kind {
	case KindImage:
		return nil, fmt.Errorf("extract-audio: input is an image — no audio track to extract")
	case KindAudio:
		return nil, fmt.Errorf("extract-audio: input is already audio — use `convert` to change format")
	case KindUnknown:
		return nil, fmt.Errorf("extract-audio: could not classify input (format=%q)", probed.Format.FormatName)
	}
	// kind == KindVideo

	outFmt := params.Format
	if outFmt == "" {
		outFmt = extractAudioDefaults.Format
	}
	job.outPath = job.outPath + "." + outFmt

	args := []string{
		"-y", "-i", job.inPath,
		"-vn", // strip video stream
		"-c:a", audioCodecForFormat(outFmt),
	}
	// flac / wav are lossless — bitrate would be ignored. Lossy
	// codecs get a sensible default; opus runs at 96k since libopus
	// is meaningfully more efficient than mp3/aac at the same rate.
	if br := extractAudioBitrate(outFmt); br != "" {
		args = append(args, "-b:a", br)
	}
	args = append(args, job.outPath)

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

func extractAudioBitrate(format string) string {
	switch format {
	case "flac", "wav":
		return ""
	case "opus":
		return "96k"
	}
	return "192k"
}

func parseExtractAudioParams(raw map[string]any) (extractAudioParams, error) {
	var p extractAudioParams
	if v, ok := raw["format"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`format` must be a string")
		}
		valid := map[string]bool{
			"mp3": true, "wav": true, "flac": true,
			"m4a": true, "ogg": true, "opus": true,
		}
		if s != "" && !valid[s] {
			return p, fmt.Errorf("`format` must be one of mp3, wav, flac, m4a, ogg, opus — got %q", s)
		}
		p.Format = s
	}
	return p, nil
}
