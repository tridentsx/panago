package romfs

import (
	"encoding/binary"
	"encoding/json"
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

	// order is the original (non-alphabetical) directory listing order
	// captured at extraction time, keyed by "/"-joined logical path
	// ("" for root). Nil if no sidecar was found, in which case entries
	// fall back to alphabetical order.
	order map[string][]string
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

// loadVolnameSidecar loads the original volume name captured at extraction
// time, if present alongside inputDir. Real romfs volume names often embed a
// per-build identifier (e.g. a hex timestamp) that can't be reconstructed
// any other way.
func loadVolnameSidecar(inputDir string) (string, bool) {
	data, err := os.ReadFile(volnameSidecarPath(inputDir))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// loadOrderSidecar loads the original directory-listing order captured at
// extraction time, if present alongside inputDir. Returns nil if absent
// (e.g. a tree not produced by this package's extractor), in which case
// callers fall back to alphabetical order.
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

// Build creates a romfs image from a directory
func (b *Builder) Build(inputDir string) ([]byte, error) {
	if name, ok := loadVolnameSidecar(inputDir); ok {
		b.volumeName = name
	}
	b.order = loadOrderSidecar(inputDir)

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
	if err := b.writeDirectory(inputDir); err != nil {
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

// fileNode represents a file/directory in the tree we're building.
//
// isDot is nonzero for "." (1) / ".." (2) marker nodes injected from the
// order sidecar for NESTED (non-root) directories. genromfs's processdir()
// only synthesizes "." and ".." specially for the root directory; for every
// other directory, whatever "." and ".." entries readdir() naturally
// returned on the original build machine are preserved as-is — at
// WHATEVER position they occurred, not necessarily first — and written out
// as hard-link entries (never as real directory entries, and never
// carrying the executable bit — see dumpnode() in genromfs.c). A marker
// node has no fileInfo/path since it isn't a real file to stat or read.
type fileNode struct {
	name     string
	path     string // Full path on disk
	fileInfo fs.FileInfo
	children []*fileNode
	isDot    int // 0 = normal, 1 = ".", 2 = ".."
}

// writeDirectory writes the root directory and its contents
func (b *Builder) writeDirectory(diskPath string) error {
	root, err := b.buildTree(diskPath, "")
	if err != nil {
		return err
	}

	// The root directory's own listing starts right after the superblock —
	// i.e. wherever we are right now, before writing anything. This also
	// serves as root's canonical offset, since root's own "." entry (its
	// canonical self-reference for children's ".." hardlinks) is the very
	// first thing written into that listing.
	rootOffset := uint32(len(b.output))
	return b.writeChildren(root.children, rootOffset, rootOffset, true)
}

// buildTree builds a tree of fileNodes from a directory, ordering entries to
// match the original romfs image's on-disk listing order (captured via the
// order sidecar) when available. Real mkfs.romfs does not sort entries
// alphabetically — like Panasonic's cramfs tool, it preserves the original
// build machine's readdir order. Entries absent from the captured order
// (e.g. newly added files) are appended afterward, sorted alphabetically.
//
// For nested (non-root) directories, "." and ".." tokens recorded in the
// order sidecar are turned into marker nodes at their original position
// (see fileNode.isDot); at root, they're dropped here since writeDirectory
// already synthesizes root's own "." / ".." explicitly.
func (b *Builder) buildTree(diskPath, orderPath string) (*fileNode, error) {
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

	dirEntries, err := os.ReadDir(diskPath)
	if err != nil {
		return nil, err
	}

	byName := make(map[string]bool, len(dirEntries))
	var names []string
	for _, de := range dirEntries {
		byName[de.Name()] = true
		names = append(names, de.Name())
	}
	sort.Strings(names)

	isRoot := orderPath == ""

	var orderedTokens []string
	used := make(map[string]bool, len(names))
	if captured, ok := b.order[orderPath]; ok {
		for _, tok := range captured {
			if tok == "." || tok == ".." {
				if !isRoot {
					orderedTokens = append(orderedTokens, tok)
				}
				continue
			}
			if byName[tok] && !used[tok] {
				orderedTokens = append(orderedTokens, tok)
				used[tok] = true
			}
		}
	}
	for _, name := range names {
		if !used[name] {
			orderedTokens = append(orderedTokens, name)
			used[name] = true
		}
	}

	for _, tok := range orderedTokens {
		if tok == "." {
			node.children = append(node.children, &fileNode{name: ".", isDot: 1})
			continue
		}
		if tok == ".." {
			node.children = append(node.children, &fileNode{name: "..", isDot: 2})
			continue
		}
		childPath := filepath.Join(diskPath, tok)
		child, err := b.buildTree(childPath, orderKey(orderPath, tok))
		if err != nil {
			return nil, err
		}
		node.children = append(node.children, child)
	}

	return node, nil
}

// orderKey joins a logical directory path and child name using "/" — not
// filepath.Join, so the sidecar file is portable across OSes.
func orderKey(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

// writeChildren writes a directory's listing.
//
// selfCanonical is this directory's own canonical offset: for root, that's
// the offset of root's own "." entry; for a nested directory D, that's the
// offset of D's own header entry as written in D's parent's listing (i.e.
// where you'd land scanning the parent for "D"). It's what this directory's
// OWN "." marker (if any, at root only synthesized, at nested levels drawn
// from the order sidecar) hardlinks to, and what gets passed down as the
// child's parentCanonical.
//
// Each child is written and, if it's a directory, recursed into
// IMMEDIATELY — writing its entire subtree — before moving on to the next
// sibling. This is NOT the same as cramfs's two-global-pass layout: real
// romfs images show a directory's full recursive content landing right
// after its own header, with later siblings displaced far away as a result
// (confirmed against real firmware: a large early subdirectory pushes every
// later sibling's offset to near the end of the image).
func (b *Builder) writeChildren(children []*fileNode, selfCanonical, parentCanonical uint32, isRoot bool) error {
	writeEntry := func(name string, fileType uint32, specInfo uint32, fileData []byte) uint32 {
		headerOffset := uint32(len(b.output))
		header := make([]byte, 16)
		binary.BigEndian.PutUint32(header[0:4], fileType) // next offset patched in below
		binary.BigEndian.PutUint32(header[4:8], specInfo)
		binary.BigEndian.PutUint32(header[8:12], uint32(len(fileData)))
		b.output = append(b.output, header...)
		b.output = append(b.output, []byte(name)...)
		b.output = append(b.output, 0)
		b.padTo16()
		if len(fileData) > 0 {
			b.output = append(b.output, fileData...)
			b.padTo16()
		}
		return headerOffset
	}

	var prevHeaderOffset uint32
	havePrev := false
	link := func(headerOffset uint32) {
		if havePrev {
			current := binary.BigEndian.Uint32(b.output[prevHeaderOffset : prevHeaderOffset+4])
			typeBits := current & 0xF
			binary.BigEndian.PutUint32(b.output[prevHeaderOffset:prevHeaderOffset+4], headerOffset|typeBits)
			b.updateHeaderChecksum(prevHeaderOffset)
		}
		prevHeaderOffset = headerOffset
		havePrev = true
	}

	if isRoot {
		// Directories are conventionally "executable" (traversable), and
		// real images set this bit even on the pseudo "." entry — but not
		// on "..", which uses TypeHardLink with no exec bit (confirmed
		// against real firmware).
		dotOffset := writeEntry(".", TypeDirectory|ExecutableBit, selfCanonical, nil)
		link(dotOffset)
		b.updateHeaderChecksum(dotOffset)

		dotdotOffset := writeEntry("..", TypeHardLink, parentCanonical, nil)
		link(dotdotOffset)
		b.updateHeaderChecksum(dotdotOffset)
	}

	for _, child := range children {
		if child.isDot != 0 {
			// Nested "." / ".." are always hard links (never real
			// directory entries, never carrying the exec bit — see
			// dumpnode()'s "Don't allow hardlinks to convey attributes").
			target := selfCanonical
			if child.isDot == 2 {
				target = parentCanonical
			}
			offset := writeEntry(child.name, TypeHardLink, target, nil)
			link(offset)
			b.updateHeaderChecksum(offset)
			continue
		}

		mode := child.fileInfo.Mode()
		var fileType uint32
		var fileData []byte

		switch {
		case mode.IsDir():
			fileType = TypeDirectory | ExecutableBit

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

		headerOffset := writeEntry(child.name, fileType, 0, fileData)
		link(headerOffset)
		b.updateHeaderChecksum(headerOffset)

		if mode.IsDir() {
			// specInfo points to where this child's own listing starts;
			// its canonical offset (what ITS children's ".." should
			// hardlink to) is its own header position instead — the two
			// coincide only for root, which has no separate header entry
			// of its own.
			childListOffset := uint32(len(b.output))
			binary.BigEndian.PutUint32(b.output[headerOffset+4:headerOffset+8], childListOffset)
			b.updateHeaderChecksum(headerOffset)
			if err := b.writeChildren(child.children, headerOffset, selfCanonical, false); err != nil {
				return err
			}
		}
	}

	return nil
}

// updateHeaderChecksum calculates and updates the checksum for a file
// header. Per romfs.txt: the checksum covers "the meta data, including the
// file name, and padding" — i.e. the 16-byte header PLUS the name padded to
// a 16-byte boundary, not just the header alone.
func (b *Builder) updateHeaderChecksum(offset uint32) {
	nameStart := offset + 16
	nameEnd := nameStart
	for b.output[nameEnd] != 0 {
		nameEnd++
	}
	metaLen := 16 + align16(nameEnd-nameStart+1)

	var sum uint32
	binary.BigEndian.PutUint32(b.output[offset+12:offset+16], 0)
	for i := uint32(0); i < metaLen; i += 4 {
		sum += binary.BigEndian.Uint32(b.output[offset+i : offset+i+4])
	}

	checksum := -sum
	binary.BigEndian.PutUint32(b.output[offset+12:offset+16], checksum)
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
