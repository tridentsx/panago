# Contributing to Panago

## Development Setup

```bash
git clone https://github.com/tridentsx/panago.git
cd panago
go build ./...
go test ./...
```

## Code Style

- Follow standard Go formatting (`gofmt`)
- Run `go vet ./...` before submitting
- Keep functions focused; avoid adding abstractions for one-off use

## Tests

Unit tests (no firmware file needed):

```bash
go test ./pkg/firmware/ -run TestLZSS
go test ./pkg/cramfs/
go test ./pkg/romfs/
```

Integration tests (require a real firmware file):

```bash
PANAGO_TEST_FIRMWARE=/path/to/PANAEUSB.FRM go test ./pkg/firmware/
```

## Pull Request Process

1. Ensure all tests pass (`go test ./...`)
2. Ensure `go vet ./...` is clean
3. Update the relevant section of the root README if behaviour changes
4. Submit PR with a clear description of what changed and why
