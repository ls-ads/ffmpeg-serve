// Package server is the HTTP daemon for ffmpeg-serve.
//
// Wire shape mirrors real-esrgan-serve's daemon — one POST endpoint
// (/runsync) accepting a JSON envelope, returning a JSON envelope.
// iosuite serve is opaque pass-through, so the same envelope flows
// from iosuite-api through the daemon and into this server unchanged.
//
//	POST /runsync     application/json
//	{"input": {"transform": "<name>", "params": {...}, "media": {...}}}
//
//	→ {"status": "COMPLETED", "output": {"outputs": [...]}}
//
// Errors return non-2xx with a JSON body so iosuite can branch on
// status code + structured error.
//
// GET /health returns {"status":"ok"} once ffmpeg + ffprobe are
// resolved.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ls-ads/ffmpeg-serve/internal/runtime"
	"github.com/ls-ads/ffmpeg-serve/internal/transforms"
	"github.com/spf13/cobra"
)

// Command returns the cobra command for `ffmpeg-serve serve`.
func Command() *cobra.Command {
	o := &opts{}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the long-lived HTTP transform daemon",
		Long: `Run the ffmpeg-backed transform daemon. Accepts the JSON envelope
shared with the rest of the iosuite ecosystem — see ARCHITECTURE.md
for the wire contract.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), o)
		},
	}
	cmd.Flags().StringVar(&o.bind, "bind", "127.0.0.1",
		"Bind address (use 0.0.0.0 to expose on LAN)")
	cmd.Flags().IntVar(&o.port, "port", 8313,
		"TCP port to bind. 8313 by convention so ffmpeg-serve coexists "+
			"with iosuite serve (8312) and real-esrgan-serve (8311).")
	cmd.Flags().IntVar(&o.concurrency, "concurrency", 1,
		"Max concurrent transforms; further requests block until a "+
			"slot frees. 1 is conservative — bump to match available CPU/GPU.")
	cmd.Flags().StringVar(&o.ffmpegBin, "ffmpeg", "",
		"Override path to ffmpeg (defaults to PATH lookup)")
	cmd.Flags().StringVar(&o.ffprobeBin, "ffprobe", "",
		"Override path to ffprobe (defaults to PATH lookup)")
	cmd.Flags().DurationVar(&o.shutdownTimeout, "shutdown-timeout", 30*time.Second,
		"Grace period for in-flight requests on SIGINT / SIGTERM.")
	return cmd
}

type opts struct {
	bind            string
	port            int
	concurrency     int
	ffmpegBin       string
	ffprobeBin      string
	shutdownTimeout time.Duration
}

// Server holds the runtime state — resolved binaries + the
// concurrency gate that throttles inbound requests.
type Server struct {
	resolved runtime.Resolved
	gates    chan struct{}
}

func run(ctx context.Context, o *opts) error {
	resolved, err := runtime.Locate(o.ffmpegBin, o.ffprobeBin)
	if err != nil {
		return err
	}

	srv := &Server{
		resolved: resolved,
		gates:    make(chan struct{}, o.concurrency),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", srv.handleHealth)
	mux.HandleFunc("/runsync", srv.handleRunSync)

	addr := fmt.Sprintf("%s:%d", o.bind, o.port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	signalCtx, cancel := signal.NotifyContext(ctx,
		syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-signalCtx.Done()
		fmt.Fprintln(os.Stderr, "ffmpeg-serve: shutting down…")
		shutCtx, shutCancel := context.WithTimeout(
			context.Background(), o.shutdownTimeout)
		defer shutCancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	fmt.Fprintf(os.Stderr,
		"ffmpeg-serve serving on http://%s (ffmpeg=%s, ffprobe=%s, concurrency=%d)\n",
		addr, resolved.FFmpeg, resolved.FFprobe, o.concurrency)

	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http: %w", err)
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// envelope mirrors the iosuite serve / real-esrgan-serve wire shape.
type envelope struct {
	Input transforms.Request `json:"input"`
}

type response struct {
	Status string         `json:"status"`
	Output responseOutput `json:"output"`
}

type responseOutput struct {
	Outputs []transforms.Output `json:"outputs"`
}

func (s *Server) handleRunSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	// 256 MB cap. Larger than real-esrgan-serve's 25 MB because video
	// payloads are an order of magnitude bigger; iosuite-side enforces
	// per-tier quotas before reaching us.
	const maxBody = 256 * 1024 * 1024
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)

	var env envelope
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&env); err != nil {
		http.Error(w, fmt.Sprintf("decode JSON: %v", err), http.StatusBadRequest)
		return
	}
	if env.Input.Transform == "" {
		http.Error(w, `request needs "input.transform" naming the verb`, http.StatusBadRequest)
		return
	}
	handler, ok := transforms.Lookup(env.Input.Transform)
	if !ok {
		writeError(w, http.StatusBadRequest, &transforms.ErrUnknownTransform{Name: env.Input.Transform})
		return
	}

	// Backpressure gate. Same pattern real-esrgan-serve uses.
	select {
	case s.gates <- struct{}{}:
	case <-r.Context().Done():
		return
	}
	defer func() { <-s.gates }()

	outputs, err := handler(r.Context(), s.resolved.FFmpeg, s.resolved.FFprobe, env.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response{
		Status: "COMPLETED",
		Output: responseOutput{Outputs: outputs},
	})
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "FAILED",
		"error":  err.Error(),
	})
}
