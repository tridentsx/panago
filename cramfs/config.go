package cramfs

import "encoding/binary"

// Config represents the configuration options for cramfs operations
type Config struct {
	// Endianness specifies the byte order to use when reading/writing cramfs structures
	// Defaults to binary.LittleEndian if not specified
	Endianness binary.ByteOrder
}

// DefaultConfig returns a new Config with default values
func DefaultConfig() *Config {
	return &Config{
		Endianness: binary.LittleEndian,
	}
}