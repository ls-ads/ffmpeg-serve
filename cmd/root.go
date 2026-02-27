package cmd

import (
	"ffmpeg-serve/pkg/ffmpeg"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	inputPath  string
	outputPath string
	gpuID      int
)

var rootCmd = &cobra.Command{
	Use:   "ffmpeg-serve",
	Short: "FFmpeg Serve is a CLI and HTTP server for media processing",
	Long:  `A standalone Go CLI tool that wraps FFmpeg for local and remote media processing.`,
	Run: func(cmd *cobra.Command, args []string) {
		if inputPath == "" {
			cmd.Help()
			return
		}

		if outputPath == "" {
			outputPath = inputPath + "_out"
			fmt.Printf("No output specified, using: %s\n", outputPath)
		}

		ff, err := ffmpeg.NewFFmpeg(gpuID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error initializing FFmpeg: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Processing input: %s -> %s\n", inputPath, outputPath)

		// Get additional args from trailing args after --
		extraArgs := cmd.Flags().Args()

		if err := ff.Process(cmd.Context(), inputPath, outputPath, extraArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error processing file: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Processing completed successfully.")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&inputPath, "input", "i", "", "Input file path or directory (optional for server mode)")
	rootCmd.PersistentFlags().StringVarP(&outputPath, "output", "o", "", "Output file path or directory")
	rootCmd.PersistentFlags().IntVarP(&gpuID, "gpu-id", "g", -1, "GPU device ID to use for hardware acceleration (-1 to disable)")
}
