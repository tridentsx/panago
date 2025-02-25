package cramfs

import (
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Detects if a file is compressed by checking the first two bytes for zlib magic numbers
func isCompressed(file *os.File) (bool, error) {
	var header [2]byte
	_, err := file.Read(header[:])
	if err != nil {
		return false, err
	}

	// Reset file read position
	_, err = file.Seek(-2, io.SeekCurrent)
	if err != nil {
		return false, err
	}

	// Check for zlib magic header
	return header[0] == 0x78 && (header[1] == 0x9C || header[1] == 0xDA), nil
}

// ExtractFile extracts a single file from the Cramfs image with decompression if needed
func ExtractFile(imagePath string, outputPath string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	file, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer file.Close()

	sb, err := ReadSuperblock(file, cfg)
	if err != nil {
		return err
	}

	fmt.Printf("Extracting file from Cramfs image (size: %d bytes)\n", sb.Size)

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	compressed, err := isCompressed(file)
	if err != nil {
		return err
	}

	var reader io.Reader = file
	if compressed {
		fmt.Println("Detected compressed file, decompressing...")
		zlibReader, err := zlib.NewReader(file)
		if err != nil {
			return err
		}
		defer zlibReader.Close()
		reader = zlibReader
	}

	_, err = io.Copy(outFile, reader)
	if err != nil {
		return err
	}

	fmt.Println("File extraction completed successfully.")
	return nil
}

// ExtractAll extracts all files from the Cramfs image while preserving the directory structure and detecting compression
func ExtractAll(imagePath string, outputDir string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	fmt.Println("Extracting all files from Cramfs image...")
	file, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer file.Close()

	sb, err := ReadSuperblock(file, cfg)
	if err != nil {
		return err
	}

	fmt.Printf("Extracting all files (size: %d bytes)\n", sb.Size)

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	// Read and extract each file while preserving directories
	for {
		var inode Inode
		err := binary.Read(file, cfg.Endianness, &inode)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		nameBuf := make([]byte, inode.Namelen)
		_, err = file.Read(nameBuf)
		if err != nil {
			return err
		}
		filename := string(nameBuf)

		if filename == "" {
			filename = fmt.Sprintf("file_%d", inode.Offset) // Default if no name
		}

		outputPath := filepath.Join(outputDir, filename)

		if inode.Mode&0x4000 != 0 { // Directory check
			if err := os.MkdirAll(outputPath, 0755); err != nil {
				return err
			}
			continue
		}

		fmt.Printf("Extracting: %s (size: %d bytes)\n", outputPath, inode.Size)

		// Ensure the parent directories exist
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return err
		}

		outFile, err := os.Create(outputPath)
		if err != nil {
			return err
		}
		defer outFile.Close()

		compressed, err := isCompressed(file)
		if err != nil {
			return err
		}

		var reader io.Reader = file
		if compressed {
			fmt.Println("Detected compressed file, decompressing...")
			zlibReader, err := zlib.NewReader(file)
			if err != nil {
				return err
			}
			defer zlibReader.Close()
			reader = zlibReader
		}

		_, err = io.CopyN(outFile, reader, int64(inode.Size))
		if err != nil {
			return err
		}
	}

	fmt.Println("Full extraction completed successfully while preserving directory structure and handling decompression.")
	return nil
}
