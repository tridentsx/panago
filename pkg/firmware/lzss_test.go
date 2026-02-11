package firmware

import (
	"bytes"
	"math/rand"
	"testing"
)

// makeTestData generates semi-compressible data (mix of repeated patterns and random bytes)
func makeTestData(size int) []byte {
	rng := rand.New(rand.NewSource(42))
	data := make([]byte, size)
	i := 0
	for i < size {
		if rng.Intn(3) == 0 && i+20 < size {
			// Insert a repeated pattern
			patLen := 4 + rng.Intn(15)
			pat := make([]byte, patLen)
			rng.Read(pat)
			reps := 2 + rng.Intn(4)
			for r := 0; r < reps && i+patLen <= size; r++ {
				copy(data[i:], pat)
				i += patLen
			}
		} else {
			// Random byte
			data[i] = byte(rng.Intn(256))
			i++
		}
	}
	return data[:size]
}

func TestLZSSRoundtrip(t *testing.T) {
	sizes := []int{0, 1, 100, 4096, 64 * 1024, 256 * 1024}
	for _, size := range sizes {
		var input []byte
		if size == 0 {
			input = []byte{}
		} else {
			input = makeTestData(size)
		}

		compressed := CompressLZSS(input)
		decompressed := DecompressLZSS(compressed)

		if !bytes.Equal(input, decompressed) {
			t.Errorf("roundtrip failed for size %d: got %d bytes, want %d bytes", size, len(decompressed), len(input))
			if len(input) < 200 {
				t.Errorf("  input:  %x", input)
				t.Errorf("  output: %x", decompressed)
			} else {
				// Find first differing byte
				for i := 0; i < len(input) && i < len(decompressed); i++ {
					if input[i] != decompressed[i] {
						t.Errorf("  first diff at byte %d: got 0x%02x, want 0x%02x", i, decompressed[i], input[i])
						break
					}
				}
			}
		}
	}
}

func TestLZSSRoundtripWithSize(t *testing.T) {
	sizes := []int{1, 100, 4096, 64 * 1024, 256 * 1024}
	for _, size := range sizes {
		input := makeTestData(size)
		compressed := CompressLZSS(input)
		decompressed := DecompressLZSSWithSize(compressed, size)

		if !bytes.Equal(input, decompressed) {
			t.Errorf("roundtrip (WithSize) failed for size %d: got %d bytes, want %d bytes", size, len(decompressed), len(input))
		}
	}
}

func BenchmarkCompressLZSS_64KB(b *testing.B) {
	data := makeTestData(64 * 1024)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompressLZSS(data)
	}
}

func BenchmarkCompressLZSS_1MB(b *testing.B) {
	data := makeTestData(1024 * 1024)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CompressLZSS(data)
	}
}

func BenchmarkDecompressLZSSWithSize_64KB(b *testing.B) {
	data := makeTestData(64 * 1024)
	compressed := CompressLZSS(data)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecompressLZSSWithSize(compressed, len(data))
	}
}

func BenchmarkDecompressLZSSWithSize_1MB(b *testing.B) {
	data := makeTestData(1024 * 1024)
	compressed := CompressLZSS(data)
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DecompressLZSSWithSize(compressed, len(data))
	}
}
