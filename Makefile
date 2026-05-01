.PHONY: build clean fmt vet test install \
        docker-cpu docker-cuda \
        docker-push-cpu docker-push-cuda \
        validate-manifest

BIN_DIR ?= bin
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

LDFLAGS = -s -w \
          -X main.version=$(VERSION) \
          -X main.commit=$(COMMIT)

# Pure-Go build. The Go binary itself shells out to ffmpeg + ffprobe,
# so the binary has no native dependencies and cross-compiles cleanly.
build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
	    -o $(BIN_DIR)/ffmpeg-serve ./cmd/ffmpeg-serve

clean:
	rm -rf $(BIN_DIR)

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

install: build
	install -m 0755 $(BIN_DIR)/ffmpeg-serve /usr/local/bin/ffmpeg-serve

# CI gate. The same script runs on every push to deploy/** so a
# manifest typo is caught before iosuite tries to fetch it.
validate-manifest:
	python3 build/validate_manifest.py

# ----------------------------------------------------------------------
# Docker images. Two flavours: cpu (LGPL FFmpeg, no NVIDIA) and cuda
# (LGPL FFmpeg + NVENC). See Dockerfile comments + NOTICE.md for the
# license posture; both stay Apache-2.0-distributable.
# ----------------------------------------------------------------------

docker-cpu:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
	    -f Dockerfile.cpu \
	    -t ffmpeg-serve:cpu-$(VERSION) .

docker-cuda:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
	    -f Dockerfile.cuda \
	    -t ffmpeg-serve:cuda-$(VERSION) .

docker-push-cpu: docker-cpu
	docker tag ffmpeg-serve:cpu-$(VERSION) ghcr.io/ls-ads/ffmpeg-serve:cpu-$(VERSION)
	docker push ghcr.io/ls-ads/ffmpeg-serve:cpu-$(VERSION)
	@echo "pushed: ghcr.io/ls-ads/ffmpeg-serve:cpu-$(VERSION)"

docker-push-cuda: docker-cuda
	docker tag ffmpeg-serve:cuda-$(VERSION) ghcr.io/ls-ads/ffmpeg-serve:cuda-$(VERSION)
	docker push ghcr.io/ls-ads/ffmpeg-serve:cuda-$(VERSION)
	@echo "pushed: ghcr.io/ls-ads/ffmpeg-serve:cuda-$(VERSION)"
