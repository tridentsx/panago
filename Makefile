SHELL := /bin/bash

BINARY_DIR := bin
DISKUTIL_BINARY := $(BINARY_DIR)/diskutil
PANAGO_BINARY  := $(BINARY_DIR)/panago
CRAMFS_BINARY  := $(BINARY_DIR)/cramfs
DISCOVER_BINARY  := $(BINARY_DIR)/discover

.PHONY: all build build-diskutil build-panago build-cramfs build-discover clean test fmt lint

all: build

## Build all binaries
build: build-diskutil build-panago build-patches build-cramfs build-discover


## Build the diskutil binary
build-diskutil:
	@echo "Building diskutil..."
	@mkdir -p $(BINARY_DIR)
	# If using the root go.mod only, you can do:
	go build -o $(DISKUTIL_BINARY) ./cmd/diskutil

## Build the panago binary
build-panago:
	@echo "Building panago..."
	@mkdir -p $(BINARY_DIR)
	# If using the root go.mod only, you can do:
	go build -o $(PANAGO_BINARY) ./cmd/panago

## Build the panago binary
build-cramfs:
	@echo "Building cramfs..."
	@mkdir -p $(BINARY_DIR)
	# If using the root go.mod only, you can do:
	go build -o $(CRAMFS_BINARY) ./cmd/cramfs

## Build the discover binary
build-discover:
	@echo "Building discover..."
	@mkdir -p $(BINARY_DIR)
	# If using the root go.mod only, you can do:
	go build -o $(DISCOVER_BINARY) ./cmd/discover

build-patches:
	@echo "Putting patches in bin folder..."
	cp -r patches bin/patches

build-scripts:
	@echo "Putting patches in bin folder..."
	cp -r scripts bin/scripts

## Remove built artifacts
clean:
	@echo "Cleaning..."
	rm -rf $(BINARY_DIR)

## Run all tests in the module
test:
	@echo "Running tests..."
	go test -v ./...

## Format all Go code
fmt:
	@echo "Formatting code..."
	go fmt ./...

## Lint the code (requires golangci-lint or another lint tool installed)
lint:
	@echo "Linting code..."
	golangci-lint run
