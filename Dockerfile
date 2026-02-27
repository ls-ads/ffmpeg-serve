# Build stage
FROM golang:1.21-bullseye AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN go build -o ffmpeg-serve main.go

# Final stage
FROM nvidia/cuda:12.1.0-runtime-ubuntu22.04

# Install ffmpeg and required libraries
RUN apt-get update && apt-get install -y ffmpeg libnvidia-encode-1 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/ffmpeg-serve .

# Expose the server port
EXPOSE 8080

# Set the entrypoint
ENTRYPOINT ["/app/ffmpeg-serve"]
