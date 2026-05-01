package transforms

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func init() {
	Register("reframe", reframe)
}

// reframe changes the aspect ratio of an image or video by either
// blur-padding (default) or cropping. Audio inputs error out — the
// concept doesn't apply.
//
// Params:
//   to    — target aspect ratio. "9:16", "1:1", "16:9", "4:5", "4:3"
//           are the obvious ones; any "W:H" with positive integers
//           is accepted. Required.
//   fit   — "blur-pad" (default), "crop", "letterbox" (black bars),
//           or "stretch" (anisotropic, ugly — use only when the user
//           explicitly knows they want it).
//
// Output dimensions: longest side preserves the input's longest side,
// shorter side computed from the target aspect, rounded to even.
// Concretely:
//   1920×1080 → 9:16  →  1920×1080-portrait → 1080×1920 (out long-side
//                                              = in long-side = 1920).
//   1080×1920 → 1:1   →  1920×1920.
//   3840×2160 → 9:16  →  2160×3840 (preserves 4K-equivalent).
type reframeParams struct {
	To  string `json:"to,omitempty"`
	Fit string `json:"fit,omitempty"`
}

func reframe(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseReframeParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("reframe: %w", err)
	}
	if params.To == "" {
		return nil, fmt.Errorf("reframe: `to` is required (e.g. \"9:16\")")
	}
	targetW, targetH, err := parseAspect(params.To)
	if err != nil {
		return nil, fmt.Errorf("reframe: %w", err)
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("reframe: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("reframe: %w", err)
	}

	switch kind {
	case KindAudio:
		return nil, fmt.Errorf("reframe: doesn't apply to audio")
	case KindUnknown:
		return nil, fmt.Errorf("reframe: could not classify input (format=%q)", probed.Format.FormatName)
	}

	// Pull input dimensions from the first video stream.
	inW, inH, err := videoStreamDims(probed)
	if err != nil {
		return nil, fmt.Errorf("reframe: %w", err)
	}

	outW, outH := outputDims(inW, inH, targetW, targetH)

	fit := params.Fit
	if fit == "" {
		fit = "blur-pad"
	}
	vf, err := reframeFilter(fit, outW, outH)
	if err != nil {
		return nil, fmt.Errorf("reframe: %w", err)
	}

	if kind == KindImage {
		return reframeImage(ctx, ffmpegBin, job, vf, params.Fit)
	}
	return reframeVideo(ctx, ffmpegBin, job, vf, probed)
}

// reframeFilter returns the -vf chain for the given fit mode and
// output dimensions.
func reframeFilter(fit string, outW, outH int) (string, error) {
	switch fit {
	case "blur-pad":
		// Two-stream filter: scale the input twice — once big-and-
		// blurred to fill the canvas as background, once fit-inside
		// to preserve the actual content. Overlay the foreground on
		// the blurred background.
		return fmt.Sprintf(
			"split=2[bg][fg];"+
				"[bg]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,boxblur=luma_radius=20:luma_power=2[bgblur];"+
				"[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgfit];"+
				"[bgblur][fgfit]overlay=(W-w)/2:(H-h)/2,setsar=1",
			outW, outH, outW, outH, outW, outH), nil
	case "letterbox":
		// Black bars. force_original_aspect_ratio=decrease + pad.
		return fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1",
			outW, outH, outW, outH), nil
	case "crop":
		// Cover the canvas (force_original_aspect_ratio=increase),
		// then center-crop. Loses content at the edges.
		return fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1",
			outW, outH, outW, outH), nil
	case "stretch":
		// Anisotropic. Loud about it in the name.
		return fmt.Sprintf("scale=%d:%d,setsar=1", outW, outH), nil
	}
	return "", fmt.Errorf("unknown fit mode %q (known: blur-pad, letterbox, crop, stretch)", fit)
}

func reframeImage(ctx context.Context, ffmpegBin string, job *stagedJob, vf, fit string) ([]Output, error) {
	_ = fit // (fit is informational only here; encoded into vf already)
	// Default to jpg unless the input was a lossless format we should
	// preserve. Mirrors compress's image-format choice but we don't
	// have the params.Format hint here; user can always run convert
	// after. Default jpg is the right call for "I want to crosspost
	// this to TikTok-or-similar".
	outFmt := "jpg"
	job.outPath = job.outPath + "." + outFmt
	args := []string{
		"-y", "-i", job.inPath,
		"-vf", vf,
		"-q:v", "2", // visually-lossless JPEG
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

func reframeVideo(ctx context.Context, ffmpegBin string, job *stagedJob, vf string, probed *ffprobeResult) ([]Output, error) {
	outFmt := videoOutFormat(compressParams{}, probed) // reuse the same default-format logic
	job.outPath = job.outPath + "." + outFmt

	codec := "h264_nvenc"
	if outFmt == "webm" {
		codec = "libvpx-vp9"
	}

	args := []string{
		"-y", "-i", job.inPath,
		"-vf", vf,
		"-c:v", codec,
		"-c:a", "copy", // audio passes through unchanged
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

// parseAspect parses "9:16" → (9, 16). Rejects zero/negative.
func parseAspect(s string) (int, int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("aspect %q must be \"W:H\"", s)
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || w <= 0 {
		return 0, 0, fmt.Errorf("aspect %q: width must be a positive integer", s)
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || h <= 0 {
		return 0, 0, fmt.Errorf("aspect %q: height must be a positive integer", s)
	}
	return w, h, nil
}

// outputDims computes the output canvas size from the input
// dimensions + target aspect. Long side preserves the input's long
// side; short side falls out of the ratio. Both clamped to even
// (h264 / NVENC require even dims).
func outputDims(inW, inH, aspW, aspH int) (int, int) {
	inLong := inW
	if inH > inW {
		inLong = inH
	}
	var outW, outH int
	if aspW >= aspH {
		// Landscape (or square) target.
		outW = inLong
		outH = int(float64(inLong) * float64(aspH) / float64(aspW))
	} else {
		// Portrait target.
		outH = inLong
		outW = int(float64(inLong) * float64(aspW) / float64(aspH))
	}
	return makeEven(outW), makeEven(outH)
}

func makeEven(n int) int {
	if n%2 != 0 {
		return n - 1
	}
	return n
}

// videoStreamDims pulls (width, height) from the first video stream
// in the ffprobe result. Errors when there isn't one — reframe needs
// dimensions, and we've already gated on `kind ≠ audio` upstream.
func videoStreamDims(probed *ffprobeResult) (int, int, error) {
	for _, s := range probed.Streams {
		if s.CodecType == "video" {
			if s.Width > 0 && s.Height > 0 {
				return s.Width, s.Height, nil
			}
		}
	}
	return 0, 0, fmt.Errorf("no video stream with non-zero dimensions in input")
}

func parseReframeParams(raw map[string]any) (reframeParams, error) {
	var p reframeParams
	if v, ok := raw["to"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`to` must be a string")
		}
		p.To = s
	}
	if v, ok := raw["fit"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`fit` must be a string")
		}
		p.Fit = s
	}
	return p, nil
}
