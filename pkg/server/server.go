package server

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"ffmpeg-serve/pkg/ffmpeg"
)

// Server handles HTTP requests for FFmpeg operations.
type Server struct {
	port   int
	ffmpeg *ffmpeg.FFmpeg
	gpuID  int
}

// NewServer initializes a new server.
func NewServer(port int, gpuID int) (*Server, error) {
	ff, err := ffmpeg.NewFFmpeg(gpuID)
	if err != nil {
		return nil, err
	}

	return &Server{
		port:   port,
		ffmpeg: ff,
		gpuID:  gpuID,
	}, nil
}

// Start launches the HTTP server and blocks.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/process", s.handleProcess)
	mux.HandleFunc("/probe", s.handleProbe)
	mux.HandleFunc("/health", s.handleHealth)

	addr := fmt.Sprintf(":%d", s.port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	log.Printf("FFmpeg Serve REST API listening on http://localhost%s", addr)
	return srv.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file from form. Key must be 'file'", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// For probe, we save to a temporary file
	tmpFile, err := osCreateTempFile(file)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer tmpFile.Cleanup()

	output, err := s.ffmpeg.Probe(r.Context(), tmpFile.Path)
	if err != nil {
		log.Printf("Probe failed: %v", err)
		http.Error(w, "Probe failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(output)
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to read file from form. Key must be 'file'", http.StatusBadRequest)
		return
	}
	defer file.Close()

	argsStr := r.URL.Query().Get("args")
	var args []string
	if argsStr != "" {
		// Handlers like runpod-ffmpeg send args separated by commas.
		// Example: -c:v,libx264,-preset,fast
		args = strings.Split(argsStr, ",")
	}

	// Read input file
	inputBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Failed to read input data", http.StatusInternalServerError)
		return
	}

	outputBytes, err := s.ffmpeg.ProcessBuffer(r.Context(), inputBytes, args)
	if err != nil {
		log.Printf("Process failed: %v", err)
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	// Guess content type or just use application/octet-stream
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(outputBytes)
}

// Helpers

type tmpFile struct {
	Path string
}

func (t *tmpFile) Cleanup() {
	os.Remove(t.Path)
}

func osCreateTempFile(r io.Reader) (*tmpFile, error) {
	f, err := os.CreateTemp("", "ffmpeg_probe_*")
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	f.Close()
	return &tmpFile{Path: f.Name()}, nil
}
