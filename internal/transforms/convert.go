package transforms

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	Register("convert", convert)
}

// convert changes the output container/codec of an input without
// trying to shrink it. For size-targeted re-encoding use `compress`
// instead — convert errs on the side of high quality.
//
// Params:
//   to — target format (required). One of:
//        image:  jpg, png, webp, avif
//        video:  mp4, mov, webm, mkv, gif, avi
//        audio:  mp3, aac, m4a, flac, wav, opus, ogg
//
// Cross-type targets (image → video, audio → video, etc.) error out
// with a clear message. Audio-to-mp4 is aliased to m4a.
type convertParams struct {
	To string `json:"to,omitempty"`
}

// formatKind returns which media class a target format belongs to.
// Used both for validation (can a video be converted to a target?)
// and for choosing the right per-mime dispatcher.
func formatKind(target string) MediaKind {
	switch strings.ToLower(target) {
	case "jpg", "jpeg", "png", "webp", "avif", "bmp", "tiff":
		return KindImage
	case "mp4", "mov", "webm", "mkv", "gif", "avi":
		return KindVideo
	case "mp3", "aac", "m4a", "flac", "wav", "opus", "ogg":
		return KindAudio
	}
	return KindUnknown
}

func convert(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
	params, err := parseConvertParams(req.Params)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}
	if params.To == "" {
		return nil, fmt.Errorf("convert: `to` is required (e.g. \"webm\")")
	}

	target := strings.ToLower(strings.TrimPrefix(params.To, "."))
	if formatKind(target) == KindUnknown {
		return nil, fmt.Errorf("convert: unknown target format %q", target)
	}

	job, err := stageInput(req, req.Media.Format, "")
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}
	defer job.cleanup()

	kind, probed, err := Probe(ctx, ffprobeBin, job.inPath)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}

	// Validate target compatibility with detected input.
	resolvedTarget, err := resolveConvertTarget(kind, target)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
	}

	args, outFmt, err := buildConvertArgs(kind, resolvedTarget, job, probed)
	if err != nil {
		return nil, fmt.Errorf("convert: %w", err)
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

// resolveConvertTarget validates + canonicalises the target for the
// detected input kind. Returns the (possibly aliased) target the
// arg-builder should use.
//
// Rules:
//   - image input → image target (or error)
//   - audio input → audio target (mp4/m4a aliased; aac/m4a treated
//     identically since they share a container; mp3 stays mp3 etc.)
//   - video input → video target. Special case: video → gif goes
//     through the palette-gen filter chain; gif → video re-encodes
//     to h264.
func resolveConvertTarget(kind MediaKind, target string) (string, error) {
	tk := formatKind(target)
	switch kind {
	case KindImage:
		if tk != KindImage {
			return "", fmt.Errorf("input is image; cannot convert to %s format %q (try a different `to`)", tk, target)
		}
	case KindAudio:
		if target == "mp4" {
			return "m4a", nil // alias: audio in mp4 container = m4a
		}
		if tk != KindAudio {
			return "", fmt.Errorf("input is audio; cannot convert to %s format %q (try `extract-audio` for the inverse)", tk, target)
		}
	case KindVideo:
		if tk != KindVideo {
			return "", fmt.Errorf("input is video; cannot convert to %s format %q", tk, target)
		}
	default:
		return "", fmt.Errorf("could not classify input")
	}
	if target == "jpeg" {
		return "jpg", nil
	}
	return target, nil
}

func buildConvertArgs(kind MediaKind, target string, job *stagedJob, probed *ffprobeResult) ([]string, string, error) {
	job.outPath = job.outPath + "." + target

	switch kind {
	case KindImage:
		return buildConvertImage(target, job), target, nil
	case KindAudio:
		return buildConvertAudio(target, job), target, nil
	case KindVideo:
		return buildConvertVideo(target, job, probed), target, nil
	}
	return nil, "", fmt.Errorf("unreachable: dispatch on unknown kind")
}

// ─── per-kind arg builders ─────────────────────────────────────

func buildConvertImage(target string, job *stagedJob) []string {
	// Default to high quality. `compress` is for size targeting.
	args := []string{"-y", "-i", job.inPath}
	switch target {
	case "jpg":
		args = append(args, "-q:v", "2") // visually lossless
	case "webp":
		args = append(args, "-c:v", "libwebp", "-quality", "90")
	case "avif":
		args = append(args, "-c:v", "libaom-av1", "-crf", "20", "-b:v", "0")
	case "png":
		args = append(args, "-compression_level", "6") // mid; lossless either way
	case "bmp", "tiff":
		// FFmpeg's defaults are fine; both are lossless.
	}
	return append(args, job.outPath)
}

func buildConvertAudio(target string, job *stagedJob) []string {
	args := []string{"-y", "-i", job.inPath, "-vn"} // strip cover-art
	switch target {
	case "mp3":
		args = append(args, "-c:a", "libmp3lame", "-b:a", "192k")
	case "aac", "m4a":
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	case "opus":
		args = append(args, "-c:a", "libopus", "-b:a", "128k")
	case "ogg":
		args = append(args, "-c:a", "libvorbis", "-q:a", "6")
	case "flac":
		args = append(args, "-c:a", "flac", "-compression_level", "5")
	case "wav":
		// PCM 16-bit stereo. Lossless, large.
		args = append(args, "-c:a", "pcm_s16le")
	}
	return append(args, job.outPath)
}

func buildConvertVideo(target string, job *stagedJob, probed *ffprobeResult) []string {
	switch target {
	case "gif":
		// Palette dance for clean GIFs. fps=15, scale long side
		// to 480 (small files), lanczos resample, palette via
		// diff stats, paletteuse with bayer dither.
		filter := "fps=15,scale=480:-1:flags=lanczos,split[s0][s1];" +
			"[s0]palettegen=stats_mode=diff[p];" +
			"[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle"
		return []string{
			"-y", "-i", job.inPath,
			"-vf", filter,
			job.outPath,
		}
	case "webm":
		// libvpx-vp9 (BSD), libopus (BSD). CRF 30 is the
		// "indistinguishable from source" sweet spot for libvpx-vp9.
		return []string{
			"-y", "-i", job.inPath,
			"-c:v", "libvpx-vp9", "-crf", "30", "-b:v", "0",
			"-c:a", "libopus", "-b:a", "128k",
			job.outPath,
		}
	case "mkv":
		// Container swap. We re-encode video via NVENC because
		// some source codecs (e.g., h264 high10) don't transmux
		// cleanly to mkv with -c copy. Caller wanting fast remux
		// should use `compress` with size_mb=∞ or a future remux
		// transform.
		return []string{
			"-y", "-i", job.inPath,
			"-c:v", "h264_nvenc", "-cq", "19",
			"-c:a", "aac", "-b:a", "192k",
			job.outPath,
		}
	case "avi":
		// AVI is largely legacy; we re-encode to mpeg4 (MPEG-4
		// part 2) which is LGPL-clean and AVI-compatible.
		return []string{
			"-y", "-i", job.inPath,
			"-c:v", "mpeg4", "-q:v", "5",
			"-c:a", "ac3", "-b:a", "192k",
			job.outPath,
		}
	default: // mp4, mov
		return []string{
			"-y", "-i", job.inPath,
			"-c:v", "h264_nvenc", "-cq", "19",
			"-c:a", "aac", "-b:a", "192k",
			"-movflags", "+faststart",
			job.outPath,
		}
	}
}

func parseConvertParams(raw map[string]any) (convertParams, error) {
	var p convertParams
	if v, ok := raw["to"]; ok {
		s, ok := v.(string)
		if !ok {
			return p, fmt.Errorf("`to` must be a string")
		}
		p.To = s
	}
	return p, nil
}
