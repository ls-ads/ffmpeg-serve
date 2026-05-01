package transforms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// MediaKind is what the daemon dispatches on. Every transform's
// per-mime branch keys off this enum, computed from ffprobe's
// `format_name`.
type MediaKind int

const (
	KindUnknown MediaKind = iota
	KindImage
	KindVideo
	KindAudio
)

func (k MediaKind) String() string {
	switch k {
	case KindImage:
		return "image"
	case KindVideo:
		return "video"
	case KindAudio:
		return "audio"
	}
	return "unknown"
}

// audio container/codec format_name fragments. ffprobe returns a
// comma-separated list (e.g., "mp3" or "wav,aiff" depending on the
// container's compatibility list); we match if any fragment hits.
var audioFormats = []string{
	"mp3", "wav", "flac", "ogg", "opus", "m4a", "aiff", "wma",
	"aac", "ac3", "dts", "matroska,webm,audio", "mov,mp4,m4a,3gp,3g2,mj2",
}

// image format_name fragments. Note that "gif" is intentionally NOT
// here — animated GIFs share the gif container with single-frame
// stills, and downstream transforms (compress, reframe) treat them
// as video. The duration heuristic separates the two cases.
var imageFormats = []string{
	"image2", "png_pipe", "jpeg_pipe", "mjpeg", "webp_pipe",
	"tiff_pipe", "bmp_pipe", "avif", "heif",
}

// ffprobeResult captures the subset of ffprobe's JSON output we use.
// Format.FormatName is a comma-separated list of compatible format
// strings; Format.Duration is "N/A" or seconds-as-string.
type ffprobeResult struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		NbFrames  string `json:"nb_frames"`
	} `json:"streams"`
}

// Probe runs ffprobe against a path and classifies the file as
// image / video / audio. The path can be a real file or "-" to read
// from stdin (caller's responsibility to pipe bytes in).
//
// Classification rules, in order:
//  1. format_name matches a known image container → image (special-
//     cases gif via duration check below).
//  2. format_name matches a known audio container AND no video stream
//     OR every video stream is "attached_pic" cover art → audio.
//  3. has any video stream with duration > 1s OR multiple frames
//     → video.
//  4. has a video stream but only 1 frame, no other streams → image.
//  5. fallback: video.
func Probe(ctx context.Context, ffprobeBin, path string) (MediaKind, *ffprobeResult, error) {
	cmd := exec.CommandContext(ctx, ffprobeBin,
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams",
		path,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return KindUnknown, nil, fmt.Errorf("ffprobe %s: %w (stderr: %s)",
			path, err, strings.TrimSpace(stderr.String()))
	}

	var out ffprobeResult
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return KindUnknown, nil, fmt.Errorf("parse ffprobe JSON: %w", err)
	}

	formatName := out.Format.FormatName

	// Step 1: image-container short-circuit.
	if matchesAny(formatName, imageFormats) {
		return KindImage, &out, nil
	}

	// Step 2: audio-container — only if no video stream OR video is
	// just attached cover art (codec_name png/jpeg, nb_frames 1).
	if matchesAny(formatName, audioFormats) {
		hasRealVideo := false
		for _, s := range out.Streams {
			if s.CodecType != "video" {
				continue
			}
			// Cover art for music files: single still frame, codec
			// is an image codec. Treat as audio.
			if s.NbFrames == "1" && (s.CodecName == "png" || s.CodecName == "mjpeg" || s.CodecName == "jpeg") {
				continue
			}
			hasRealVideo = true
			break
		}
		if !hasRealVideo {
			return KindAudio, &out, nil
		}
	}

	// Step 3 + 4: distinguish video from single-frame "video"-format
	// stills (e.g., a one-frame gif). Animated gif → video; static
	// gif → image. mp4 with one frame → image (rare, but real).
	hasVideo := false
	hasAudio := false
	maxFrames := 0
	for _, s := range out.Streams {
		switch s.CodecType {
		case "video":
			hasVideo = true
			if s.NbFrames != "" && s.NbFrames != "N/A" {
				var n int
				_, _ = fmt.Sscanf(s.NbFrames, "%d", &n)
				if n > maxFrames {
					maxFrames = n
				}
			}
		case "audio":
			hasAudio = true
		}
	}

	if hasVideo && maxFrames == 1 && !hasAudio {
		return KindImage, &out, nil
	}
	if hasVideo {
		return KindVideo, &out, nil
	}
	if hasAudio {
		return KindAudio, &out, nil
	}
	return KindUnknown, &out, nil
}

func matchesAny(formatName string, candidates []string) bool {
	parts := strings.Split(formatName, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		for _, c := range candidates {
			if p == c {
				return true
			}
		}
	}
	// Also check the full string for compound names like "matroska,webm".
	for _, c := range candidates {
		if strings.Contains(c, ",") && c == formatName {
			return true
		}
	}
	return false
}
