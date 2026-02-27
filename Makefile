BINARY_NAME=ffmpeg-serve
BIN_DIR=bin

platforms := linux windows darwin
architectures := amd64 arm64

build:
	go build -o $(BIN_DIR)/$(BINARY_NAME) main.go

build-all: $(foreach p,$(platforms),$(foreach a,$(architectures),build-$(p)-$(a)))

# Template for dynamic target generation
define BUILD_TARGET
build-$(1)-$(2):
	GOOS=$(1) GOARCH=$(2) go build -o $(BIN_DIR)/$(BINARY_NAME)-$(1)-$(2)$(if $(filter windows,$(1)),.exe,) main.go
endef

# Generate targets for all platform/architecture combinations
$(foreach p,$(platforms),$(foreach a,$(architectures),$(eval $(call BUILD_TARGET,$(p),$(a)))))

test:
	go test ./...

clean:
	rm -rf $(BIN_DIR)
	rm -f /tmp/ffmpeg_serve.pid

run: build
	./$(BIN_DIR)/$(BINARY_NAME)

server: build
	./$(BIN_DIR)/$(BINARY_NAME) server start
