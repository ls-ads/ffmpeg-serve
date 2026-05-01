package transforms

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	Register("upscale", upscale)
}

// upscale resamples an image or video to a larger size using one of
// ffmpeg's classical kernels. This is the "fast, deterministic,
// CPU-only, no-AI-budget" upscaler — `iosuite upscale` routes here
// when the user passes `--method=lanczos|bicubic|bilinear|neighbor`.
// The AI super-resolution path (real-esrgan / future SwinIR) lives
// in real-esrgan-serve and is what `iosuite upscale` defaults to;
// the two complement each other.
//
// Params:
//   scale  — multiplier on each axis. Float, default 4. Range
//            [1.5, 16] — below 1.5 is barely worth the re-encode,
//            above 16 produces buffers nobody can afford.
//   method — kernel name. "lanczos" (default), "bicubic", "bilinear",
//            "neighbor" (nearest-neighbor; for pixel art).
type upscaleParams struct {
	Scale  float64 `json:"scale,omitempty"`
	Method string  `json:"method,omitempty"`
}

// allowedMethods keys map directly to ffmpeg's `flags` argument on
// the scale filter.
var allowedMethods = map[string]bool{
	"lanczos":  true,
	"bicubic":  true,
	"bilinear": true,
	"neighbor": true,
}

func upscale(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseUpscaleParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}
	scale := params.Scale
	if scale == 0 {
		scale = 4
	}
	if scale < 1.5 || scale > 16 {
		return nil, fmt.Errorf("upscale: scale must be in [1.5, 16], got %g", scale)
	}
	method := params.Method
	if method == "" {
		method = "lanczos"
	}
	if !allowedMethods[method] {
		known := []string{"lanczos", "bicubic", "bilinear", "neighbor"}
		return nil, fmt.Errorf("upscale: unknown method %q (known: %s)", method, strings.Join(known, ", "))
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}

	if kind == KindAudio {
		return nil, fmt.Errorf("upscale: doesn't apply to audio")
	}
	if kind == KindUnknown {
		return nil, fmt.Errorf("upscale: could not classify input (format=%q)", probed.Format.FormatName)
	}

	inW, inH, err := videoStreamDims(probed)
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}
	outW := makeEven(int(float64(inW) * scale))
	outH := makeEven(int(float64(inH) * scale))

	vf := fmt.Sprintf("scale=%d:%d:flags=%s", outW, outH, method)

	if kind == KindImage {
		return upscaleImage(ctx, ffmpegBin, job, vf, probed)
	}
	return upscaleVideo(ctx, ffmpegBin, job, vf, probed)
}

func upscaleImage(ctx context.Context, ffmpegBin string, job *stagedJob, vf string, probed *ffprobeResult) ([]Output, error) {
	outFmt := imageOutFormat(compressParams{}, probed)
	job.outPath = job.outPath + "." + outFmt
	args := []string{
		"-y", "-i", job.inPath,
		"-vf", vf,
	}
	switch outFmt {
	case "jpg":
		args = append(args, "-q:v", "2") // visually lossless
	case "webp":
		args = append(args, "-c:v", "libwebp", "-quality", "92")
	case "png":
		args = append(args, "-compression_level", "6")
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

func upscaleVideo(ctx context.Context, ffmpegBin string, job *stagedJob, vf string, probed *ffprobeResult) ([]Output, error) {
	outFmt := videoOutFormat(compressParams{}, probed)
	job.outPath = job.outPath + "." + outFmt
	codec := "h264_nvenc"
	if outFmt == "webm" {
		codec = "libvpx-vp9"
	}
	args := []string{
		"-y", "-i", job.inPath,
		"-vf", vf,
		"-c:v", codec, "-cq", "19",
		"-c:a", "copy",
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

func parseUpscaleParams(raw map[string]any) (upscaleParams, error) {
	var p upscaleParams
	if v, ok := raw["scale"]; ok {
		f, err := toFloat(v)
		if err != nil {
			return p, fmt.Errorf("`scale`: %w", err)
		}
		p.Scale = f
	}
	if v, ok := raw["method"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`method` must be a string")
		}
		p.Method = s
	}
	return p, nil
}
