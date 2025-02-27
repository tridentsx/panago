.PHONY: all build test clean

BINARIES := bin/panago bin/shell bin/diskutil bin/discover

all: build test

build:
	@mkdir -p bin
	go build -o bin/panago ./cmd/panago
	go build -o bin/shell ./cmd/shell
	go build -o bin/diskutil ./cmd/diskutil
	go build -o bin/discover ./cmd/discover

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