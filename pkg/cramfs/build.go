package cramfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Builder creates cramfs images
type Builder struct {
	data      []byte
	config    *Config
	fileCount int
}

// NewBuilder creates a new cramfs builder
func NewBuilder(cfg *Config) *Builder {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Builder{
		config: cfg,
	}
}

// Build creates a cramfs image from a directory
func (b *Builder) Build(inputDir string) ([]byte, error) {
	// Reserve space for header (64 bytes) + root inode (12 bytes)
	b.data = make([]byte, 76)

	// Collect all entries
	entries, err := b.collectEntries(inputDir)
	if err != nil {
		return nil, err
	}

	// Build directory structure and file data
	rootOffset, rootSize, err := b.writeDirectory(entries)
	if err != nil {
		return nil, err
	}

	// Write root inode at offset 64
	rootInode := Inode{
		Mode:    S_IFDIR | 0755,
		UID:     0,
		GID:     0,
		Size:    rootSize,
		Namelen: 0,
		Offset:  rootOffset / 4,
	}
	copy(b.data[64:76], rootInode.Serialize(b.config.Endianness))

	// Write header
	b.writeHeader()

	return b.data, nil
}

// writeHeader writes the cramfs superblock
func (b *Builder) writeHeader() {
	endian := b.config.Endianness

	// Magic
	endian.PutUint32(b.data[0:4], Magic)
	// Total size
	endian.PutUint32(b.data[4:8], uint32(len(b.data)))
	// Flags = 0 (old format)
	endian.PutUint32(b.data[8:12], 0)
	// Future
	endian.PutUint32(b.data[12:16], 0)
	// Signature
	copy(b.data[16:32], []byte("Compressed ROMFS"))
	// fsid fields (match Panasonic's placeholder values)
	endian.PutUint32(b.data[32:36], 0x60d7f58d)
	endian.PutUint32(b.data[36:40], 0x3bd04c4a)
	endian.PutUint32(b.data[40:44], 0x879abb9a)
	endian.PutUint32(b.data[44:48], 0x5652c6cf)
	// Name
	copy(b.data[48:64], []byte("Compressed\x00\x00\x00\x00\x00\x00"))
}

// entryInfo holds info about a directory entry
type entryInfo struct {
	name     string
	path     string
	mode     uint16
	uid      uint16
	gid      uint8
	isDir    bool
	isLink   bool
	target   string
	children []entryInfo
}

// collectEntries recursively collects directory entries
func (b *Builder) collectEntries(dirPath string) ([]entryInfo, error) {
	var entries []entryInfo

	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	// Sort entries by name
	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() < dirEntries[j].Name()
	})

	for _, de := range dirEntries {
		name := de.Name()
		if name == "." || name == ".." {
			continue
		}

		fullPath := filepath.Join(dirPath, name)
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}

		entry := entryInfo{
			name: name,
			path: fullPath,
			mode: uint16(info.Mode().Perm()) | uint16(info.Mode()&fs.ModeType)>>16,
			uid:  0, // Could extract from syscall if needed
			gid:  0,
		}

		// Convert Go file mode to cramfs mode
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			entry.mode = S_IFLNK | uint16(info.Mode().Perm())
			entry.isLink = true
			target, err := os.Readlink(fullPath)
			if err == nil {
				entry.target = target
			}

		case info.IsDir():
			entry.mode = S_IFDIR | uint16(info.Mode().Perm())
			entry.isDir = true
			children, err := b.collectEntries(fullPath)
			if err == nil {
				entry.children = children
			}

		case info.Mode().IsRegular():
			entry.mode = S_IFREG | uint16(info.Mode().Perm())

		default:
			continue // Skip special files
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// writeDirectory writes directory entries and returns (offset, size)
func (b *Builder) writeDirectory(entries []entryInfo) (uint32, uint32, error) {
	if len(entries) == 0 {
		return 0, 0, nil
	}

	// First pass: write all file/symlink data and subdirectory contents
	type entryData struct {
		inode     Inode
		nameBytes []byte
	}

	var entryDatas []entryData

	for _, entry := range entries {
		nameBytes := []byte(entry.name)
		namePadded := align4(len(nameBytes))
		namelenField := uint8(namePadded / 4)

		var dataOffset uint32
		var size uint32

		if entry.isDir {
			// Recursively write subdirectory
			subOffset, subSize, err := b.writeDirectory(entry.children)
			if err != nil {
				return 0, 0, err
			}
			dataOffset = subOffset
			size = subSize

		} else if entry.isLink {
			// Write compressed symlink target
			targetBytes := []byte(entry.target)
			dataOffset = b.writeCompressedData(targetBytes)
			size = uint32(len(targetBytes))
			b.fileCount++

		} else {
			// Write compressed file data
			fileData, err := os.ReadFile(entry.path)
			if err != nil {
				continue
			}
			dataOffset = b.writeCompressedData(fileData)
			size = uint32(len(fileData))
			b.fileCount++
		}

		inode := Inode{
			Mode:    entry.mode,
			UID:     entry.uid,
			GID:     entry.gid,
			Size:    size,
			Namelen: namelenField,
			Offset:  dataOffset / 4,
		}

		// Pad name to 4-byte boundary
		paddedName := make([]byte, namePadded)
		copy(paddedName, nameBytes)

		entryDatas = append(entryDatas, entryData{
			inode:     inode,
			nameBytes: paddedName,
		})
	}

	// Second pass: write directory entries
	b.align4()
	dirOffset := uint32(len(b.data))
	var dirSize uint32

	for _, ed := range entryDatas {
		b.data = append(b.data, ed.inode.Serialize(b.config.Endianness)...)
		b.data = append(b.data, ed.nameBytes...)
		dirSize += 12 + uint32(len(ed.nameBytes))
	}

	return dirOffset, dirSize, nil
}

// writeCompressedData writes compressed file data with block pointers
func (b *Builder) writeCompressedData(data []byte) uint32 {
	if len(data) == 0 {
		return 0
	}

	b.align4()
	dataOffset := uint32(len(b.data))

	numBlocks := (len(data) + BlockSize - 1) / BlockSize

	// Reserve space for block pointers
	ptrStart := len(b.data)
	b.data = append(b.data, make([]byte, numBlocks*4)...)

	// Compress and write each block
	blockEnds := make([]uint32, numBlocks)
	for i := 0; i < numBlocks; i++ {
		start := i * BlockSize
		end := start + BlockSize
		if end > len(data) {
			end = len(data)
		}
		blockData := data[start:end]

		compressed := compressBlock(blockData)
		b.data = append(b.data, compressed...)

		blockEnds[i] = uint32(len(b.data))
	}

	// Fill in block pointers
	for i, endPos := range blockEnds {
		b.config.Endianness.PutUint32(b.data[ptrStart+i*4:ptrStart+i*4+4], endPos)
	}

	return dataOffset
}

// align4 pads data to 4-byte boundary
func (b *Builder) align4() {
	for len(b.data)%4 != 0 {
		b.data = append(b.data, 0)
	}
}

// Public API

// CompressToCramfs creates a cramfs image from a directory
func CompressToCramfs(inputDir, outputPath string, cfg *Config) error {
	builder := NewBuilder(cfg)

	data, err := builder.Build(inputDir)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	fmt.Printf("Created %s: %d bytes, %d files\n", outputPath, len(data), builder.fileCount)
	return nil
}
