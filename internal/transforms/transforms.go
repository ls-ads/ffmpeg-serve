// Package transforms is the dispatch layer between the JSON envelope
// and the media-specific ffmpeg invocations.
//
// Wire shape (mirrors the real-esrgan-serve / iosuite serve contract):
//
//	POST /runsync
//	{"input": {
//	    "transform": "compress",      // verb name; daemon dispatches by mime
//	    "params":    {...},           // transform-specific knobs
//	    "media":     {"input_b64": "...", "format": "mp4"}
//	}}
//
//	→ {"status": "COMPLETED",
//	   "output": {"outputs": [
//	       {"media_b64": "...", "format": "mp4", "exec_ms": 2840}
//	   ]}}
//
// Each transform registers a `Handler`. The dispatcher detects the
// input mime via ffprobe (later phase) and routes to the right handler.
//
// Phase A ships only `noop` so the wire is verifiable end-to-end.
// Real transforms land in Phase B onwards.
package transforms

import (
	"context"
	"fmt"
)

// Request is the parsed input envelope (already JSON-decoded by the
// HTTP layer). Handlers receive a Request and return one Output per
// produced artifact (single-item slice for most transforms; batch
// transforms can return multiple).
//
// Aux carries optional secondary inputs — used by transforms that
// need a second media file alongside the primary one. Examples:
// subtitle-burn (the .srt/.vtt file), watermark (the overlay
// image), color-lut (the .cube file). Handlers ignore the slice
// they don't need and the wire stays the same shape across every
// verb.
type Request struct {
	Transform string         `json:"transform"`
	Params    map[string]any `json:"params,omitempty"`
	Media     Media          `json:"media"`
	Aux       []Media        `json:"aux,omitempty"`
}

// Media carries the input bytes + a format hint. `input_b64` is the
// canonical ingestion path (matches real-esrgan-serve's image_base64
// field). `input_url` and `input_path` are reserved for future
// fetch/mount flows.
type Media struct {
	InputB64    string `json:"input_b64,omitempty"`
	InputURL    string `json:"input_url,omitempty"`
	InputPath   string `json:"input_path,omitempty"`
	Format      string `json:"format,omitempty"`
}

// Output is one produced artefact. Most transforms return one; some
// (e.g., frame extraction) return many.
type Output struct {
	MediaB64 string `json:"media_b64,omitempty"`
	Format   string `json:"format,omitempty"`
	ExecMS   int    `json:"exec_ms,omitempty"`
}

// Handler runs a single transform. Receives the parsed request +
// already-located ffmpeg/ffprobe paths; returns one or more outputs.
type Handler func(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error)

// registry holds every transform name → handler mapping.
var registry = map[string]Handler{}

// Register associates a transform name with its handler. Called from
// init() in each transform's source file.
func Register(name string, h Handler) {
	if _, dup := registry[name]; dup {
		panic("transforms: duplicate registration: " + name)
	}
	registry[name] = h
}

// Lookup returns the handler for a given transform name, or
// (nil, false) if unknown. Callers should treat unknown as a 400.
func Lookup(name string) (Handler, bool) {
	h, ok := registry[name]
	return h, ok
}

// Names returns every registered transform, sorted alphabetically.
// Used by `ffmpeg-serve list-transforms` (future) and by error
// messages so callers see the closed set.
func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	// In Go 1.21+ this could use slices.Sort; stdlib sort is fine.
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j-1] > out[j] {
			out[j-1], out[j] = out[j], out[j-1]
			j--
		}
	}
	return out
}

// ErrUnknownTransform is returned when the request names a transform
// the daemon doesn't implement. Wraps the bad name + the closed set
// of known names so the caller can recover without re-reading docs.
type ErrUnknownTransform struct {
	Name string
}

func (e *ErrUnknownTransform) Error() string {
	return fmt.Sprintf("unknown transform %q. Known: %v", e.Name, Names())
}
