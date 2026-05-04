package transforms

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ls-ads/ffmpeg-serve/internal/runtime"
)

// Command returns the cobra command for `ffmpeg-serve transform`,
// the one-shot CLI mode. Reads an input file, runs a registered
// transform handler in-process (no HTTP), writes the output file.
//
// This is what `iosuite compress`, `iosuite reframe`, etc. subprocess
// to. Mirrors `real-esrgan-serve super-resolution`'s shape: the same Go
// process that hosts the daemon also runs single-shot transforms
// from the command line, sharing the handler code path.
func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "transform <name>",
		Short: "Run one transform on a local file (no HTTP)",
		Long: `Run a registered transform on a local file. Same handler the
HTTP daemon dispatches; just driven from the CLI directly so iosuite
can subprocess this rather than spinning up a daemon per invocation.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			input, _ := cmd.Flags().GetString("input")
			output, _ := cmd.Flags().GetString("output")
			paramsJSON, _ := cmd.Flags().GetString("params")
			ffmpegOverride, _ := cmd.Flags().GetString("ffmpeg")
			ffprobeOverride, _ := cmd.Flags().GetString("ffprobe")
			auxPaths, _ := cmd.Flags().GetStringSlice("aux")

			if input == "" {
				return fmt.Errorf("--input is required")
			}
			return runOnce(cmd.Context(), args[0], input, output, paramsJSON,
				ffmpegOverride, ffprobeOverride, auxPaths)
		},
	}
	cmd.Flags().StringP("input", "i", "", "Input file path (required)")
	cmd.Flags().StringP("output", "o", "", "Output file path. Auto-derived from input + transform if omitted.")
	cmd.Flags().String("params", "{}", "Transform-specific params as a JSON object string")
	cmd.Flags().String("ffmpeg", "", "Override path to ffmpeg")
	cmd.Flags().String("ffprobe", "", "Override path to ffprobe")
	cmd.Flags().StringSlice("aux", nil, "Secondary input file path (repeatable). Used by subtitle-burn, watermark, color-lut.")
	return cmd
}

// runOnce reads the input file, builds a Request the same shape the
// HTTP daemon receives (input_b64), invokes the registered handler,
// writes the output bytes to disk. Errors are returned to the cobra
// layer for stderr formatting.
func runOnce(ctx context.Context, name, inputPath, outputPath, paramsJSON, ffmpegOverride, ffprobeOverride string, auxPaths []string) error {
	handler, ok := Lookup(name)
	if !ok {
		return &ErrUnknownTransform{Name: name}
	}

	var params map[string]any
	if strings.TrimSpace(paramsJSON) != "" {
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			return fmt.Errorf("--params: %w", err)
		}
	}
	if params == nil {
		params = map[string]any{}
	}

	resolved, err := runtime.Locate(ffmpegOverride, ffprobeOverride)
	if err != nil {
		return err
	}

	rawIn, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input %s: %w", inputPath, err)
	}

	req := Request{
		Transform: name,
		Params:    params,
		Media: Media{
			InputB64: base64.StdEncoding.EncodeToString(rawIn),
			Format:   strings.TrimPrefix(filepath.Ext(inputPath), "."),
		},
	}

	// Read each aux file, base64-encode, populate req.Aux. Same
	// ingestion shape the HTTP path uses — handlers don't see a
	// distinction between aux from CLI vs aux from JSON.
	for _, ap := range auxPaths {
		raw, err := os.ReadFile(ap)
		if err != nil {
			return fmt.Errorf("read aux %s: %w", ap, err)
		}
		req.Aux = append(req.Aux, Media{
			InputB64: base64.StdEncoding.EncodeToString(raw),
			Format:   strings.TrimPrefix(filepath.Ext(ap), "."),
		})
	}

	outputs, err := handler(ctx, resolved.FFmpeg, resolved.FFprobe, req)
	if err != nil {
		return err
	}
	if len(outputs) == 0 {
		return fmt.Errorf("transform %q returned no output", name)
	}

	// Single-output transforms (every Tier 1 today) write to the
	// requested path. Multi-output transforms (future frame-extract,
	// say) get suffix indexes appended.
	for i, out := range outputs {
		dest := outputPath
		if dest == "" {
			dest = derivedOutputPath(inputPath, name, out.Format)
		}
		if i > 0 {
			ext := filepath.Ext(dest)
			dest = strings.TrimSuffix(dest, ext) + fmt.Sprintf("_%d", i) + ext
		}
		if out.MediaB64 == "" {
			return fmt.Errorf("transform %q output[%d] has no media bytes", name, i)
		}
		raw, err := base64.StdEncoding.DecodeString(out.MediaB64)
		if err != nil {
			return fmt.Errorf("decode output bytes: %w", err)
		}
		if err := os.WriteFile(dest, raw, 0o644); err != nil {
			return fmt.Errorf("write output %s: %w", dest, err)
		}
		fmt.Fprintf(os.Stderr, "  ✓ %s (%d bytes, %d ms)\n", dest, len(raw), out.ExecMS)
	}
	return nil
}

// derivedOutputPath builds <stem>_<verb>.<ext> alongside the input
// when --output is omitted. Mirrors what `iosuite super-resolution`
// does for real-esrgan-serve outputs.
//
// Examples:
//   compress  photo.jpg            → photo_compressed.jpg
//   reframe   video.mp4            → video_reframed.mp4
//   convert   photo.jpg (to=webp)  → photo.webp   (extension changes)
//   resize    photo.png            → photo_resized.png
func derivedOutputPath(input, verb, outputFormat string) string {
	dir := filepath.Dir(input)
	base := filepath.Base(input)
	stem := strings.TrimSuffix(base, filepath.Ext(base))

	suffix := map[string]string{
		"compress":      "_compressed",
		"reframe":       "_reframed",
		"normalize":     "_normalized",
		"resize":        "_resized",
		"trim":          "_trimmed",
		"speed":         "_speed",
		"extract-audio": "_audio",
		"silence-remove": "_dehushed",
		"denoise":        "_denoised",
		"subtitle-burn":  "_subtitled",
		"watermark":      "_watermarked",
		"color-lut":      "_graded",
	}[verb]
	if suffix == "" {
		suffix = "_" + verb
	}

	// `convert` is special — the whole point is the new extension.
	if verb == "convert" {
		return filepath.Join(dir, stem+"."+outputFormat)
	}

	ext := outputFormat
	if ext == "" {
		ext = strings.TrimPrefix(filepath.Ext(base), ".")
	}
	return filepath.Join(dir, stem+suffix+"."+ext)
}
