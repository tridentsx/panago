package firmware

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/adler32"
	"os"
	"sync"
)

// EncodeMainPartition encodes raw MAIN data into the firmware format.
// If metadataPath points to a valid MAIN_metadata.json, it uses the original
// structural parameters for a faithful encoding. Otherwise falls back to legacy.
func (e *Encoder) EncodeMainPartition(rawData []byte, metadataPath string) ([]byte, error) {
	if metadataPath != "" {
		meta, err := loadMainMetadata(metadataPath)
		if err == nil {
			if e.verbose {
				fmt.Printf("  Using MAIN metadata (%d entries, chunk=%d)\n", meta.EntryCount, meta.ChunkSize)
			}
			return e.encodeMainPartitionWithMetadata(rawData, meta)
		}
		if e.verbose {
			fmt.Printf("  MAIN metadata not available (%v), using legacy encoding\n", err)
		}
	}
	return e.encodeMainPartitionLegacy(rawData)
}

// loadMainMetadata loads the MAIN_metadata.json sidecar file
func loadMainMetadata(path string) (*MainPartitionMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta MainPartitionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.EntryCount == 0 || meta.ChunkSize == 0 {
		return nil, fmt.Errorf("metadata has no entries or zero chunk size")
	}
	return &meta, nil
}

// estimateBufferConstant computes a conservative (deliberately generous)
// BufferConstant for a chunk whose decompressed size doesn't match any
// value captured from a real firmware image.
//
// The real formula is unconfirmed: BufferConstant depends only on an
// entry's decompressed size (not on how well it happens to compress — all
// full 16MB chunks in one real firmware shared an identical BufferConstant
// despite very different actual compressed sizes), and appears to scale
// roughly proportionally, but the only two real data points available
// (16777216 -> 18846664, ratio ~1.1233; 7864320 -> 8888808, ratio ~1.1303)
// don't pin down an exact formula. Rather than guess low and risk an
// under-sized allocation on the real device, this scales up using the
// larger of the two observed ratios plus a further safety margin — trading
// some unused reserved space for confidence that we never undershoot.
//
// If you have another real Panasonic UniPhier firmware image (different
// version or model) with a different last-chunk size, extracting it adds a
// third (decompSize, BufferConstant) data point, which may be enough to
// replace this estimate with a confirmed formula.
func estimateBufferConstant(decompSize uint32) uint32 {
	const safetyRatio = 1.20
	return uint32(float64(decompSize) * safetyRatio)
}

// align4 rounds up to next multiple of 4
func align4(n uint32) uint32 {
	return (n + 3) &^ 3
}

// align8 rounds up to next multiple of 8
func align8(n uint32) uint32 {
	return (n + 7) &^ 7
}

// buildEntryHeader constructs a 64-byte entry header from structural parameters.
// FooterOffset = 64 + align4(compSize)
// Slack = bufferConstant - FooterOffset
func buildEntryHeader(signature []byte, compType uint16, decompSize, compSize, bufferConstant uint32, checksumFlag uint8) []byte {
	hdr := make([]byte, MainEntryHeaderLen)

	// [0:14] Signature
	copy(hdr[:14], signature)

	// [14:16] CompType
	binary.LittleEndian.PutUint16(hdr[14:16], compType)

	// [16:20] DecompSize
	binary.LittleEndian.PutUint32(hdr[16:20], decompSize)

	// [20:24] DestAddr = 0
	binary.LittleEndian.PutUint32(hdr[20:24], 0)

	// [24:28] CompSize
	binary.LittleEndian.PutUint32(hdr[24:28], compSize)

	// Compute FooterOffset and Slack
	footerOff := uint32(MainEntryHeaderLen) + align4(compSize)
	slack := bufferConstant - footerOff

	// [28:32] Slack (buffer unused space)
	binary.LittleEndian.PutUint32(hdr[28:32], slack)

	// [32:36] FooterOffset
	binary.LittleEndian.PutUint32(hdr[32:36], footerOff)

	// [36:40] BaseAddr = 0
	binary.LittleEndian.PutUint32(hdr[36:40], 0)

	// [40:44] Checksum placeholder — caller sets via computeHdrChecksum(decompressedData)
	binary.LittleEndian.PutUint32(hdr[40:44], 0)

	// [44] ChecksumFlag
	hdr[44] = checksumFlag

	// [45:64] Unused/padding — leave zeroed

	return hdr
}

// computeHdrChecksum computes the entry header checksum: Adler32 of every 16th byte of decompressed data.
func computeHdrChecksum(decompData []byte) uint32 {
	strided := make([]byte, 0, len(decompData)/16+1)
	for i := 0; i < len(decompData); i += 16 {
		strided = append(strided, decompData[i])
	}
	return adler32.Checksum(strided)
}

// encodeMainPartitionWithMetadata re-encodes MAIN using structural parameters from metadata
func (e *Encoder) encodeMainPartitionWithMetadata(rawData []byte, meta *MainPartitionMetadata) ([]byte, error) {
	// Decode structural parameters
	signature, err := hex.DecodeString(meta.EntrySignature)
	if err != nil || len(signature) < 14 {
		return nil, fmt.Errorf("invalid entry_signature in metadata")
	}

	chunkSize := int(meta.ChunkSize)
	entryCount := meta.EntryCount

	// Determine decrypt size from the list header's DecompSize field
	// (meta.ChunkSize) directly — matching decode.go's extractMainPartition,
	// which bases this on hdr.DecompSize, NOT on entryCount*chunkSize. Using
	// the total here (as a previous version of this function did) picks the
	// wrong threshold whenever entryCount*chunkSize crosses 0x2000000 while
	// the per-chunk size itself doesn't, silently encrypting the wrong
	// number of boundary bytes.
	decryptSize := 5120
	if meta.ChunkSize >= 0x2000000 {
		decryptSize = 10240
	}

	// Determine actual number of chunks from the real data size — NOT capped
	// at the original entryCount. Capping here would silently truncate any
	// modified content that needs MORE chunks than the original had (e.g.
	// adding enough files to push MAIN.bin past the original's chunk
	// capacity), dropping the tail with no error.
	actualCount := (len(rawData) + chunkSize - 1) / chunkSize
	if actualCount != entryCount && e.verbose {
		fmt.Printf("  MAIN content needs %d chunks (template had %d)\n", actualCount, entryCount)
	}

	// BufferConstant scales with an entry's own decompressed size (confirmed
	// against real firmware: the last, smaller partial chunk uses a
	// proportionally smaller BufferConstant than the full-size chunks) —
	// it is NOT simply "entry index i" once content is modified, since
	// modding can add/remove chunks or resize the last one. Look it up by
	// the chunk's actual decompressed size instead, so a chunk that keeps
	// the standard chunk size (nearly always true except for the last
	// chunk) always gets an exact, real BufferConstant regardless of how
	// many chunks came before it.
	bufferConstantBySize := make(map[uint32]uint32, len(meta.DecompSizes))
	for i, sz := range meta.DecompSizes {
		if i < len(meta.BufferConstants) {
			bufferConstantBySize[sz] = meta.BufferConstants[i]
		}
	}

	// Compress chunks in parallel using goroutines
	encodedEntries := make([]mainEncodedEntry, actualCount)
	var wg sync.WaitGroup
	for i := 0; i < actualCount; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(rawData) {
			end = len(rawData)
		}
		chunk := rawData[start:end]

		bufferConstant, exact := bufferConstantBySize[uint32(len(chunk))]
		if !exact {
			bufferConstant = estimateBufferConstant(uint32(len(chunk)))
			fmt.Printf("  Warning: no captured BufferConstant for a %d-byte chunk (entry %d) — "+
				"using an estimated value (%d), not verified against real firmware. "+
				"This can happen when modified content changes the chunk layout.\n",
				len(chunk), i, bufferConstant)
		}

		wg.Add(1)
		go func(idx int, chunk []byte, bufferConstant uint32) {
			defer wg.Done()

			// Compress with LZSS (comp_type 2 and 0 both use raw LZSS)
			compData := CompressLZSS(chunk)

			// Build entry header with computed fields
			header := buildEntryHeader(signature, meta.CompType, uint32(len(chunk)),
				uint32(len(compData)), bufferConstant, meta.ChecksumFlag)

			// Compute and set HDR checksum (stride-16 Adler32 of decompressed data)
			hdrCk := computeHdrChecksum(chunk)
			binary.LittleEndian.PutUint32(header[40:44], hdrCk)

			// Combine header + compressed data
			entryData := make([]byte, 0, len(header)+len(compData))
			entryData = append(entryData, header...)
			entryData = append(entryData, compData...)

			// Pad entry to align8(FooterOff + 8) to match original entry sizing
			footerOff := uint32(MainEntryHeaderLen) + align4(uint32(len(compData)))
			targetSize := align8(footerOff + 8)
			if uint32(len(entryData)) < targetSize {
				entryData = append(entryData, make([]byte, targetSize-uint32(len(entryData)))...)
			}

			// Write "EXTRFOOT" footer at footerOff
			copy(entryData[footerOff:footerOff+8], []byte("EXTRFOOT"))

			encodedEntries[idx] = mainEncodedEntry{
				data:       entryData,
				decompSize: uint32(len(chunk)),
			}
		}(i, chunk, bufferConstant)
	}
	wg.Wait()

	// Apply Feistel encryption to entry boundaries
	for i := range encodedEntries {
		entryData := encodedEntries[i].data
		if len(entryData) > decryptSize {
			e.feistel.Encrypt(entryData[:decryptSize])
			e.feistel.Encrypt(entryData[len(entryData)-decryptSize:])
		} else {
			e.feistel.Encrypt(entryData)
		}
	}

	// Build entry list
	entryListSize := len(encodedEntries) * MainListEntryLen
	listHeaderSize := MainListHeaderLen
	entryList := make([]byte, listHeaderSize+entryListSize)

	// List header: restore Unknown fields, set computed fields
	binary.LittleEndian.PutUint32(entryList[4:8], meta.FormatVersion)
	binary.LittleEndian.PutUint32(entryList[8:12], uint32(listHeaderSize+entryListSize))
	// DecompSize is the per-chunk decompressed size (e.g. 0x1000000 = 16MB),
	// not the total raw data length — confirmed against real firmware.
	binary.LittleEndian.PutUint32(entryList[12:16], meta.ChunkSize)
	binary.LittleEndian.PutUint32(entryList[16:20], meta.ListHeaderCompType)

	// Entry records: Size + Adler32 checksum of encrypted entry data
	listOffset := listHeaderSize
	for _, entry := range encodedEntries {
		binary.LittleEndian.PutUint32(entryList[listOffset:listOffset+4], uint32(len(entry.data)))
		binary.LittleEndian.PutUint32(entryList[listOffset+4:listOffset+8], adler32.Checksum(entry.data))
		listOffset += MainListEntryLen
	}

	// Recalculate list header checksum
	checksum := calculateChecksum(entryList[4:])
	binary.LittleEndian.PutUint32(entryList[0:4], checksum)

	// Restore original first header
	firstHeader, err := hex.DecodeString(meta.FirstHeader)
	if err != nil {
		return nil, fmt.Errorf("invalid first_header hex: %w", err)
	}
	if len(firstHeader) != MainListHeaderOff {
		return nil, fmt.Errorf("first_header is %d bytes, expected %d", len(firstHeader), MainListHeaderOff)
	}

	// Combine all parts
	totalSize := len(firstHeader) + len(entryList)
	for _, entry := range encodedEntries {
		totalSize += len(entry.data)
	}
	output := make([]byte, 0, totalSize)
	output = append(output, firstHeader...)
	output = append(output, entryList...)
	for _, entry := range encodedEntries {
		output = append(output, entry.data...)
	}

	// Feistel encrypt the first 0x30 bytes
	e.feistel.Encrypt(output[:MainListHeaderOff])

	return output, nil
}

// encodeMainPartitionLegacy is the original encoding logic (arbitrary 1MB chunks, raw LZSS only)
func (e *Encoder) encodeMainPartitionLegacy(rawData []byte) ([]byte, error) {
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

		// Build entry header
		header := make([]byte, MainEntryHeaderLen)
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

	binary.LittleEndian.PutUint32(entryList[8:12], uint32(listHeaderSize+entryListSize))
	binary.LittleEndian.PutUint32(entryList[12:16], totalDecomp)

	// Entry records: Size + Adler32 checksum of encrypted entry data
	listOffset := listHeaderSize
	for _, entry := range entries {
		binary.LittleEndian.PutUint32(entryList[listOffset:listOffset+4], uint32(len(entry.data)))
		binary.LittleEndian.PutUint32(entryList[listOffset+4:listOffset+8], adler32.Checksum(entry.data))
		listOffset += 8
	}

	// Calculate list header checksum
	checksum := calculateChecksum(entryList[4:])
	binary.LittleEndian.PutUint32(entryList[0:4], checksum)

	// Build first 0x30-byte header (will be Feistel encrypted)
	firstHeader := make([]byte, MainListHeaderOff)

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

// calculateChecksum computes the MAIN list header's checksum: Adler32 over
// the list header (minus its own checksum field) plus the entry records —
// confirmed against real firmware. Consistent with every other checksum in
// this format (entry blobs, entry headers, module entries) also using
// Adler32, rather than a plain word sum.
func calculateChecksum(data []byte) uint32 {
	return adler32.Checksum(data)
}
