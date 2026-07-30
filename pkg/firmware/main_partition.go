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

// GetTemplatePartitionSizes decodes a template firmware and returns each
// MAIN sub-partition's total on-disk size (fma4..fma7), keyed by name.
//
// The real firmware allocates each sub-partition a FIXED size (matching the
// actual NAND flash partition table) that's larger than its real content —
// e.g. fma6 (romfs) is allocated exactly 18 MiB, fma7 (cramfs) exactly
// 113.75 MiB, regardless of how much of that space the filesystem actually
// uses. Rebuilding a byte-identical firmware requires padding each rebuilt
// sub-image back out to that same fixed size, which this looks up from the
// template rather than hardcoding (so it stays correct across firmware
// versions/models with different partition tables).
func GetTemplatePartitionSizes(templatePath string) (map[string]int64, error) {
	tempDir, err := os.MkdirTemp("", "panago-template-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	decoder := NewDecoder(false)
	if err := decoder.DecodeFile(templatePath, tempDir); err != nil {
		return nil, fmt.Errorf("failed to decode template: %w", err)
	}

	mainPath := filepath.Join(tempDir, "MAIN.bin")
	parts, err := SplitMainBin(mainPath, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to split template MAIN.bin: %w", err)
	}

	sizes := make(map[string]int64, len(parts))
	for _, p := range parts {
		sizes[p.Name] = p.Size
	}
	return sizes, nil
}

// PadPartitionToSize reproduces the real firmware's reserved-space
// convention for a rebuilt sub-partition: zero-pad the real content up to
// alignBoundary (confirmed against real firmware: cramfs uses its own
// BLOCK_SIZE of 4096; romfs uses 1024, matching genromfs's own dumpall()
// end-of-image alignment), then fill the remaining reserved space up to
// targetSize with 0xFF (the conventional "erased flash" byte value).
//
// Returns an error if the real content (after alignment) already exceeds
// targetSize — meaning the modified content no longer fits in the
// original NAND partition allocation.
func PadPartitionToSize(data []byte, alignBoundary int, targetSize int64) ([]byte, error) {
	if rem := len(data) % alignBoundary; rem != 0 {
		data = append(data, make([]byte, alignBoundary-rem)...)
	}

	if int64(len(data)) > targetSize {
		return nil, fmt.Errorf("content is %d bytes after alignment, exceeds the %d-byte partition allocation", len(data), targetSize)
	}

	if int64(len(data)) < targetSize {
		pad := make([]byte, targetSize-int64(len(data)))
		for i := range pad {
			pad[i] = 0xFF
		}
		data = append(data, pad...)
	}

	return data, nil
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

