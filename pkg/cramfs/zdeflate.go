package cramfs

// Pure-Go, byte-exact port of zlib 1.2.12's deflate_slow() + trees.c Huffman
// encoder, specialized for a single one-shot block (the whole input handed to
// deflate() at once with Z_FINISH) at level=6, memLevel=7, windowBits=14,
// strategy=Z_DEFAULT_STRATEGY. These are the exact parameters Panasonic's
// firmware toolchain used to compress each 4KB cramfs block (confirmed by
// brute-force matching real zlib against captured firmware blocks — see
// project notes). Go's standard library compress/flate is a different DEFLATE
// implementation and cannot reproduce these bytes, hence this port.
//
// Ported directly from deflate.c/trees.c (zlib 1.2.12, zlib license) rather
// than reimplemented from memory, since DEFLATE's match-finding tie-breaks and
// Huffman tree construction are exactly the kind of subtle behavior that's
// easy to get almost-but-not-quite right.

import (
	"encoding/binary"
	"hash/adler32"
)

const (
	zMinMatch     = 3
	zMaxMatch     = 258
	zMinLookahead = zMaxMatch + zMinMatch + 1 // 262

	zLengthCodes = 29
	zLiterals    = 256
	zLCodes      = zLiterals + 1 + zLengthCodes // 286
	zDCodes      = 30
	zBLCodes     = 19
	zHeapSize    = 2*zLCodes + 1 // 573
	zMaxBits     = 15
	zMaxBLBits   = 7

	zEndBlock  = 256
	zRep36     = 16
	zRepz310   = 17
	zRepz11138 = 18

	zStoredBlock = 0
	zStaticTrees = 1
	zDynTrees    = 2

	zTooFar = 4096

	// Fixed parameters: level=6, windowBits=14, memLevel=7
	zWBits        = 14
	zWSize        = 1 << zWBits // 16384
	zWMask        = zWSize - 1
	zHashBits     = 7 + 7 // memLevel + 7
	zHashSize     = 1 << zHashBits
	zHashMask     = zHashSize - 1
	zHashShift    = (zHashBits + zMinMatch - 1) / zMinMatch // 5
	zGoodMatch    = 8
	zMaxLazyMatch = 16
	zNiceMatch    = 128
	zMaxChainLen  = 128
	zMaxDist      = zWSize - zMinLookahead // 16122
)

var extraLBits = [zLengthCodes]int{0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0}
var extraDBits = [zDCodes]int{0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13}
var extraBLBits = [zBLCodes]int{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 3, 7}
var blOrder = [zBLCodes]int{16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15}

type ctData struct {
	freqOrCode uint16
	dadOrLen   uint16
}

type treeDesc struct {
	dynTree    []ctData
	maxCode    int
	staticLen  []uint16 // nil if no static tree (bl_desc)
	extraBits  []int
	extraBase  int
	elems      int
	maxLength  int
}

type zState struct {
	// input / window
	window []byte // data + zMaxMatch zero padding
	dataLen int

	prev []int32 // size zWSize
	head []int32 // size zHashSize

	insH int

	strstart    int
	lookahead   int
	matchLength int
	matchStart  int
	prevLength  int
	prevMatch   int
	matchAvailable bool

	dynLtree [zHeapSize]ctData
	dynDtree [2*zDCodes + 1]ctData
	blTree   [2*zBLCodes + 1]ctData

	lDesc  treeDesc
	dDesc  treeDesc
	blDesc treeDesc

	blCount [zMaxBits + 1]uint16
	heap    [zHeapSize]int
	heapLen int
	heapMax int
	depth   [zHeapSize]uint8

	symBuf  []byte
	symNext int

	optLen    int64
	staticLen int64
	matches   int

	biBuf   uint16
	biValid int

	pending []byte
}

func newZState(data []byte) *zState {
	s := &zState{}
	s.dataLen = len(data)
	s.window = make([]byte, len(data)+zMaxMatch)
	copy(s.window, data)
	s.prev = make([]int32, zWSize)
	s.head = make([]int32, zHashSize) // zero-valued = NIL (0)

	s.lDesc = treeDesc{dynTree: s.dynLtree[:], staticLen: staticLTreeLen[:], extraBits: extraLBits[:], extraBase: zLiterals + 1, elems: zLCodes, maxLength: zMaxBits}
	s.dDesc = treeDesc{dynTree: s.dynDtree[:], staticLen: staticDTreeLen[:], extraBits: extraDBits[:], extraBase: 0, elems: zDCodes, maxLength: zMaxBits}
	s.blDesc = treeDesc{dynTree: s.blTree[:], staticLen: nil, extraBits: extraBLBits[:], extraBase: 0, elems: zBLCodes, maxLength: zMaxBLBits}

	s.symBuf = make([]byte, 0, len(data)*3+16)

	s.initBlock()

	s.lookahead = len(data)
	s.matchLength = zMinMatch - 1
	s.prevLength = zMinMatch - 1

	return s
}

func (s *zState) initBlock() {
	for n := range s.dynLtree {
		s.dynLtree[n].freqOrCode = 0
	}
	for n := range s.dynDtree {
		s.dynDtree[n].freqOrCode = 0
	}
	for n := range s.blTree {
		s.blTree[n].freqOrCode = 0
	}
	s.dynLtree[zEndBlock].freqOrCode = 1
	s.optLen = 0
	s.staticLen = 0
	s.symNext = 0
	s.matches = 0
}

func updateHash(h int, c byte) int {
	return ((h << zHashShift) ^ int(c)) & zHashMask
}

// insertString inserts window[str:] into the hash table, returns previous head.
func (s *zState) insertString(str int) int {
	s.insH = updateHash(s.insH, s.window[str+zMinMatch-1])
	matchHead := s.head[s.insH]
	s.prev[str&zWMask] = matchHead
	s.head[s.insH] = int32(str)
	return int(matchHead)
}

func dCode(dist int) uint8 {
	if dist < 256 {
		return distCodeTable[dist]
	}
	return distCodeTable[256+(dist>>7)]
}

func (s *zState) tallyLit(c byte) bool {
	s.symBuf = append(s.symBuf, 0, 0, c)
	s.symNext += 3
	s.dynLtree[c].freqOrCode++
	return false // sym_end never reached for our block sizes
}

func (s *zState) tallyDist(dist, length int) bool {
	s.symBuf = append(s.symBuf, byte(dist), byte(dist>>8), byte(length))
	s.symNext += 3
	s.matches++
	d := dist - 1
	s.dynLtree[int(lengthCodeTable[length])+zLiterals+1].freqOrCode++
	s.dynDtree[dCode(d)].freqOrCode++
	return false
}

// longestMatch mirrors zlib's longest_match(), simplified to a plain
// byte-comparison (the C fast-path byte-skipping is purely a speed
// optimization; it produces the identical best_len/match_start).
func (s *zState) longestMatch(curMatch int) int {
	chainLength := zMaxChainLen
	bestLen := s.prevLength
	niceMatch := zNiceMatch

	limit := 0
	if s.strstart > zMaxDist {
		limit = s.strstart - zMaxDist
	}

	if s.prevLength >= zGoodMatch {
		chainLength >>= 2
	}
	if niceMatch > s.lookahead {
		niceMatch = s.lookahead
	}

	maxLen := zMaxMatch
	if s.strstart+maxLen > len(s.window) {
		maxLen = len(s.window) - s.strstart
	}

	cur := curMatch
	for {
		length := 0
		for length < maxLen && s.window[cur+length] == s.window[s.strstart+length] {
			length++
		}
		if length > bestLen {
			s.matchStart = cur
			bestLen = length
			if bestLen >= niceMatch {
				break
			}
		}

		next := int(s.prev[cur&zWMask])
		if next <= limit {
			break
		}
		cur = next
		chainLength--
		if chainLength == 0 {
			break
		}
	}

	if bestLen <= s.lookahead {
		return bestLen
	}
	return s.lookahead
}

// deflateSlow runs the lazy-matching compressor over the whole window,
// populating symBuf/dyn_ltree/dyn_dtree. Input is always fully available
// (single-shot), so this only needs the interior loop of zlib's deflate_slow.
func (s *zState) deflateSlow() {
	n := s.dataLen
	if n >= zMinMatch {
		s.insH = int(s.window[0])
		s.insH = updateHash(s.insH, s.window[1])
	}

	for s.lookahead != 0 {
		var hashHead int = 0
		if s.lookahead >= zMinMatch {
			hashHead = s.insertString(s.strstart)
		}

		s.prevLength = s.matchLength
		s.prevMatch = s.matchStart
		s.matchLength = zMinMatch - 1

		if hashHead != 0 && s.prevLength < zMaxLazyMatch && s.strstart-hashHead <= zMaxDist {
			s.matchLength = s.longestMatch(hashHead)
			if s.matchLength <= 5 && s.matchLength == zMinMatch && s.strstart-s.matchStart > zTooFar {
				s.matchLength = zMinMatch - 1
			}
		}

		if s.prevLength >= zMinMatch && s.matchLength <= s.prevLength {
			maxInsert := s.strstart + s.lookahead - zMinMatch

			s.tallyDist(s.strstart-1-s.prevMatch, s.prevLength-zMinMatch)

			s.lookahead -= s.prevLength - 1
			s.prevLength -= 2
			for {
				s.strstart++
				if s.strstart <= maxInsert {
					s.insertString(s.strstart)
				}
				s.prevLength--
				if s.prevLength == 0 {
					break
				}
			}
			s.matchAvailable = false
			s.matchLength = zMinMatch - 1
			s.strstart++
		} else if s.matchAvailable {
			s.tallyLit(s.window[s.strstart-1])
			s.strstart++
			s.lookahead--
		} else {
			s.matchAvailable = true
			s.strstart++
			s.lookahead--
		}
	}

	if s.matchAvailable {
		s.tallyLit(s.window[s.strstart-1])
		s.matchAvailable = false
	}
}

// ---- bit output ----

func (s *zState) putByte(b byte) {
	s.pending = append(s.pending, b)
}

func (s *zState) putShort(w uint16) {
	s.putByte(byte(w))
	s.putByte(byte(w >> 8))
}

func (s *zState) sendBits(value uint32, length int) {
	if s.biValid > 16-length {
		val := uint16(value)
		s.biBuf |= val << uint(s.biValid)
		s.putShort(s.biBuf)
		s.biBuf = val >> uint(16-s.biValid)
		s.biValid += length - 16
	} else {
		s.biBuf |= uint16(value) << uint(s.biValid)
		s.biValid += length
	}
}

func (s *zState) sendCode(c int, tree []ctData) {
	s.sendBits(uint32(tree[c].freqOrCode), int(tree[c].dadOrLen))
}

func (s *zState) biFlush() {
	if s.biValid == 16 {
		s.putShort(s.biBuf)
		s.biBuf = 0
		s.biValid = 0
	} else if s.biValid >= 8 {
		s.putByte(byte(s.biBuf))
		s.biBuf >>= 8
		s.biValid -= 8
	}
}

func (s *zState) biWindup() {
	if s.biValid > 8 {
		s.putShort(s.biBuf)
	} else if s.biValid > 0 {
		s.putByte(byte(s.biBuf))
	}
	s.biBuf = 0
	s.biValid = 0
}

func bitReverse(code uint32, length int) uint32 {
	res := uint32(0)
	for {
		res |= code & 1
		code >>= 1
		res <<= 1
		length--
		if length <= 0 {
			break
		}
	}
	return res >> 1
}

// ---- Huffman tree construction (trees.c) ----

func treeSmaller(tree []ctData, n, m int, depth []uint8) bool {
	return tree[n].freqOrCode < tree[m].freqOrCode ||
		(tree[n].freqOrCode == tree[m].freqOrCode && depth[n] <= depth[m])
}

func (s *zState) pqdownheap(tree []ctData, k int) {
	v := s.heap[k]
	j := k << 1
	for j <= s.heapLen {
		if j < s.heapLen && treeSmaller(tree, s.heap[j+1], s.heap[j], s.depth[:]) {
			j++
		}
		if treeSmaller(tree, v, s.heap[j], s.depth[:]) {
			break
		}
		s.heap[k] = s.heap[j]
		k = j
		j <<= 1
	}
	s.heap[k] = v
}

func (s *zState) genBitlen(desc *treeDesc) {
	tree := desc.dynTree
	maxCode := desc.maxCode
	stree := desc.staticLen
	extra := desc.extraBits
	base := desc.extraBase
	maxLength := desc.maxLength

	for bits := 0; bits <= zMaxBits; bits++ {
		s.blCount[bits] = 0
	}

	tree[s.heap[s.heapMax]].dadOrLen = 0

	overflow := 0
	for h := s.heapMax + 1; h < zHeapSize; h++ {
		n := s.heap[h]
		bits := int(tree[tree[n].dadOrLen].dadOrLen) + 1
		if bits > maxLength {
			bits = maxLength
			overflow++
		}
		tree[n].dadOrLen = uint16(bits)

		if n > maxCode {
			continue
		}

		s.blCount[bits]++
		xbits := 0
		if n >= base {
			xbits = extra[n-base]
		}
		f := tree[n].freqOrCode
		s.optLen += int64(f) * int64(bits+xbits)
		if stree != nil {
			s.staticLen += int64(f) * int64(int(stree[n])+xbits)
		}
	}
	if overflow == 0 {
		return
	}

	for {
		bits := maxLength - 1
		for s.blCount[bits] == 0 {
			bits--
		}
		s.blCount[bits]--
		s.blCount[bits+1] += 2
		s.blCount[maxLength]--
		overflow -= 2
		if overflow <= 0 {
			break
		}
	}

	h := zHeapSize
	for bits := maxLength; bits != 0; bits-- {
		n := int(s.blCount[bits])
		for n != 0 {
			h--
			m := s.heap[h]
			if m > maxCode {
				continue
			}
			if int(tree[m].dadOrLen) != bits {
				s.optLen += int64(bits-int(tree[m].dadOrLen)) * int64(tree[m].freqOrCode)
				tree[m].dadOrLen = uint16(bits)
			}
			n--
		}
	}
}

func genCodes(tree []ctData, maxCode int, blCount []uint16) {
	var nextCode [zMaxBits + 1]uint32
	code := uint32(0)
	for bits := 1; bits <= zMaxBits; bits++ {
		code = (code + uint32(blCount[bits-1])) << 1
		nextCode[bits] = code
	}
	for n := 0; n <= maxCode; n++ {
		length := int(tree[n].dadOrLen)
		if length == 0 {
			continue
		}
		tree[n].freqOrCode = uint16(bitReverse(nextCode[length], length))
		nextCode[length]++
	}
}

func (s *zState) buildTree(desc *treeDesc) {
	tree := desc.dynTree
	elems := desc.elems

	s.heapLen = 0
	s.heapMax = zHeapSize
	maxCode := -1

	for n := 0; n < elems; n++ {
		if tree[n].freqOrCode != 0 {
			s.heapLen++
			s.heap[s.heapLen] = n
			maxCode = n
			s.depth[n] = 0
		} else {
			tree[n].dadOrLen = 0
		}
	}

	for s.heapLen < 2 {
		var node int
		if maxCode < 2 {
			maxCode++
			node = maxCode
		} else {
			node = 0
		}
		s.heapLen++
		s.heap[s.heapLen] = node
		tree[node].freqOrCode = 1
		s.depth[node] = 0
		s.optLen--
		if desc.staticLen != nil {
			s.staticLen -= int64(desc.staticLen[node])
		}
	}
	desc.maxCode = maxCode

	for n := s.heapLen / 2; n >= 1; n-- {
		s.pqdownheap(tree, n)
	}

	node := elems
	for {
		// pqremove
		n := s.heap[1]
		s.heap[1] = s.heap[s.heapLen]
		s.heapLen--
		s.pqdownheap(tree, 1)

		m := s.heap[1]

		s.heapMax--
		s.heap[s.heapMax] = n
		s.heapMax--
		s.heap[s.heapMax] = m

		tree[node].freqOrCode = tree[n].freqOrCode + tree[m].freqOrCode
		dn, dm := s.depth[n], s.depth[m]
		if dn >= dm {
			s.depth[node] = dn + 1
		} else {
			s.depth[node] = dm + 1
		}
		tree[n].dadOrLen = uint16(node)
		tree[m].dadOrLen = uint16(node)

		s.heap[1] = node
		node++
		s.pqdownheap(tree, 1)

		if s.heapLen < 2 {
			break
		}
	}

	s.heapMax--
	s.heap[s.heapMax] = s.heap[1]

	s.genBitlen(desc)
	genCodes(tree, desc.maxCode, s.blCount[:])
}

func (s *zState) scanTree(tree []ctData, maxCode int) {
	prevlen := -1
	nextlen := int(tree[0].dadOrLen)
	count := 0
	maxCount := 7
	minCount := 4

	if nextlen == 0 {
		maxCount = 138
		minCount = 3
	}
	tree[maxCode+1].dadOrLen = 0xffff

	for n := 0; n <= maxCode; n++ {
		curlen := nextlen
		nextlen = int(tree[n+1].dadOrLen)
		count++
		if count < maxCount && curlen == nextlen {
			continue
		} else if count < minCount {
			s.blTree[curlen].freqOrCode += uint16(count)
		} else if curlen != 0 {
			if curlen != prevlen {
				s.blTree[curlen].freqOrCode++
			}
			s.blTree[zRep36].freqOrCode++
		} else if count <= 10 {
			s.blTree[zRepz310].freqOrCode++
		} else {
			s.blTree[zRepz11138].freqOrCode++
		}
		count = 0
		prevlen = curlen
		if nextlen == 0 {
			maxCount = 138
			minCount = 3
		} else if curlen == nextlen {
			maxCount = 6
			minCount = 3
		} else {
			maxCount = 7
			minCount = 4
		}
	}
}

func (s *zState) sendTree(tree []ctData, maxCode int) {
	prevlen := -1
	nextlen := int(tree[0].dadOrLen)
	count := 0
	maxCount := 7
	minCount := 4

	if nextlen == 0 {
		maxCount = 138
		minCount = 3
	}

	for n := 0; n <= maxCode; n++ {
		curlen := nextlen
		nextlen = int(tree[n+1].dadOrLen)
		count++
		if count < maxCount && curlen == nextlen {
			continue
		} else if count < minCount {
			for i := 0; i < count; i++ {
				s.sendCode(curlen, s.blTree[:])
			}
		} else if curlen != 0 {
			if curlen != prevlen {
				s.sendCode(curlen, s.blTree[:])
				count--
			}
			s.sendCode(zRep36, s.blTree[:])
			s.sendBits(uint32(count-3), 2)
		} else if count <= 10 {
			s.sendCode(zRepz310, s.blTree[:])
			s.sendBits(uint32(count-3), 3)
		} else {
			s.sendCode(zRepz11138, s.blTree[:])
			s.sendBits(uint32(count-11), 7)
		}
		count = 0
		prevlen = curlen
		if nextlen == 0 {
			maxCount = 138
			minCount = 3
		} else if curlen == nextlen {
			maxCount = 6
			minCount = 3
		} else {
			maxCount = 7
			minCount = 4
		}
	}
}

func (s *zState) buildBlTree() int {
	s.scanTree(s.lDesc.dynTree, s.lDesc.maxCode)
	s.scanTree(s.dDesc.dynTree, s.dDesc.maxCode)

	s.buildTree(&s.blDesc)

	maxBlindex := zBLCodes - 1
	for ; maxBlindex >= 3; maxBlindex-- {
		if s.blTree[blOrder[maxBlindex]].dadOrLen != 0 {
			break
		}
	}
	s.optLen += 3*int64(maxBlindex+1) + 5 + 5 + 4
	return maxBlindex
}

func (s *zState) sendAllTrees(lcodes, dcodes, blcodes int) {
	s.sendBits(uint32(lcodes-257), 5)
	s.sendBits(uint32(dcodes-1), 5)
	s.sendBits(uint32(blcodes-4), 4)
	for rank := 0; rank < blcodes; rank++ {
		s.sendBits(uint32(s.blTree[blOrder[rank]].dadOrLen), 3)
	}
	s.sendTree(s.lDesc.dynTree, lcodes-1)
	s.sendTree(s.dDesc.dynTree, dcodes-1)
}

func (s *zState) compressBlock(ltree, dtree []ctData) {
	sx := 0
	if s.symNext != 0 {
		for {
			dist := int(s.symBuf[sx]) | int(s.symBuf[sx+1])<<8
			lc := int(s.symBuf[sx+2])
			sx += 3
			if dist == 0 {
				s.sendCode(lc, ltree)
			} else {
				code := int(lengthCodeTable[lc])
				s.sendCode(code+zLiterals+1, ltree)
				extra := extraLBits[code]
				if extra != 0 {
					lc -= baseLengthTable[code]
					s.sendBits(uint32(lc), extra)
				}
				dist--
				code = int(dCode(dist))
				s.sendCode(code, dtree)
				extra = extraDBits[code]
				if extra != 0 {
					dist -= baseDistTable[code]
					s.sendBits(uint32(dist), extra)
				}
			}
			if sx >= s.symNext {
				break
			}
		}
	}
	s.sendCode(zEndBlock, ltree)
}

func (s *zState) flushBlockLast() {
	// Construct the literal/length and distance trees.
	s.buildTree(&s.lDesc)
	s.buildTree(&s.dDesc)

	maxBlindex := s.buildBlTree()

	optLenb := (s.optLen + 3 + 7) >> 3
	staticLenb := (s.staticLen + 3 + 7) >> 3
	if staticLenb <= optLenb {
		optLenb = staticLenb
	}

	storedLen := int64(s.dataLen)

	const last = 1

	if storedLen+4 <= optLenb {
		// stored block
		s.sendBits(uint32((zStoredBlock<<1)+last), 3)
		s.biWindup()
		s.putShort(uint16(storedLen))
		s.putShort(uint16(^uint16(storedLen)))
		if storedLen > 0 {
			s.pending = append(s.pending, s.window[:s.dataLen]...)
		}
	} else if staticLenb == optLenb {
		s.sendBits(uint32((zStaticTrees<<1)+last), 3)
		s.compressBlock(staticLtree(), staticDtree())
	} else {
		s.sendBits(uint32((zDynTrees<<1)+last), 3)
		s.sendAllTrees(s.lDesc.maxCode+1, s.dDesc.maxCode+1, maxBlindex+1)
		s.compressBlock(s.lDesc.dynTree, s.dDesc.dynTree)
	}

	s.initBlock()
	s.biWindup()
}

func staticLtreeData() [zLCodes + 2]ctData {
	var t [zLCodes + 2]ctData
	for i := range t {
		t[i] = ctData{freqOrCode: staticLTreeCode[i], dadOrLen: staticLTreeLen[i]}
	}
	return t
}

func staticDtreeData() [zDCodes]ctData {
	var t [zDCodes]ctData
	for i := range t {
		t[i] = ctData{freqOrCode: staticDTreeCode[i], dadOrLen: staticDTreeLen[i]}
	}
	return t
}

var cachedStaticLtree = staticLtreeData()
var cachedStaticDtree = staticDtreeData()

func staticLtree() []ctData { return cachedStaticLtree[:] }
func staticDtree() []ctData { return cachedStaticDtree[:] }

// zlibDeflate compresses data exactly as zlib 1.2.12 would with
// deflateInit2(level=6, Z_DEFLATED, windowBits=14, memLevel=7, Z_DEFAULT_STRATEGY),
// producing a standard zlib-wrapped stream (2-byte header + deflate data +
// 4-byte big-endian Adler32 trailer).
func zlibDeflate(data []byte) []byte {
	s := newZState(data)

	// zlib header: CM=8 (deflate), CINFO=w_bits-8=6; FLEVEL=2 for level 6; FDICT=0.
	// Written MSB-first (putShortMSB in zlib), unlike the LSB-first put_short
	// used elsewhere for stored-block length fields.
	header := uint16(8+((zWBits-8)<<4)) << 8
	header |= 2 << 6 // level_flags = 2 for level == 6
	header += uint16(31 - (header % 31))
	s.putByte(byte(header >> 8))
	s.putByte(byte(header & 0xff))

	if len(data) > 0 {
		s.deflateSlow()
	}
	s.flushBlockLast()

	sum := adler32.Checksum(data)
	trailer := make([]byte, 4)
	binary.BigEndian.PutUint32(trailer, sum)
	s.pending = append(s.pending, trailer...)

	return s.pending
}
