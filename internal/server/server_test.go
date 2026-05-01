package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ls-ads/ffmpeg-serve/internal/runtime"
	"github.com/ls-ads/ffmpeg-serve/internal/transforms"
)

// newTestServer wires the production mux against a stub Server with
// no real ffmpeg binary — tests for the wire layer don't need it.
// The transforms registry is process-global, so `noop` is registered
// by its package init() and visible here.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := &Server{
		resolved: runtime.Resolved{FFmpeg: "/bin/true", FFprobe: "/bin/true"},
		gates:    make(chan struct{}, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", srv.handleHealth)
	mux.HandleFunc("/runsync", srv.handleRunSync)
	return httptest.NewServer(mux)
}

func TestHealth(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok"`) {
		t.Errorf("body = %s", body)
	}
}

func TestRunSync_Noop_RoundTripsBytes(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	const inputB64 = "aGVsbG8="
	body := []byte(`{"input": {"transform": "noop", "media": {"input_b64": "` + inputB64 + `"}}}`)

	resp, err := http.Post(ts.URL+"/runsync", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		got, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %s", resp.StatusCode, got)
	}

	var env struct {
		Status string `json:"status"`
		Output struct {
			Outputs []struct {
				MediaB64 string `json:"media_b64"`
			} `json:"outputs"`
		} `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatal(err)
	}
	if env.Status != "COMPLETED" {
		t.Errorf("status = %q", env.Status)
	}
	if len(env.Output.Outputs) != 1 || env.Output.Outputs[0].MediaB64 != inputB64 {
		t.Errorf("noop should echo input, got: %+v", env.Output.Outputs)
	}
}

func TestRunSync_RejectsUnknownTransform(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"input": {"transform": "imagined-transform", "media": {"input_b64": "x"}}}`
	resp, err := http.Post(ts.URL+"/runsync", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(got), "imagined-transform") {
		t.Errorf("error should name the bad transform: %s", got)
	}
	// And include the closed set so the caller can recover.
	if !strings.Contains(string(got), "noop") {
		t.Errorf("error should list known transforms: %s", got)
	}
}

func TestRunSync_RejectsMissingTransform(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	body := `{"input": {"media": {"input_b64": "x"}}}`
	resp, err := http.Post(ts.URL+"/runsync", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestRunSync_RejectsMethodOther(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/runsync")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

// Compile-time guard that the transforms registry has at least
// `noop` after package import. If this list grows beyond `noop` and
// nothing else in Phase A, that's a smell.
func TestTransformsRegistry_NoopRegistered(t *testing.T) {
	if _, ok := transforms.Lookup("noop"); !ok {
		t.Error("expected `noop` transform to be registered after package import")
	}
}

// Make `context` import survive `go vet`.
var _ = context.Background
