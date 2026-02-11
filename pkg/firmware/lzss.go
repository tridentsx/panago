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

	const (
		hashSize    = 65536
		prevSize    = 1 << 16
		prevMask    = prevSize - 1
		maxChain    = 128 // Hash chain walk limit
		maxMatch    = 18  // Maximum LZSS match length
		windowSize  = 4096
		ringInitVal = LZSSRingInit
	)

	head := make([]int, hashSize)
	prev := make([]int, prevSize)
	for i := range head {
		head[i] = -1
	}

	// Estimate output size
	dst := make([]byte, 0, len(src)+len(src)/8+16)

	si := 0
	srcLen := len(src)

	for si < srcLen {
		flagPos := len(dst)
		dst = append(dst, 0) // Placeholder for flags
		var flags byte

		for bit := 0; bit < 8 && si < srcLen; bit++ {
			// Compute 2-byte hash for current position
			var hash uint16
			if si+1 < srcLen {
				hash = uint16(src[si]) | (uint16(src[si+1]) << 8)
			} else {
				hash = uint16(src[si])
			}

			// Find best match using hash chain
			bestLen := 0
			bestOff := 0

			// Compute max possible match at this position
			maxLen := maxMatch
			if si+maxLen > srcLen {
				maxLen = srcLen - si
			}

			pos := head[hash]
			chain := maxChain
			for pos >= 0 && chain > 0 {
				dist := si - pos
				if dist > windowSize || dist <= 0 {
					break
				}

				// Try to extend match — split fast/slow paths to avoid modulo
				matchLen := 0
				if dist >= maxLen {
					// Fast path: match cannot wrap around (common case, no modulo)
					for matchLen < maxLen && src[pos+matchLen] == src[si+matchLen] {
						matchLen++
					}
				} else {
					// Slow path: match may exceed distance (repeated patterns)
					// First match up to dist bytes directly
					limit := dist
					if limit > maxLen {
						limit = maxLen
					}
					for matchLen < limit && src[pos+matchLen] == src[si+matchLen] {
						matchLen++
					}
					// If full pattern matched, continue comparing against the repeated pattern
					if matchLen == dist {
						for matchLen < maxLen && src[si+matchLen-dist] == src[si+matchLen] {
							matchLen++
						}
					}
				}

				if matchLen > bestLen {
					bestLen = matchLen
					// Compute ring offset: ringPos = ringInitVal + si, so
					// ring position for source pos = (ringInitVal + pos) & 0xfff
					bestOff = (ringInitVal + pos) & 0xfff
					if bestLen == maxMatch {
						break // Can't do better
					}
				}

				pos = prev[pos&prevMask]
				chain--
			}

			// Update hash chain for current position
			prev[si&prevMask] = head[hash]
			head[hash] = si

			if bestLen >= 3 {
				// Output back-reference
				dst = append(dst, byte(bestOff&0xff),
					byte(((bestOff>>4)&0xf0)|((bestLen-3)&0x0f)))

				// Advance source and update hash chains for skipped positions
				si++
				for j := 1; j < bestLen; j++ {
					if si+1 < srcLen {
						h := uint16(src[si]) | (uint16(src[si+1]) << 8)
						prev[si&prevMask] = head[h]
						head[h] = si
					}
					si++
				}
			} else {
				// Output literal byte
				flags |= 1 << bit
				dst = append(dst, src[si])
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
