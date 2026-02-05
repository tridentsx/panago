.PHONY: all build test clean

BINARIES := bin/panago bin/panago-cli bin/shell bin/diskutil bin/discover bin/firmware bin/cramfsck

all: build test

build:
	@mkdir -p bin
	go build -o bin/panago ./cmd/panago
	go build -o bin/panago-cli ./cmd/cli
	go build -o bin/shell ./cmd/shell
	go build -o bin/diskutil ./cmd/diskutil
	go build -o bin/discover ./cmd/discover
	go build -o bin/firmware ./cmd/firmware
	go build -o bin/cramfsck ./cmd/cramfs

test:
	go test -v ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/
	rm -rf dist/
	rm -rf build/
	rm -rf release/
	rm -rf .cache/
	rm -rf .temp/
	rm -rf .output/