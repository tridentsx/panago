package firmware

// DecompressLZSS decompresses LZSS-compressed data
// Uses 4KB ring buffer with initial position at 4078
func DecompressLZSS(src []byte) []byte {
	ring := make([]byte, LZSSRingSize)
	ringPos := LZSSRingInit

	dst := make([]byte, 0, len(src)*4) // Pre-allocate with estimate

	si := 0
	for si < len(src) {
		flags := src[si]
		si++

		for bit := 0; bit < 8 && si < len(src); bit++ {
			if flags&(1<<bit) != 0 {
				// Literal byte
				c := src[si]
				si++
				dst = append(dst, c)
				ring[ringPos&0xfff] = c
				ringPos++
			} else {
				// Back-reference
				if si+1 >= len(src) {
					break
				}
				b1 := src[si]
				b2 := src[si+1]
				si += 2

				offset := int(b1) | (int(b2&0xf0) << 4)
				length := int(b2&0x0f) + 3

				for j := 0; j < length; j++ {
					c := ring[(offset+j)&0xfff]
					dst = append(dst, c)
					ring[ringPos&0xfff] = c
					ringPos++
				}
			}
		}
	}

	return dst
}

// CompressLZSS compresses data using LZSS algorithm
// Uses hash chain for fast matching, compatible with Panasonic format
func CompressLZSS(src []byte) []byte {
	if len(src) == 0 {
		return []byte{}
	}

	ring := make([]byte, LZSSRingSize)
	ringPos := LZSSRingInit

	// Hash table for fast matching
	// head[hash] = most recent source position with this hash
	// prev[si & 0x7fff] = previous source position with same hash
	head := make([]int, 65536)
	prev := make([]int, 32768)
	for i := range head {
		head[i] = -1
	}

	// Estimate output size
	dst := make([]byte, 0, len(src)+len(src)/8+16)

	si := 0
	for si < len(src) {
		flagPos := len(dst)
		dst = append(dst, 0) // Placeholder for flags
		var flags byte

		for bit := 0; bit < 8 && si < len(src); bit++ {
			// Compute hash for current position
			var hash uint16
			if si+1 < len(src) {
				hash = uint16(src[si]) | (uint16(src[si+1]) << 8)
			} else {
				hash = uint16(src[si])
			}

			// Find best match using hash chain
			bestLen := 0
			bestOff := 0

			// Walk the hash chain (limit iterations to avoid O(n²))
			pos := head[hash]
			maxChain := 4096
			for pos >= 0 && maxChain > 0 {
				// Position must be within ring buffer distance
				dist := si - pos
				if dist > 4096 || dist <= 0 {
					break
				}

				// Calculate what the ring offset would be for this source position
				ringOff := (ringPos - dist) & 0xfff

				// Try to extend match
				matchLen := 0
				maxLen := 18
				if si+maxLen > len(src) {
					maxLen = len(src) - si
				}
				// Also limit by available ring buffer data
				if maxLen > dist {
					maxLen = dist
				}

				for matchLen < maxLen && src[pos+matchLen] == src[si+matchLen] {
					matchLen++
				}

				if matchLen > bestLen {
					bestLen = matchLen
					bestOff = ringOff
				}

				// Follow chain
				pos = prev[pos&0x7fff]
				maxChain--
			}

			// Update hash chain for current position
			prev[si&0x7fff] = head[hash]
			head[hash] = si

			if bestLen >= 3 {
				// Output back-reference
				dst = append(dst, byte(bestOff&0xff))
				dst = append(dst, byte(((bestOff>>4)&0xf0)|((bestLen-3)&0x0f)))

				// Update ring buffer and advance
				for j := 0; j < bestLen; j++ {
					ring[ringPos&0xfff] = src[si]
					ringPos++
					si++

					// Update hash chain for skipped positions
					if j > 0 && si < len(src) {
						if si+1 < len(src) {
							h := uint16(src[si]) | (uint16(src[si+1]) << 8)
							prev[si&0x7fff] = head[h]
							head[h] = si
						}
					}
				}
			} else {
				// Output literal byte
				flags |= 1 << bit
				dst = append(dst, src[si])
				ring[ringPos&0xfff] = src[si]
				ringPos++
				si++
			}
		}

		dst[flagPos] = flags
	}

	return dst
}

// DecompressLZSSWithSize decompresses LZSS with a size limit
func DecompressLZSSWithSize(src []byte, maxSize int) []byte {
	ring := make([]byte, LZSSRingSize)
	ringPos := LZSSRingInit

	dst := make([]byte, 0, maxSize)

	si := 0
	for si < len(src) && len(dst) < maxSize {
		flags := src[si]
		si++

		for bit := 0; bit < 8 && si < len(src) && len(dst) < maxSize; bit++ {
			if flags&(1<<bit) != 0 {
				// Literal byte
				c := src[si]
				si++
				dst = append(dst, c)
				ring[ringPos&0xfff] = c
				ringPos++
			} else {
				// Back-reference
				if si+1 >= len(src) {
					break
				}
				b1 := src[si]
				b2 := src[si+1]
				si += 2

				offset := int(b1) | (int(b2&0xf0) << 4)
				length := int(b2&0x0f) + 3

				for j := 0; j < length && len(dst) < maxSize; j++ {
					c := ring[(offset+j)&0xfff]
					dst = append(dst, c)
					ring[ringPos&0xfff] = c
					ringPos++
				}
			}
		}
	}

	return dst
}
