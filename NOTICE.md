# NOTICE

ffmpeg-serve  
Copyright 2026 ls-ads / Andrew Damon-Smith.

This product is licensed under the Apache License, Version 2.0 (see
[`LICENSE`](./LICENSE)). It redistributes and depends on third-party
software listed below. All attributions, license obligations, and
redistribution rules are catalogued here. When forking or vendoring
this repository, preserve `LICENSE`, `NOTICE.md`, and (when present)
the `third-party-licenses/` directory.

---

## License compliance posture

ffmpeg-serve invokes the FFmpeg binary as a subprocess. The Apache-2.0
boundary holds at the process boundary — we do **not** statically or
dynamically link FFmpeg's C libraries into our Go binary, so FFmpeg's
LGPL / GPL licenses do not propagate into our source distribution.

To keep the **runtime container images** clean for redistribution
under Apache-2.0:

- We build LGPL-only FFmpeg. We do **not** pass `--enable-gpl`,
  `--enable-nonfree`, or `--enable-version3`.
- We use NVIDIA NVENC (under the NVIDIA Software Development Kit
  EULA) for hardware H.264 / H.265 / AV1 encoding. NVENC's binding
  in FFmpeg is LGPL.
- Software fallback codecs are libvpx (BSD) for VP9 and libaom (BSD)
  for AV1. We do not redistribute libx264, libx265, libxvid,
  libfdk-aac, or vidstab — all of which would require GPL or
  non-free relicensing of the binary distribution.

If a future operator needs encoders we don't ship (e.g., libx264 for
strict H.264 compliance with non-NVENC hardware), they may rebuild
the worker image with `--enable-gpl`. The resulting binary
distribution is GPL-licensed; the source code in this repository
remains Apache-2.0. We do not ship that variant.

---

## Third-party software

### FFmpeg

- **Project:** <https://ffmpeg.org/>
- **License:** LGPL 2.1 or later (we do not enable GPL components).
- **Where used:** invoked as a subprocess by `internal/transforms/*`
  via the path resolved in `internal/runtime/locator.go`.
- **Redistribution:** the LGPL FFmpeg binary is included in the
  release Docker images under `/usr/bin/ffmpeg`. The corresponding
  source is the upstream FFmpeg repository at the version tag pinned
  in our Dockerfiles; we provide a written offer in this NOTICE to
  supply the source for any binary release on request.
- **Attribution requirement satisfied:** by this NOTICE entry plus
  the `--version` output baked into the binary.

### libvpx (VP9 encoder)

- **Project:** <https://www.webmproject.org/code/>
- **License:** BSD 3-Clause.
- **Where used:** linked into the FFmpeg binary at build time;
  enabled with `--enable-libvpx`.

### libaom (AV1 encoder)

- **Project:** <https://aomedia.googlesource.com/aom/>
- **License:** BSD 2-Clause + AOM Patent License 1.0.
- **Where used:** linked into the FFmpeg binary at build time;
  enabled with `--enable-libaom`.

### libwebp (WebP image codec)

- **Project:** <https://chromium.googlesource.com/webm/libwebp/>
- **License:** BSD 3-Clause.
- **Where used:** linked into the FFmpeg binary; enabled with
  `--enable-libwebp`.

### NVIDIA Video Codec SDK (NVENC / NVDEC) — CUDA build only

- **Project:** <https://developer.nvidia.com/video-codec-sdk>
- **License:** NVIDIA Software Development Kit EULA.
- **Where used:** linked dynamically by the FFmpeg binary in the
  `Dockerfile.cuda` flavour for hardware H.264 / H.265 / AV1 encoding
  and decoding.
- **Redistribution:** the NVIDIA EULA permits redistribution of the
  driver libraries inside containers. We do not ship the NVENC SDK
  headers in the runtime image; the FFmpeg build links against
  `libnvidia-encode.so` / `libnvidia-decode.so` provided by the host's
  NVIDIA driver at runtime.

### NVIDIA CUDA Runtime — CUDA build only

- **Project:** <https://developer.nvidia.com/cuda-zone>
- **License:** NVIDIA CUDA Toolkit EULA.
- **Where used:** the `Dockerfile.cuda` base image
  (`nvidia/cuda:12.x-runtime-ubuntu22.04`) carries the CUDA runtime
  libraries.

### Go modules

- **github.com/spf13/cobra** — Apache-2.0
- **github.com/spf13/pflag** — BSD 3-Clause
- **github.com/inconshreveable/mousetrap** — Apache-2.0

Pinned versions are in `go.mod` / `go.sum`. Each module's full text
licence is included in its source tree at the pinned version.

---

## Source-availability offer for LGPL components

For any binary release of ffmpeg-serve (Docker image, GitHub Releases
asset) that includes LGPL-licensed components, we will, on written
request and at no charge beyond reasonable shipping costs, provide a
machine-readable copy of the corresponding source code as required
by Section 6 of the GNU Lesser General Public License. Requests:
<https://github.com/ls-ads/ffmpeg-serve/issues>.
