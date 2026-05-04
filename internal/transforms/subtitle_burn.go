package transforms

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	Register("subtitle-burn", subtitleBurn)
}

// subtitleBurn hardcodes a subtitle track into a video. The
// subtitle file (SRT or VTT) is passed via req.Aux[0]; the video
// goes through the primary input.
//
// Why hardcode + not soft-mux: hardcoded subtitles render on
// every player without needing a track-aware client. The trade-
// off is they can't be turned off — that's the point for a
// "burn" verb.
//
// Params:
//
//	font_size — subtitle render size in pixels. Default 28.
//	font_color — Hex `RRGGBB`. Default ffffff (white).
//	outline    — outline width in pixels for legibility on busy
//	             backgrounds. Default 2.
//	format     — output container override. Default mp4.
type subtitleBurnParams struct {
	FontSize  *int    `json:"font_size,omitempty"`
	FontColor string  `json:"font_color,omitempty"`
	Outline   *int    `json:"outline,omitempty"`
	Format    string  `json:"format,omitempty"`
}

func subtitleBurn(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseSubtitleBurnParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("subtitle-burn: %w", err)
	}
	if len(req.Aux) == 0 {
		return nil, fmt.Errorf("subtitle-burn: aux[0] (subtitle file) is required — pass an SRT or VTT")
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("subtitle-burn: %w", err)
	}
	defer job.cleanup()

	auxPaths, err := stageAux(job, req.Aux)
	if err != nil {
		return nil, fmt.Errorf("subtitle-burn: %w", err)
	}

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("subtitle-burn: %w", err)
	}
	if kind != KindVideo {
		return nil, fmt.Errorf("subtitle-burn: input must be video, got %s", kind)
	}

	outFmt := params.Format
	if outFmt == "" {
		outFmt = videoOutFormat(compressParams{}, probed)
	}
	job.outPath = job.outPath + "." + outFmt

	filter := buildSubtitleFilter(auxPaths[0], params)

	args := []string{
		"-y", "-i", job.inPath,
		"-vf", filter,
		"-c:v", "libx264", "-crf", "20", "-preset", "veryfast",
		"-c:a", "aac", "-b:a", "192k",
		"-movflags", "+faststart",
		job.outPath,
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

// buildSubtitleFilter renders the `subtitles=` filter expression
// with optional force_style overrides. ffmpeg accepts both SRT and
// VTT through the same filter — libass picks the right parser by
// extension/content. force_style is comma-separated key=value
// pairs (FontSize, PrimaryColour, OutlineWidth).
func buildSubtitleFilter(subtitlePath string, p subtitleBurnParams) string {
	// Escape the path so colons / commas in the path don't break
	// filter-expression parsing. ffmpeg's escape rules for
	// filter args: backslash before special chars, single-quote
	// the whole thing.
	escaped := strings.ReplaceAll(subtitlePath, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, ":", "\\:")
	escaped = strings.ReplaceAll(escaped, "'", "\\'")

	style := []string{}
	if p.FontSize != nil {
		style = append(style, fmt.Sprintf("FontSize=%d", *p.FontSize))
	}
	if p.FontColor != "" {
		// libass colour format is &H<AABBGGRR>& — alpha-first,
		// then BGR not RGB. Convert the user-friendly RRGGBB.
		style = append(style, fmt.Sprintf("PrimaryColour=&H00%s&", bgrFromRgb(p.FontColor)))
	}
	if p.Outline != nil {
		style = append(style, fmt.Sprintf("OutlineWidth=%d", *p.Outline))
	}

	expr := fmt.Sprintf("subtitles='%s'", escaped)
	if len(style) > 0 {
		expr = fmt.Sprintf("%s:force_style='%s'", expr, strings.Join(style, ","))
	}
	return expr
}

// bgrFromRgb converts a 6-character hex string from RGB order
// (RRGGBB) to BGR order (BBGGRR) for libass. Returns the input
// unchanged when it isn't 6 hex chars — caller should validate
// upstream.
func bgrFromRgb(rgb string) string {
	if len(rgb) != 6 {
		return rgb
	}
	return rgb[4:6] + rgb[2:4] + rgb[0:2]
}

func parseSubtitleBurnParams(raw map[string]any) (subtitleBurnParams, error) {
	var p subtitleBurnParams
	if v, ok := raw["font_size"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`font_size`: %w", err)
		}
		if f < 8 || f > 200 {
			return p, fmt.Errorf("`font_size` must be in [8, 200], got %g", f)
		}
		i := int(f)
		p.FontSize = &i
	}
	if v, ok := raw["font_color"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`font_color` must be a hex string like ffffff")
		}
		s = strings.TrimPrefix(s, "#")
		if len(s) != 6 {
			return p, fmt.Errorf("`font_color` must be 6 hex chars (RRGGBB), got %q", s)
		}
		for _, c := range s {
			ok := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !ok {
				return p, fmt.Errorf("`font_color` has non-hex char %q", c)
			}
		}
		p.FontColor = strings.ToLower(s)
	}
	if v, ok := raw["outline"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`outline`: %w", err)
		}
		if f < 0 || f > 10 {
			return p, fmt.Errorf("`outline` must be in [0, 10], got %g", f)
		}
		i := int(f)
		p.Outline = &i
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
