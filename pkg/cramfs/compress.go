package cramfs

import (
	"bytes"
	"compress/flate"
	"io"
)

// newRawInflateReader creates a reader for raw deflate data
func newRawInflateReader(data []byte) io.Reader {
	return flate.NewReader(bytes.NewReader(data))
}

// compressBlock compresses data exactly as Panasonic's firmware toolchain did:
// real zlib at level=6/windowBits=14/memLevel=7/Z_DEFAULT_STRATEGY, producing
// a standard zlib-wrapped stream. See zdeflate.go for why Go's stdlib
// compress/flate (a different DEFLATE implementation) cannot reproduce this.
func compressBlock(data []byte) []byte {
	return zlibDeflate(data)
}

// align4 rounds up to 4-byte boundary
func align4(n int) int {
	return (n + 3) &^ 3
}
