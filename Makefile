BINARY_NAME=ffmpeg-serve

build:
	go build -o $(BINARY_NAME) main.go

test:
	go test ./...

clean:
	rm -f $(BINARY_NAME)
	rm -f /tmp/ffmpeg_serve.pid

run: build
	./$(BINARY_NAME)

server: build
	./$(BINARY_NAME) server start
