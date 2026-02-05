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

// compressBlock compresses data using raw deflate
func compressBlock(data []byte) []byte {
	var buf bytes.Buffer
	// Use best compression level
	w, err := flate.NewWriter(&buf, flate.BestCompression)
	if err != nil {
		return data
	}
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

// align4 rounds up to 4-byte boundary
func align4(n int) int {
	return (n + 3) &^ 3
}
