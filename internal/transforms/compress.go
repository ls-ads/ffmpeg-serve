package transforms

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func init() {
	Register("compress", compress)
}

// compress reduces the byte size of the input.
//
// Dispatches by ffprobe-detected mime:
//   - image: re-encode at a quality target (jpeg/webp/avif).
//   - video: re-encode at a target file-size in MB (calculated bitrate).
//   - audio: re-encode at a target bitrate.
//
// Params (one of):
//   target          — preset shorthand. Currently:
//                       "discord"   → video 10 MB
//                       "whatsapp"  → video 16 MB
//                       "x"         → video 512 MB
//                       "twitter"   → alias for "x"
//   size_mb         — video target file size, MB (float allowed).
//   quality         — image quality, 1–100. Higher = bigger + better.
//                     Default 75 (matches PIL/ImageMagick defaults).
//   bitrate_kbps    — audio bitrate, integer. Default 128.
//   format          — output container; defaults to the input's.
//
// All params are optional; the handler picks sensible defaults for
// whichever media type was detected.
type compressParams struct {
	Target      string  `json:"target,omitempty"`
	SizeMB      float64 `json:"size_mb,omitempty"`
	Quality     int     `json:"quality,omitempty"`
	BitrateKbps int     `json:"bitrate_kbps,omitempty"`
	Format      string  `json:"format,omitempty"`
}

// presetSizesMB maps the convenience-target strings to a video MB cap.
// Image and audio ignore these (their targets are quality / bitrate).
var presetSizesMB = map[string]float64{
	"discord":  10,
	"whatsapp": 16,
	"x":        512,
	"twitter":  512,
}

func compress(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseCompressParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}

	// Stage with the input's declared format if any. We don't need
	// the output extension yet — per-mime branches set it before
	// renaming the staged out-path.
	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("compress: %w", err)
	}

	switch kind {
	case KindImage:
		return compressImage(ctx, ffmpegBin, job, params, probed)
	case KindVideo:
		return compressVideo(ctx, ffmpegBin, job, params, probed)
	case KindAudio:
		return compressAudio(ctx, ffmpegBin, job, params, probed)
	}
	return nil, fmt.Errorf("compress: could not classify input (format=%q)", probed.Format.FormatName)
}

// ─── image ──────────────────────────────────────────────────────

// imageOutFormat picks the output container for image compression.
// Defaults to the input's format if it's something we can write at
// quality; otherwise jpg.
func imageOutFormat(params compressParams, probed *ffprobeResult) string {
	if params.Format != "" {
		return params.Format
	}
	// ffprobe's image format names: png_pipe / jpeg_pipe / etc.
	// Strip the _pipe suffix and lowercase.
	f := strings.TrimSuffix(probed.Format.FormatName, "_pipe")
	switch f {
	case "jpeg", "mjpeg":
		return "jpg"
	case "png", "webp", "avif":
		return f
	}
	return "jpg"
}

func compressImage(ctx context.Context, ffmpegBin string, job *stagedJob, params compressParams, probed *ffprobeResult) ([]Output, error) {
	outFmt := imageOutFormat(params, probed)
	job.outPath = strings.TrimSuffix(job.outPath, "") + "." + outFmt

	quality := params.Quality
	if quality == 0 {
		quality = 75
	}
	if quality < 1 || quality > 100 {
		return nil, fmt.Errorf("compress image: quality must be 1-100, got %d", quality)
	}

	args := []string{"-y", "-i", job.inPath}

	switch outFmt {
	case "jpg", "jpeg":
		// FFmpeg JPEG quality is 2-31 where lower=better. Map
		// 1-100 (higher=better) to 31-2 linearly.
		// 100 → 2, 1 → 31.
		q := 31 - int(float64(quality-1)*29.0/99.0)
		if q < 2 {
			q = 2
		}
		if q > 31 {
			q = 31
		}
		args = append(args, "-q:v", strconv.Itoa(q))
	case "webp":
		// libwebp's `quality` is 0-100, higher=better — direct map.
		args = append(args, "-c:v", "libwebp", "-quality", strconv.Itoa(quality))
	case "avif":
		// libaom's `crf` is 0-63 lower=better. Map 100→0, 1→63.
		crf := 63 - int(float64(quality-1)*63.0/99.0)
		args = append(args, "-c:v", "libaom-av1", "-crf", strconv.Itoa(crf), "-b:v", "0")
	case "png":
		// PNG is lossless — quality is irrelevant. Use a higher
		// compression level instead. 0=fastest, 9=smallest.
		args = append(args, "-compression_level", "9")
	default:
		return nil, fmt.Errorf("compress image: unsupported output format %q", outFmt)
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

// ─── video ──────────────────────────────────────────────────────

// videoOutFormat picks the output container. Defaults to mp4 when
// the input is a non-mp4 format we can't trivially reuse (avi, flv
// → mp4 is a sensible default; matroska stays matroska, webm stays
// webm). Caller can always force via params.Format.
func videoOutFormat(params compressParams, probed *ffprobeResult) string {
	if params.Format != "" {
		return params.Format
	}
	f := probed.Format.FormatName
	switch {
	case strings.Contains(f, "matroska"):
		return "mkv"
	case strings.Contains(f, "webm"):
		return "webm"
	case strings.Contains(f, "mov"), strings.Contains(f, "mp4"):
		return "mp4"
	case strings.Contains(f, "gif"):
		return "mp4" // animated gif → mp4 is the right default
	}
	return "mp4"
}

func compressVideo(ctx context.Context, ffmpegBin string, job *stagedJob, params compressParams, probed *ffprobeResult) ([]Output, error) {
	outFmt := videoOutFormat(params, probed)
	job.outPath = job.outPath + "." + outFmt

	// Resolve target size.
	sizeMB := params.SizeMB
	if sizeMB == 0 && params.Target != "" {
		s, ok := presetSizesMB[strings.ToLower(params.Target)]
		if !ok {
			return nil, fmt.Errorf("compress video: unknown target %q (known: discord, whatsapp, x)", params.Target)
		}
		sizeMB = s
	}
	if sizeMB == 0 {
		sizeMB = 10 // sensible default if neither target nor size_mb passed.
	}

	durationSec, err := strconv.ParseFloat(probed.Format.Duration, 64)
	if err != nil || durationSec <= 0 {
		return nil, fmt.Errorf("compress video: invalid duration %q from ffprobe", probed.Format.Duration)
	}

	// Reserve audio bitrate from the budget. AAC at 128 kbps is the
	// floor we won't go below for general-purpose audio.
	const audioKbps = 128
	totalKbps := (sizeMB * 8 * 1024) / durationSec
	videoKbps := int(totalKbps) - audioKbps
	if videoKbps < 100 {
		return nil, fmt.Errorf("compress video: target %.2f MB over %.1fs leaves only %d kbps for video (need ≥100). Lower the target duration or raise the size.",
			sizeMB, durationSec, videoKbps)
	}

	// Encoder selection. Default to NVENC where available; fall back
	// to libvpx-vp9 (BSD) for CPU-only builds. NOTE: callers can
	// force the libvpx path via `params.format = "webm"` since NVENC
	// can't write VP9.
	videoCodec := "h264_nvenc"
	if outFmt == "webm" {
		videoCodec = "libvpx-vp9"
	}

	args := []string{
		"-y", "-i", job.inPath,
		"-c:v", videoCodec,
		"-b:v", fmt.Sprintf("%dk", videoKbps),
		"-maxrate", fmt.Sprintf("%dk", videoKbps),
		"-bufsize", fmt.Sprintf("%dk", videoKbps*2),
		"-c:a", "aac",
		"-b:a", fmt.Sprintf("%dk", audioKbps),
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

// ─── audio ──────────────────────────────────────────────────────

// audioOutFormat picks the output container.
func audioOutFormat(params compressParams, probed *ffprobeResult) string {
	if params.Format != "" {
		return params.Format
	}
	f := probed.Format.FormatName
	switch {
	case strings.Contains(f, "mp3"):
		return "mp3"
	case strings.Contains(f, "flac"):
		return "mp3" // flac → mp3 is "compress to lossy"; user can opt out via Format
	case strings.Contains(f, "wav"), strings.Contains(f, "aiff"):
		return "mp3"
	case strings.Contains(f, "opus"):
		return "opus"
	case strings.Contains(f, "ogg"):
		return "ogg"
	case strings.Contains(f, "m4a"), strings.Contains(f, "aac"):
		return "m4a"
	}
	return "mp3"
}

func compressAudio(ctx context.Context, ffmpegBin string, job *stagedJob, params compressParams, probed *ffprobeResult) ([]Output, error) {
	outFmt := audioOutFormat(params, probed)
	job.outPath = job.outPath + "." + outFmt

	bitrate := params.BitrateKbps
	if bitrate == 0 {
		bitrate = 128
	}
	if bitrate < 32 || bitrate > 320 {
		return nil, fmt.Errorf("compress audio: bitrate_kbps must be 32-320, got %d", bitrate)
	}

	codec := "libmp3lame"
	switch outFmt {
	case "m4a", "aac":
		codec = "aac"
	case "opus":
		codec = "libopus"
	case "ogg":
		codec = "libvorbis"
	case "mp3":
		codec = "libmp3lame"
	}

	args := []string{
		"-y", "-i", job.inPath,
		"-c:a", codec,
		"-b:a", fmt.Sprintf("%dk", bitrate),
		"-vn", // strip cover-art video stream if any
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

// parseCompressParams decodes the loose `map[string]any` params bag
// into a typed struct. Returns an error if a known field is the
// wrong type; ignores unknown fields (they may be from a future
// schema version).
func parseCompressParams(raw map[string]any) (compressParams, error) {
	var p compressParams
	if v, ok := raw["target"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`target` must be a string")
		}
		p.Target = s
	}
	if v, ok := raw["size_mb"]; ok {
		// Accept int or float from JSON.
		switch x := v.(type) {
		case float64:
			p.SizeMB = x
		case int:
			p.SizeMB = float64(x)
		default:
			return p, fmt.Errorf("`size_mb` must be a number")
		}
	}
	if v, ok := raw["quality"]; ok {
		switch x := v.(type) {
		case float64:
			p.Quality = int(x)
		case int:
			p.Quality = x
		default:
			return p, fmt.Errorf("`quality` must be an integer")
		}
	}
	if v, ok := raw["bitrate_kbps"]; ok {
		switch x := v.(type) {
		case float64:
			p.BitrateKbps = int(x)
		case int:
			p.BitrateKbps = x
		default:
			return p, fmt.Errorf("`bitrate_kbps` must be an integer")
		}
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
