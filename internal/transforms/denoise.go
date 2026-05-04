package transforms

import (
	"context"
	"fmt"
)

func init() {
	Register("denoise", denoise)
}

// denoise applies FFT-based noise reduction to the audio track via
// the `afftdn` filter. Video inputs get the same treatment with
// the video stream copied through. Image inputs error out — the
// ML-based image denoiser (Restormer / FastDVDnet) is its own
// `*-serve` module; routing it through ffmpeg-serve would lie
// about how iosuite is structured.
//
// Params:
//
//	noise_floor_db  — `nf` parameter, dBFS. Default -25; quieter
//	                  recordings should push toward -35, noisier
//	                  ones (smartphone in a café) toward -20.
//	noise_reduction — `nr` parameter, 0.01–97 dB of suppression.
//	                  Default 12. Higher = more aggressive cut at
//	                  the cost of artefacts on speech sibilants.
//	format          — output format override; defaults to input
//	                  format.
type denoiseParams struct {
	NoiseFloorDB   *float64 `json:"noise_floor_db,omitempty"`
	NoiseReduction *float64 `json:"noise_reduction,omitempty"`
	Format         string   `json:"format,omitempty"`
}

func denoise(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseDenoiseParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("denoise: %w", err)
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("denoise: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("denoise: %w", err)
	}

	switch kind {
	case KindImage:
		return nil, fmt.Errorf("denoise: doesn't apply to images yet (ML image denoise lives in its own module — coming)")
	case KindUnknown:
		return nil, fmt.Errorf("denoise: could not classify input (format=%q)", probed.Format.FormatName)
	}

	filter := afftdnFilter(params)

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

func afftdnFilter(p denoiseParams) string {
	noiseFloor := -25.0
	if p.NoiseFloorDB != nil {
		noiseFloor = *p.NoiseFloorDB
	}
	noiseReduction := 12.0
	if p.NoiseReduction != nil {
		noiseReduction = *p.NoiseReduction
	}
	return fmt.Sprintf("afftdn=nf=%s:nr=%s", formatFloat(noiseFloor), formatFloat(noiseReduction))
}

func parseDenoiseParams(raw map[string]any) (denoiseParams, error) {
	var p denoiseParams
	if v, ok := raw["noise_floor_db"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`noise_floor_db`: %w", err)
		}
		if f >= 0 || f < -80 {
			return p, fmt.Errorf("`noise_floor_db` must be in [-80, 0), got %g", f)
		}
		p.NoiseFloorDB = &f
	}
	if v, ok := raw["noise_reduction"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`noise_reduction`: %w", err)
		}
		if f < 0.01 || f > 97 {
			return p, fmt.Errorf("`noise_reduction` must be in [0.01, 97], got %g", f)
		}
		p.NoiseReduction = &f
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
