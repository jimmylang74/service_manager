#!/bin/bash
set -e

OUTPUT_DIR=".."
mkdir -p "$OUTPUT_DIR"

echo "Building for Linux amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$OUTPUT_DIR/service-manager" ./cmd/service-manager

echo "Copying config..."
cp ../configs/service-manager.yaml "$OUTPUT_DIR/config.yaml"

echo "Creating directories..."
mkdir -p "$OUTPUT_DIR/services"
mkdir -p "$OUTPUT_DIR/logs"
mkdir -p "$OUTPUT_DIR/web/dist"

echo "Build complete: $OUTPUT_DIR/service-manager"
