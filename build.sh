#!/usr/bin/bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build -o myapp-linux-amd64

# Windows ARM64
GOOS=windows GOARCH=arm64 go build -o myapp-windows-arm64.exe

# macOS AMD64
GOOS=darwin GOARCH=amd64 go build -o myapp-darwin-amd64
