package cmd

import (
	"context"
	"fmt"
	"os"

	"ffmpeg-serve/pkg/ffmpeg"

	"github.com/spf13/cobra"
)

var (
	gpuID int
)

var rootCmd = &cobra.Command{
	Use:                "ffmpeg-serve [flags] [-- ffmpeg_args...]",
	Short:              "FFmpeg Serve is a CLI and HTTP server for media processing",
	Long:               `A standalone Go CLI tool that wraps FFmpeg for local and remote media processing.`,
	DisableFlagParsing: true,
	Run: func(cmd *cobra.Command, args []string) {
		// Get all args manually parsing out -g/--gpu-id if present
		// Since we disabled parsing to ensure ffmpeg args pass through transparently
		gpuID := -1
		var extraArgs []string

		for i := 0; i < len(args); i++ {
			if (args[i] == "-g" || args[i] == "--gpu-id") && i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &gpuID)
				i++ // skip value
			} else if args[i] == "--" {
				// skip the separator
			} else {
				extraArgs = append(extraArgs, args[i])
			}
		}

		ff, err := ffmpeg.NewFFmpeg(gpuID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing FFmpeg: %v\n", err)
			os.Exit(1)
		}

		if len(extraArgs) == 0 {
			fmt.Println("No ffmpeg arguments provided.")
			cmd.Help()
			return
		}

		if err := ff.Process(cmd.Context(), extraArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Processing completed successfully.")
	},
}

func Execute() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "server" || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if err := rootCmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// Bypass Cobra for raw ffmpeg execution to prevent command parsing errors like "unknown command 'error'"
	gpuID := -1
	var extraArgs []string

	for i := 0; i < len(args); i++ {
		if (args[i] == "-g" || args[i] == "--gpu-id") && i+1 < len(args) {
			fmt.Sscanf(args[i+1], "%d", &gpuID)
			i++ // skip value
		} else if args[i] == "--" {
			// skip the separator
		} else {
			extraArgs = append(extraArgs, args[i])
		}
	}

	ff, err := ffmpeg.NewFFmpeg(gpuID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing FFmpeg: %v\n", err)
		os.Exit(1)
	}

	if len(extraArgs) == 0 {
		fmt.Println("No ffmpeg arguments provided.")
		rootCmd.Help()
		os.Exit(1)
	}

	// For raw execution, we use the background context
	importPath := "context"
	_ = importPath // Keep track of imports

	// Create context
	ctx := context.Background()

	if err := ff.Process(ctx, extraArgs); err != nil {
		fmt.Fprintf(os.Stderr, "Error processing: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Flags are now parsed manually in Run due to DisableFlagParsing: true
	// We keep this here just for help text generation
	rootCmd.Flags().IntVarP(&gpuID, "gpu-id", "g", -1, "GPU device ID to use for hardware acceleration (-1 to disable)")
}
