.PHONY: all build test clean

BINARIES := bin/discover bin/shell bin/cramfs

all: build test

build:
	@mkdir -p bin
	go build -o bin/discover ./cmd/discover
	go build -o bin/shell ./cmd/shell
	go build -o bin/cramfs ./cmd/cramfs

test:
	go test -v ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/
