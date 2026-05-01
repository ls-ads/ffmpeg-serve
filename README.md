# ffmpeg-serve

FFmpeg-backed transform daemon for the [iosuite](https://github.com/ls-ads/iosuite)
ecosystem. Runs image / video / audio transforms locally (Go binary
subprocesses the system `ffmpeg` + `ffprobe`) or as a RunPod
serverless template that the iosuite CLI deploys + tears down on
demand.

The user-facing CLI is `iosuite`. Reach for this repo if you want to:

- Self-host transforms on your own GPU box, with or without iosuite.
- Add a new transform (single Go file + a tiny manifest entry).
- Build an alternative runtime image flavour.

## License posture

Apache-2.0. The runtime image distribution stays Apache-2.0-clean by
**building LGPL-only FFmpeg** (no `--enable-gpl`, no
`--enable-nonfree`) and using **NVIDIA NVENC** for hardware video
encoding — NVENC's FFmpeg binding is LGPL, NVENC itself is licensed
under NVIDIA's SDK EULA.

What we don't ship: libx264, libx265, libxvid, libfdk-aac, vidstab.
What we do: NVENC (H.264/H.265/AV1 hardware), libvpx (VP9, BSD),
libaom (AV1, BSD), libwebp (BSD), the LGPL FFmpeg native codecs.

Full third-party catalogue: [`NOTICE.md`](./NOTICE.md).

## Quick start (local)

Requirements: `ffmpeg` + `ffprobe` on `$PATH` (Linux: `apt install
ffmpeg` or equivalent; macOS: `brew install ffmpeg`). Go 1.25+ if
building from source.

```bash
git clone https://github.com/ls-ads/ffmpeg-serve
cd ffmpeg-serve
make build               # → ./bin/ffmpeg-serve

# Run as a daemon. Picks up ffmpeg/ffprobe from $PATH.
./bin/ffmpeg-serve serve --bind 0.0.0.0 --port 8313
```

## Quick start (Docker)

```bash
make docker-cpu           # ~340 MB, no GPU dep
make docker-cuda          # ~1.1 GB, NVIDIA + NVENC hardware encode

# Run the cuda flavour (requires --gpus all + nvidia-container-toolkit):
docker run --rm --gpus all -p 8313:8313 ffmpeg-serve:cuda-dev
```

## Wire shape

The daemon speaks the same JSON envelope as iosuite serve and
real-esrgan-serve. iosuite serve is opaque pass-through, so this
envelope flows from iosuite-api straight to here:

```
POST /runsync   Content-Type: application/json
{"input": {
  "transform": "compress",                  # verb name
  "params":    {"target": "discord-10mb"},  # transform-specific
  "media":     {"input_b64": "...", "format": "mp4"}
}}

→ {"status": "COMPLETED",
   "output": {"outputs": [
     {"media_b64": "...", "format": "mp4", "exec_ms": 2840}
   ]}}
```

`GET /health` returns `{"status":"ok"}` once `ffmpeg` + `ffprobe`
are resolved.

## Transforms

The wire is one verb per transform; the daemon dispatches by mime
internally (image / video / audio handler picked from `ffprobe`
output). Verbs that don't apply to a given media type return a clear
error before any work runs.

Phase A (this commit) ships only `noop` — proves the wire end-to-end
without any actual ffmpeg work. Real transforms land in subsequent
phases:

**Tier 1 (next):** `compress`, `reframe`, `convert` (covers gif↔mp4
and every other format-conversion case), `normalize` (audio LUFS),
`upscale --method=lanczos`.

**Tier 2:** `trim`, `speed`, `extract-audio`, `silence-remove`.

**Tier 3 (later):** `subtitle-burn`, `watermark` (add-only),
`color-lut`, `denoise`.

## Deploy manifests

`deploy/runpod.json` is the source-of-truth for how iosuite deploys
this module to RunPod: image tag, container disk, GPU pool map per
class, FlashBoot default, CUDA pin, env vars. iosuite reads it at
deploy time — bumping any of those fields lands here, not in iosuite.

Field reference: [`deploy/SCHEMA.md`](./deploy/SCHEMA.md).
Validator (CI gate): [`build/validate_manifest.py`](./build/validate_manifest.py).

## Repo layout

```
ffmpeg-serve/
├── ARCHITECTURE.md           design rationale and contracts
├── cmd/ffmpeg-serve/         Go CLI entry point
├── internal/
│   ├── server/               HTTP daemon (POST /runsync)
│   ├── transforms/           one Go file per transform handler
│   └── runtime/              ffmpeg + ffprobe locator
├── deploy/                   iosuite-readable deploy manifest
├── build/                    manifest validator (CI gate)
├── Dockerfile.cpu            ubuntu + LGPL FFmpeg
├── Dockerfile.cuda           ubuntu + LGPL FFmpeg + NVENC
└── tests/
```

## Documentation

- iosuite CLI reference (the user-facing surface for these
  transforms): <https://iosuite.io/cli-docs>
- Architecture: [`ARCHITECTURE.md`](./ARCHITECTURE.md)
- Manifest schema: [`deploy/SCHEMA.md`](./deploy/SCHEMA.md)

## License

Apache-2.0. See [`LICENSE`](./LICENSE) for the text and
[`NOTICE.md`](./NOTICE.md) for third-party attributions
(FFmpeg LGPL, NVIDIA NVENC + CUDA EULAs, libvpx / libaom / libwebp
BSD, et al). When forking or vendoring, preserve `LICENSE` and
`NOTICE.md`.
