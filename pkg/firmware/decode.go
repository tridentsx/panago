package firmware

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Decoder handles firmware decryption and extraction
type Decoder struct {
	feistel *feistelCipher
	verbose bool
}

// NewDecoder creates a new firmware decoder
func NewDecoder(verbose bool) *Decoder {
	return &Decoder{
		feistel: NewFeistelCipher(),
		verbose: verbose,
	}
}

// DecodeFile decodes a firmware file and extracts all partitions
func (d *Decoder) DecodeFile(inputPath, outputDir string) error {
	// Read input file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	if d.verbose {
		fmt.Printf("Read %d bytes from %s\n", len(data), inputPath)
	}

	// Step 1: AES decrypt entire file
	decrypted, err := AESDecryptCBC(data)
	if err != nil {
		return fmt.Errorf("AES decryption failed: %w", err)
	}

	if d.verbose {
		fmt.Printf("AES decrypted %d bytes\n", len(decrypted))
	}

	// Step 2: Feistel decrypt header (first 0x30 bytes)
	if len(decrypted) < HeaderSize {
		return fmt.Errorf("file too small for header")
	}
	d.feistel.Decrypt(decrypted[:HeaderSize])

	// Step 3: Parse partition table
	partitions, err := d.parsePartitionTable(decrypted)
	if err != nil {
		return fmt.Errorf("failed to parse partition table: %w", err)
	}

	if d.verbose {
		fmt.Printf("Found %d partitions\n", len(partitions))
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Step 4: Extract each partition
	for _, p := range partitions {
		if err := d.extractPartition(decrypted, p, outputDir); err != nil {
			fmt.Printf("Warning: failed to extract %s: %v\n", p.Name, err)
		}
	}

	return nil
}

// parsePartitionTable parses the firmware partition table
func (d *Decoder) parsePartitionTable(data []byte) ([]PartitionInfo, error) {
	var partitions []PartitionInfo

	// Module header block starts at offset 0x30 (48) and is 0x2000 bytes
	// The entire block needs to be Feistel decrypted at once
	moduleHeaderSize := 0x2000
	if len(data) < HeaderSize+moduleHeaderSize {
		return nil, fmt.Errorf("file too small for module headers")
	}

	// Copy and decrypt the module header block
	modHdr := make([]byte, moduleHeaderSize)
	copy(modHdr, data[HeaderSize:HeaderSize+moduleHeaderSize])
	d.feistel.Decrypt(modHdr)

	// Entry 0 is a "$PaT" header marker - skip it
	// Actual module entries start at entry 1
	for i := 1; i < 20; i++ {
		offset := i * ModuleEntrySize
		if offset+ModuleEntrySize > len(modHdr) {
			break
		}

		entry := modHdr[offset : offset+ModuleEntrySize]

		// Parse entry
		name := strings.TrimRight(string(entry[0:4]), "\x00")
		version := strings.TrimRight(string(entry[4:8]), "\x00")

		// Check if this is a valid entry (name should be ASCII)
		if len(name) == 0 || name[0] < 0x20 || name[0] > 0x7e {
			break
		}

		partOffset := binary.LittleEndian.Uint32(entry[12:16])
		partSize := binary.LittleEndian.Uint32(entry[32:36])
		checksum := binary.LittleEndian.Uint32(entry[36:40])

		if partOffset == 0 && partSize == 0 {
			break
		}

		p := PartitionInfo{
			Name:     name,
			Version:  version,
			Offset:   partOffset,
			Size:     partSize,
			Checksum: checksum,
		}
		partitions = append(partitions, p)

		if d.verbose {
			fmt.Printf("  %s v%s: offset=0x%x, size=%d\n", name, version, partOffset, partSize)
		}
	}

	return partitions, nil
}

// extractPartition extracts a single partition
func (d *Decoder) extractPartition(data []byte, p PartitionInfo, outputDir string) error {
	if p.Offset == 0 || p.Size == 0 {
		return nil
	}

	if int(p.Offset+p.Size) > len(data) {
		return fmt.Errorf("partition extends beyond file")
	}

	partData := make([]byte, p.Size)
	copy(partData, data[p.Offset:p.Offset+p.Size])

	// Handle MAIN partition specially (contains sub-entries)
	if p.Name == "MAIN" {
		return d.extractMainPartition(partData, outputDir)
	}

	// For other partitions, Feistel decrypt the entire partition
	d.feistel.Decrypt(partData)

	// Save decrypted data
	filename := fmt.Sprintf("%s_%s.bin", p.Name, p.Version)
	outPath := filepath.Join(outputDir, filename)

	if err := os.WriteFile(outPath, partData, 0644); err != nil {
		return err
	}

	if d.verbose {
		fmt.Printf("  Saved %s (%d bytes)\n", outPath, len(partData))
	}

	return nil
}

// extractMainPartition extracts the MAIN partition with its sub-entries
func (d *Decoder) extractMainPartition(data []byte, outputDir string) error {
	// MAIN has a 0x30-byte Feistel-encrypted header before the list
	if len(data) < MainListHeaderOff+MainListHeaderLen {
		return fmt.Errorf("MAIN partition too small")
	}

	// Decrypt the header portion and capture for metadata
	headerPart := make([]byte, MainListHeaderOff)
	copy(headerPart, data[:MainListHeaderOff])
	d.feistel.Decrypt(headerPart)

	// Parse list header
	listHdr := data[MainListHeaderOff : MainListHeaderOff+MainListHeaderLen]
	hdr := MainListHeader{
		Checksum:      binary.LittleEndian.Uint32(listHdr[0:4]),
		FormatVersion: binary.LittleEndian.Uint32(listHdr[4:8]),
		ListSize:      binary.LittleEndian.Uint32(listHdr[8:12]),
		DecompSize:    binary.LittleEndian.Uint32(listHdr[12:16]),
		CompType:      binary.LittleEndian.Uint32(listHdr[16:20]),
	}

	entryCount := (hdr.ListSize - 20) / 8
	if entryCount > 200 || entryCount == 0 {
		return fmt.Errorf("invalid entry count: %d", entryCount)
	}

	// Determine decrypt size based on decomp size
	decryptSize := 5120
	if hdr.DecompSize >= 0x2000000 {
		decryptSize = 10240
	}

	if d.verbose {
		fmt.Printf("  MAIN: %d entries, decrypt_size=%d\n", entryCount, decryptSize)
	}

	// Parse entry list
	offset := uint32(MainListHeaderOff + MainListHeaderLen)
	entries := make([]MainListEntry, entryCount)
	for i := uint32(0); i < entryCount; i++ {
		entries[i].Size = binary.LittleEndian.Uint32(data[offset : offset+4])
		entries[i].Checksum = binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		offset += 8
	}

	// Concatenated output for MAIN.bin
	var mainOut []byte

	// Collect structural constants from first valid entry for metadata
	var entrySignature []byte
	var compType uint16
	var chunkSize uint32
	var bufferConstant uint32
	var checksumFlag uint8
	processedEntries := 0

	// Process each entry
	for i, entry := range entries {
		if entry.Size == 0 || int(offset)+int(entry.Size) > len(data) {
			break
		}

		entryData := make([]byte, entry.Size)
		copy(entryData, data[offset:offset+entry.Size])

		// Decrypt entry boundaries
		if entry.Size > uint32(decryptSize) {
			d.feistel.Decrypt(entryData[:decryptSize])
			d.feistel.Decrypt(entryData[entry.Size-uint32(decryptSize):])
		} else {
			d.feistel.Decrypt(entryData)
		}

		// Parse entry header
		if len(entryData) < MainEntryHeaderLen {
			offset += entry.Size
			continue
		}

		ehdr := MainEntryHeader{
			CompType:   binary.LittleEndian.Uint16(entryData[14:16]),
			DecompSize: binary.LittleEndian.Uint32(entryData[16:20]),
			DestAddr:   binary.LittleEndian.Uint32(entryData[20:24]),
			CompSize:   binary.LittleEndian.Uint32(entryData[24:28]),
		}

		if ehdr.CompSize == 0 || ehdr.DecompSize == 0 || ehdr.CompSize > entry.Size {
			offset += entry.Size
			continue
		}

		// Capture structural constants from first entry
		if processedEntries == 0 {
			entrySignature = make([]byte, 14)
			copy(entrySignature, entryData[:14])
			compType = ehdr.CompType
			chunkSize = ehdr.DecompSize
			checksumFlag = entryData[44]

			// Derive buffer constant: Slack + FooterOffset
			slack := binary.LittleEndian.Uint32(entryData[28:32])
			footerOff := binary.LittleEndian.Uint32(entryData[32:36])
			bufferConstant = slack + footerOff
		}

		// Use only compSize bytes for decompression (not the full entry)
		compData := entryData[MainEntryHeaderLen : MainEntryHeaderLen+int(ehdr.CompSize)]

		var decompData []byte

		if ehdr.CompType == 1 {
			// Gzip + LZSS
			decompData = d.decompressGzipLZSS(compData)
		} else {
			// Raw LZSS
			decompData = DecompressLZSSWithSize(compData, int(ehdr.DecompSize))
		}

		if len(decompData) > 0 {
			mainOut = append(mainOut, decompData...)
			if d.verbose {
				fmt.Printf("    Entry %d: %d -> %d bytes\n", i, ehdr.CompSize, len(decompData))
			}
		}
		processedEntries++

		offset += entry.Size
	}

	// Save MAIN.bin
	if len(mainOut) > 0 {
		outPath := filepath.Join(outputDir, "MAIN.bin")
		if err := os.WriteFile(outPath, mainOut, 0644); err != nil {
			return err
		}
		if d.verbose {
			fmt.Printf("  Saved MAIN.bin (%d bytes)\n", len(mainOut))
		}
	}

	// Save lean MAIN_metadata.json
	meta := MainPartitionMetadata{
		FirstHeader:    hex.EncodeToString(headerPart),
		FormatVersion:  hdr.FormatVersion,
		ListHeaderCompType: hdr.CompType,
		EntrySignature: hex.EncodeToString(entrySignature),
		CompType:       compType,
		ChunkSize:      chunkSize,
		BufferConstant: bufferConstant,
		ChecksumFlag:   checksumFlag,
		EntryCount:     processedEntries,
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MAIN metadata: %w", err)
	}
	metaPath := filepath.Join(outputDir, "MAIN_metadata.json")
	if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
		return fmt.Errorf("failed to write MAIN metadata: %w", err)
	}
	if d.verbose {
		fmt.Printf("  Saved MAIN_metadata.json (%d entries)\n", processedEntries)
	}

	return nil
}

// decompressGzipLZSS decompresses gzip-wrapped LZSS data
func (d *Decoder) decompressGzipLZSS(data []byte) []byte {
	// First decompress gzip
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	defer r.Close()

	gzipOut, err := io.ReadAll(r)
	if err != nil {
		return nil
	}

	// The gzip output contains another header + LZSS data
	if len(gzipOut) < MainEntryHeaderLen {
		return gzipOut
	}

	// Parse inner header
	innerCompSize := binary.LittleEndian.Uint32(gzipOut[24:28])
	innerDecompSize := binary.LittleEndian.Uint32(gzipOut[16:20])

	if innerCompSize == 0 || innerDecompSize == 0 {
		return gzipOut
	}

	// Decompress inner LZSS
	innerData := gzipOut[MainEntryHeaderLen:]
	return DecompressLZSSWithSize(innerData, int(innerDecompSize))
}

// GetInfo returns information about a firmware file without extracting
func (d *Decoder) GetInfo(inputPath string) (*FirmwareInfo, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, err
	}

	// AES decrypt
	decrypted, err := AESDecryptCBC(data)
	if err != nil {
		return nil, err
	}

	// Feistel decrypt header
	d.feistel.Decrypt(decrypted[:HeaderSize])

	// Parse partitions
	partitions, err := d.parsePartitionTable(decrypted)
	if err != nil {
		return nil, err
	}

	return &FirmwareInfo{
		TotalSize:  int64(len(data)),
		Partitions: partitions,
	}, nil
}
