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

// CompressLZSS compresses data using a faithful port of Haruhiko Okumura's
// classic public-domain LZSS reference encoder (binary-tree match finder), adapted
// for this format's zero-filled ring buffer instead of the original's space-filled one.
// This reproduces the exact match/tie-break decisions of the Panasonic firmware
// toolchain's compressor byte-for-byte (verified against real firmware sub-entries).
func CompressLZSS(src []byte) []byte {
	if len(src) == 0 {
		return []byte{}
	}

	const (
		N         = LZSSRingSize // 4096
		F         = 18
		THRESHOLD = 2
		NIL       = N
	)

	textBuf := make([]byte, N+F-1)
	lson := make([]int, N+1)
	rson := make([]int, N+257)
	dad := make([]int, N+1)

	var matchPosition, matchLength int

	insertNode := func(r int) {
		cmp := 1
		key := textBuf[r:]
		p := N + 1 + int(key[0])
		rson[r] = NIL
		lson[r] = NIL
		matchLength = 0
		for {
			if cmp >= 0 {
				if rson[p] != NIL {
					p = rson[p]
				} else {
					rson[p] = r
					dad[r] = p
					return
				}
			} else {
				if lson[p] != NIL {
					p = lson[p]
				} else {
					lson[p] = r
					dad[r] = p
					return
				}
			}
			i := 1
			for ; i < F; i++ {
				cmp = int(key[i]) - int(textBuf[p+i])
				if cmp != 0 {
					break
				}
			}
			if i > matchLength {
				matchPosition = p
				matchLength = i
				if matchLength >= F {
					break
				}
			}
		}
		dad[r] = dad[p]
		lson[r] = lson[p]
		rson[r] = rson[p]
		dad[lson[p]] = r
		dad[rson[p]] = r
		if rson[dad[p]] == p {
			rson[dad[p]] = r
		} else {
			lson[dad[p]] = r
		}
		dad[p] = NIL
	}

	deleteNode := func(p int) {
		if dad[p] == NIL {
			return
		}
		var q int
		if rson[p] == NIL {
			q = lson[p]
		} else if lson[p] == NIL {
			q = rson[p]
		} else {
			q = lson[p]
			if rson[q] != NIL {
				for rson[q] != NIL {
					q = rson[q]
				}
				rson[dad[q]] = lson[q]
				dad[lson[q]] = dad[q]
				lson[q] = lson[p]
				dad[lson[p]] = q
			}
			rson[q] = rson[p]
			dad[rson[p]] = q
		}
		dad[q] = dad[p]
		if rson[dad[p]] == p {
			rson[dad[p]] = q
		} else {
			lson[dad[p]] = q
		}
		dad[p] = NIL
	}

	for i := N + 1; i <= N+256; i++ {
		rson[i] = NIL
	}
	for i := 0; i < N; i++ {
		dad[i] = NIL
	}

	dst := make([]byte, 0, len(src)+len(src)/8+16)

	getbyte := func(pos int) (byte, bool) {
		if pos < len(src) {
			return src[pos], true
		}
		return 0, false
	}

	codeBuf := make([]byte, 17)
	codeBufPtr := 1
	var mask byte = 1
	codeBuf[0] = 0

	s := 0
	r := N - F
	srcPos := 0

	for i := s; i < r; i++ {
		textBuf[i] = 0
	}

	length := 0
	for ; length < F; length++ {
		c, ok := getbyte(srcPos)
		if !ok {
			break
		}
		srcPos++
		textBuf[r+length] = c
	}
	if length == 0 {
		return []byte{}
	}

	for i := 1; i <= F; i++ {
		insertNode(r - i)
	}
	insertNode(r)

	for {
		if matchLength > length {
			matchLength = length
		}
		if matchLength <= THRESHOLD {
			matchLength = 1
			codeBuf[0] |= mask
			codeBuf[codeBufPtr] = textBuf[r]
			codeBufPtr++
		} else {
			codeBuf[codeBufPtr] = byte(matchPosition)
			codeBufPtr++
			codeBuf[codeBufPtr] = byte(((matchPosition >> 4) & 0xf0) | (matchLength - (THRESHOLD + 1)))
			codeBufPtr++
		}
		mask <<= 1
		if mask == 0 {
			dst = append(dst, codeBuf[:codeBufPtr]...)
			codeBuf[0] = 0
			codeBufPtr = 1
			mask = 1
		}
		lastMatchLength := matchLength
		i := 0
		for i < lastMatchLength {
			c, ok := getbyte(srcPos)
			if !ok {
				break
			}
			srcPos++
			deleteNode(s)
			textBuf[s] = c
			if s < F-1 {
				textBuf[s+N] = c
			}
			s = (s + 1) & (N - 1)
			r = (r + 1) & (N - 1)
			insertNode(r)
			i++
		}
		for {
			old := i
			i++
			if !(old < lastMatchLength) {
				break
			}
			deleteNode(s)
			s = (s + 1) & (N - 1)
			r = (r + 1) & (N - 1)
			length--
			if length > 0 {
				insertNode(r)
			}
		}
		if length <= 0 {
			break
		}
	}
	if codeBufPtr > 1 {
		dst = append(dst, codeBuf[:codeBufPtr]...)
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
