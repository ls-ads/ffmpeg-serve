# Deploy manifests

iosuite reads these manifests to drive `iosuite endpoint deploy
--tool ffmpeg` and `iosuite endpoint benchmark --tool ffmpeg`. The
shape mirrors real-esrgan-serve's manifests one-to-one — same
`schema_version`, same field names, same validator pattern. iosuite
holds no per-tool implementation knowledge; everything that varies
between tools lives here.

## `runpod.json`

The RunPod-serverless deploy manifest. Field reference:

| Field | Type | Required | Notes |
|---|---|---|---|
| `schema_version` | string | yes | `"1"` today. |
| `tool` | string | yes | Stable kebab-case name. Must match what iosuite users pass to `--tool` (`ffmpeg`). |
| `description` | string | no | One-liner shown in `iosuite endpoint list` output. |
| `image` | string | yes | Full image ref including tag. iosuite passes this verbatim to RunPod. Bumped in lockstep with this manifest's git tag. |
| `endpoint.container_disk_gb` | int | yes | LGPL FFmpeg image is ~1 GB; 8 GB headroom for layer cache + scratch tmp. |
| `endpoint.workers_max_default` | int | yes | Default concurrency cap. |
| `endpoint.idle_timeout_s_default` | int | yes | RunPod scaler idle timeout. |
| `endpoint.flashboot_default` | bool | yes | Strong default — image pull is most of the cold-start cost. |
| `endpoint.min_cuda_version` | string \| null | yes | NVENC needs a recent driver; `"12.0"` is comfortable. Tighten to `"12.8"` if we adopt newer NVENC features. |
| `gpu_pools` | object | yes | Map of user-facing GPU class → RunPod pool ID. Mirror the canonical list at <https://docs.runpod.io/references/gpu-types#gpu-pools>. |
| `env` | array | yes | Container env-var entries. Empty is fine. |

## `benchmark.json`

(Phase A doesn't ship benchmark.json yet; Phase B onward will, once
real transforms exist that have meaningful latency to measure. The
real-esrgan-serve benchmark schema applies — see that repo's
`deploy/SCHEMA.md`.)

## How iosuite resolves the URL

```
https://raw.githubusercontent.com/<owner>/<repo>/<git-tag>/deploy/runpod.json
```

`iosuite endpoint deploy --tool ffmpeg --version <tag>` fetches at
that ref. With `--version` omitted, iosuite uses the
`StableVersion` declared in its registry for `ffmpeg`.

## Why this lives here

Same reason it lives in real-esrgan-serve: the *-serve module owns
its image tag, container disk, GPU pool map, FlashBoot default, CUDA
pin. Bumping any of those lands in the same commit as the change
that needs them — image, deploy spec, and CI gate all stay in sync.
iosuite is one binary supporting many tools.
