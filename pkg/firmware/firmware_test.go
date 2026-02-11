package firmware

import (
	"bytes"
	"encoding/binary"
	"hash/adler32"
	"os"
	"path/filepath"
	"testing"
)

// testFirmwarePath points to a real firmware file for integration tests.
// Set via PANAGO_TEST_FIRMWARE env var or defaults to the known location.
func testFirmwarePath(t *testing.T) string {
	t.Helper()
	p := os.Getenv("PANAGO_TEST_FIRMWARE")
	if p == "" {
		p = "/home/tridentsx/src/panasonic/org/PANAEUSB.FRM"
	}
	if _, err := os.Stat(p); err != nil {
		t.Skipf("firmware file not available at %s: %v", p, err)
	}
	return p
}

func TestMainPartitionRoundtrip(t *testing.T) {
	fwPath := testFirmwarePath(t)

	// Step 1: Decode original firmware
	decodeDir := t.TempDir()
	decoder := NewDecoder(false)
	if err := decoder.DecodeFile(fwPath, decodeDir); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	// Verify MAIN.bin and MAIN_metadata.json were created
	mainBinPath := filepath.Join(decodeDir, "MAIN.bin")
	metaPath := filepath.Join(decodeDir, "MAIN_metadata.json")

	origMainBin, err := os.ReadFile(mainBinPath)
	if err != nil {
		t.Fatalf("MAIN.bin not created: %v", err)
	}
	if len(origMainBin) == 0 {
		t.Fatal("MAIN.bin is empty")
	}

	if _, err := os.Stat(metaPath); err != nil {
		t.Fatalf("MAIN_metadata.json not created: %v", err)
	}
	t.Logf("Decoded MAIN.bin: %d bytes", len(origMainBin))

	// Step 2: Re-encode using decoded files + original as template
	encodeDir := t.TempDir()
	outputFW := filepath.Join(encodeDir, "output.FRM")
	encoder := NewEncoder(false)
	if err := encoder.EncodeFile(decodeDir, outputFW, fwPath); err != nil {
		t.Fatalf("encode failed: %v", err)
	}

	// Step 3: Decode the re-encoded firmware
	redecodeDir := t.TempDir()
	decoder2 := NewDecoder(false)
	if err := decoder2.DecodeFile(outputFW, redecodeDir); err != nil {
		t.Fatalf("re-decode failed: %v", err)
	}

	// Step 4: Compare MAIN.bin from both decodes
	redecMainBin, err := os.ReadFile(filepath.Join(redecodeDir, "MAIN.bin"))
	if err != nil {
		t.Fatalf("re-decoded MAIN.bin not created: %v", err)
	}

	if len(origMainBin) != len(redecMainBin) {
		t.Errorf("MAIN.bin size mismatch: original=%d, roundtrip=%d", len(origMainBin), len(redecMainBin))
	}

	if !bytes.Equal(origMainBin, redecMainBin) {
		// Find first difference for debugging
		minLen := len(origMainBin)
		if len(redecMainBin) < minLen {
			minLen = len(redecMainBin)
		}
		for i := 0; i < minLen; i++ {
			if origMainBin[i] != redecMainBin[i] {
				t.Errorf("MAIN.bin content differs at offset 0x%X (byte %d): orig=0x%02X, roundtrip=0x%02X",
					i, i, origMainBin[i], redecMainBin[i])
				break
			}
		}
		t.Fatal("MAIN.bin content does not match after roundtrip")
	}

	t.Logf("Roundtrip successful: MAIN.bin %d bytes match exactly", len(origMainBin))
}

func TestEntryStructure(t *testing.T) {
	// Test that a single entry encode/decode roundtrip works correctly,
	// including EXTRFOOT footer and Adler32 checksum.
	// Uses a small (64KB) chunk to avoid slow LZSS compression.

	// Create test data
	chunkSize := 64 * 1024
	chunk := make([]byte, chunkSize)
	for i := range chunk {
		chunk[i] = byte(i * 7) // Deterministic pattern
	}

	// Compress with LZSS
	compData := CompressLZSS(chunk)
	t.Logf("Chunk: %d bytes -> compressed: %d bytes", chunkSize, len(compData))

	// Build entry header
	signature := []byte("EXTRHEADDRVD  ")
	bufferConstant := uint32(MainEntryHeaderLen) + align4(uint32(len(compData))) + 1024 // slack=1024
	header := buildEntryHeader(signature, 2, uint32(chunkSize),
		uint32(len(compData)), bufferConstant, 0x10)

	// Build entry data: header + compressed + padding + EXTRFOOT
	footerOff := uint32(MainEntryHeaderLen) + align4(uint32(len(compData)))
	targetSize := align8(footerOff + 8)

	entryData := make([]byte, targetSize)
	copy(entryData, header)
	copy(entryData[MainEntryHeaderLen:], compData)
	copy(entryData[footerOff:footerOff+8], []byte("EXTRFOOT"))

	// Verify EXTRFOOT is at the right position
	if string(entryData[footerOff:footerOff+8]) != "EXTRFOOT" {
		t.Fatalf("EXTRFOOT not at offset %d", footerOff)
	}

	// Verify header fields
	gotFooterOff := binary.LittleEndian.Uint32(entryData[32:36])
	if gotFooterOff != footerOff {
		t.Errorf("header FooterOffset: got %d, want %d", gotFooterOff, footerOff)
	}

	// Feistel encrypt
	fc := NewFeistelCipher()
	decryptSize := 5120
	if len(entryData) > decryptSize {
		fc.Encrypt(entryData[:decryptSize])
		fc.Encrypt(entryData[len(entryData)-decryptSize:])
	} else {
		fc.Encrypt(entryData)
	}

	// Compute Adler32 of encrypted entry data
	listChecksum := adler32.Checksum(entryData)
	t.Logf("List entry Adler32 checksum: 0x%08X", listChecksum)

	// Now decode: Feistel decrypt
	fc2 := NewFeistelCipher()
	if len(entryData) > decryptSize {
		fc2.Decrypt(entryData[:decryptSize])
		fc2.Decrypt(entryData[len(entryData)-decryptSize:])
	} else {
		fc2.Decrypt(entryData)
	}

	// Verify EXTRFOOT is restored after decrypt
	if string(entryData[footerOff:footerOff+8]) != "EXTRFOOT" {
		t.Errorf("EXTRFOOT not restored after Feistel decrypt, got: %x", entryData[footerOff:footerOff+8])
	}

	// Parse header and decompress using only compSize bytes (like the fixed decoder)
	ehdrCompSize := binary.LittleEndian.Uint32(entryData[24:28])
	ehdrDecompSize := binary.LittleEndian.Uint32(entryData[16:20])

	if ehdrCompSize != uint32(len(compData)) {
		t.Fatalf("CompSize mismatch: got %d, want %d", ehdrCompSize, len(compData))
	}

	// Extract only compSize bytes for decompression (not the whole entry)
	extractedComp := entryData[MainEntryHeaderLen : MainEntryHeaderLen+int(ehdrCompSize)]
	decompData := DecompressLZSSWithSize(extractedComp, int(ehdrDecompSize))

	if !bytes.Equal(decompData, chunk) {
		t.Fatalf("Decompressed data doesn't match original (len: %d vs %d)", len(decompData), len(chunk))
	}

	t.Logf("Entry roundtrip successful: %d -> %d -> %d bytes", chunkSize, len(compData), len(decompData))
}

func TestMainMetadataCreated(t *testing.T) {
	fwPath := testFirmwarePath(t)

	decodeDir := t.TempDir()
	decoder := NewDecoder(false)
	if err := decoder.DecodeFile(fwPath, decodeDir); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	metaPath := filepath.Join(decodeDir, "MAIN_metadata.json")
	meta, err := loadMainMetadata(metaPath)
	if err != nil {
		t.Fatalf("failed to load metadata: %v", err)
	}

	if meta.EntryCount == 0 {
		t.Fatal("metadata has no entries")
	}
	if meta.ChunkSize == 0 {
		t.Error("chunk_size is 0")
	}
	if meta.FirstHeader == "" {
		t.Error("first_header is empty")
	}
	if meta.EntrySignature == "" {
		t.Error("entry_signature is empty")
	}
	if meta.BufferConstant == 0 {
		t.Error("buffer_constant is 0")
	}

	t.Logf("Metadata: %d entries, chunk=%d, comp_type=%d, buffer_const=%d",
		meta.EntryCount, meta.ChunkSize, meta.CompType, meta.BufferConstant)
}
