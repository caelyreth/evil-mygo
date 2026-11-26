package update

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/adler32"
	"io"
)

// RFC 3284 VCDIFF binary differencing and compression format.
var vcdiffMagic = [4]byte{0xD6, 0xC3, 0xC4, 0x00}

const (
	vcdDecompress = 1 << 0
	vcdCodetable  = 1 << 1

	vcdSource  = 1 << 0
	vcdTarget  = 1 << 1
	vcdAdler32 = 1 << 2 // common extended flag (xdelta3)

	vcdNOOP uint8 = 0
	vcdADD  uint8 = 1
	vcdRUN  uint8 = 2
	vcdCOPY uint8 = 3

	vcdNearSize = 4
	vcdSameSize = 3

	vcdWindowSize  = 8 << 20
	vcdMaxWindow   = 64 << 20
	vcdMaxWholeSrc = 64 << 20
	vcdMinMatch    = 8
	vcdMaxChain    = 16
)

var errDamagedPatch = errors.New("damaged patch")

type vcdEntry struct {
	op1, size1, mode1 uint8
	op2, size2, mode2 uint8
}

var (
	vcdDefaultTable [256]vcdEntry
	vcdSingleADD    [18]uint8
	vcdSingleCOPY   [9][19]uint8
	vcdPairADDCOPY  [9][5][7]uint8
)

func init() {
	// Populate RFC 3284 Section 5.6 default code table.
	idx := 0
	vcdDefaultTable[idx] = vcdEntry{op1: vcdRUN}
	idx++
	for sz := 0; sz <= 17; sz++ {
		vcdDefaultTable[idx] = vcdEntry{op1: vcdADD, size1: uint8(sz)}
		vcdSingleADD[sz] = uint8(idx)
		idx++
	}
	for mode := range 9 {
		vcdDefaultTable[idx] = vcdEntry{op1: vcdCOPY, size1: 0, mode1: uint8(mode)}
		vcdSingleCOPY[mode][0] = uint8(idx)
		idx++
		for sz := 4; sz <= 18; sz++ {
			vcdDefaultTable[idx] = vcdEntry{op1: vcdCOPY, size1: uint8(sz), mode1: uint8(mode)}
			vcdSingleCOPY[mode][sz] = uint8(idx)
			idx++
		}
	}
	for mode := 0; mode <= 5; mode++ {
		for addSz := 1; addSz <= 4; addSz++ {
			for copySz := 4; copySz <= 6; copySz++ {
				vcdDefaultTable[idx] = vcdEntry{
					op1: vcdADD, size1: uint8(addSz),
					op2: vcdCOPY, size2: uint8(copySz), mode2: uint8(mode),
				}
				vcdPairADDCOPY[mode][addSz][copySz] = uint8(idx)
				idx++
			}
		}
	}
	for mode := 6; mode <= 8; mode++ {
		for addSz := 1; addSz <= 4; addSz++ {
			vcdDefaultTable[idx] = vcdEntry{
				op1: vcdADD, size1: uint8(addSz),
				op2: vcdCOPY, size2: 4, mode2: uint8(mode),
			}
			vcdPairADDCOPY[mode][addSz][4] = uint8(idx)
			idx++
		}
	}
	for mode := 0; mode <= 8; mode++ {
		vcdDefaultTable[idx] = vcdEntry{
			op1: vcdCOPY, size1: 4, mode1: uint8(mode),
			op2: vcdADD, size2: 1,
		}
		idx++
	}
}

type vcdAddrCache struct {
	near [vcdNearSize]int64
	next int
	same [vcdSameSize * 256]int64
}

func (c *vcdAddrCache) reset() {
	clear(c.near[:])
	c.next = 0
	clear(c.same[:])
}

func (c *vcdAddrCache) update(addr int64) {
	c.near[c.next] = addr
	c.next = (c.next + 1) % vcdNearSize
	c.same[addr%(vcdSameSize*256)] = addr
}

func (c *vcdAddrCache) encode(addr, here int64) (mode uint8, raw []byte) {
	var buf [10]byte
	bestMode := uint8(0)
	best := appendVCDInt(buf[:0], uint64(addr))

	if d := here - addr; d >= 0 {
		cand := appendVCDInt(nil, uint64(d))
		if len(cand) < len(best) {
			bestMode, best = 1, cand
		}
	}
	for m := range vcdNearSize {
		if d := addr - c.near[m]; d >= 0 {
			cand := appendVCDInt(nil, uint64(d))
			if len(cand) < len(best) {
				bestMode, best = uint8(2+m), cand
			}
		}
	}
	if slot := addr % (vcdSameSize * 256); c.same[slot] == addr {
		bestMode = uint8(6 + slot/256)
		best = []byte{byte(slot % 256)}
	}
	c.update(addr)
	return bestMode, best
}

func (c *vcdAddrCache) decode(mode uint8, here int64, r *bytes.Reader) (int64, error) {
	var addr int64
	switch {
	case mode == 0:
		v, err := readVCDInt(r)
		if err != nil || v > uint64(here) {
			return 0, errDamagedPatch
		}
		addr = int64(v)
	case mode == 1:
		v, err := readVCDInt(r)
		if err != nil || v > uint64(here) {
			return 0, errDamagedPatch
		}
		addr = here - int64(v)
	case mode >= 2 && mode < 2+vcdNearSize:
		v, err := readVCDInt(r)
		if err != nil {
			return 0, errDamagedPatch
		}
		base := c.near[mode-2]
		if v > uint64(here-base) {
			return 0, errDamagedPatch
		}
		addr = base + int64(v)
	case mode >= 2+vcdNearSize && mode < 2+vcdNearSize+vcdSameSize:
		b, err := r.ReadByte()
		if err != nil {
			return 0, errDamagedPatch
		}
		m := int(mode - 2 - vcdNearSize)
		addr = c.same[m*256+int(b)]
	default:
		return 0, errDamagedPatch
	}
	if addr < 0 || addr >= here {
		return 0, errDamagedPatch
	}
	c.update(addr)
	return addr, nil
}

// EncodeVCDIFF returns a pure IETF RFC 3284 VCDIFF delta turning old into new.
func EncodeVCDIFF(old, new []byte) []byte {
	return encodeRawVCDIFF(old, new)
}

func encodeRawVCDIFF(old, new []byte) []byte {
	var out bytes.Buffer
	out.Write(vcdiffMagic[:])
	out.WriteByte(0) // Hdr_Indicator = 0 (no secondary compressor, default code table)

	if len(new) == 0 {
		return out.Bytes()
	}

	var fullIdx *sourceIndex
	if len(old) >= vcdMinMatch && len(old) <= vcdMaxWholeSrc {
		fullIdx = buildSourceIndex(old)
	}

	for winOff := 0; winOff < len(new); winOff += vcdWindowSize {
		target := new[winOff:min(winOff+vcdWindowSize, len(new))]
		var src []byte
		var srcOff int
		idx := fullIdx
		if len(old) >= vcdMinMatch {
			if fullIdx != nil {
				src, srcOff = old, 0
			} else {
				pad := vcdMaxWholeSrc/2 - len(target)/2
				lo := max(0, winOff-pad)
				hi := min(len(old), lo+vcdMaxWholeSrc)
				lo = max(0, hi-vcdMaxWholeSrc)
				src, srcOff = old[lo:hi], lo
				idx = buildSourceIndex(src)
			}
		}
		writeVCDWindow(&out, src, srcOff, target, idx)
	}
	return out.Bytes()
}

type sourceIndex struct {
	src    []byte
	stride int
	head   []int32
	prev   []int32
	mask   uint32
}

func buildSourceIndex(src []byte) *sourceIndex {
	if len(src) < vcdMinMatch {
		return nil
	}
	stride := 1
	if len(src) > 16<<20 {
		stride = 4
	}
	nSlots := (len(src)-vcdMinMatch)/stride + 1
	bits := 16
	for (1<<bits) < nSlots && bits < 24 {
		bits++
	}
	tableSize := 1 << bits
	head := make([]int32, tableSize)
	for i := range head {
		head[i] = -1
	}
	prev := make([]int32, nSlots)
	mask := uint32(tableSize - 1)

	slot := 0
	for pos := 0; pos+vcdMinMatch <= len(src); pos += stride {
		h := hash8(src[pos:]) & mask
		prev[slot] = head[h]
		head[h] = int32(slot)
		slot++
	}
	return &sourceIndex{src: src, stride: stride, head: head, prev: prev, mask: mask}
}

func hash8(b []byte) uint32 {
	v := binary.LittleEndian.Uint64(b[:8]) * 11400714819323198485
	return uint32(v >> 32)
}

func (idx *sourceIndex) bestMatch(src []byte, target []byte, pos, pendingStart, lastSrcMatch int) (srcPos, back, length int) {
	if idx == nil || pos+vcdMinMatch > len(target) {
		return 0, 0, 0
	}
	// Fast check right after the previous source match (handles minor shifts cleanly).
	if lastSrcMatch >= 0 && lastSrcMatch+vcdMinMatch <= len(src) {
		if l := commonPrefix(src[lastSrcMatch:], target[pos:]); l >= vcdMinMatch {
			length, srcPos = l, lastSrcMatch
		}
	}

	// Probe shifted offsets if stride > 1 to catch unaligned matches too.
	for probe := 0; probe < idx.stride && pos+probe+vcdMinMatch <= len(target); probe++ {
		h := hash8(target[pos+probe:]) & idx.mask
		steps := 0
		for slot := idx.head[h]; slot >= 0 && steps < vcdMaxChain; slot = idx.prev[slot] {
			steps++
			candSrc := int(slot)*idx.stride - probe
			if candSrc < 0 || candSrc+vcdMinMatch > len(src) {
				continue
			}
			if length > 0 && candSrc+length <= len(src) && pos+length <= len(target) {
				if src[candSrc+length-1] != target[pos+length-1] {
					continue
				}
			}
			fwd := commonPrefix(src[candSrc:], target[pos:])
			if fwd < vcdMinMatch {
				continue
			}
			b := 0
			for b < pos-pendingStart && b < candSrc && src[candSrc-1-b] == target[pos-1-b] {
				b++
			}
			if fwd+b > length+back {
				srcPos, back, length = candSrc-b, b, fwd+b
			}
		}
	}
	return srcPos, back, length
}

func commonPrefix(a, b []byte) int {
	n := min(len(a), len(b))
	i := 0
	for i+8 <= n && binary.LittleEndian.Uint64(a[i:]) == binary.LittleEndian.Uint64(b[i:]) {
		i += 8
	}
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

type rawVCDInst struct {
	op   uint8
	size int
	addr int64 // relative to window's U (source + target)
	here int64
}

func writeVCDWindow(out *bytes.Buffer, src []byte, srcOff int, target []byte, idx *sourceIndex) {
	var dataSec, instSec, addrSec bytes.Buffer
	var cache vcdAddrCache
	cache.reset()

	var insts []rawVCDInst
	srcLen := int64(len(src))
	pendingStart := 0
	lastSrcEnd := -1
	pos := 0

	flushADD := func(end int) {
		if end > pendingStart {
			dataSec.Write(target[pendingStart:end])
			insts = append(insts, rawVCDInst{op: vcdADD, size: end - pendingStart})
			pendingStart = end
		}
	}

	for pos < len(target) {
		// Detect runs of >= 8 identical bytes.
		runLen := 1
		if pos+vcdMinMatch <= len(target) {
			b0 := target[pos]
			for pos+runLen < len(target) && target[pos+runLen] == b0 {
				runLen++
			}
		}
		if runLen >= vcdMinMatch {
			flushADD(pos)
			dataSec.WriteByte(target[pos])
			insts = append(insts, rawVCDInst{op: vcdRUN, size: runLen})
			pos += runLen
			pendingStart = pos
			lastSrcEnd = -1
			continue
		}

		srcPos, back, mlen := idx.bestMatch(src, target, pos, pendingStart, lastSrcEnd)
		if mlen >= vcdMinMatch {
			matchStart := pos - back
			flushADD(matchStart)
			here := srcLen + int64(matchStart)
			insts = append(insts, rawVCDInst{
				op:   vcdCOPY,
				size: mlen,
				addr: int64(srcPos),
				here: here,
			})
			pos = matchStart + mlen
			pendingStart = pos
			lastSrcEnd = srcPos + mlen
			continue
		}
		pos++
	}
	flushADD(len(target))

	// Encode instruction + address sections with RFC 3284 default code-table pairing.
	var iBuf, aBuf [10]byte
	for i := 0; i < len(insts); i++ {
		cur := insts[i]
		// Attempt (ADD 1..4, COPY 4..6 mode 0..5) or (ADD 1..4, COPY 4 mode 6..8) pair.
		if cur.op == vcdADD && cur.size <= 4 && i+1 < len(insts) && insts[i+1].op == vcdCOPY {
			nxt := insts[i+1]
			mode, rawAddr := cache.encode(nxt.addr, nxt.here)
			if (mode <= 5 && nxt.size >= 4 && nxt.size <= 6) || (mode >= 6 && nxt.size == 4) {
				instSec.WriteByte(vcdPairADDCOPY[mode][cur.size][nxt.size])
				addrSec.Write(rawAddr)
				i++
				continue
			}
			// Didn't pair — emit standalone ADD and handle COPY on next iteration cleanly
			// without consuming cache yet. Resetting isn't needed if we emit COPY right now:
			emitADD(&instSec, &iBuf, cur.size)
			emitCOPY(&instSec, &addrSec, &iBuf, &aBuf, nxt.size, mode, rawAddr)
			i++
			continue
		}
		switch cur.op {
		case vcdADD:
			emitADD(&instSec, &iBuf, cur.size)
		case vcdRUN:
			instSec.WriteByte(0)
			instSec.Write(appendVCDInt(iBuf[:0], uint64(cur.size)))
		case vcdCOPY:
			mode, rawAddr := cache.encode(cur.addr, cur.here)
			emitCOPY(&instSec, &addrSec, &iBuf, &aBuf, cur.size, mode, rawAddr)
		}
	}

	var winBody bytes.Buffer
	var tmp [10]byte
	winBody.Write(appendVCDInt(tmp[:0], uint64(len(target))))
	winBody.WriteByte(0) // Delta_Indicator = 0 (uncompressed sections)
	winBody.Write(appendVCDInt(tmp[:0], uint64(dataSec.Len())))
	winBody.Write(appendVCDInt(tmp[:0], uint64(instSec.Len())))
	winBody.Write(appendVCDInt(tmp[:0], uint64(addrSec.Len())))
	winBody.Write(dataSec.Bytes())
	winBody.Write(instSec.Bytes())
	winBody.Write(addrSec.Bytes())

	var winInd byte
	if len(src) > 0 {
		winInd = vcdSource
	}
	out.WriteByte(winInd)
	if winInd == vcdSource {
		out.Write(appendVCDInt(tmp[:0], uint64(len(src))))
		out.Write(appendVCDInt(tmp[:0], uint64(srcOff)))
	}
	out.Write(appendVCDInt(tmp[:0], uint64(winBody.Len())))
	out.Write(winBody.Bytes())
}

func emitADD(instSec *bytes.Buffer, tmp *[10]byte, size int) {
	if size <= 17 {
		instSec.WriteByte(vcdSingleADD[size])
		return
	}
	instSec.WriteByte(vcdSingleADD[0])
	instSec.Write(appendVCDInt(tmp[:0], uint64(size)))
}

func emitCOPY(instSec, addrSec *bytes.Buffer, tmp, _ *[10]byte, size int, mode uint8, rawAddr []byte) {
	if size >= 4 && size <= 18 {
		instSec.WriteByte(vcdSingleCOPY[mode][size])
	} else {
		instSec.WriteByte(vcdSingleCOPY[mode][0])
		instSec.Write(appendVCDInt(tmp[:0], uint64(size)))
	}
	addrSec.Write(rawAddr)
}

// DecodeVCDIFF applies a pure IETF RFC 3284 VCDIFF patch to old, writing
// exactly size target bytes to w.
func DecodeVCDIFF(w io.Writer, size int64, old io.ReaderAt, oldSize int64, patch io.Reader) error {
	return decodeRawVCDIFF(w, size, old, oldSize, patch)
}

type byteReaderAdapter struct {
	r   io.Reader
	buf [1]byte
}

func (b *byteReaderAdapter) ReadByte() (byte, error) {
	n, err := b.r.Read(b.buf[:])
	if n == 1 {
		return b.buf[0], nil
	}
	if err == nil {
		return 0, io.EOF
	}
	return 0, err
}

func decodeRawVCDIFF(w io.Writer, size int64, old io.ReaderAt, oldSize int64, srcReader io.Reader) error {
	if size < 0 {
		return errDamagedPatch
	}
	r := &byteReaderAdapter{r: srcReader}
	var hdr [5]byte
	if _, err := io.ReadFull(srcReader, hdr[:]); err != nil {
		return errDamagedPatch
	}
	if [4]byte(hdr[:4]) != vcdiffMagic {
		return errDamagedPatch
	}
	hdrInd := hdr[4]
	if hdrInd != 0 {
		// Standard RFC 3284 allows VCD_DECOMPRESS (1) or VCD_CODETABLE (2).
		return errDamagedPatch
	}

	var cache vcdAddrCache
	var written int64
	var targetHist []byte

	for {
		winInd, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errDamagedPatch
		}
		if winInd&^(vcdSource|vcdTarget|vcdAdler32) != 0 {
			return errDamagedPatch
		}
		srcKind := winInd & (vcdSource | vcdTarget)
		if srcKind == vcdSource|vcdTarget {
			return errDamagedPatch
		}

		var srcSegLen, srcSegPos int64
		if srcKind != 0 {
			uLen, err := readVCDInt(r)
			if err != nil || uLen > vcdMaxWindow {
				return errDamagedPatch
			}
			uPos, err := readVCDInt(r)
			if err != nil || uPos > 1<<50 {
				return errDamagedPatch
			}
			srcSegLen, srcSegPos = int64(uLen), int64(uPos)
			if srcKind == vcdSource && (srcSegPos+srcSegLen > oldSize) {
				return errDamagedPatch
			}
			if srcKind == vcdTarget && (srcSegPos+srcSegLen > int64(len(targetHist))) {
				return errDamagedPatch
			}
		}

		deltaLen, err := readVCDInt(r)
		if err != nil || deltaLen == 0 || deltaLen > vcdMaxWindow {
			return errDamagedPatch
		}
		winRaw := make([]byte, deltaLen)
		if _, err := io.ReadFull(srcReader, winRaw); err != nil {
			return errDamagedPatch
		}

		br := bytes.NewReader(winRaw)
		tgtLenU, err := readVCDInt(br)
		if err != nil || tgtLenU > vcdMaxWindow || int64(tgtLenU) > size-written {
			return errDamagedPatch
		}
		tgtLen := int(tgtLenU)

		deltaInd, err := br.ReadByte()
		if err != nil || deltaInd != 0 {
			return errDamagedPatch
		}
		dLenU, err := readVCDInt(br)
		if err != nil {
			return errDamagedPatch
		}
		iLenU, err := readVCDInt(br)
		if err != nil {
			return errDamagedPatch
		}
		aLenU, err := readVCDInt(br)
		if err != nil {
			return errDamagedPatch
		}
		hdrBytes := uint64(len(winRaw) - br.Len())
		adlerBytes := uint64(0)
		if winInd&vcdAdler32 != 0 {
			adlerBytes = 4
		}
		if hdrBytes+dLenU+iLenU+aLenU+adlerBytes != deltaLen {
			return errDamagedPatch
		}
		off := int(hdrBytes)
		dataSec := bytes.NewReader(winRaw[off : off+int(dLenU)])
		off += int(dLenU)
		instSec := bytes.NewReader(winRaw[off : off+int(iLenU)])
		off += int(iLenU)
		addrSec := bytes.NewReader(winRaw[off : off+int(aLenU)])
		off += int(aLenU)

		var wantAdler uint32
		hasAdler := winInd&vcdAdler32 != 0
		if hasAdler {
			wantAdler = binary.BigEndian.Uint32(winRaw[off : off+4])
		}

		// Window working buffer: [sourceSeg | decodedTargetWindow] for O(1) self/source COPY.
		uBuf := make([]byte, int(srcSegLen)+tgtLen)
		if srcSegLen > 0 {
			switch srcKind {
			case vcdSource:
				if _, err := io.NewSectionReader(old, srcSegPos, srcSegLen).Read(uBuf[:srcSegLen]); err != nil {
					return err
				}
			case vcdTarget:
				copy(uBuf[:srcSegLen], targetHist[srcSegPos:srcSegPos+srcSegLen])
			}
		}

		cache.reset()
		outPos := int(srcSegLen)
		outEnd := len(uBuf)

		execOne := func(op, szImm, mode uint8) error {
			if op == vcdNOOP {
				return nil
			}
			sz := int(szImm)
			if sz == 0 {
				v, err := readVCDInt(instSec)
				if err != nil || v > uint64(outEnd-outPos) {
					return errDamagedPatch
				}
				sz = int(v)
			} else if sz > outEnd-outPos {
				return errDamagedPatch
			}
			switch op {
			case vcdADD:
				if _, err := io.ReadFull(dataSec, uBuf[outPos:outPos+sz]); err != nil {
					return errDamagedPatch
				}
				outPos += sz
			case vcdRUN:
				b, err := dataSec.ReadByte()
				if err != nil {
					return errDamagedPatch
				}
				for i := range sz {
					uBuf[outPos+i] = b
				}
				outPos += sz
			case vcdCOPY:
				addr, err := cache.decode(mode, int64(outPos), addrSec)
				if err != nil {
					return errDamagedPatch
				}
				a := int(addr)
				if a+sz <= outPos {
					copy(uBuf[outPos:outPos+sz], uBuf[a:a+sz])
				} else {
					for i := range sz {
						uBuf[outPos+i] = uBuf[a+i]
					}
				}
				outPos += sz
			default:
				return errDamagedPatch
			}
			return nil
		}

		for {
			code, err := instSec.ReadByte()
			if err == io.EOF {
				break
			}
			if err != nil {
				return errDamagedPatch
			}
			ent := vcdDefaultTable[code]
			if err := execOne(ent.op1, ent.size1, ent.mode1); err != nil {
				return err
			}
			if err := execOne(ent.op2, ent.size2, ent.mode2); err != nil {
				return err
			}
		}

		if outPos != outEnd || dataSec.Len() != 0 || addrSec.Len() != 0 {
			return errDamagedPatch
		}
		winTarget := uBuf[srcSegLen:]
		if hasAdler && adler32.Checksum(winTarget) != wantAdler {
			return errDamagedPatch
		}
		if _, err := w.Write(winTarget); err != nil {
			return err
		}
		written += int64(tgtLen)
		// Keep previous target bytes only if file fits within VCD_TARGET window limit.
		if int64(len(targetHist))+int64(len(winTarget)) <= vcdMaxWholeSrc {
			targetHist = append(targetHist, winTarget...)
		}
	}

	if written != size {
		return errDamagedPatch
	}
	return nil
}

func appendVCDInt(dst []byte, v uint64) []byte {
	var buf [10]byte
	i := len(buf) - 1
	buf[i] = byte(v & 0x7F)
	v >>= 7
	for v > 0 {
		i--
		buf[i] = byte(v&0x7F) | 0x80
		v >>= 7
	}
	return append(dst, buf[i:]...)
}

func readVCDInt(r io.ByteReader) (uint64, error) {
	var v uint64
	for range 9 {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		if v > (1<<(63-7))-1 {
			return 0, errDamagedPatch
		}
		v = (v << 7) | uint64(b&0x7F)
		if b&0x80 == 0 {
			return v, nil
		}
	}
	return 0, errDamagedPatch
}

func streamEOF(r io.Reader) bool {
	var b [1]byte
	n, err := r.Read(b[:])
	return n == 0 && err == io.EOF
}
