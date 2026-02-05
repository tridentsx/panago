// Package romfs provides tools for reading and writing romfs filesystem images.
// romfs is a simple read-only filesystem with no compression.
package romfs

import "encoding/binary"

// Constants for romfs
const (
	Magic     = "-rom1fs-"
	Alignment = 16 // All structures are 16-byte aligned
)

// File type constants (low 4 bits of NextOffset field)
const (
	TypeHardLink  = 0
	TypeDirectory = 1
	TypeFile      = 2
	TypeSymlink   = 3
	TypeBlockDev  = 4
	TypeCharDev   = 5
	TypeSocket    = 6
	TypeFifo      = 7
)

// Executable bit in NextOffset field
const ExecutableBit = 0x8

// Superblock represents the romfs superblock
type Superblock struct {
	Magic    [8]byte // "-rom1fs-"
	Size     uint32  // Full filesystem size (big-endian)
	Checksum uint32  // Checksum of first 512 bytes
	Name     string  // Volume name (null-terminated, 16-byte aligned)
}

// FileHeader represents a romfs file header
type FileHeader struct {
	NextOffset uint32 // Next file header offset (16-byte aligned) | type in low 4 bits
	SpecInfo   uint32 // For dirs: first file offset; for links: target offset; for devs: dev number
	Size       uint32 // File size
	Checksum   uint32 // Checksum of header (first 16 bytes)
	Name       string // Filename (null-terminated, 16-byte aligned)
	DataOffset uint32 // Offset where file data starts
}

// Type returns the file type (low 4 bits of NextOffset)
func (h *FileHeader) Type() int {
	return int(h.NextOffset & 0x7)
}

// IsExecutable returns true if the executable bit is set
func (h *FileHeader) IsExecutable() bool {
	return h.NextOffset&ExecutableBit != 0
}

// Next returns the offset of the next file header (masked off type bits)
func (h *FileHeader) Next() uint32 {
	return h.NextOffset &^ 0xF
}

// Config represents configuration for romfs operations
type Config struct {
	Endianness binary.ByteOrder
	Debug      bool
}

// DefaultConfig returns default configuration (big-endian for romfs)
func DefaultConfig() *Config {
	return &Config{
		Endianness: binary.BigEndian,
		Debug:      false,
	}
}

// FileInfo holds information about a file in the romfs
type FileInfo struct {
	Path       string
	Size       uint32
	Mode       uint32
	IsDir      bool
	IsLink     bool
	IsFile     bool
	Target     string // For symlinks
	DevMajor   uint32 // For device nodes
	DevMinor   uint32
	FileType   int
	DataOffset uint32
}

// align16 rounds up to the next 16-byte boundary
func align16(n uint32) uint32 {
	return (n + 15) &^ 15
}
