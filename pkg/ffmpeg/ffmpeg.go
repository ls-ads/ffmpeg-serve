package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
)

// FFmpeg handles FFmpeg operations.
type FFmpeg struct {
	binaryPath string
	probePath  string
	gpuID      int
}

// NewFFmpeg initializes the FFmpeg wrapper.
func NewFFmpeg(gpuID int) (*FFmpeg, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found: %w", err)
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, fmt.Errorf("ffprobe not found: %w", err)
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
