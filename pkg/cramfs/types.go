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

// Superblock represents the cramfs superblock
type Superblock struct {
	Magic     uint32   // Magic number
	Size      uint32   // Size of file system
	Flags     uint32   // Flags
	Future    uint32   // Reserved
	Signature [16]byte // Signature
	FSCRC     uint32   // Filesystem CRC
	Edition   uint32   // Edition number
	Blocks    uint32   // Number of blocks
	Files     uint32   // Number of files
	Name      [16]byte // Volume name
}

// Inode represents a cramfs inode
type Inode struct {
	Mode    uint16 // File mode
	UID     uint16 // User ID
	Size    uint32 // File size
	GID     uint16 // Group ID
	Namelen uint16 // Length of name
	Offset  uint32 // Offset of data
}

// Constants for cramfs
const (
	CRAMFS_MAGIC      = 0x28cd3d45
	CRAMFS_BLOCK_SIZE = 4096
)
