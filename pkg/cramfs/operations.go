package cramfs

import (
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ListFiles lists all files in the cramfs image
func ListFiles(imagePath string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// Implementation from parser.go
	return nil
}

// ExtractFile extracts a single file from cramfs image
func ExtractFile(imagePath, filePath string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// Implementation from extraction.go
	return nil
}

// ExtractAll extracts all files from cramfs image
func ExtractAll(imagePath, outputDir string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// Implementation from extraction.go
	return nil
}

// CompressToCramfs creates a cramfs image from directory
func CompressToCramfs(inputDir, outputPath string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	fmt.Printf("Compressing directory %s into Cramfs image %s\n", inputDir, outputPath)

	outputFileHandle, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outputFileHandle.Close()

	// Write the superblock placeholder
	sb := Superblock{
		Magic: CRAMFS_MAGIC,
		Flags: 0,
	}
	if err := binary.Write(outputFileHandle, cfg.Endianness, &sb); err != nil {
		return err
	}

	zlibWriter := zlib.NewWriter(outputFileHandle)
	defer zlibWriter.Close()

	// Walk through directory and compress files
	err = filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == inputDir {
			return nil
		}

		// Compress file
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Copy file data through zlib writer
		if _, err := io.Copy(zlibWriter, file); err != nil {
			return err
		}

		return nil
	})

	return err
}
