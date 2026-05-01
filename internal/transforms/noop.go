// Phase-A "noop" transform — echoes the input back unchanged.
//
// Purpose: prove the wire end-to-end (iosuite-serve → ffmpeg-serve →
// reply) without depending on ffmpeg actually running. The first real
// transforms land in Phase B; until then `noop` is what the
// integration smoke test exercises.
//
// Wire:
//   POST /runsync
//   {"input": {"transform": "noop", "media": {"input_b64": "..."}}}
//   → {"status": "COMPLETED", "output": {"outputs": [{"media_b64": "...", "exec_ms": 0}]}}
package transforms

import (
	"context"
	"errors"
)

func init() {
	Register("noop", noop)
}

func noop(_ context.Context, _, _ string, req Request) ([]Output, error) {
	if req.Media.InputB64 == "" {
		return nil, errors.New("noop: media.input_b64 is required")
	}
	return []Output{{
		MediaB64: req.Media.InputB64,
		Format:   req.Media.Format,
		ExecMS:   0,
	}}, nil
}
