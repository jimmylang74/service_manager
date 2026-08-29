#!/bin/bash
set -e

OUTPUT_DIR=".."
mkdir -p "$OUTPUT_DIR"

echo "Building for Windows amd64..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "$OUTPUT_DIR/service-manager.exe" ./cmd/service-manager

echo "Copying config..."
cp ../configs/service-manager.yaml "$OUTPUT_DIR/config.yaml"

echo "Creating directories..."
mkdir -p "$OUTPUT_DIR/services"
mkdir -p "$OUTPUT_DIR/logs"
mkdir -p "$OUTPUT_DIR/web/dist"

echo "Build complete: $OUTPUT_DIR/service-manager.exe"
