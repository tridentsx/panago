.PHONY: all build test clean

BINARIES := bin/panago bin/panago-cli

all: build test

build:
	@mkdir -p bin
	go build -o bin/panago ./cmd/panago
	go build -o bin/panago-cli ./cmd/cli

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
