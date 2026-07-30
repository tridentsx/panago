package cramfs

import (
	"crypto/sha256"
	"encoding/json"
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

	// order is the original (non-alphabetical) directory listing order
	// captured at extraction time, keyed by "/"-joined logical path
	// ("" for root). Nil if no sidecar was found (e.g. a user-created tree),
	// in which case entries fall back to alphabetical order.
	order map[string][]string

	// contentCache deduplicates file/symlink data by exact content: real
	// mkcramfs stores identical content (e.g. many "libfoo.so -> libfoo.so.1"
	// style symlinks, or duplicate files) only once and points every
	// matching inode at the same compressed block.
	contentCache map[[32]byte]uint32
}

// Panasonic's mkcramfs writes these fixed UID/GID values into every inode
// (root, directories, files, symlinks alike) rather than real ownership —
// confirmed identical across real fma5/fma7 images with very different content.
const (
	panasonicUID = 41605
	panasonicGID = 100
)

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
	b.contentCache = make(map[[32]byte]uint32)

	b.order = loadOrderSidecar(inputDir)

	// Collect all entries
	entries, err := b.collectEntries(inputDir, "")
	if err != nil {
		return nil, err
	}

	// Two global passes, matching real mkcramfs: first every directory's
	// listing across the whole tree, then every file's/symlink's data.
	rootOffset, rootSize := b.writeDirListings(entries)
	if err := b.writeFileData(entries); err != nil {
		return nil, err
	}

	// Write root inode at offset 64
	rootInode := Inode{
		Mode:    S_IFDIR | 0755,
		UID:     panasonicUID,
		GID:     panasonicGID,
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
	// Total size: Panasonic's build tool writes a fixed placeholder here
	// (0x00010000) rather than the real image size — confirmed identical
	// across differently-sized real fma5/fma7 images.
	endian.PutUint32(b.data[4:8], 0x00010000)
	// Flags = 0 (old format)
	endian.PutUint32(b.data[8:12], 0)
	// Future
	endian.PutUint32(b.data[12:16], 0)
	// Signature
	copy(b.data[16:32], []byte("Compressed ROMFS"))
	// fsid fields (match Panasonic's placeholder values)
	endian.PutUint32(b.data[32:36], 0x60d7f58d)
	endian.PutUint32(b.data[36:40], 0x3bd04c4a)
	endian.PutUint32(b.data[40:44], 0x87bb9a98)
	endian.PutUint32(b.data[44:48], 0x560652c6)
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

	// Set by writeDirListings, consumed by writeFileData/patchInode.
	headerOffset uint32
	namelenField uint8
}

// loadOrderSidecar loads the original directory-listing order captured at
// extraction time, if a sidecar file is present alongside inputDir. Returns
// nil if absent (e.g. a tree that wasn't produced by this package's
// extractor), in which case callers fall back to alphabetical order.
func loadOrderSidecar(inputDir string) map[string][]string {
	data, err := os.ReadFile(orderSidecarPath(inputDir))
	if err != nil {
		return nil
	}
	var order map[string][]string
	if json.Unmarshal(data, &order) != nil {
		return nil
	}
	return order
}

// collectEntries collects directory entries for dirPath, ordering them to
// match the original cramfs image's on-disk listing order (captured via
// orderKey) when available. Panasonic's mkcramfs does not sort entries
// alphabetically — it preserves the original build machine's readdir order —
// so reproducing that order is required for a byte-identical rebuild.
// Entries absent from the captured order (e.g. newly added files) are
// appended afterward, sorted alphabetically.
func (b *Builder) collectEntries(dirPath, orderPath string) ([]entryInfo, error) {
	dirEntries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]os.DirEntry, len(dirEntries))
	var names []string
	for _, de := range dirEntries {
		name := de.Name()
		if name == "." || name == ".." {
			continue
		}
		byName[name] = de
		names = append(names, name)
	}
	sort.Strings(names)

	var orderedNames []string
	used := make(map[string]bool, len(names))
	if captured, ok := b.order[orderPath]; ok {
		for _, name := range captured {
			if byName[name] != nil && !used[name] {
				orderedNames = append(orderedNames, name)
				used[name] = true
			}
		}
	}
	for _, name := range names {
		if !used[name] {
			orderedNames = append(orderedNames, name)
			used[name] = true
		}
	}

	var entries []entryInfo
	for _, name := range orderedNames {
		fullPath := filepath.Join(dirPath, name)
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}

		entry := entryInfo{
			name: name,
			path: fullPath,
			mode: uint16(info.Mode().Perm()) | uint16(uint32(info.Mode()&fs.ModeType)>>16),
			uid:  panasonicUID,
			gid:  panasonicGID,
		}

		// Convert Go file mode to cramfs mode
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			// Symlink permission bits are conventionally always 0777 on
			// Linux/cramfs; macOS's Lstat can report the umask-restricted
			// bits used when the symlink was recreated during extraction,
			// which would not match the original image.
			entry.mode = S_IFLNK | 0777
			entry.isLink = true
			target, err := os.Readlink(fullPath)
			if err == nil {
				entry.target = target
			}

		case info.IsDir():
			entry.mode = S_IFDIR | uint16(info.Mode().Perm())
			entry.isDir = true
			children, err := b.collectEntries(fullPath, orderKey(orderPath, name))
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

// writeDirListings recursively writes every directory's own listing (inode +
// name for each child, in original on-disk order) across the WHOLE subtree —
// a complete depth-first pass that touches every directory but writes no
// file/symlink data at all. Real mkcramfs does this as an entirely separate
// global pass before writing any file content (confirmed: in real images,
// directories many levels deep are laid out contiguously, long before the
// first file's data appears). File/symlink entries get a placeholder here;
// writeFileData fills in their real offset afterward.
func (b *Builder) writeDirListings(entries []entryInfo) (uint32, uint32) {
	if len(entries) == 0 {
		return 0, 0
	}

	b.align4()
	dirOffset := uint32(len(b.data))

	for i := range entries {
		e := &entries[i]
		nameBytes := []byte(e.name)
		namePadded := align4(len(nameBytes))
		e.namelenField = uint8(namePadded / 4)

		e.headerOffset = uint32(len(b.data))
		b.data = append(b.data, make([]byte, 12)...)
		paddedName := make([]byte, namePadded)
		copy(paddedName, nameBytes)
		b.data = append(b.data, paddedName...)
	}

	dirSize := uint32(len(b.data)) - dirOffset

	for i := range entries {
		e := &entries[i]
		if !e.isDir {
			continue
		}
		subOffset, subSize := b.writeDirListings(e.children)
		b.patchInode(e, subOffset, subSize)
	}

	return dirOffset, dirSize
}

// writeFileData recursively writes every file's/symlink's compressed data,
// in the same depth-first, original-listing order as writeDirListings,
// patching each entry's placeholder header. Matches real mkcramfs's second
// global pass, and applies the same global content dedup (writeCompressedDataDedup)
// real images show (e.g. many "libfoo.so -> libfoo.so.N" symlinks across
// unrelated directories sharing one compressed block).
func (b *Builder) writeFileData(entries []entryInfo) error {
	for i := range entries {
		e := &entries[i]
		if e.isDir {
			if err := b.writeFileData(e.children); err != nil {
				return err
			}
			continue
		}

		var data []byte
		if e.isLink {
			data = []byte(e.target)
		} else {
			fileData, err := os.ReadFile(e.path)
			if err != nil {
				continue
			}
			data = fileData
		}

		dataOffset := b.writeCompressedDataDedup(data)
		b.fileCount++
		b.patchInode(e, dataOffset, uint32(len(data)))
	}
	return nil
}

// patchInode writes the final inode for entry e at its reserved header slot.
func (b *Builder) patchInode(e *entryInfo, dataOffset, size uint32) {
	inode := Inode{
		Mode:    e.mode,
		UID:     e.uid,
		GID:     e.gid,
		Size:    size,
		Namelen: e.namelenField,
		Offset:  dataOffset / 4,
	}
	copy(b.data[e.headerOffset:e.headerOffset+12], inode.Serialize(b.config.Endianness))
}

// writeCompressedDataDedup writes compressed data, reusing an existing block
// if identical content (by exact byte match) has already been written
// anywhere in the image — matching real mkcramfs's global content dedup
// (confirmed: many "libfoo.so -> libfoo.so.N" style symlinks in unrelated
// directories share one compressed block in real firmware images).
func (b *Builder) writeCompressedDataDedup(data []byte) uint32 {
	if len(data) == 0 {
		return 0
	}
	key := sha256.Sum256(data)
	if off, ok := b.contentCache[key]; ok {
		return off
	}
	off := b.writeCompressedData(data)
	b.contentCache[key] = off
	return off
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
