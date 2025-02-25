package cramfs

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// ReadSuperblock reads and validates the Cramfs superblock
func ReadSuperblock(file *os.File, cfg *Config) (*Superblock, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	var sb Superblock
	if err := binary.Read(file, cfg.Endianness, &sb); err != nil {
		return nil, err
	}
	if sb.Magic != 0x28cd3d45 {
		return nil, errors.New("invalid Cramfs magic number")
	}
	return &sb, nil
}

// Aligns fileOffset to the next 4KB boundary
func alignOffset(offset uint32, blockSize uint32) uint32 {
	return (offset + blockSize - 1) &^ (blockSize - 1)
}

// ListFiles lists the files inside a Cramfs image
func ListFiles(imagePath string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	file, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var sb Superblock
	if err := binary.Read(file, cfg.Endianness, &sb); err != nil {
		return err
	}

	if sb.Magic != 0x28cd3d45 {
		return errors.New("invalid Cramfs magic number")
	}

	fmt.Printf("Listing files in Cramfs image: %s\n", imagePath)

	// Read inodes
	for {
		var inode Inode

		// Read inode structure
		err := binary.Read(file, cfg.Endianness, &inode)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Read filename correctly
		nameBuf := make([]byte, inode.Namelen)
		_, err = file.Read(nameBuf)
		if err != nil {
			return err
		}
		filename := string(nameBuf)

		// Align to next 4-byte boundary
		if inode.Namelen%4 != 0 {
			padding := 4 - (inode.Namelen % 4)
			file.Seek(int64(padding), io.SeekCurrent)
		}

		// Determine file type
		fileType := "File"
		if inode.Mode&0x4000 != 0 {
			fileType = "Directory"
		} else if inode.Mode&0xA000 != 0 {
			fileType = "Symlink"
		} else if inode.Mode&0x2000 != 0 {
			fileType = "Device"
		}

		fmt.Printf("%s: %s (size: %d bytes)\n", fileType, filename, inode.Size)
	}

	return nil
}
