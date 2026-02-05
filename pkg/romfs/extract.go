package romfs

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Extractor handles romfs extraction
type Extractor struct {
	data   []byte
	config *Config
	super  *Superblock
}

// NewExtractor creates a new romfs extractor
func NewExtractor(imagePath string, cfg *Config) (*Extractor, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	e := &Extractor{
		data:   data,
		config: cfg,
	}

	if err := e.parseSuper(); err != nil {
		return nil, err
	}

	return e, nil
}

// parseSuper parses the romfs superblock
func (e *Extractor) parseSuper() error {
	if len(e.data) < 16 {
		return fmt.Errorf("file too small for romfs")
	}

	// Check magic
	if string(e.data[0:8]) != Magic {
		return fmt.Errorf("invalid romfs magic: %s", string(e.data[0:8]))
	}

	e.super = &Superblock{}
	copy(e.super.Magic[:], e.data[0:8])
	e.super.Size = binary.BigEndian.Uint32(e.data[8:12])
	e.super.Checksum = binary.BigEndian.Uint32(e.data[12:16])

	// Read volume name (null-terminated, starts at offset 16)
	nameEnd := 16
	for nameEnd < len(e.data) && e.data[nameEnd] != 0 {
		nameEnd++
	}
	e.super.Name = string(e.data[16:nameEnd])

	return nil
}

// GetSuperblock returns the parsed superblock
func (e *Extractor) GetSuperblock() *Superblock {
	return e.super
}

// firstFileOffset returns the offset of the first file header
func (e *Extractor) firstFileOffset() uint32 {
	// First file is after superblock header (16 bytes) + volume name (16-byte aligned)
	nameLen := uint32(len(e.super.Name) + 1) // +1 for null terminator
	return align16(16 + nameLen)
}

// parseFileHeader parses a file header at the given offset
func (e *Extractor) parseFileHeader(offset uint32) (*FileHeader, error) {
	if offset == 0 || int(offset)+16 > len(e.data) {
		return nil, fmt.Errorf("invalid file header offset: %d", offset)
	}

	h := &FileHeader{}
	h.NextOffset = binary.BigEndian.Uint32(e.data[offset : offset+4])
	h.SpecInfo = binary.BigEndian.Uint32(e.data[offset+4 : offset+8])
	h.Size = binary.BigEndian.Uint32(e.data[offset+8 : offset+12])
	h.Checksum = binary.BigEndian.Uint32(e.data[offset+12 : offset+16])

	// Read filename (null-terminated, starts at offset+16)
	nameStart := offset + 16
	nameEnd := nameStart
	for int(nameEnd) < len(e.data) && e.data[nameEnd] != 0 {
		nameEnd++
	}
	h.Name = string(e.data[nameStart:nameEnd])

	// Data starts after the name (16-byte aligned)
	h.DataOffset = align16(nameEnd + 1)

	return h, nil
}

// ListFiles returns a list of all files in the romfs
func (e *Extractor) ListFiles() ([]FileInfo, error) {
	var files []FileInfo

	err := e.walkDirectory(e.firstFileOffset(), "", &files)
	if err != nil {
		return nil, err
	}

	return files, nil
}

// walkDirectory recursively walks a directory
func (e *Extractor) walkDirectory(offset uint32, basePath string, files *[]FileInfo) error {
	for offset != 0 {
		header, err := e.parseFileHeader(offset)
		if err != nil {
			return err
		}

		// Skip . and .. entries
		if header.Name == "." || header.Name == ".." {
			offset = header.Next()
			continue
		}

		path := header.Name
		if basePath != "" {
			path = basePath + "/" + header.Name
		}

		info := FileInfo{
			Path:       path,
			Size:       header.Size,
			FileType:   header.Type(),
			DataOffset: header.DataOffset,
		}

		// Determine file type and mode
		switch header.Type() {
		case TypeDirectory:
			info.IsDir = true
			info.Mode = 0755
		case TypeFile:
			info.IsFile = true
			if header.IsExecutable() {
				info.Mode = 0755
			} else {
				info.Mode = 0644
			}
		case TypeSymlink:
			info.IsLink = true
			info.Mode = 0777
			// Read symlink target
			if header.Size > 0 && int(header.DataOffset+header.Size) <= len(e.data) {
				info.Target = strings.TrimRight(string(e.data[header.DataOffset:header.DataOffset+header.Size]), "\x00")
			}
		case TypeBlockDev, TypeCharDev:
			info.DevMajor = (header.SpecInfo >> 8) & 0xFF
			info.DevMinor = header.SpecInfo & 0xFF
			info.Mode = 0660
		case TypeSocket:
			info.Mode = 0755
		case TypeFifo:
			info.Mode = 0644
		case TypeHardLink:
			info.IsFile = true
			info.Mode = 0644
		}

		*files = append(*files, info)

		// Recurse into directories
		if header.Type() == TypeDirectory && header.SpecInfo != 0 {
			if err := e.walkDirectory(header.SpecInfo, path, files); err != nil {
				return err
			}
		}

		offset = header.Next()
	}

	return nil
}

// ExtractAll extracts all files to the given directory
func (e *Extractor) ExtractAll(outputDir string) error {
	files, err := e.ListFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		outPath := filepath.Join(outputDir, f.Path)

		switch {
		case f.IsDir:
			if err := os.MkdirAll(outPath, os.FileMode(f.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", f.Path, err)
			}

		case f.IsLink:
			// Create parent directory
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return err
			}
			// Remove existing file/link
			os.Remove(outPath)
			if err := os.Symlink(f.Target, outPath); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", f.Path, err)
			}

		case f.IsFile:
			// Create parent directory
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return err
			}

			// Extract file data
			var data []byte
			if f.Size > 0 {
				if int(f.DataOffset+f.Size) > len(e.data) {
					return fmt.Errorf("file %s data extends beyond image", f.Path)
				}
				data = e.data[f.DataOffset : f.DataOffset+f.Size]
			}

			if err := os.WriteFile(outPath, data, os.FileMode(f.Mode)); err != nil {
				return fmt.Errorf("failed to write %s: %w", f.Path, err)
			}

		default:
			// Skip special files (devices, sockets, fifos) with a warning
			if e.config.Debug {
				fmt.Printf("Skipping special file: %s (type %d)\n", f.Path, f.FileType)
			}
		}
	}

	return nil
}

// ExtractAll is a convenience function
func ExtractAll(imagePath, outputDir string, cfg *Config) error {
	e, err := NewExtractor(imagePath, cfg)
	if err != nil {
		return err
	}
	return e.ExtractAll(outputDir)
}

// ListFiles is a convenience function that lists and prints all files
func ListFiles(imagePath string, cfg *Config) error {
	e, err := NewExtractor(imagePath, cfg)
	if err != nil {
		return err
	}

	files, err := e.ListFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		typeChar := '-'
		switch {
		case f.IsDir:
			typeChar = 'd'
		case f.IsLink:
			typeChar = 'l'
		case f.FileType == TypeBlockDev:
			typeChar = 'b'
		case f.FileType == TypeCharDev:
			typeChar = 'c'
		case f.FileType == TypeSocket:
			typeChar = 's'
		case f.FileType == TypeFifo:
			typeChar = 'p'
		}

		if f.IsLink {
			fmt.Printf("%c%04o %8d %s -> %s\n", typeChar, f.Mode, f.Size, f.Path, f.Target)
		} else {
			fmt.Printf("%c%04o %8d %s\n", typeChar, f.Mode, f.Size, f.Path)
		}
	}

	return nil
}
