// Package cramfs provides tools for reading and writing cramfs filesystem images.
// This implementation is compatible with Panasonic's "old cramfs format" (flags=0).
package cramfs

import "encoding/binary"

// Config represents configuration for cramfs operations
type Config struct {
	Endianness binary.ByteOrder
	Debug      bool
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Endianness: binary.LittleEndian,
		Debug:      false,
	}
}

// Constants for cramfs
const (
	Magic     = 0x28cd3d45
	BlockSize = 4096
	MaxPath   = 255
)

// File type constants (mode & 0xF000)
const (
	S_IFDIR  = 0x4000 // Directory
	S_IFREG  = 0x8000 // Regular file
	S_IFLNK  = 0xA000 // Symbolic link
	S_IFBLK  = 0x6000 // Block device
	S_IFCHR  = 0x2000 // Character device
	S_IFIFO  = 0x1000 // FIFO
	S_IFSOCK = 0xC000 // Socket
)

// Superblock represents the 64-byte cramfs superblock
type Superblock struct {
	Magic     uint32   // 0x00: Magic number (0x28cd3d45)
	Size      uint32   // 0x04: Total filesystem size
	Flags     uint32   // 0x08: Feature flags
	Future    uint32   // 0x0C: Reserved
	Signature [16]byte // 0x10: "Compressed ROMFS"
	FSCRC     uint32   // 0x20: Filesystem CRC
	Edition   uint32   // 0x24: Edition number
	Blocks    uint32   // 0x28: Number of blocks
	Files     uint32   // 0x2C: Number of files
	Name      [16]byte // 0x30: Volume name
}

// Inode represents a 12-byte packed cramfs inode
// The cramfs inode is packed as follows:
//   Word 0: mode (16 bits) | uid (16 bits)
//   Word 1: size (24 bits) | gid (8 bits)
//   Word 2: namelen (6 bits) | offset (26 bits)
type Inode struct {
	Mode    uint16 // File mode (type + permissions)
	UID     uint16 // User ID
	Size    uint32 // File size (24 bits, max 16MB)
	GID     uint8  // Group ID
	Namelen uint8  // Name length in 4-byte units
	Offset  uint32 // Data offset in 4-byte units
}

// ParseInode parses a 12-byte cramfs inode from raw bytes
func ParseInode(data []byte, endian binary.ByteOrder) Inode {
	word0 := endian.Uint32(data[0:4])
	word1 := endian.Uint32(data[4:8])
	word2 := endian.Uint32(data[8:12])

	return Inode{
		Mode:    uint16(word0 & 0xFFFF),
		UID:     uint16((word0 >> 16) & 0xFFFF),
		Size:    word1 & 0xFFFFFF,
		GID:     uint8((word1 >> 24) & 0xFF),
		Namelen: uint8(word2 & 0x3F),
		Offset:  (word2 >> 6) & 0x3FFFFFF,
	}
}

// Serialize converts an inode back to 12 bytes
func (i *Inode) Serialize(endian binary.ByteOrder) []byte {
	data := make([]byte, 12)

	word0 := uint32(i.Mode) | (uint32(i.UID) << 16)
	word1 := (i.Size & 0xFFFFFF) | (uint32(i.GID) << 24)
	word2 := uint32(i.Namelen&0x3F) | ((i.Offset & 0x3FFFFFF) << 6)

	endian.PutUint32(data[0:4], word0)
	endian.PutUint32(data[4:8], word1)
	endian.PutUint32(data[8:12], word2)

	return data
}

// NamelenBytes returns the name length in bytes
func (i *Inode) NamelenBytes() int {
	return int(i.Namelen) * 4
}

// DataOffset returns the data offset in bytes
func (i *Inode) DataOffset() uint32 {
	return i.Offset * 4
}

// IsDir returns true if the inode is a directory
func (i *Inode) IsDir() bool {
	return (i.Mode & 0xF000) == S_IFDIR
}

// IsFile returns true if the inode is a regular file
func (i *Inode) IsFile() bool {
	return (i.Mode & 0xF000) == S_IFREG
}

// IsSymlink returns true if the inode is a symbolic link
func (i *Inode) IsSymlink() bool {
	return (i.Mode & 0xF000) == S_IFLNK
}

// FileInfo holds information about a file in the cramfs
type FileInfo struct {
	Path    string
	Mode    uint16
	UID     uint16
	GID     uint8
	Size    uint32
	IsDir   bool
	IsLink  bool
	Target  string // For symlinks
}
