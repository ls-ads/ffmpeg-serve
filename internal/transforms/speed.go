package transforms

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	Register("speed", speed)
}

// speed changes the playback rate of video / audio. Image inputs
// error out (no time axis).
//
// Params:
//
//	factor — playback multiplier. 2.0 = 2× faster, 0.5 = half-speed.
//	         Range [0.25, 4.0]. Required.
//	format — output format override; defaults to input format.
//
// Mechanics:
//
//   - Audio uses the `atempo` filter, which keeps pitch constant.
//     atempo only accepts factors in [0.5, 2.0]; for factors outside
//     that range we chain multiple atempo stages (atempo=2,atempo=2
//     for 4×; atempo=0.5,atempo=0.5 for 0.25×). The chain still
//     ends up at the requested factor.
//   - Video uses the `setpts` filter, which retimes presentation
//     timestamps. setpts=PTS/F means "play frame F× faster".
//   - Video files with audio get both filters applied to the
//     respective streams via -filter:v / -filter:a.
type speedParams struct {
	Factor float64 `json:"factor"`
	Format string  `json:"format,omitempty"`
}

func speed(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseSpeedParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("speed: %w", err)
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("speed: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("speed: %w", err)
	}

	switch kind {
	case KindImage:
		return nil, fmt.Errorf("speed: doesn't apply to images (no time axis)")
	case KindUnknown:
		return nil, fmt.Errorf("speed: could not classify input (format=%q)", probed.Format.FormatName)
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

	args := buildSpeedArgs(job.inPath, job.outPath, kind, outFmt, params.Factor)

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

func buildSpeedArgs(inPath, outPath string, kind MediaKind, outFmt string, factor float64) []string {
	atempo := atempoChain(factor)

	if kind == KindAudio {
		return []string{
			"-y", "-i", inPath,
			"-filter:a", atempo,
			"-c:a", audioCodecForFormat(outFmt),
			"-vn",
			outPath,
		}
	}
	// kind == KindVideo
	setpts := fmt.Sprintf("setpts=PTS/%s", formatFloat(factor))
	return []string{
		"-y", "-i", inPath,
		"-filter:v", setpts,
		"-filter:a", atempo,
		"-c:v", "libx264", "-crf", "20", "-preset", "veryfast",
		"-c:a", "aac", "-b:a", "192k",
		"-movflags", "+faststart",
		outPath,
	}
}

// atempoChain renders the audio-tempo filter expression for any
// factor in [0.25, 4.0]. atempo's per-stage range is [0.5, 2.0];
// outside that we cascade. Resulting product equals the requested
// factor exactly so playback duration matches setpts.
func atempoChain(factor float64) string {
	if factor >= 0.5 && factor <= 2.0 {
		return fmt.Sprintf("atempo=%s", formatFloat(factor))
	}

	stages := []string{}
	remaining := factor
	for remaining > 2.0 {
		stages = append(stages, "atempo=2.0")
		remaining /= 2.0
	}
	for remaining < 0.5 {
		stages = append(stages, "atempo=0.5")
		remaining *= 2.0
	}
	stages = append(stages, fmt.Sprintf("atempo=%s", formatFloat(remaining)))
	return strings.Join(stages, ",")
}

func parseSpeedParams(raw map[string]any) (speedParams, error) {
	var p speedParams
	v, ok := raw["factor"]
	if !ok {
		return p, fmt.Errorf("`factor` is required")
	}
	f, err := toFloat(v)
	if err != nil {
		return p, fmt.Errorf("`factor`: %w", err)
	}
	if f < 0.25 || f > 4.0 {
		return p, fmt.Errorf("`factor` must be in [0.25, 4.0], got %g", f)
	}
	p.Factor = f
	if v, ok := raw["format"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`format` must be a string")
		}
		p.Format = s
	}
	return p, nil
}
