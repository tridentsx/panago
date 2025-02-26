package cramfs

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func ReadSuperblock(file *os.File, cfg *Config) (*Superblock, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Try to read with current endianness
	var sb Superblock
	if err := binary.Read(file, cfg.Endianness, &sb); err != nil {
		return nil, fmt.Errorf("failed to read superblock: %w", err)
	}

	// Check if we need to swap endianness
	if sb.Magic != CRAMFS_MAGIC {
		// Try the opposite endianness
		if cfg.Endianness == binary.LittleEndian {
			cfg.Endianness = binary.BigEndian
		} else {
			cfg.Endianness = binary.LittleEndian
		}

		// Seek back to start of file
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to seek to start: %w", err)
		}

		// Read again with new endianness
		if err := binary.Read(file, cfg.Endianness, &sb); err != nil {
			return nil, fmt.Errorf("failed to read superblock with alternate endianness: %w", err)
		}

		// Check magic number again
		if sb.Magic != CRAMFS_MAGIC {
			return nil, fmt.Errorf("invalid Cramfs magic number with both endianness: got %#x, want %#x",
				sb.Magic, CRAMFS_MAGIC)
		}
	}

	// Validate Magic Number
	if sb.Magic != CRAMFS_MAGIC {
		return nil, fmt.Errorf("invalid Cramfs magic number: got %#x, want %#x",
			sb.Magic, CRAMFS_MAGIC)
	}

	// Trim null bytes from signature and name
	signature := string(bytes.TrimRight(sb.Signature[:], "\x00"))
	name := string(bytes.TrimRight(sb.Name[:], "\x00"))

	// Basic validation
	if sb.Size == 0 {
		return nil, errors.New("invalid filesystem size")
	}

	// Print Superblock Information
	fmt.Printf("Cramfs Filesystem Details:\n")
	fmt.Printf("  Magic: %#x\n", sb.Magic)
	fmt.Printf("  Size: %d bytes\n", sb.Size)
	fmt.Printf("  Flags: %#x\n", sb.Flags)
	fmt.Printf("  Signature: %q\n", signature)
	fmt.Printf("  CRC: %#x\n", sb.FSCRC)
	fmt.Printf("  Edition: %d\n", sb.Edition)
	fmt.Printf("  Blocks: %d\n", sb.Blocks)
	fmt.Printf("  Files: %d\n", sb.Files)
	fmt.Printf("  Name: %q\n", name)

	return &sb, nil
}

// Aligns fileOffset to the next 4KB boundary
func alignOffset(offset uint32, blockSize uint32) uint32 {
	return (offset + blockSize - 1) &^ (blockSize - 1)
}

// Add this helper function
func alignTo4(n uint32) uint32 {
	return (n + 3) &^ 3
}

// Add this function to check if a file is likely a cramfs image
func IsLikelyCramfs(file *os.File) (bool, uint32, error) {
	// Save current position
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, 0, err
	}
	defer file.Seek(currentPos, io.SeekStart)

	// Check for cramfs magic at different offsets
	possibleOffsets := []int64{0, 512, 1024, 2048, 4096}

	for _, offset := range possibleOffsets {
		// Seek to the potential offset
		_, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			continue
		}

		// Read magic number
		var magic uint32
		err = binary.Read(file, binary.LittleEndian, &magic)
		if err != nil {
			continue
		}

		// Check if it's cramfs magic (in either endianness)
		if magic == CRAMFS_MAGIC || magic == 0x453dcd28 {
			return true, uint32(offset), nil
		}
	}

	return false, 0, nil
}

// Modify the ListFiles function to use a more robust approach
func ListFiles(imagePath string, cfg *Config) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	file, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Check if this is likely a cramfs image and find the offset
	isCramfs, offset, err := IsLikelyCramfs(file)
	if err != nil {
		return err
	}

	if !isCramfs {
		return fmt.Errorf("file does not appear to be a valid cramfs image")
	}

	fmt.Printf("Found cramfs signature at offset: %d (0x%x)\n", offset, offset)

	// Seek to the start of the cramfs image
	_, err = file.Seek(int64(offset), io.SeekStart)
	if err != nil {
		return err
	}

	// Call ReadSuperblock to read and validate the superblock
	sb, err := ReadSuperblock(file, cfg)
	if err != nil {
		return err
	}

	// Add size validation
	if err := ValidateCramfs(sb, file); err != nil {
		return fmt.Errorf("invalid cramfs image: %w", err)
	}

	fmt.Printf("Listing files in Cramfs image: %s\n", imagePath)
	fmt.Printf("Filesystem Size: %d bytes, Blocks: %d, Files: %d\n",
		sb.Size, sb.Blocks, sb.Files)

	// After reading superblock
	offset = uint32(binary.Size(Superblock{}))

	// Add debug output
	fmt.Printf("Starting file listing at offset: %d (0x%x)\n", offset, offset)

	// Add a limit to prevent infinite loops
	maxEntries := uint32(1000) // Reasonable limit
	entryCount := uint32(0)

	for offset < sb.Size && entryCount < maxEntries {
		entryCount++

		// Print current offset for debugging
		fmt.Printf("\nProcessing entry %d at offset: %d (0x%x)\n", entryCount, offset, offset)

		// Read the inode
		var inode Inode
		if _, err := file.Seek(int64(offset), io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek to offset %d: %w", offset, err)
		}

		// Dump the raw bytes for debugging
		rawBytes := make([]byte, 24) // Size of inode structure
		n, err := file.Read(rawBytes)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read raw bytes at offset %d: %w", offset, err)
		}

		fmt.Printf("Raw inode bytes: ")
		for i := 0; i < n; i++ {
			fmt.Printf("%02x ", rawBytes[i])
		}
		fmt.Println()

		// Seek back to read the inode properly
		if _, err := file.Seek(int64(offset), io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek back to offset %d: %w", offset, err)
		}

		if err := binary.Read(file, cfg.Endianness, &inode); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to read inode at offset %d: %w", offset, err)
		}

		// Print inode details for debugging
		fmt.Printf("Inode: Mode=%04x UID=%d Size=%d GID=%d Namelen=%d Offset=%d\n",
			inode.Mode, inode.UID, inode.Size, inode.GID, inode.Namelen, inode.Offset)

		// Sanity check the inode values
		if inode.Size > 100*1024*1024 { // 100MB is probably too large
			fmt.Printf("Warning: Suspiciously large file size: %d bytes\n", inode.Size)
		}

		// Extract the actual name length (only lower 6 bits are used)
		namelen := inode.Namelen & 0x3F

		// Validate name length more strictly
		if namelen == 0 || namelen > 255 {
			fmt.Printf("Warning: Invalid filename length %d at offset %d, skipping to next 4-byte boundary\n",
				namelen, offset)
			// Try to recover by skipping to the next 4-byte boundary
			offset = (offset + 4) & ^uint32(3)
			continue
		}

		// Read filename with extra validation
		nameBuf := make([]byte, namelen)
		bytesRead, err := io.ReadFull(file, nameBuf)
		if err != nil {
			fmt.Printf("Warning: Failed to read filename (read %d of %d bytes): %v\n",
				bytesRead, namelen, err)
			// Try to recover by skipping to the next 4-byte boundary
			offset = (offset + 4) & ^uint32(3)
			continue
		}

		// Print raw filename bytes for debugging
		fmt.Printf("Raw filename bytes: ")
		for i := 0; i < bytesRead; i++ {
			fmt.Printf("%02x ", nameBuf[i])
		}
		fmt.Println()

		// Clean the filename and check for printable characters
		filename := string(bytes.TrimRight(nameBuf, "\x00"))
		printableCount := 0
		for _, c := range filename {
			if c >= 32 && c <= 126 { // ASCII printable range
				printableCount++
			}
		}

		// If less than half the characters are printable, this is probably not a valid filename
		if float64(printableCount) < float64(len(filename))*0.5 {
			fmt.Printf("Warning: Filename contains too many non-printable characters, likely corrupt\n")
		}

		// Determine file type
		var fileType string
		switch inode.Mode & 0xF000 {
		case 0x4000:
			fileType = "Directory"
		case 0x8000:
			fileType = "File"
		case 0xA000:
			fileType = "Symlink"
		case 0x6000:
			fileType = "Block Device"
		case 0x2000:
			fileType = "Character Device"
		case 0x1000:
			fileType = "FIFO"
		default:
			fileType = "Unknown"
		}

		fmt.Printf("%s: %s (size: %d bytes, mode: %o)\n",
			fileType, filename, inode.Size, inode.Mode)

		// Calculate next inode offset with extra validation
		entrySize := uint32(binary.Size(inode)) + alignTo4(uint32(namelen))
		if entrySize < 4 || entrySize > 1024 { // Sanity check
			fmt.Printf("Warning: Suspicious entry size: %d bytes, using minimum 4 bytes\n", entrySize)
			entrySize = 4 // Minimum reasonable size
		}

		offset += entrySize

		// Add this to the ListFiles function after the first few failed entries
		// Around line 180, after the first few entries have been processed
		if entryCount == 5 {
			// Try alternative inode format
			TryAlternativeInodeFormat(file, offset, cfg)

			// Try scanning for directory entries
			fmt.Println("\nScanning for potential directory entries...")

			// Look for common directory names in the file
			dirNames := []string{"bin", "etc", "lib", "sbin", "usr", "var", "dev", "proc", "sys"}

			// Save current position
			currentPos, _ := file.Seek(0, io.SeekCurrent)

			// Scan the file for directory names
			buf := make([]byte, 4096)
			for offset := int64(0); offset < 1024*1024; offset += 4096 {
				if _, err := file.Seek(offset, io.SeekStart); err != nil {
					break
				}

				n, err := file.Read(buf)
				if err != nil || n == 0 {
					break
				}

				// Convert to string for easier searching
				content := string(buf[:n])

				// Check for directory names
				for _, dirName := range dirNames {
					if idx := strings.Index(content, dirName); idx >= 0 {
						fmt.Printf("Found potential directory '%s' at offset: %d\n",
							dirName, offset+int64(idx))
					}
				}
			}

			// Restore position
			file.Seek(currentPos, io.SeekStart)
		}
	}

	if entryCount >= maxEntries {
		fmt.Printf("\nReached maximum entry limit (%d). Stopping to prevent infinite loop.\n", maxEntries)
	}

	return nil
}

func ValidateCramfs(sb *Superblock, file *os.File) error {
	// Check magic number first
	if sb.Magic != CRAMFS_MAGIC {
		return fmt.Errorf("invalid magic number: got %#x, want %#x", sb.Magic, CRAMFS_MAGIC)
	}

	// Get actual file size
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file size: %w", err)
	}
	actualSize := fileInfo.Size()

	// Print size information
	fmt.Printf("\nSize Analysis:\n")
	fmt.Printf("  Superblock size field: %d bytes\n", sb.Size)
	fmt.Printf("  Actual file size: %d bytes\n", actualSize)

	// Check size constraints with warnings instead of errors
	if sb.Size < uint32(binary.Size(Superblock{})) {
		fmt.Printf("Warning: declared size (%d) is smaller than superblock size (%d)\n",
			sb.Size, binary.Size(Superblock{}))
	}

	if uint64(sb.Size) > uint64(actualSize) {
		fmt.Printf("Warning: declared size (%d) is larger than file size (%d)\n",
			sb.Size, actualSize)
	}

	// More reasonable limits for files and blocks
	maxPossibleFiles := sb.Size / 64 // Assume minimum file size of 64 bytes
	if sb.Files > maxPossibleFiles {
		fmt.Printf("Warning: number of files (%d) seems high for filesystem size (%d), max reasonable: %d\n",
			sb.Files, sb.Size, maxPossibleFiles)
	}

	maxPossibleBlocks := (sb.Size / CRAMFS_BLOCK_SIZE) + 1
	if sb.Blocks > maxPossibleBlocks {
		fmt.Printf("Warning: number of blocks (%d) seems high, max possible: %d\n",
			sb.Blocks, maxPossibleBlocks)
	}

	// Validate signature
	signature := string(bytes.TrimRight(sb.Signature[:], "\x00"))
	if signature != "Compressed ROMFS" {
		return fmt.Errorf("invalid signature: %q", signature)
	}

	// Print detailed debug info
	fmt.Printf("\nDetailed Superblock Analysis:\n")
	fmt.Printf("  Block size: %d bytes\n", CRAMFS_BLOCK_SIZE)
	fmt.Printf("  Max possible blocks: %d\n", maxPossibleBlocks)
	fmt.Printf("  Bytes per file (average): %.2f\n",
		float64(sb.Size)/float64(sb.Files))
	fmt.Printf("  Files per block (average): %.2f\n",
		float64(sb.Files)/float64(sb.Blocks))

	// Look for another cramfs image
	buf := make([]byte, 4)
	for offset := int64(512); offset < actualSize-4; offset += 512 {
		if _, err := file.ReadAt(buf, offset); err != nil {
			break
		}
		if binary.BigEndian.Uint32(buf) == CRAMFS_MAGIC {
			fmt.Printf("\nFound another potential cramfs image at offset: %d\n", offset)
		}
	}

	return nil
}

// Add this function to try different inode formats
func TryAlternativeInodeFormat(file *os.File, offset uint32, cfg *Config) error {
	// Seek to the offset
	if _, err := file.Seek(int64(offset), io.SeekStart); err != nil {
		return err
	}

	fmt.Println("\nTrying alternative inode format...")

	// Define a simpler inode structure that might match the custom format
	type SimpleInode struct {
		Mode    uint16
		UID     uint16
		Size    uint32
		GID     uint16
		Namelen uint8
		Offset  uint32
	}

	var inode SimpleInode
	if err := binary.Read(file, cfg.Endianness, &inode); err != nil {
		return err
	}

	fmt.Printf("Alternative inode: Mode=%04x UID=%d Size=%d GID=%d Namelen=%d Offset=%d\n",
		inode.Mode, inode.UID, inode.Size, inode.GID, inode.Namelen, inode.Offset)

	// Try to read the name
	if inode.Namelen > 0 && inode.Namelen <= 255 {
		nameBuf := make([]byte, inode.Namelen)
		if _, err := io.ReadFull(file, nameBuf); err == nil {
			fmt.Printf("Filename: %s\n", string(bytes.TrimRight(nameBuf, "\x00")))
		}
	}

	return nil
}
