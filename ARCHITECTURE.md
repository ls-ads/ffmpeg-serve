# Architecture

ffmpeg-serve is a thin Go orchestrator that subprocesses the system
`ffmpeg` + `ffprobe` binaries, exposes a small JSON-envelope HTTP
API, and is opaquely chained behind iosuite serve. It mirrors
real-esrgan-serve's shape so the same iosuite-side code paths
(deploy, benchmark, registry) work for both.

```
iosuite-api ─►  iosuite serve  ─►  ffmpeg-serve  ─►  ffmpeg
                (JSON envelope,     (this repo:        (LGPL build,
                 opaque)             dispatch by mime,  NVENC for HW
                                     subprocess ffmpeg) encode)
```

## The wire

```
POST /runsync   Content-Type: application/json
{"input": {
  "transform": "<verb>",
  "params":    {...},
  "media":     {"input_b64": "...", "format": "mp4"}
}}

→ {"status": "COMPLETED",
   "output": {"outputs": [
     {"media_b64": "...", "format": "mp4", "exec_ms": 2840}
   ]}}
```

iosuite serve doesn't interpret `input.*` or `output.*` — it just
forwards bytes. `transform` is the only field this server requires.
Per-transform `params` schemas are owned by the transform's Go
handler and validated there.

## Smart commands, mime-aware dispatch

The user-facing CLI (`iosuite`) speaks one verb per transform —
`iosuite compress photo.jpg`, `iosuite compress video.mp4`,
`iosuite compress podcast.mp3`. The CLI sniffs the input mime, posts
the same `transform: "compress"` to the daemon, and the daemon picks
the right internal handler by ffprobing the bytes.

Verbs that can't apply to a given media type
(`iosuite trim photo.jpg`) error out at the CLI before any network
work. The daemon also rejects with a clear message if a request
slips through.

## License boundary

The Apache-2.0 boundary is enforced at the **process boundary**:
this Go binary spawns `ffmpeg` via `exec.Command`. We do not
statically or dynamically link FFmpeg's C libraries — FFmpeg's LGPL
does not propagate into our source distribution.

For the runtime container images: we build LGPL-only FFmpeg (no
`--enable-gpl`) and use NVIDIA NVENC for hardware encoding. NVENC's
FFmpeg binding is LGPL; NVENC's SDK is licensed under NVIDIA's EULA.
The combined binary distribution stays Apache-2.0-clean.

Operators who need GPL-only encoders (libx264, vidstab, etc.) can
rebuild the worker image with `--enable-gpl`. The resulting binary
distribution is GPL; the source code in this repository remains
Apache-2.0. We do not ship that variant.

See [`NOTICE.md`](./NOTICE.md) for the third-party attribution
catalogue.

## Concurrency

The daemon's `--concurrency` flag caps in-flight transforms; further
requests block at a Go channel until a slot frees. CPU-only flavours
should size to `nproc`; the CUDA flavour to the number of NVENC
sessions a card supports (1-3 on consumer cards, more on Quadro/Pro).
The defaulted `1` is conservative — bump it explicitly per host.

## How transforms get added

Each transform is a single file under `internal/transforms/<name>.go`
with a Go handler:

```go
func init() { Register("compress", compress) }

func compress(ctx context.Context, ffmpegBin, ffprobeBin string, req Request) ([]Output, error) {
    // 1. ffprobe req.Media.InputB64 → mime
    // 2. Build ffmpeg command line per (mime, params)
    // 3. exec, capture stdout (or staged tmp file) → []Output
}
```

A transform's params schema, internal mime-dispatch, and ffmpeg
invocation all stay inside its single file. Adding `convert` or
`trim` or `reframe` doesn't touch the registry or the server — just
drop a file and add a test.
