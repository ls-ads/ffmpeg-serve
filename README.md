# FFmpeg Serve

A standalone Go CLI tool using Cobra that wraps FFmpeg for local media processing and provides a persistent HTTP server for remote processing.

This tool is designed to support both "one-shot" local transcoding and a persistent REST API for media transformations.

## Installation Requirements

This project requires `ffmpeg` and `ffprobe` to be installed on your system.

- Go 1.20+
- FFmpeg (with required codecs)
- FFprobe (part of the FFmpeg suite)

Build the CLI using the provided `Makefile`:

```bash
make build
```

This will output the `ffmpeg-serve` binary.

## Usage

### 1. Local Processing (File or Directory)

```bash
# Process a single video using GPU 0 (NVENC)
./ffmpeg-serve -i path/to/video.mp4 -o path/to/output.mkv -g 0 -- -c:v h264_nvenc -preset p4

# Process a directory of images (example: convert to webp)
./ffmpeg-serve -i path/to/input_dir -o path/to/output_dir -- -c:v libwebp -lossless 1
```

- `-i`, `--input`: The input file path or directory path.
- `-o`, `--output`: The output file path or directory path. 
- `-g`, `--gpu-id`: (Optional) The GPU device ID to use for hardware acceleration (default: -1, disabled).
- `--`: Any arguments following the double-dash are passed directly to FFmpeg.

### 2. HTTP Server

```bash
# Start the server using GPU 0
./ffmpeg-serve server start -p 8080 -g 0
```

### Direct API Usage (cURL)

Once the server is running, you can send media directly to the API endpoints using standard HTTP multipart form requests.

#### Process Media (`/process`)

Send a file and specify FFmpeg arguments via the `args` query parameter (comma-separated).

```bash
# Transcode a video to H.264
curl -X POST -F "file=@video.mp4" "http://localhost:8080/process?args=-c:v,libx264,-preset,fast" -o output.mp4
```

#### Probe Media (`/probe`)

Get metadata about a media file in JSON format.

```bash
curl -X POST -F "file=@video.mp4" "http://localhost:8080/probe"
```

#### Health Check (`/health`)

```bash
curl http://localhost:8080/health
```

## Docker

If you don't want to manage FFmpeg dependencies on your host, you can run the entire tool via Docker.

**1. Build the Image:**
```bash
docker build -t ffmpeg-serve:v1 .
```

**2. Local Processing:**
```bash
docker run --rm -v $(pwd):/workspace ffmpeg-serve:v1 -i /workspace/input.mp4 -o /workspace/output.mp4
```

**3. HTTP Server:**
```bash
docker run -d -p 8080:8080 ffmpeg-serve:v1 server start -p 8080
```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
