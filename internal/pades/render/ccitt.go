package render

import (
	"fmt"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// CCITT Group 3 and Group 4 fax decoding (ITU-T T.4 and T.6), which is
// how a scanned page is stored in a PDF when it was not stored as a
// JPEG. A scanned contract is an ordinary thing to be asked to sign, so
// a preview that cannot draw one is a preview that fails on exactly the
// documents a person most needs to look at before placing a stamp.
//
// The code tables below are the ones in T.4 §2.1 and T.6, written out.
// They are the sort of data a single transposed bit makes subtly wrong,
// so the decoder is tested against images compressed by an independent
// encoder rather than against itself — see ccitt_test.go.

type ccittCode struct {
	bits uint32
	n    int
	run  int
}

// whiteCodes and blackCodes are the terminating (0..63) and make-up
// (64..1728) run lengths of T.4's one-dimensional code.
var whiteCodes = []ccittCode{
	{0x35, 8, 0}, {0x07, 6, 1}, {0x07, 4, 2}, {0x08, 4, 3}, {0x0B, 4, 4},
	{0x0C, 4, 5}, {0x0E, 4, 6}, {0x0F, 4, 7}, {0x13, 5, 8}, {0x14, 5, 9},
	{0x07, 5, 10}, {0x08, 5, 11}, {0x08, 6, 12}, {0x03, 6, 13}, {0x34, 6, 14},
	{0x35, 6, 15}, {0x2A, 6, 16}, {0x2B, 6, 17}, {0x27, 7, 18}, {0x0C, 7, 19},
	{0x08, 7, 20}, {0x17, 7, 21}, {0x03, 7, 22}, {0x04, 7, 23}, {0x28, 7, 24},
	{0x2B, 7, 25}, {0x13, 7, 26}, {0x24, 7, 27}, {0x18, 7, 28}, {0x02, 8, 29},
	{0x03, 8, 30}, {0x1A, 8, 31}, {0x1B, 8, 32}, {0x12, 8, 33}, {0x13, 8, 34},
	{0x14, 8, 35}, {0x15, 8, 36}, {0x16, 8, 37}, {0x17, 8, 38}, {0x28, 8, 39},
	{0x29, 8, 40}, {0x2A, 8, 41}, {0x2B, 8, 42}, {0x2C, 8, 43}, {0x2D, 8, 44},
	{0x04, 8, 45}, {0x05, 8, 46}, {0x0A, 8, 47}, {0x0B, 8, 48}, {0x52, 8, 49},
	{0x53, 8, 50}, {0x54, 8, 51}, {0x55, 8, 52}, {0x24, 8, 53}, {0x25, 8, 54},
	{0x58, 8, 55}, {0x59, 8, 56}, {0x5A, 8, 57}, {0x5B, 8, 58}, {0x4A, 8, 59},
	{0x4B, 8, 60}, {0x32, 8, 61}, {0x33, 8, 62}, {0x34, 8, 63},

	{0x1B, 5, 64}, {0x12, 5, 128}, {0x17, 6, 192}, {0x37, 7, 256},
	{0x36, 8, 320}, {0x37, 8, 384}, {0x64, 8, 448}, {0x65, 8, 512},
	{0x68, 8, 576}, {0x67, 8, 640}, {0xCC, 9, 704}, {0xCD, 9, 768},
	{0xD2, 9, 832}, {0xD3, 9, 896}, {0xD4, 9, 960}, {0xD5, 9, 1024},
	{0xD6, 9, 1088}, {0xD7, 9, 1152}, {0xD8, 9, 1216}, {0xD9, 9, 1280},
	{0xDA, 9, 1344}, {0xDB, 9, 1408}, {0x98, 9, 1472}, {0x99, 9, 1536},
	{0x9A, 9, 1600}, {0x18, 6, 1664}, {0x9B, 9, 1728},
}

var blackCodes = []ccittCode{
	{0x37, 10, 0}, {0x02, 3, 1}, {0x03, 2, 2}, {0x02, 2, 3}, {0x03, 3, 4},
	{0x03, 4, 5}, {0x02, 4, 6}, {0x03, 5, 7}, {0x05, 6, 8}, {0x04, 6, 9},
	{0x04, 7, 10}, {0x05, 7, 11}, {0x07, 7, 12}, {0x04, 8, 13}, {0x07, 8, 14},
	{0x18, 9, 15}, {0x17, 10, 16}, {0x18, 10, 17}, {0x08, 10, 18},
	{0x67, 11, 19}, {0x68, 11, 20}, {0x6C, 11, 21}, {0x37, 11, 22},
	{0x28, 11, 23}, {0x17, 11, 24}, {0x18, 11, 25}, {0xCA, 12, 26},
	{0xCB, 12, 27}, {0xCC, 12, 28}, {0xCD, 12, 29}, {0x68, 12, 30},
	{0x69, 12, 31}, {0x6A, 12, 32}, {0x6B, 12, 33}, {0xD2, 12, 34},
	{0xD3, 12, 35}, {0xD4, 12, 36}, {0xD5, 12, 37}, {0xD6, 12, 38},
	{0xD7, 12, 39}, {0x6C, 12, 40}, {0x6D, 12, 41}, {0xDA, 12, 42},
	{0xDB, 12, 43}, {0x54, 12, 44}, {0x55, 12, 45}, {0x56, 12, 46},
	{0x57, 12, 47}, {0x64, 12, 48}, {0x65, 12, 49}, {0x52, 12, 50},
	{0x53, 12, 51}, {0x24, 12, 52}, {0x37, 12, 53}, {0x38, 12, 54},
	{0x27, 12, 55}, {0x28, 12, 56}, {0x58, 12, 57}, {0x59, 12, 58},
	{0x2B, 12, 59}, {0x2C, 12, 60}, {0x5A, 12, 61}, {0x66, 12, 62},
	{0x67, 12, 63},

	{0x0F, 10, 64}, {0xC8, 12, 128}, {0xC9, 12, 192}, {0x5B, 12, 256},
	{0x33, 12, 320}, {0x34, 12, 384}, {0x35, 12, 448}, {0x6C, 13, 512},
	{0x6D, 13, 576}, {0x4A, 13, 640}, {0x4B, 13, 704}, {0x4C, 13, 768},
	{0x4D, 13, 832}, {0x72, 13, 896}, {0x73, 13, 960}, {0x74, 13, 1024},
	{0x75, 13, 1088}, {0x76, 13, 1152}, {0x77, 13, 1216}, {0x52, 13, 1280},
	{0x53, 13, 1344}, {0x54, 13, 1408}, {0x55, 13, 1472}, {0x5A, 13, 1536},
	{0x5B, 13, 1600}, {0x64, 13, 1664}, {0x65, 13, 1728},
}

// extendedCodes are the make-up codes above 1728, shared by both colours
// (T.4 table 3).
var extendedCodes = []ccittCode{
	{0x08, 11, 1792}, {0x0C, 11, 1856}, {0x0D, 11, 1920}, {0x12, 12, 1984},
	{0x13, 12, 2048}, {0x14, 12, 2112}, {0x15, 12, 2176}, {0x16, 12, 2240},
	{0x17, 12, 2304}, {0x1C, 12, 2368}, {0x1D, 12, 2432}, {0x1E, 12, 2496},
	{0x1F, 12, 2560},
}

type ccittTable map[uint32]int

func buildTable(sets ...[]ccittCode) ccittTable {
	t := ccittTable{}
	for _, set := range sets {
		for _, c := range set {
			t[uint32(c.n)<<16|c.bits] = c.run
		}
	}
	return t
}

var (
	whiteTable = buildTable(whiteCodes, extendedCodes)
	blackTable = buildTable(blackCodes, extendedCodes)
)

type bitReader struct {
	data []byte
	pos  int // in bits
}

func (b *bitReader) eof() bool { return b.pos >= len(b.data)*8 }

func (b *bitReader) peek(n int) (uint32, bool) {
	if b.pos+n > len(b.data)*8 {
		return 0, false
	}
	var v uint32
	for i := 0; i < n; i++ {
		p := b.pos + i
		bit := (b.data[p/8] >> uint(7-p%8)) & 1
		v = v<<1 | uint32(bit)
	}
	return v, true
}

func (b *bitReader) skip(n int) { b.pos += n }

func (b *bitReader) alignByte() { b.pos = (b.pos + 7) / 8 * 8 }

// readRun reads one complete run length: make-up codes accumulate until
// a terminating code (under 64) ends the run.
func (b *bitReader) readRun(t ccittTable) (int, bool) {
	total := 0
	for i := 0; i < 64; i++ {
		run, ok := b.readCode(t)
		if !ok {
			return 0, false
		}
		total += run
		if run < 64 {
			return total, true
		}
	}
	return total, true
}

func (b *bitReader) readCode(t ccittTable) (int, bool) {
	for n := 2; n <= 14; n++ {
		v, ok := b.peek(n)
		if !ok {
			return 0, false
		}
		if run, found := t[uint32(n)<<16|v]; found {
			b.skip(n)
			return run, true
		}
	}
	return 0, false
}

// ccittDecode decodes a CCITT-compressed image into one bit per pixel,
// packed MSB first, in the sense PDF's DeviceGray reading expects:
// a 0 bit is black.
func ccittDecode(data []byte, doc *pdf.Document, parms pdf.Dict, width, height int) ([]byte, error) {
	k := 0
	columns := 1728
	blackIs1 := false
	byteAlign := false
	if parms != nil {
		if v, ok := asInt(doc.Resolve(parms.Get(pdf.Name("K")))); ok {
			k = v
		}
		if v, ok := asInt(doc.Resolve(parms.Get(pdf.Name("Columns")))); ok {
			columns = v
		}
		if v, ok := doc.Resolve(parms.Get(pdf.Name("BlackIs1"))).(bool); ok {
			blackIs1 = v
		}
		if v, ok := doc.Resolve(parms.Get(pdf.Name("EncodedByteAlign"))).(bool); ok {
			byteAlign = v
		}
	}
	if columns <= 0 {
		columns = width
	}
	if columns != width && width > 0 {
		// The image dictionary's /Width is what the page is drawn
		// against; /Columns is what the codec was given. They agree in
		// every well-formed file, and where they do not, the image's
		// own width is what the rest of this renderer measures with.
		columns = width
	}
	if columns <= 0 || height <= 0 || columns > 1<<16 {
		return nil, fmt.Errorf("render: CCITT image has no usable size")
	}

	rowBytes := (columns + 7) / 8
	out := make([]byte, rowBytes*height)

	br := &bitReader{data: data}
	// ref is the previous row's changing elements: the positions where
	// colour changes, which two-dimensional coding is written relative
	// to. An imaginary all-white line precedes the first row.
	ref := []int{columns, columns}

	for row := 0; row < height; row++ {
		if byteAlign && k >= 0 {
			br.alignByte()
		}
		skipEOL(br)
		twoD := k < 0
		if k > 0 {
			// Mixed mode: one bit after the EOL says which coding this
			// row uses.
			if v, ok := br.peek(1); ok {
				twoD = v == 0
				br.skip(1)
			}
		}
		if byteAlign && k < 0 {
			br.alignByte()
		}

		cur, err := decodeRow(br, ref, columns, twoD)
		if err != nil {
			if row == 0 {
				return nil, err
			}
			// A truncated or damaged scan still shows the rows that did
			// decode; the rest stays white.
			break
		}
		writeRow(out[row*rowBytes:(row+1)*rowBytes], cur, columns, blackIs1)
		ref = cur
	}
	return out, nil
}

// skipEOL consumes an end-of-line code (eleven zeros and a one) and any
// fill bits before it, which T.4 allows and some encoders emit.
func skipEOL(br *bitReader) {
	for {
		v, ok := br.peek(12)
		if !ok {
			return
		}
		if v == 1 {
			br.skip(12)
			continue
		}
		if v == 0 {
			// Fill bits: step past one and look again.
			br.skip(1)
			continue
		}
		return
	}
}

// decodeRow returns the row's changing elements: the column of every
// colour change, starting from white.
func decodeRow(br *bitReader, ref []int, columns int, twoD bool) ([]int, error) {
	var cur []int
	a0 := -1
	white := true

	put := func(a1 int) {
		if a1 > columns {
			a1 = columns
		}
		cur = append(cur, a1)
	}

	for a0 < columns {
		if br.eof() {
			if len(cur) == 0 {
				return nil, fmt.Errorf("render: CCITT data ended before the row did")
			}
			break
		}
		if !twoD {
			t := whiteTable
			if !white {
				t = blackTable
			}
			run, ok := br.readRun(t)
			if !ok {
				if len(cur) == 0 {
					return nil, fmt.Errorf("render: unrecognised CCITT run-length code")
				}
				break
			}
			start := a0
			if start < 0 {
				start = 0
			}
			a0 = start + run
			put(a0)
			white = !white
			continue
		}

		mode, ok := readMode(br)
		if !ok {
			if len(cur) == 0 {
				return nil, fmt.Errorf("render: unrecognised CCITT two-dimensional mode code")
			}
			break
		}
		b1 := findB1(ref, a0, white, columns)
		b2 := columns
		for _, v := range ref {
			if v > b1 {
				b2 = v
				break
			}
		}
		switch mode {
		case modePass:
			a0 = b2
		case modeHorizontal:
			t1, t2 := whiteTable, blackTable
			if !white {
				t1, t2 = blackTable, whiteTable
			}
			r1, ok1 := br.readRun(t1)
			r2, ok2 := br.readRun(t2)
			if !ok1 || !ok2 {
				return cur, nil
			}
			start := a0
			if start < 0 {
				start = 0
			}
			put(start + r1)
			put(start + r1 + r2)
			a0 = start + r1 + r2
		default: // vertical, mode carries the offset
			a1 := b1 + int(mode)
			put(a1)
			a0 = a1
			white = !white
		}
	}
	if len(cur) == 0 || cur[len(cur)-1] < columns {
		cur = append(cur, columns)
	}
	cur = append(cur, columns)
	return cur, nil
}

type ccittMode int

const (
	modePass       ccittMode = 100
	modeHorizontal ccittMode = 101
)

// readMode reads a two-dimensional mode code. The vertical modes are
// returned as their own offset, -3..3, which is what they mean.
func readMode(br *bitReader) (ccittMode, bool) {
	type m struct {
		bits uint32
		n    int
		mode ccittMode
	}
	table := []m{
		{0x1, 1, 0},  // V0
		{0x3, 3, 1},  // VR1
		{0x2, 3, -1}, // VL1
		{0x1, 3, modeHorizontal},
		{0x1, 4, modePass},
		{0x3, 6, 2},  // VR2
		{0x2, 6, -2}, // VL2
		{0x3, 7, 3},  // VR3
		{0x2, 7, -3}, // VL3
	}
	for n := 1; n <= 7; n++ {
		v, ok := br.peek(n)
		if !ok {
			return 0, false
		}
		for _, e := range table {
			if e.n == n && e.bits == v {
				br.skip(n)
				return e.mode, true
			}
		}
	}
	return 0, false
}

// findB1 is the first changing element on the reference line to the
// right of a0 with the opposite colour of a0's own run — T.4's own
// definition, and the piece of two-dimensional coding that is easiest
// to get subtly wrong.
func findB1(ref []int, a0 int, white bool, columns int) int {
	// ref alternates colours starting with a white-to-black change at
	// index 0, so a changing element at an even index changes to black.
	i := 0
	for i < len(ref) && ref[i] <= a0 {
		i++
	}
	// The element must change to the opposite colour of the current run:
	// to black when the current run is white, which is an even index.
	wantEven := white
	for i < len(ref) {
		if (i%2 == 0) == wantEven {
			return ref[i]
		}
		i++
	}
	return columns
}

// writeRow turns changing elements back into packed bits.
func writeRow(dst []byte, changes []int, columns int, blackIs1 bool) {
	// Start white. In PDF's default reading a white pixel is a 1 bit,
	// so the row starts filled and black runs clear it; /BlackIs1
	// inverts what the codec's own bits meant, not what this function
	// produces, so it is applied at the end.
	for i := range dst {
		dst[i] = 0xff
	}
	pos := 0
	black := false
	for _, c := range changes {
		if c > columns {
			c = columns
		}
		if black {
			for x := pos; x < c; x++ {
				dst[x/8] &^= 1 << uint(7-x%8)
			}
		}
		if c > pos {
			pos = c
		}
		black = !black
		if pos >= columns {
			break
		}
	}
	if blackIs1 {
		for i := range dst {
			dst[i] = ^dst[i]
		}
	}
}
