// ffmpeg-serve — image / video / audio transformation CLI for the iosuite ecosystem.
//
// The user-facing CLI is `iosuite`; this binary is what `iosuite serve
// --provider local --tool ffmpeg` shells out to (and what RunPod runs
// when iosuite deploys an ffmpeg endpoint). It subprocesses the system
// `ffmpeg` + `ffprobe` binaries and exposes a small JSON-envelope HTTP
// API that mirrors the real-esrgan-serve wire shape.
//
// See ARCHITECTURE.md for the full design and deploy/SCHEMA.md for the
// transform-manifest reference.
package main

import (
	"fmt"
	"os"

	"github.com/ls-ads/ffmpeg-serve/internal/server"
	"github.com/ls-ads/ffmpeg-serve/internal/transforms"
	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "ffmpeg-serve",
		Short: "FFmpeg-backed transform daemon for the iosuite ecosystem",
		Long: `ffmpeg-serve runs media transforms (image / video / audio) by
subprocessing the system ffmpeg + ffprobe binaries and exposing a small
JSON-envelope HTTP API. The user-facing CLI is iosuite; this is the
worker iosuite shells out to. See ARCHITECTURE.md for the full design.`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(server.Command())
	root.AddCommand(transforms.Command())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
