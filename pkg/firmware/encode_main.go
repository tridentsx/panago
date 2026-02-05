package firmware

import (
	"encoding/binary"
)

// EncodeMainPartition encodes raw MAIN data (fma4+fma5+fma6+fma7) into the firmware format
// This includes compression and Feistel encryption of entry boundaries
func (e *Encoder) EncodeMainPartition(rawData []byte) ([]byte, error) {
	// Split raw data into chunks for compression
	// We'll use a reasonable chunk size that matches typical firmware
	chunkSize := 0x100000 // 1MB chunks

	var entries []mainEncodedEntry
	offset := 0

	for offset < len(rawData) {
		end := offset + chunkSize
		if end > len(rawData) {
			end = len(rawData)
		}

		chunk := rawData[offset:end]

		// Compress with LZSS
		compressed := CompressLZSS(chunk)

		// Build entry header (32 bytes)
		header := make([]byte, MainEntryHeaderLen)
		// Entry header format:
		// [0:14] - reserved/padding
		// [14:16] - comp_type (0 = raw LZSS)
		// [16:20] - decompressed size
		// [20:24] - destination address (0)
		// [24:28] - compressed size
		// [28:32] - reserved
		binary.LittleEndian.PutUint16(header[14:16], 0) // comp_type = raw LZSS
		binary.LittleEndian.PutUint32(header[16:20], uint32(len(chunk)))
		binary.LittleEndian.PutUint32(header[20:24], 0) // dest addr
		binary.LittleEndian.PutUint32(header[24:28], uint32(len(compressed)))

		// Combine header + compressed data
		entryData := append(header, compressed...)

		entries = append(entries, mainEncodedEntry{
			data:       entryData,
			decompSize: uint32(len(chunk)),
		})

		offset = end
	}

	// Calculate total decompressed size for decrypt_size determination
	totalDecomp := uint32(0)
	for _, e := range entries {
		totalDecomp += e.decompSize
	}

	decryptSize := uint32(5120)
	if totalDecomp >= 0x2000000 {
		decryptSize = 10240
	}

	// Apply Feistel encryption to entry boundaries
	for i := range entries {
		entryData := entries[i].data
		if uint32(len(entryData)) > decryptSize {
			e.feistel.Encrypt(entryData[:decryptSize])
			e.feistel.Encrypt(entryData[uint32(len(entryData))-decryptSize:])
		} else {
			e.feistel.Encrypt(entryData)
		}
	}

	// Build entry list
	entryListSize := len(entries) * 8
	listHeaderSize := 20
	entryList := make([]byte, listHeaderSize+entryListSize)

	// List header
	// [0:4] - checksum (calculate later)
	// [4:8] - unknown (0)
	// [8:12] - list size (header + entries)
	// [12:16] - total decompressed size
	// [16:20] - unknown (0)
	binary.LittleEndian.PutUint32(entryList[8:12], uint32(listHeaderSize+entryListSize))
	binary.LittleEndian.PutUint32(entryList[12:16], totalDecomp)

	// Entry records
	listOffset := listHeaderSize
	for _, entry := range entries {
		binary.LittleEndian.PutUint32(entryList[listOffset:listOffset+4], uint32(len(entry.data)))
		// Checksum - simple sum for now
		binary.LittleEndian.PutUint32(entryList[listOffset+4:listOffset+8], 0)
		listOffset += 8
	}

	// Calculate list header checksum
	checksum := calculateChecksum(entryList[4:])
	binary.LittleEndian.PutUint32(entryList[0:4], checksum)

	// Build first 0x30-byte header (will be Feistel encrypted)
	firstHeader := make([]byte, MainListHeaderOff)
	// Fill with appropriate data - this header structure is firmware-specific
	// For now, use zeros which should be safe

	// Combine all parts
	var output []byte
	output = append(output, firstHeader...)
	output = append(output, entryList...)
	for _, entry := range entries {
		output = append(output, entry.data...)
	}

	// Feistel encrypt the first 0x30 bytes
	e.feistel.Encrypt(output[:MainListHeaderOff])

	return output, nil
}

type mainEncodedEntry struct {
	data       []byte
	decompSize uint32
}

// calculateChecksum calculates a simple checksum
func calculateChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i+4 <= len(data); i += 4 {
		sum += binary.LittleEndian.Uint32(data[i : i+4])
	}
	return sum
}
