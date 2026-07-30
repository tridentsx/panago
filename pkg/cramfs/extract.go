package cramfs

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Extractor handles cramfs image extraction
type Extractor struct {
	data   []byte
	config *Config

	// order records each directory's child names in on-disk listing order
	// (which is NOT alphabetical in Panasonic's images — it reflects the
	// original build machine's directory order). Keyed by "/"-joined
	// logical path ("" for root), so it can be serialized as a stable
	// sidecar and replayed by Builder to reproduce the original layout.
	order map[string][]string
}

// orderKey joins a logical directory path and child name using "/" — not
// filepath.Join, so the sidecar file is portable across OSes.
func orderKey(base, name string) string {
	if base == "" {
		return name
	}
	return base + "/" + name
}

// NewExtractor creates a new cramfs extractor from file
func NewExtractor(imagePath string, cfg *Config) (*Extractor, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	return NewExtractorFromData(data, cfg)
}

// NewExtractorFromData creates a new cramfs extractor from byte slice
func NewExtractorFromData(data []byte, cfg *Config) (*Extractor, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	if len(data) < 76 { // 64-byte header + 12-byte root inode
		return nil, fmt.Errorf("file too small for cramfs")
	}

	magic := cfg.Endianness.Uint32(data[0:4])
	if magic != Magic {
		return nil, fmt.Errorf("invalid cramfs magic: 0x%08x (expected 0x%08x)", magic, Magic)
	}

	return &Extractor{
		data:   data,
		config: cfg,
	}, nil
}

// ExtractAll extracts all files to the output directory
func (e *Extractor) ExtractAll(outputDir string) error {
	// Parse root inode at offset 64
	root := ParseInode(e.data[64:76], e.config.Endianness)

	if e.config.Debug {
		fmt.Printf("Root: mode=0x%04x, size=%d, offset=0x%x\n",
			root.Mode, root.Size, root.DataOffset())
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	e.order = make(map[string][]string)

	// Extract recursively
	count, err := e.extractDirectory(&root, "", outputDir)
	if err != nil {
		return err
	}

	if e.config.Debug {
		fmt.Printf("Extracted %d files\n", count)
	}

	// Save the original (non-alphabetical) directory order as a sidecar so
	// Builder can reproduce Panasonic's exact on-disk layout on rebuild.
	if err := e.writeOrderSidecar(outputDir); err != nil {
		return fmt.Errorf("failed to write order sidecar: %w", err)
	}

	return nil
}

// orderSidecarPath returns the sidecar path for a given extraction output
// directory: a JSON file alongside the directory, named "<dir>_order.json".
func orderSidecarPath(outputDir string) string {
	return filepath.Clean(outputDir) + "_order.json"
}

// writeOrderSidecar serializes the captured directory order to JSON.
func (e *Extractor) writeOrderSidecar(outputDir string) error {
	data, err := json.MarshalIndent(e.order, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(orderSidecarPath(outputDir), data, 0644)
}

// extractDirectory extracts a directory and its contents
func (e *Extractor) extractDirectory(inode *Inode, basePath, outputDir string) (int, error) {
	if inode.Size == 0 {
		return 0, nil
	}

	dirOffset := inode.DataOffset()
	dirEnd := dirOffset + inode.Size
	offset := dirOffset
	count := 0

	for offset < dirEnd {
		if int(offset)+12 > len(e.data) {
			break
		}

		entry := ParseInode(e.data[offset:offset+12], e.config.Endianness)

		if entry.NamelenBytes() == 0 {
			break
		}

		// Read name
		nameStart := offset + 12
		nameEnd := nameStart + uint32(entry.NamelenBytes())
		if int(nameEnd) > len(e.data) {
			break
		}

		name := strings.TrimRight(string(e.data[nameStart:nameEnd]), "\x00")
		if name == "" {
			break
		}

		fullPath := filepath.Join(basePath, name)
		outPath := filepath.Join(outputDir, fullPath)

		orderPathKey := strings.ReplaceAll(basePath, string(filepath.Separator), "/")
		e.order[orderPathKey] = append(e.order[orderPathKey], name)

		fileType := entry.Mode & 0xF000

		switch fileType {
		case S_IFDIR:
			// Directory. Create permissively first (0755) so children can
			// still be written regardless of the original mode, then apply
			// the real permission bits via Chmod only after recursion
			// completes. A plain MkdirAll(mode) isn't enough on its own:
			// mkdir(2) masks the requested mode by the process umask, so an
			// explicit Chmod is needed to reproduce bits like 0777 exactly
			// (a typical 022 umask would otherwise leave it at 0755).
			if err := os.MkdirAll(outPath, 0755); err != nil {
				return count, err
			}
			subCount, err := e.extractDirectory(&entry, fullPath, outputDir)
			if err != nil {
				return count, err
			}
			count += subCount
			if err := os.Chmod(outPath, os.FileMode(entry.Mode&0777)); err != nil {
				return count, err
			}

		case S_IFREG:
			// Regular file
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return count, err
			}
			data := e.extractFileData(&entry)
			if err := os.WriteFile(outPath, data, os.FileMode(entry.Mode&0777)); err != nil {
				return count, err
			}
			count++

		case S_IFLNK:
			// Symbolic link - data is compressed like files
			if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
				return count, err
			}
			targetData := e.extractFileData(&entry)
			target := strings.TrimRight(string(targetData), "\x00")

			// Remove existing file/link if present
			os.Remove(outPath)

			if err := os.Symlink(target, outPath); err != nil {
				// If symlink fails, save as .symlink file
				os.WriteFile(outPath+".symlink", []byte(target), 0644)
			}
			count++
		}

		offset += 12 + uint32(entry.NamelenBytes())
	}

	return count, nil
}

// extractFileData extracts and decompresses file data
func (e *Extractor) extractFileData(inode *Inode) []byte {
	if inode.Size == 0 {
		return []byte{}
	}

	dataOffset := inode.DataOffset()
	numBlocks := (inode.Size + BlockSize - 1) / BlockSize

	// Read block end pointers
	blockEnds := make([]uint32, numBlocks)
	for i := uint32(0); i < numBlocks; i++ {
		ptrOffset := dataOffset + i*4
		if int(ptrOffset)+4 > len(e.data) {
			return []byte{}
		}
		blockEnds[i] = e.config.Endianness.Uint32(e.data[ptrOffset : ptrOffset+4])
	}

	// Decompress each block
	var output []byte
	ptrTableEnd := dataOffset + numBlocks*4
	prevEnd := ptrTableEnd

	for _, end := range blockEnds {
		if end <= prevEnd || int(end) > len(e.data) {
			break
		}

		blockData := e.data[prevEnd:end]
		prevEnd = end

		if len(blockData) == 0 {
			continue
		}

		decompressed := e.decompressBlock(blockData)
		output = append(output, decompressed...)
	}

	// Truncate to actual size
	if uint32(len(output)) > inode.Size {
		output = output[:inode.Size]
	}

	return output
}

// decompressBlock tries various decompression methods
func (e *Extractor) decompressBlock(data []byte) []byte {
	// Try raw deflate with 16KB window (Panasonic's format)
	if r, err := zlib.NewReaderDict(bytes.NewReader(data), nil); err == nil {
		if decompressed, err := io.ReadAll(r); err == nil {
			r.Close()
			return decompressed
		}
		r.Close()
	}

	// Try with -14 window bits (raw deflate, 16KB)
	// Go's zlib doesn't support negative wbits directly, so we try standard zlib
	if r, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		if decompressed, err := io.ReadAll(r); err == nil {
			r.Close()
			return decompressed
		}
		r.Close()
	}

	// Try raw inflate using flate package
	// For raw deflate streams without zlib header
	r := newRawInflateReader(data)
	if decompressed, err := io.ReadAll(r); err == nil {
		return decompressed
	}

	// Return as-is if decompression fails
	return data
}

// ListFiles returns a list of all files in the cramfs
func (e *Extractor) ListFiles() ([]FileInfo, error) {
	root := ParseInode(e.data[64:76], e.config.Endianness)
	var files []FileInfo
	e.listDirectory(&root, "", &files)
	return files, nil
}

// listDirectory recursively lists directory contents
func (e *Extractor) listDirectory(inode *Inode, basePath string, files *[]FileInfo) {
	if inode.Size == 0 {
		return
	}

	dirOffset := inode.DataOffset()
	dirEnd := dirOffset + inode.Size
	offset := dirOffset

	for offset < dirEnd {
		if int(offset)+12 > len(e.data) {
			break
		}

		entry := ParseInode(e.data[offset:offset+12], e.config.Endianness)

		if entry.NamelenBytes() == 0 {
			break
		}

		nameStart := offset + 12
		nameEnd := nameStart + uint32(entry.NamelenBytes())
		if int(nameEnd) > len(e.data) {
			break
		}

		name := strings.TrimRight(string(e.data[nameStart:nameEnd]), "\x00")
		if name == "" {
			break
		}

		fullPath := filepath.Join(basePath, name)
		fileType := entry.Mode & 0xF000

		info := FileInfo{
			Path:  fullPath,
			Mode:  entry.Mode,
			UID:   entry.UID,
			GID:   entry.GID,
			Size:  entry.Size,
			IsDir: fileType == S_IFDIR,
			IsLink: fileType == S_IFLNK,
		}

		if info.IsLink {
			targetData := e.extractFileData(&entry)
			info.Target = strings.TrimRight(string(targetData), "\x00")
		}

		*files = append(*files, info)

		if fileType == S_IFDIR {
			e.listDirectory(&entry, fullPath, files)
		}

		offset += 12 + uint32(entry.NamelenBytes())
	}
}

// Public API functions

// ListFiles lists all files in a cramfs image
func ListFiles(imagePath string, cfg *Config) error {
	ext, err := NewExtractor(imagePath, cfg)
	if err != nil {
		return err
	}

	files, err := ext.ListFiles()
	if err != nil {
		return err
	}

	for _, f := range files {
		modeStr := fmt.Sprintf("%04o", f.Mode&0777)
		typeChar := "-"
		if f.IsDir {
			typeChar = "d"
		} else if f.IsLink {
			typeChar = "l"
		}

		if f.IsLink && f.Target != "" {
			fmt.Printf("%s%s %5d %s -> %s\n", typeChar, modeStr, f.Size, f.Path, f.Target)
		} else {
			fmt.Printf("%s%s %5d %s\n", typeChar, modeStr, f.Size, f.Path)
		}
	}

	return nil
}

// ExtractFile extracts a single file from cramfs (not implemented - extracts all)
func ExtractFile(imagePath, outputPath string, cfg *Config) error {
	return ExtractAll(imagePath, outputPath, cfg)
}

// ExtractAll extracts all files from a cramfs image
func ExtractAll(imagePath, outputDir string, cfg *Config) error {
	ext, err := NewExtractor(imagePath, cfg)
	if err != nil {
		return err
	}

	return ext.ExtractAll(outputDir)
}
