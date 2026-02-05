package firmware

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
)

// MAIN.bin contains concatenated sub-images at fixed offsets
// These offsets are determined by the firmware structure
const (
	CramfsMagic = 0x28cd3d45
	RomfsMagic  = 0x2d726f6d // "-rom"
)

// MainPartition represents a sub-partition within MAIN.bin
type MainPartition struct {
	Name   string
	Offset int64
	Size   int64
	Type   string // "cramfs", "romfs", "raw"
}

// SplitMainBin splits MAIN.bin into its component images (fma4, fma5, fma6, fma7)
// Partitions are extracted using the space up to the next partition boundary.
// Note: Panasonic cramfs headers have incorrect size fields, so we use boundaries instead.
func SplitMainBin(mainPath, outputDir string) ([]MainPartition, error) {
	data, err := os.ReadFile(mainPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read MAIN.bin: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	// Find all partition start offsets by scanning for magic numbers
	var offsets []struct {
		offset int64
		fsType string
	}

	for i := int64(0); i < int64(len(data))-4; i += 4 {
		magic := binary.LittleEndian.Uint32(data[i : i+4])
		if magic == CramfsMagic {
			offsets = append(offsets, struct {
				offset int64
				fsType string
			}{i, "cramfs"})
		} else if string(data[i:i+4]) == "-rom" {
			offsets = append(offsets, struct {
				offset int64
				fsType string
			}{i, "romfs"})
		}
	}

	var partitions []MainPartition

	// fma4 is the kernel at offset 0 (before first filesystem)
	partNum := 4
	if len(offsets) > 0 && offsets[0].offset > 0 {
		partitions = append(partitions, MainPartition{
			Name:   "fma4",
			Offset: 0,
			Size:   offsets[0].offset,
			Type:   "raw",
		})
		partNum++
	}

	// Process each filesystem - size extends to next partition boundary
	for i, o := range offsets {
		nextBoundary := int64(len(data))
		if i+1 < len(offsets) {
			nextBoundary = offsets[i+1].offset
		}

		partitions = append(partitions, MainPartition{
			Name:   fmt.Sprintf("fma%d", partNum),
			Offset: o.offset,
			Size:   nextBoundary - o.offset,
			Type:   o.fsType,
		})
		partNum++
	}

	// Write each partition to a file
	for _, p := range partitions {
		outPath := filepath.Join(outputDir, p.Name+".bin")
		partData := data[p.Offset : p.Offset+p.Size]

		if err := os.WriteFile(outPath, partData, 0644); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", p.Name, err)
		}

		fmt.Printf("  %s: offset=0x%x, size=%d (%s)\n", p.Name, p.Offset, p.Size, p.Type)
	}

	return partitions, nil
}

// findNextPartition scans for the next cramfs or romfs magic
func findNextPartition(data []byte, start int64) int64 {
	for i := start; i < int64(len(data))-4; i += 4 {
		magic := binary.LittleEndian.Uint32(data[i : i+4])
		if magic == CramfsMagic {
			return i
		}
		if string(data[i:i+4]) == "-rom" {
			return i
		}
	}
	return int64(len(data))
}

// CombineMainBin combines fma4, fma5, fma6, fma7 back into MAIN.bin
// Partitions are concatenated directly - they should already include any needed padding.
func CombineMainBin(inputDir, outputPath string) error {
	var combined []byte

	// Read and concatenate partitions in order
	for i := 4; i <= 7; i++ {
		name := fmt.Sprintf("fma%d.bin", i)
		path := filepath.Join(inputDir, name)

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", name, err)
		}

		fmt.Printf("  %s: offset=0x%x, size=%d bytes\n", name, len(combined), len(data))
		combined = append(combined, data...)
	}

	if err := os.WriteFile(outputPath, combined, 0644); err != nil {
		return fmt.Errorf("failed to write MAIN.bin: %w", err)
	}

	fmt.Printf("Created %s: %d bytes\n", outputPath, len(combined))
	return nil
}

// ExtractFMA5 is a convenience function to extract and split fma5 in one step
func ExtractFMA5(mainPath, outputDir string) error {
	// First split MAIN.bin
	parts, err := SplitMainBin(mainPath, outputDir)
	if err != nil {
		return err
	}

	// Find fma5
	for _, p := range parts {
		if p.Name == "fma5" && p.Type == "cramfs" {
			fmt.Printf("\nfma5 found: %s/fma5.bin (%d bytes)\n", outputDir, p.Size)
			fmt.Printf("Extract with: panago-cli cramfs extract %s/fma5.bin %s/fma5_root/\n",
				outputDir, outputDir)
			return nil
		}
	}

	return fmt.Errorf("fma5 not found in MAIN.bin")
}
