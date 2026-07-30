package firmware

import (
	"encoding/binary"
	"fmt"
	"hash/adler32"
	"os"
	"path/filepath"
	"strings"
)

// Encoder handles firmware encryption and repacking
type Encoder struct {
	feistel *feistelCipher
	verbose bool
}

// NewEncoder creates a new firmware encoder
func NewEncoder(verbose bool) *Encoder {
	return &Encoder{
		feistel: NewFeistelCipher(),
		verbose: verbose,
	}
}

// EncodeFile creates a firmware file from extracted partitions
// Uses the template for structure and replaces partition data where available
func (e *Encoder) EncodeFile(inputDir, outputPath, templatePath string) error {
	// Read original firmware as template
	template, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template: %w", err)
	}

	if e.verbose {
		fmt.Printf("Read template: %d bytes\n", len(template))
	}

	// Decrypt template to get structure
	decrypted, err := AESDecryptCBC(template)
	if err != nil {
		return fmt.Errorf("AES decryption failed: %w", err)
	}

	// Decrypt header
	e.feistel.Decrypt(decrypted[:HeaderSize])

	// Decrypt module header block
	moduleHeaderSize := 0x2000
	modHdr := make([]byte, moduleHeaderSize)
	copy(modHdr, decrypted[HeaderSize:HeaderSize+moduleHeaderSize])
	e.feistel.Decrypt(modHdr)

	// Parse original partition table
	partitions, err := e.parsePartitionTable(modHdr)
	if err != nil {
		return fmt.Errorf("failed to parse partition table: %w", err)
	}

	if e.verbose {
		fmt.Printf("Template has %d partitions\n", len(partitions))
	}

	// Make output copy of decrypted data
	output := make([]byte, len(decrypted))
	copy(output, decrypted)

	// Track which module entries need checksum updates (entry index → true)
	modifiedEntries := make(map[int]bool)

	// Replace partition data with modified versions where available
	for i, p := range partitions {
		if p.Offset == 0 || p.Size == 0 {
			continue
		}

		// Try to find replacement file
		var replacementPath string

		if p.Name == "MAIN" {
			// For MAIN, look for MAIN.bin (raw fma4+fma5+fma6+fma7 data)
			mainPath := filepath.Join(inputDir, "MAIN.bin")
			if _, err := os.Stat(mainPath); err == nil {
				rawData, err := os.ReadFile(mainPath)
				if err != nil {
					fmt.Printf("Warning: failed to read MAIN.bin: %v\n", err)
					continue
				}

				// Encode MAIN.bin with compression and encryption
				if e.verbose {
					fmt.Printf("  Encoding MAIN.bin (%d bytes raw)...\n", len(rawData))
				}

				metadataPath := filepath.Join(inputDir, "MAIN_metadata.json")
				encodedData, err := e.EncodeMainPartition(rawData, metadataPath)
				if err != nil {
					fmt.Printf("Warning: failed to encode MAIN: %v\n", err)
					continue
				}

				// MAIN's declared partition size is itself sometimes short
				// by a few dozen bytes (the same "declared sizes lie"
				// quirk as the cramfs superblock and the decode-side
				// extractPartition fix) — real firmware's own encoded MAIN
				// data can legitimately exceed p.Size, using slack space up
				// to wherever the next partition actually starts. Only
				// truncating/erroring beyond that TRUE physical boundary
				// avoids silently corrupting real compressed data.
				trueLimit := int64(len(output)) - int64(p.Offset)
				for _, other := range partitions {
					if other.Offset > p.Offset && int64(other.Offset-p.Offset) < trueLimit {
						trueLimit = int64(other.Offset - p.Offset)
					}
				}

				if int64(len(encodedData)) > trueLimit {
					return fmt.Errorf("encoded MAIN (%d bytes) exceeds the space available before the next partition (%d bytes) — modified content no longer fits in the original firmware layout", len(encodedData), trueLimit)
				} else if len(encodedData) < int(p.Size) {
					// Pad with 0xFF
					if e.verbose {
						fmt.Printf("  Padding encoded MAIN from %d to %d bytes\n", len(encodedData), p.Size)
					}
					padding := make([]byte, int(p.Size)-len(encodedData))
					for i := range padding {
						padding[i] = 0xFF
					}
					encodedData = append(encodedData, padding...)
				}

				// Copy to output at the actual data position (offset + HeaderSize)
				copy(output[p.Offset:], encodedData)

				// MAIN dataCk (module entry[36:40]) is preserved from template — PROG does
				// not verify it for CompType=2 (LZSS) firmware; per-entry Adler32 handles integrity.
				if e.verbose {
					fmt.Printf("  Replaced MAIN (%d bytes encoded)\n", len(encodedData))
				}
				continue
			}
		} else {
			// For other partitions, look for NAME_VERSION.bin (exact match)
			exactPath := filepath.Join(inputDir, fmt.Sprintf("%s_%s.bin", p.Name, p.Version))
			if _, err := os.Stat(exactPath); err == nil {
				replacementPath = exactPath
			}
		}

		if replacementPath != "" {
			newData, err := os.ReadFile(replacementPath)
			if err != nil {
				fmt.Printf("Warning: failed to read %s: %v\n", replacementPath, err)
				continue
			}

			// Size must match for simple replacement
			if len(newData) != int(p.Size) {
				fmt.Printf("Warning: %s size mismatch (%d vs %d), skipping\n",
					p.Name, len(newData), p.Size)
				continue
			}

			// Feistel encrypt the replacement data
			e.feistel.Encrypt(newData)

			// Copy to output at the partition offset
			copy(output[p.Offset:], newData)

			// Mark entry for checksum update (i is 0-based in partitions slice,
			// but module entries start at index 1 in the header)
			modifiedEntries[i] = true

			if e.verbose {
				fmt.Printf("  Replaced %s (%d bytes)\n", p.Name, len(newData))
			}
		}
	}

	// Update dataCk and EntryCk for modified non-MAIN partitions
	for i, p := range partitions {
		if !modifiedEntries[i] {
			continue
		}
		// Compute dataCk from the output buffer
		dataCk := e.computeDataChecksum(output, p.Offset, p.Size)

		// Update module entry (entry index = i+1 because entry 0 is "$PaT" marker)
		entryIdx := i + 1
		updateModuleEntry(modHdr, entryIdx, dataCk)

		if e.verbose {
			fmt.Printf("  Updated %s dataCk=0x%08X\n", p.Name, dataCk)
		}
	}

	// Re-encrypt module header block
	copy(output[HeaderSize:HeaderSize+moduleHeaderSize], modHdr)
	e.feistel.Encrypt(output[HeaderSize : HeaderSize+moduleHeaderSize])

	// Re-encrypt header
	e.feistel.Encrypt(output[:HeaderSize])

	// AES encrypt entire file
	encrypted, err := AESEncryptCBC(output)
	if err != nil {
		return fmt.Errorf("AES encryption failed: %w", err)
	}

	// Write output
	if err := os.WriteFile(outputPath, encrypted, 0644); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	if e.verbose {
		fmt.Printf("Created %s (%d bytes)\n", outputPath, len(encrypted))
	}

	return nil
}

// parsePartitionTable parses the decrypted module header
func (e *Encoder) parsePartitionTable(modHdr []byte) ([]PartitionInfo, error) {
	var partitions []PartitionInfo

	// Entry 0 is "$PaT" header marker - skip it
	for i := 1; i < 20; i++ {
		offset := i * ModuleEntrySize
		if offset+ModuleEntrySize > len(modHdr) {
			break
		}

		entry := modHdr[offset : offset+ModuleEntrySize]

		name := strings.TrimRight(string(entry[0:4]), "\x00")
		version := strings.TrimRight(string(entry[4:8]), "\x00")

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
	}

	return partitions, nil
}

// computeDataChecksum computes the dataCk for a non-MAIN partition.
// dataCk = Adler32(FeistelDecrypt(output[offset + HeaderSize : offset + HeaderSize + size]))
// The partition offset in module entries is relative to the file header start,
// so the actual partition data begins HeaderSize (0x30) bytes later.
func (e *Encoder) computeDataChecksum(output []byte, offset, size uint32) uint32 {
	actualOff := offset + uint32(HeaderSize)
	data := make([]byte, size)
	copy(data, output[actualOff:actualOff+size])
	fc := NewFeistelCipher()
	fc.Decrypt(data)
	return adler32.Checksum(data)
}

// updateModuleEntry updates the dataCk and EntryCk fields in a module entry.
// EntryCk = Adler32(entry[0:44]).
func updateModuleEntry(modHdr []byte, entryIndex int, dataCk uint32) {
	off := entryIndex * ModuleEntrySize
	binary.LittleEndian.PutUint32(modHdr[off+36:off+40], dataCk)
	// Recompute EntryCk = Adler32 of first 44 bytes
	entryCk := adler32.Checksum(modHdr[off : off+44])
	binary.LittleEndian.PutUint32(modHdr[off+44:off+48], entryCk)
}

