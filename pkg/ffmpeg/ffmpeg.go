package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FFmpeg handles FFmpeg operations.
type FFmpeg struct {
	binaryPath string
	probePath  string
	gpuID      int
}

// NewFFmpeg initializes the FFmpeg wrapper.
func NewFFmpeg(gpuID int) (*FFmpeg, error) {
	// 1. Try system PATH
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		// 2. Try local directory (where this binary is)
		if exe, err2 := os.Executable(); err2 == nil {
			localPath := filepath.Join(filepath.Dir(exe), "ffmpeg")
			if runtime.GOOS == "windows" {
				localPath += ".exe"
			}
			if _, err3 := os.Stat(localPath); err3 == nil {
				ffmpegPath = localPath
			}
		}
	}

	if ffmpegPath == "" {
		return nil, fmt.Errorf("ffmpeg not found in PATH or local directory")
	}

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		// 2. Try local directory
		if exe, err2 := os.Executable(); err2 == nil {
			localPath := filepath.Join(filepath.Dir(exe), "ffprobe")
			if runtime.GOOS == "windows" {
				localPath += ".exe"
			}
			if _, err3 := os.Stat(localPath); err3 == nil {
				ffprobePath = localPath
			}
		}
	}

	if ffprobePath == "" {
		return nil, fmt.Errorf("ffprobe not found in PATH or local directory")
	}

	return &FFmpeg{
		binaryPath: ffmpegPath,
		probePath:  ffprobePath,
		gpuID:      gpuID,
	}, nil
}

// Process executes an FFmpeg command with the given arguments.
func (f *FFmpeg) Process(ctx context.Context, inputPath string, outputPath string, args []string) error {
	fullArgs := []string{"-y"}

	if f.gpuID >= 0 {
		fullArgs = append(fullArgs, "-hwaccel", "cuda", "-hwaccel_device", fmt.Sprintf("%d", f.gpuID))
	}

	fullArgs = append(fullArgs, "-i", inputPath)
	fullArgs = append(fullArgs, args...)
	fullArgs = append(fullArgs, outputPath)

	cmd := exec.CommandContext(ctx, f.binaryPath, fullArgs...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg error: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// Probe executes ffprobe on the given input file and returns the JSON output.
func (f *FFmpeg) Probe(ctx context.Context, inputPath string) ([]byte, error) {
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		inputPath,
	}

	cmd := exec.CommandContext(ctx, f.probePath, args...)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe error: %w", err)
	}

	return output, nil
}

// ProcessBuffer executes an FFmpeg command using stdin/stdout.
func (f *FFmpeg) ProcessBuffer(ctx context.Context, input []byte, args []string) ([]byte, error) {
	// Create temporary files for input and output since FFmpeg pipes can be tricky with some codecs
	tmpIn, err := os.CreateTemp("", "ffmpeg_in_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpIn.Name())
	defer tmpIn.Close()

	if _, err := tmpIn.Write(input); err != nil {
		return nil, err
	}

	tmpOut, err := os.CreateTemp("", "ffmpeg_out_*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpOut.Name())
	defer tmpOut.Close()

	if err := f.Process(ctx, tmpIn.Name(), tmpOut.Name(), args); err != nil {
		return nil, err
	}

	return os.ReadFile(tmpOut.Name())
}
