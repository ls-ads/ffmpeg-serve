package transforms

import (
	"context"
	"fmt"
)

func init() {
	Register("color-lut", colorLUT)
}

// colorLUT applies a 3D colour LUT (.cube file) to every frame of
// an image or video. The .cube text file is passed via
// req.Aux[0]. Pro-creator workflow: grade once, drop the LUT
// across hundreds of clips.
//
// Params:
//
//	intensity — blend factor between original and LUT-applied
//	            (0.0 = no change, 1.0 = full LUT). Default 1.0.
//	            Implemented via the `lut3d` filter's `interp` +
//	            a per-pixel blend post-step when < 1.0.
//	format    — output container override.
type colorLUTParams struct {
	Intensity *float64 `json:"intensity,omitempty"`
	Format    string   `json:"format,omitempty"`
}

func colorLUT(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseColorLUTParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("color-lut: %w", err)
	}
	if len(req.Aux) == 0 {
		return nil, fmt.Errorf("color-lut: aux[0] (LUT .cube file) is required")
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("color-lut: %w", err)
	}
	defer job.cleanup()

	// Force the aux extension to .cube — lut3d sniffs by extension
	// and silently no-ops if the file looks like something else.
	auxCopy := make([]Media, len(req.Aux))
	copy(auxCopy, req.Aux)
	for i := range auxCopy {
		if auxCopy[i].Format == "" {
			auxCopy[i].Format = "cube"
		}
	}
	auxPaths, err := stageAux(job, auxCopy)
	if err != nil {
		return nil, fmt.Errorf("color-lut: %w", err)
	}

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("color-lut: %w", err)
	}
	if kind == KindAudio {
		return nil, fmt.Errorf("color-lut: doesn't apply to audio (no frames to grade)")
	}
	if kind == KindUnknown {
		return nil, fmt.Errorf("color-lut: could not classify input (format=%q)", probed.Format.FormatName)
	}

	filter := buildLUTFilter(auxPaths[0], params)

	var outFmt string
	if kind == KindImage {
		outFmt = params.Format
		if outFmt == "" {
			outFmt = "jpg"
		}
	} else {
		outFmt = params.Format
		if outFmt == "" {
			outFmt = videoOutFormat(compressParams{}, probed)
		}
	}
	job.outPath = job.outPath + "." + outFmt

	args := []string{
		"-y", "-i", job.inPath,
		"-vf", filter,
	}
	if kind == KindImage {
		args = append(args, "-frames:v", "1")
	} else {
		args = append(args,
			"-c:v", "libx264", "-crf", "20", "-preset", "veryfast",
			"-c:a", "aac", "-b:a", "192k",
			"-movflags", "+faststart",
		)
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

// buildLUTFilter renders the -vf chain. For full intensity (1.0)
// it's just lut3d=<path>. For partial intensity, blend the LUT
// pass with the original frame: `[a]lut3d=<file>[graded];
// [a][graded]blend=all_mode='normal':all_opacity=<intensity>`.
func buildLUTFilter(lutPath string, p colorLUTParams) string {
	intensity := 1.0
	if p.Intensity != nil {
		intensity = *p.Intensity
	}
	if intensity >= 0.999 {
		return fmt.Sprintf("lut3d=%s", lutPath)
	}
	return fmt.Sprintf(
		"split=2[a][b];[b]lut3d=%s[graded];[a][graded]blend=all_mode=normal:all_opacity=%s",
		lutPath, formatFloat(intensity),
	)
}

func parseColorLUTParams(raw map[string]any) (colorLUTParams, error) {
	var p colorLUTParams
	if v, ok := raw["intensity"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`intensity`: %w", err)
		}
		if f < 0 || f > 1 {
			return p, fmt.Errorf("`intensity` must be in [0, 1], got %g", f)
		}
		p.Intensity = &f
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
