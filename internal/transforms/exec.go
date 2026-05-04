package transforms

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// stagedJob holds the on-disk paths every transform needs: the
// per-job tmpdir, the staged input file, and a planned output path.
// `cleanup` removes the tmpdir after the handler returns.
type stagedJob struct {
	tmpDir   string
	inPath   string
	outPath  string
	cleanup  func()
}

// stageInput decodes the request's input_b64 + writes it to a fresh
// tmpdir. inputExt is the suffix to give the staged file (e.g. ".mp4")
// — usually the request's media.format. Some ffmpeg encoders sniff
// by extension, others by content; staging with the right extension
// is cheap insurance.
//
// outputExt is the planned output's extension (e.g. ".mp4", ".jpg").
// The caller may pass an empty string and rename later.
//
// Caller MUST defer job.cleanup().
func stageInput(req Request, inputExt, outputExt string) (*stagedJob, error) {
	raw, err := decodeMediaBytes(req)
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp("", "ffmpeg-serve-")
	if err != nil {
		return nil, fmt.Errorf("tmpdir: %w", err)
	}
	inPath := filepath.Join(tmpDir, "input"+normalizeExt(inputExt))
	outPath := filepath.Join(tmpDir, "output"+normalizeExt(outputExt))
	if err := os.WriteFile(inPath, raw, 0o644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("write input: %w", err)
	}
	return &stagedJob{
		tmpDir:  tmpDir,
		inPath:  inPath,
		outPath: outPath,
		cleanup: func() { _ = os.RemoveAll(tmpDir) },
	}, nil
}

// stageAux writes each Aux entry into the same tmpdir as the
// primary input, returning the on-disk paths in the same order.
// Callers reference these paths in the ffmpeg arg list (e.g.,
// `-i <auxPath>` for an overlay image, or `-vf
// "subtitles=<auxPath>"` for a hardcoded subtitle file).
//
// Each aux entry's Format determines the file extension; missing
// formats default to ".bin" since ffmpeg sniffs by content for
// most cases. Caller MUST have already created `job` via
// stageInput so the tmpdir exists.
func stageAux(job *stagedJob, aux []Media) ([]string, error) {
	if len(aux) == 0 {
		return nil, nil
	}
	paths := make([]string, len(aux))
	for i, m := range aux {
		raw, err := decodeAuxBytes(m)
		if err != nil {
			return nil, fmt.Errorf("aux[%d]: %w", i, err)
		}
		ext := normalizeExt(m.Format)
		if ext == "" {
			ext = ".bin"
		}
		path := filepath.Join(job.tmpDir, fmt.Sprintf("aux_%d%s", i, ext))
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return nil, fmt.Errorf("write aux[%d]: %w", i, err)
		}
		paths[i] = path
	}
	return paths, nil
}

// decodeAuxBytes mirrors decodeMediaBytes for the aux slice — only
// input_b64 supported, data-URL prefix tolerated.
func decodeAuxBytes(m Media) ([]byte, error) {
	if m.InputB64 == "" {
		if m.InputURL != "" || m.InputPath != "" {
			return nil, errors.New("aux input_url and input_path are not supported yet — use input_b64")
		}
		return nil, errors.New("aux input_b64 is required")
	}
	s := m.InputB64
	if i := strings.Index(s, ","); i >= 0 && strings.HasPrefix(s, "data:") {
		s = s[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode input_b64: %w", err)
	}
	return raw, nil
}

// runFFmpeg executes ffmpeg with the given args, returning stderr on
// failure (which is where ffmpeg writes its actual error messages).
// stdout is captured but discarded — we read the encoded output via
// the staged tmp file, not via pipes, to avoid the base64 round-trip
// overhead that pipes would add.
func runFFmpeg(ctx context.Context, ffmpegBin string, args []string) error {
	cmd := exec.CommandContext(ctx, ffmpegBin, args...)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Tail the last few stderr lines — ffmpeg's most recent
		// output is almost always the actual cause; the prelude is
		// version banners.
		tail := tailLines(stderr.String(), 6)
		return fmt.Errorf("ffmpeg %s: %w\n%s",
			strings.Join(redactPaths(args), " "), err, tail)
	}
	return nil
}

// readOutput slurps the staged output file and base64-encodes it.
// Returns an error if the output is missing OR zero-length — both
// usually mean ffmpeg silently produced nothing despite a 0 exit.
func readOutput(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read output %s: %w", path, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("ffmpeg produced empty output at %s", path)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// timeIt is a tiny wrapper that records the wall-clock duration of
// the inner func + returns it as ms (matches real-esrgan-serve's
// exec_ms field convention).
func timeIt(fn func() error) (int, error) {
	start := time.Now()
	err := fn()
	return int(time.Since(start).Milliseconds()), err
}

// decodeMediaBytes pulls the input bytes out of a Request. Only
// input_b64 is supported in Phase B; input_url / input_path are
// reserved for future fetch / mount flows (real-esrgan-serve has
// the same shape and the same single-source-today behaviour).
func decodeMediaBytes(req Request) ([]byte, error) {
	if req.Media.InputB64 == "" {
		if req.Media.InputURL != "" || req.Media.InputPath != "" {
			return nil, errors.New("media.input_url and media.input_path are not supported yet — use media.input_b64")
		}
		return nil, errors.New("media.input_b64 is required")
	}
	// Strip an optional `data:image/png;base64,` prefix so callers
	// that already have a data-URL don't have to slice it themselves.
	s := req.Media.InputB64
	if i := strings.Index(s, ","); i >= 0 && strings.HasPrefix(s, "data:") {
		s = s[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode media.input_b64: %w", err)
	}
	return raw, nil
}

// normalizeExt accepts ".mp4" / "mp4" / "" and returns ".mp4" / ""
// so callers don't have to remember which form they have.
func normalizeExt(ext string) string {
	if ext == "" {
		return ""
	}
	if ext[0] != '.' {
		return "." + ext
	}
	return ext
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// redactPaths shortens `/tmp/ffmpeg-serve-XXXX/input.mp4` etc. in
// error messages so the user-visible error doesn't leak the tmpdir
// machinery. Pure cosmetic.
func redactPaths(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.Contains(a, "/ffmpeg-serve-") {
			out[i] = filepath.Base(a)
		} else {
			out[i] = a
		}
	}
	return out
}
