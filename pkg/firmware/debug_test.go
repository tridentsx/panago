package firmware

import (
	"encoding/binary"
	"hash/adler32"
	"os"
	"testing"
)

func TestListEntryAdler32(t *testing.T) {
	// Verify that list entry checksums are Adler32 of the encrypted entry data
	path := "/home/tridentsx/src/panasonic/org/PANAEUSB.FRM"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("firmware not available: %v", err)
	}

	rawFW, _ := os.ReadFile(path)
	decFW, _ := AESDecryptCBC(rawFW)

	fc := NewFeistelCipher()
	fc.Decrypt(decFW[:HeaderSize])

	modHdrSize := 0x2000
	modHdr := make([]byte, modHdrSize)
	copy(modHdr, decFW[HeaderSize:HeaderSize+modHdrSize])
	fc.Decrypt(modHdr)

	var mainOff, mainSize uint32
	for i := 1; i < 20; i++ {
		off := i * ModuleEntrySize
		if string(modHdr[off:off+4]) == "MAIN" {
			mainOff = binary.LittleEndian.Uint32(modHdr[off+12 : off+16])
			mainSize = binary.LittleEndian.Uint32(modHdr[off+32 : off+36])
			break
		}
	}
	mainData := decFW[mainOff : mainOff+mainSize]

	listHdr := mainData[MainListHeaderOff : MainListHeaderOff+MainListHeaderLen]
	lhListSize := binary.LittleEndian.Uint32(listHdr[8:12])

	entryCount := (lhListSize - 20) / 8

	offset := uint32(MainListHeaderOff + MainListHeaderLen)
	type listEntry struct {
		size     uint32
		checksum uint32
	}
	entries := make([]listEntry, entryCount)
	for i := uint32(0); i < entryCount; i++ {
		entries[i].size = binary.LittleEndian.Uint32(mainData[offset : offset+4])
		entries[i].checksum = binary.LittleEndian.Uint32(mainData[offset+4 : offset+8])
		offset += 8
	}

	allMatch := true
	for i := uint32(0); i < entryCount; i++ {
		le := entries[i]
		if le.size == 0 || int(offset)+int(le.size) > len(mainData) {
			offset += le.size
			continue
		}

		// Adler32 of the RAW (encrypted) entry data
		encryptedEntry := mainData[offset : offset+le.size]
		computed := adler32.Checksum(encryptedEntry)

		if computed != le.checksum {
			t.Errorf("Entry %d: Adler32(encrypted)=0x%08X, stored=0x%08X — MISMATCH", i, computed, le.checksum)
			allMatch = false
		} else {
			t.Logf("Entry %d: Adler32=0x%08X ✓", i, computed)
		}

		offset += le.size
	}

	if allMatch {
		t.Logf("All %d entries: Adler32(encrypted) matches stored checksum", entryCount)
	}
}

func TestHdrChecksumStride16Adler32(t *testing.T) {
	// Verify that entry header checksums are Adler32 with stride 16 on decompressed data.
	// "Stride 16" means we compute Adler32 on every 16th byte: data[0], data[16], data[32], ...
	firmwares := []string{
		"/home/tridentsx/src/panasonic/firmwares/PANAEUSB_V145.FRM",
		"/home/tridentsx/src/panasonic/firmwares/PANAEUSB_V160.FRM",
		"/home/tridentsx/src/panasonic/firmwares/PANAEUSB_V169.FRM",
		"/home/tridentsx/src/panasonic/firmwares/PANAEUSB_V176.FRM",
		"/home/tridentsx/src/panasonic/firmwares/PANAEUSB_V182.FRM",
	}

	totalChecked := 0
	totalMatch := 0

	for _, fwPath := range firmwares {
		if _, err := os.Stat(fwPath); err != nil {
			t.Skipf("firmware not available: %s", fwPath)
		}

		rawFW, err := os.ReadFile(fwPath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", fwPath, err)
		}
		decFW, err := AESDecryptCBC(rawFW)
		if err != nil {
			t.Fatalf("AES decrypt failed for %s: %v", fwPath, err)
		}

		fc := NewFeistelCipher()
		fc.Decrypt(decFW[:HeaderSize])

		modHdrSize := 0x2000
		modHdr := make([]byte, modHdrSize)
		copy(modHdr, decFW[HeaderSize:HeaderSize+modHdrSize])
		fc.Decrypt(modHdr)

		var mainOff, mainSize uint32
		for i := 1; i < 20; i++ {
			off := i * ModuleEntrySize
			if string(modHdr[off:off+4]) == "MAIN" {
				mainOff = binary.LittleEndian.Uint32(modHdr[off+12 : off+16])
				mainSize = binary.LittleEndian.Uint32(modHdr[off+32 : off+36])
				break
			}
		}
		mainData := decFW[mainOff : mainOff+mainSize]

		listHdr := mainData[MainListHeaderOff : MainListHeaderOff+MainListHeaderLen]
		lhListSize := binary.LittleEndian.Uint32(listHdr[8:12])
		lhDecomp := binary.LittleEndian.Uint32(listHdr[12:16])

		entryCount := (lhListSize - 20) / 8
		decryptSize := 5120
		if lhDecomp >= 0x2000000 {
			decryptSize = 10240
		}

		offset := uint32(MainListHeaderOff + MainListHeaderLen)
		type listEntry struct {
			size     uint32
			checksum uint32
		}
		entries := make([]listEntry, entryCount)
		for i := uint32(0); i < entryCount; i++ {
			entries[i].size = binary.LittleEndian.Uint32(mainData[offset : offset+4])
			entries[i].checksum = binary.LittleEndian.Uint32(mainData[offset+4 : offset+8])
			offset += 8
		}

		for i := uint32(0); i < entryCount; i++ {
			le := entries[i]
			if le.size == 0 || int(offset)+int(le.size) > len(mainData) {
				offset += le.size
				continue
			}

			entryData := make([]byte, le.size)
			copy(entryData, mainData[offset:offset+le.size])
			if le.size > uint32(decryptSize) {
				fc3 := NewFeistelCipher()
				fc3.Decrypt(entryData[:decryptSize])
				fc3.Decrypt(entryData[le.size-uint32(decryptSize):])
			} else {
				fc3 := NewFeistelCipher()
				fc3.Decrypt(entryData)
			}

			compSize := binary.LittleEndian.Uint32(entryData[24:28])
			decompSize := binary.LittleEndian.Uint32(entryData[16:20])
			hdrChecksum := binary.LittleEndian.Uint32(entryData[40:44])
			compData := entryData[MainEntryHeaderLen : MainEntryHeaderLen+int(compSize)]

			decompData := DecompressLZSSWithSize(compData, int(decompSize))
			if len(decompData) == 0 {
				offset += le.size
				continue
			}

			// Compute Adler32 with stride 16: process every 16th byte
			strided := make([]byte, 0, len(decompData)/16+1)
			for j := 0; j < len(decompData); j += 16 {
				strided = append(strided, decompData[j])
			}
			computed := adler32.Checksum(strided)

			totalChecked++
			if computed == hdrChecksum {
				totalMatch++
				t.Logf("%s entry %d: stride16_adler32=0x%08X OK (decomp=%d)", fwPath, i, computed, len(decompData))
			} else {
				t.Errorf("%s entry %d: stride16_adler32=0x%08X, stored=0x%08X MISMATCH (decomp=%d)",
					fwPath, i, computed, hdrChecksum, len(decompData))
			}

			offset += le.size
		}
	}

	t.Logf("Total: %d/%d entries matched stride-16 Adler32", totalMatch, totalChecked)
	if totalMatch != totalChecked {
		t.Fatalf("Not all entries matched! %d/%d", totalMatch, totalChecked)
	}
}
