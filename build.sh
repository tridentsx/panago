#!/usr/bin/bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o dist/panago-linux-amd64

# Windows ARM64
GOOS=windows GOARCH=amd64 go build -o dist/panago-windows-amd64.exe

# macOS AMD64 and ARM64
GOOS=darwin GOARCH=amd64 go build -o dist/panago-darwin-amd64
GOOS=darwin GOARCH=arm64 go build -o dist/panago-darwin-arm64
