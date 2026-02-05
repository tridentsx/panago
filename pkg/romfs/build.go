package romfs

import (
	"encoding/binary"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Builder handles romfs image creation
type Builder struct {
	config     *Config
	volumeName string
	output     []byte
}

// NewBuilder creates a new romfs builder
func NewBuilder(volumeName string, cfg *Config) *Builder {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if volumeName == "" {
		volumeName = "rom"
	}
	return &Builder{
		config:     cfg,
		volumeName: volumeName,
	}
}

// Build creates a romfs image from a directory
func (b *Builder) Build(inputDir string) ([]byte, error) {
	// Start with superblock placeholder (will fill in size later)
	b.output = make([]byte, 0, 1024*1024) // Pre-allocate 1MB

	// Write magic
	b.output = append(b.output, []byte(Magic)...)

	// Placeholder for size (will update later)
	b.output = append(b.output, 0, 0, 0, 0)

	// Placeholder for checksum (will update later)
	b.output = append(b.output, 0, 0, 0, 0)

	// Write volume name (null-terminated, 16-byte aligned)
	b.output = append(b.output, []byte(b.volumeName)...)
	b.output = append(b.output, 0) // null terminator
	b.padTo16()

	// Build the directory tree starting from root
	if err := b.writeDirectory(inputDir, ""); err != nil {
		return nil, err
	}

	// Update size in header
	binary.BigEndian.PutUint32(b.output[8:12], uint32(len(b.output)))

	// Calculate and update checksum (sum of first 512 bytes as big-endian uint32s)
	checksum := b.calculateChecksum()
	binary.BigEndian.PutUint32(b.output[12:16], checksum)

	return b.output, nil
}

// padTo16 pads output to 16-byte boundary with zeros
func (b *Builder) padTo16() {
	for len(b.output)%16 != 0 {
		b.output = append(b.output, 0)
	}
}

// calculateChecksum calculates the romfs checksum
func (b *Builder) calculateChecksum() uint32 {
	var sum uint32
	end := 512
	if end > len(b.output) {
		end = len(b.output)
	}

	// Temporarily zero out the checksum field for calculation
	origChecksum := binary.BigEndian.Uint32(b.output[12:16])
	binary.BigEndian.PutUint32(b.output[12:16], 0)

	for i := 0; i < end; i += 4 {
		if i+4 <= len(b.output) {
			sum += binary.BigEndian.Uint32(b.output[i : i+4])
		}
	}

	// Restore original (will be overwritten anyway, but for correctness)
	binary.BigEndian.PutUint32(b.output[12:16], origChecksum)

	return -sum // Checksum is negated so that sum of first 512 bytes = 0
}


// fileNode represents a file/directory in the tree we're building
type fileNode struct {
	name     string
	path     string // Full path on disk
	fileInfo fs.FileInfo
	children []*fileNode
}

// writeDirectory writes a directory and its contents
func (b *Builder) writeDirectory(diskPath string, romPath string) error {
	// Build the complete directory tree first
	root, err := b.buildTree(diskPath)
	if err != nil {
		return err
	}

	// Write the tree recursively
	return b.writeNode(root)
}

// buildTree builds a tree of fileNodes from a directory
func (b *Builder) buildTree(diskPath string) (*fileNode, error) {
	info, err := os.Lstat(diskPath)
	if err != nil {
		return nil, err
	}

	node := &fileNode{
		name:     filepath.Base(diskPath),
		path:     diskPath,
		fileInfo: info,
	}

	if !info.IsDir() {
		return node, nil
	}

	entries, err := os.ReadDir(diskPath)
	if err != nil {
		return nil, err
	}

	// Sort entries alphabetically
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, e := range entries {
		childPath := filepath.Join(diskPath, e.Name())
		child, err := b.buildTree(childPath)
		if err != nil {
			return nil, err
		}
		node.children = append(node.children, child)
	}

	return node, nil
}

// writeNode writes a node and all its children (for root directory, we write children only)
func (b *Builder) writeNode(node *fileNode) error {
	// For the root, just write its children
	return b.writeChildren(node.children)
}

// writeChildren writes a list of sibling nodes
func (b *Builder) writeChildren(children []*fileNode) error {
	if len(children) == 0 {
		return nil
	}

	// Track header offsets for each child so we can link them
	headerOffsets := make([]uint32, len(children))

	for i, child := range children {
		headerOffsets[i] = uint32(len(b.output))

		// Calculate next sibling offset (0 if last)
		// We'll update this after writing all siblings
		nextOffset := uint32(0)

		mode := child.fileInfo.Mode()
		var fileType uint32
		var specInfo uint32
		var fileData []byte

		switch {
		case mode.IsDir():
			fileType = TypeDirectory
			// specInfo will be updated after children are written

		case mode&fs.ModeSymlink != 0:
			fileType = TypeSymlink
			target, err := os.Readlink(child.path)
			if err != nil {
				return fmt.Errorf("failed to read symlink %s: %w", child.path, err)
			}
			fileData = []byte(target)

		case mode.IsRegular():
			fileType = TypeFile
			data, err := os.ReadFile(child.path)
			if err != nil {
				return fmt.Errorf("failed to read file %s: %w", child.path, err)
			}
			fileData = data
			if mode&0111 != 0 {
				fileType |= ExecutableBit
			}

		case mode&fs.ModeDevice != 0:
			if mode&fs.ModeCharDevice != 0 {
				fileType = TypeCharDev
			} else {
				fileType = TypeBlockDev
			}

		case mode&fs.ModeSocket != 0:
			fileType = TypeSocket

		case mode&fs.ModeNamedPipe != 0:
			fileType = TypeFifo

		default:
			continue
		}

		// Write header
		header := make([]byte, 16)
		binary.BigEndian.PutUint32(header[0:4], nextOffset|fileType)
		binary.BigEndian.PutUint32(header[4:8], specInfo)
		binary.BigEndian.PutUint32(header[8:12], uint32(len(fileData)))
		b.output = append(b.output, header...)

		// Write name
		b.output = append(b.output, []byte(child.name)...)
		b.output = append(b.output, 0)
		b.padTo16()

		// Update checksum
		b.updateHeaderChecksum(headerOffsets[i])

		// Write file data
		if len(fileData) > 0 {
			b.output = append(b.output, fileData...)
			b.padTo16()
		}
	}

	// Now update the next pointers for all siblings
	for i := 0; i < len(children)-1; i++ {
		nextOffset := headerOffsets[i+1]
		// Read current value to preserve type bits
		current := binary.BigEndian.Uint32(b.output[headerOffsets[i] : headerOffsets[i]+4])
		typeBits := current & 0xF
		binary.BigEndian.PutUint32(b.output[headerOffsets[i]:headerOffsets[i]+4], nextOffset|typeBits)
		b.updateHeaderChecksum(headerOffsets[i])
	}

	// Now write children of directories and update their specInfo
	for i, child := range children {
		if !child.fileInfo.IsDir() || len(child.children) == 0 {
			continue
		}

		childrenStart := uint32(len(b.output))

		// Recursively write this directory's children
		if err := b.writeChildren(child.children); err != nil {
			return err
		}

		// Update specInfo to point to first child
		binary.BigEndian.PutUint32(b.output[headerOffsets[i]+4:headerOffsets[i]+8], childrenStart)
		b.updateHeaderChecksum(headerOffsets[i])
	}

	return nil
}

// updateHeaderChecksum calculates and updates the checksum for a file header
func (b *Builder) updateHeaderChecksum(offset uint32) {
	// Checksum is sum of first 16 bytes (with checksum field as 0)
	// Actually romfs checksum is: sum of all 32-bit words in header such that total = 0
	var sum uint32

	// Zero out checksum field temporarily
	origChecksum := binary.BigEndian.Uint32(b.output[offset+12 : offset+16])
	binary.BigEndian.PutUint32(b.output[offset+12:offset+16], 0)

	// Sum the 16-byte header
	for i := uint32(0); i < 16; i += 4 {
		sum += binary.BigEndian.Uint32(b.output[offset+i : offset+i+4])
	}

	// Checksum makes the sum zero
	checksum := -sum
	binary.BigEndian.PutUint32(b.output[offset+12:offset+16], checksum)

	_ = origChecksum // Suppress unused warning
}

// BuildToFile builds and writes a romfs image to a file
func (b *Builder) BuildToFile(inputDir, outputPath string) error {
	data, err := b.Build(inputDir)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, data, 0644)
}

// CompressToRomfs is a convenience function to create a romfs image
func CompressToRomfs(inputDir, outputPath string, cfg *Config) error {
	builder := NewBuilder("rom", cfg)
	return builder.BuildToFile(inputDir, outputPath)
}
