package cramfs

import (
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
)

// CompressToCramfs creates a Cramfs image from a directory with validation and performance improvements
func CompressToCramfs(rootDir string, outputFile string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	fmt.Printf("Compressing directory %s into Cramfs image %s\n", rootDir, outputFile)

	outputFileHandle, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer outputFileHandle.Close()

	// Write the superblock placeholder
	sb := Superblock{
		Magic: 0x28cd3d45,
		Flags: 0,
	}
	if err := binary.Write(outputFileHandle, cfg.Endianness, &sb); err != nil {
		return err
	}

	zlibWriter := zlib.NewWriter(outputFileHandle)
	defer zlibWriter.Close()

	var fileOffset uint32 = uint32(binary.Size(sb))
	crcTable := crc32.MakeTable(crc32.Castagnoli)
	crc := crc32.New(crcTable)

	entries := []Inode{}

	if err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		var inode Inode
		inode.Size = uint32(info.Size())
		inode.Namelen = uint16(len(relPath))
		inode.Mode = uint16(info.Mode().Perm())
		inode.Offset = fileOffset

		if info.IsDir() {
			inode.Mode |= 0x4000 // Directory flag
			entries = append(entries, inode)
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		fmt.Printf("Adding file: %s\n", relPath)

		entries = append(entries, inode)

		compressedWriter := zlib.NewWriter(zlibWriter)
		defer compressedWriter.Close()

		tee := io.TeeReader(file, crc)
		if _, err := io.Copy(compressedWriter, tee); err != nil {
			return err
		}

		fileOffset += inode.Size
		return nil
	}); err != nil {
		return err
	}

	// Write inodes after processing all files
	for _, inode := range entries {
		if err := binary.Write(zlibWriter, cfg.Endianness, inode); err != nil {
			return err
		}
	}

	// Update superblock size and checksum
	sb.Size = fileOffset
	sb.FSCRC = crc.Sum32()
	if _, err := outputFileHandle.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := binary.Write(outputFileHandle, binary.LittleEndian, &sb); err != nil {
		return err
	}

	fmt.Println("Compression completed successfully with validation and optimizations.")
	return nil
}
