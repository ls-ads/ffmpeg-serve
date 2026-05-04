package transforms

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	Register("watermark", watermark)
}

// watermark overlays an image onto every frame of a video, or
// onto an image. The overlay image is passed via req.Aux[0].
//
// Removal is intentionally not supported here — that's a legal
// grey area we don't want to host.
//
// Params:
//
//	position — predefined corner: "top-left", "top-right",
//	           "bottom-left", "bottom-right" (default), "center".
//	margin   — pixels from the chosen edge. Default 16.
//	opacity  — 0.0 (invisible) to 1.0 (opaque). Default 0.85.
//	scale    — overlay width as a fraction of the input width.
//	           Default 0.2 (20% of input width).
//	format   — output container override.
type watermarkParams struct {
	Position string   `json:"position,omitempty"`
	Margin   *int     `json:"margin,omitempty"`
	Opacity  *float64 `json:"opacity,omitempty"`
	Scale    *float64 `json:"scale,omitempty"`
	Format   string   `json:"format,omitempty"`
}

func watermark(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseWatermarkParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("watermark: %w", err)
	}
	if len(req.Aux) == 0 {
		return nil, fmt.Errorf("watermark: aux[0] (overlay image) is required")
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("watermark: %w", err)
	}
	defer job.cleanup()

	auxPaths, err := stageAux(job, req.Aux)
	if err != nil {
		return nil, fmt.Errorf("watermark: %w", err)
	}

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("watermark: %w", err)
	}
	if kind == KindAudio {
		return nil, fmt.Errorf("watermark: doesn't apply to audio (no frames to overlay)")
	}
	if kind == KindUnknown {
		return nil, fmt.Errorf("watermark: could not classify input (format=%q)", probed.Format.FormatName)
	}

	filter := buildWatermarkFilter(params)

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
		"-y", "-i", job.inPath, "-i", auxPaths[0],
		"-filter_complex", filter,
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

// buildWatermarkFilter renders the filter_complex chain that
// scales the overlay, sets its opacity, and overlays it at the
// chosen corner. Layout:
//
//	[1:v]scale=...[ov_scaled]
//	[ov_scaled]format=rgba,colorchannelmixer=aa=<opacity>[ov_alpha]
//	[0:v][ov_alpha]overlay=<x>:<y>
func buildWatermarkFilter(p watermarkParams) string {
	scale := 0.2
	if p.Scale != nil {
		scale = *p.Scale
	}
	opacity := 0.85
	if p.Opacity != nil {
		opacity = *p.Opacity
	}
	margin := 16
	if p.Margin != nil {
		margin = *p.Margin
	}
	x, y := overlayPosition(p.Position, margin)

	// `main_w` references the primary input's width inside the
	// filter_complex graph; -1 lets ffmpeg pick the height that
	// preserves the overlay's aspect ratio.
	scaleExpr := fmt.Sprintf("scale=main_w*%s:-1", formatFloat(scale))

	return fmt.Sprintf(
		"[1:v]%s[ov_s];[ov_s]format=rgba,colorchannelmixer=aa=%s[ov];[0:v][ov]overlay=%s:%s",
		scaleExpr, formatFloat(opacity), x, y,
	)
}

// overlayPosition maps the canonical position label to the
// overlay-filter x:y expression. `main_w`/`main_h` are the
// primary input's dimensions; `overlay_w`/`overlay_h` are the
// scaled-overlay dimensions.
func overlayPosition(pos string, margin int) (string, string) {
	m := margin
	switch strings.ToLower(pos) {
	case "top-left":
		return fmt.Sprintf("%d", m), fmt.Sprintf("%d", m)
	case "top-right":
		return fmt.Sprintf("main_w-overlay_w-%d", m), fmt.Sprintf("%d", m)
	case "bottom-left":
		return fmt.Sprintf("%d", m), fmt.Sprintf("main_h-overlay_h-%d", m)
	case "center":
		return "(main_w-overlay_w)/2", "(main_h-overlay_h)/2"
	}
	// default: bottom-right
	return fmt.Sprintf("main_w-overlay_w-%d", m), fmt.Sprintf("main_h-overlay_h-%d", m)
}

func parseWatermarkParams(raw map[string]any) (watermarkParams, error) {
	var p watermarkParams
	if v, ok := raw["position"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`position` must be a string")
		}
		valid := map[string]bool{
			"top-left": true, "top-right": true,
			"bottom-left": true, "bottom-right": true,
			"center": true,
		}
		if s != "" && !valid[s] {
			return p, fmt.Errorf("`position` must be one of top-left, top-right, bottom-left, bottom-right, center — got %q", s)
		}
		p.Position = s
	}
	if v, ok := raw["margin"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`margin`: %w", err)
		}
		if f < 0 || f > 500 {
			return p, fmt.Errorf("`margin` must be in [0, 500], got %g", f)
		}
		i := int(f)
		p.Margin = &i
	}
	if v, ok := raw["opacity"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`opacity`: %w", err)
		}
		if f < 0 || f > 1 {
			return p, fmt.Errorf("`opacity` must be in [0, 1], got %g", f)
		}
		p.Opacity = &f
	}
	if v, ok := raw["scale"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`scale`: %w", err)
		}
		if f < 0.01 || f > 1 {
			return p, fmt.Errorf("`scale` must be in [0.01, 1], got %g", f)
		}
		p.Scale = &f
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
