package firmware

import (
	"encoding/binary"
	"fmt"
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

	// Replace partition data with modified versions where available
	for _, p := range partitions {
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

				encodedData, err := e.EncodeMainPartition(rawData)
				if err != nil {
					fmt.Printf("Warning: failed to encode MAIN: %v\n", err)
					continue
				}

				if len(encodedData) > int(p.Size) {
					fmt.Printf("Warning: encoded MAIN is larger than original (%d > %d), truncating\n",
						len(encodedData), p.Size)
					encodedData = encodedData[:p.Size]
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

				// Copy to output
				copy(output[p.Offset:], encodedData)

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

			// Copy to output
			copy(output[p.Offset:], newData)

			if e.verbose {
				fmt.Printf("  Replaced %s (%d bytes)\n", p.Name, len(newData))
			}
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

// EncodeSimple creates a firmware by re-encrypting modified decrypted data
// This is for when you have a decrypted+modified firmware blob
func (e *Encoder) EncodeSimple(decryptedPath, outputPath string) error {
	// Read decrypted data
	data, err := os.ReadFile(decryptedPath)
	if err != nil {
		return err
	}

	// AES encrypt
	encrypted, err := AESEncryptCBC(data)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, encrypted, 0644)
}
