// Package runtime locates the ffmpeg + ffprobe binaries this daemon
// shells out to.
//
// Lookup order (per binary):
//  1. Explicit path passed by the caller.
//  2. $FFMPEG_BIN / $FFPROBE_BIN env var.
//  3. ffmpeg / ffprobe on $PATH.
//  4. <ffmpeg-serve-binary-dir>/ffmpeg | /ffprobe (release tarball that
//     ships them side-by-side; not yet shipping but reserve the slot).
//
// Errors carry the candidate list so the operator can see exactly
// where we tried to look.
package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Resolved is the result of a successful locate. Both binaries are
// absolute paths verified to be executable.
type Resolved struct {
	FFmpeg  string
	FFprobe string
}

// Locate finds ffmpeg + ffprobe per the order documented in the
// package doc. ffmpegOverride / ffprobeOverride are optional explicit
// paths from the caller (e.g., a flag).
func Locate(ffmpegOverride, ffprobeOverride string) (Resolved, error) {
	ffmpeg, err := findBinary("ffmpeg", "FFMPEG_BIN", ffmpegOverride)
	if err != nil {
		return Resolved{}, err
	}
	ffprobe, err := findBinary("ffprobe", "FFPROBE_BIN", ffprobeOverride)
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{FFmpeg: ffmpeg, FFprobe: ffprobe}, nil
}

func findBinary(name, envVar, override string) (string, error) {
	candidates := []string{}
	if override != "" {
		candidates = append(candidates, override)
	}
	if env := os.Getenv(envVar); env != "" {
		candidates = append(candidates, env)
	}
	if path, err := exec.LookPath(name); err == nil {
		candidates = append(candidates, path)
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}

	for _, c := range candidates {
		if c == "" {
			continue
		}
		if abs, err := exec.LookPath(c); err == nil {
			return abs, nil
		}
	}
	return "", fmt.Errorf(
		"%s not found. Looked at: %v.\n"+
			"Install ffmpeg + ffprobe via your package manager,\n"+
			"or set $%s to the absolute path.",
		name, candidates, envVar,
	)
}
