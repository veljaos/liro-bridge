package main

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// glyphOutline is one subset glyph's simple (never composite) contour
// data, already in TrueType's own Y-up, integer design-unit space —
// golang.org/x/image/font/sfnt.LoadGlyph returns composite glyphs
// (e.g. "č" is base "c" plus a combining caron) pre-flattened into a
// single flat list of contours, which is exactly what a TrueType
// *simple* glyph description needs, so this subset never has to
// reconstruct TrueType's composite-glyph mechanism at all.
type glyphOutline struct {
	// contours holds one []point per contour; each point's onCurve flag
	// distinguishes on-curve points from quadratic control points.
	contours [][]point
	advance  int // design units, this font's own unitsPerEm
}

type point struct {
	x, y    int16
	onCurve bool
}

// subsetFont holds everything ttfbuild needs to assemble a minimal,
// valid TrueType font containing exactly these glyphs, in this order —
// glyph 0 is always .notdef.
type subsetFont struct {
	unitsPerEm      uint16
	ascent, descent int16
	lineGap         int16
	capHeight       int16
	glyphs          []glyphOutline // index 0 is .notdef
	runeToNewGID    map[rune]uint16
	createdModified uint64 // seconds since 1904-01-01, for reproducible output
}

// build assembles the complete sfnt binary (F4 §3.2/§3.3): the required
// table set for a usable TrueType outline font addressed purely by
// glyph index (cmap, glyf, head, hhea, hmtx, loca, maxp, name, post).
// OS/2 is deliberately omitted — see this package's doc comment for why.
func (sf *subsetFont) build() ([]byte, error) {
	numGlyphs := len(sf.glyphs)
	if numGlyphs == 0 || numGlyphs > 0xFFFF {
		return nil, fmt.Errorf("gensubsetfont: %d glyphs is not a valid subset size", numGlyphs)
	}

	glyfTable, locaOffsets := sf.buildGlyfLoca()
	locaTable := buildLocaLong(locaOffsets)
	headTable := sf.buildHead(locaOffsets[len(locaOffsets)-1])
	hheaTable := sf.buildHhea(uint16(numGlyphs))
	hmtxTable := sf.buildHmtx()
	maxpTable := sf.buildMaxp(uint16(numGlyphs))
	nameTable := buildName()
	postTable := buildPost()
	cmapTable, err := sf.buildCmap()
	if err != nil {
		return nil, err
	}

	tables := map[string][]byte{
		"cmap": cmapTable,
		"glyf": glyfTable,
		"head": headTable,
		"hhea": hheaTable,
		"hmtx": hmtxTable,
		"loca": locaTable,
		"maxp": maxpTable,
		"name": nameTable,
		"post": postTable,
	}
	return assembleSFNT(tables)
}

// buildGlyfLoca serialises every glyph as a simple (never composite)
// TrueType outline and returns the concatenated glyf table plus the
// loca offsets (numGlyphs+1 entries, the last being the glyf table's
// total length).
func (sf *subsetFont) buildGlyfLoca() ([]byte, []uint32) {
	var glyf []byte
	offsets := make([]uint32, 0, len(sf.glyphs)+1)
	offsets = append(offsets, 0)
	for _, g := range sf.glyphs {
		entry := encodeSimpleGlyph(g)
		glyf = append(glyf, entry...)
		if len(entry)%2 != 0 {
			glyf = append(glyf, 0) // glyf entries are padded to an even length
		}
		offsets = append(offsets, uint32(len(glyf)))
	}
	return glyf, offsets
}

// encodeSimpleGlyph writes one glyph as a TrueType "simple glyph"
// description (OpenType spec, "glyf" table, simple glyph). An empty
// outline (used for .notdef) yields a zero-length entry, which is valid:
// a loca table whose offset[i] == offset[i+1] means "no outline."
func encodeSimpleGlyph(g glyphOutline) []byte {
	if len(g.contours) == 0 {
		return nil
	}
	var b []byte
	put16 := func(v uint16) { b = append(b, byte(v>>8), byte(v)) }
	putS16 := func(v int16) { put16(uint16(v)) }

	numContours := len(g.contours)
	var allPoints []point
	endPts := make([]uint16, numContours)
	var minX, minY, maxX, maxY int16 = 0x7FFF, 0x7FFF, -0x8000, -0x8000
	for i, c := range g.contours {
		allPoints = append(allPoints, c...)
		endPts[i] = uint16(len(allPoints) - 1)
		for _, p := range c {
			if p.x < minX {
				minX = p.x
			}
			if p.y < minY {
				minY = p.y
			}
			if p.x > maxX {
				maxX = p.x
			}
			if p.y > maxY {
				maxY = p.y
			}
		}
	}

	putS16(int16(numContours))
	putS16(minX)
	putS16(minY)
	putS16(maxX)
	putS16(maxY)
	for _, e := range endPts {
		put16(e)
	}
	put16(0) // instructionLength: this subset never carries hinting bytecode

	// Flags: bit 0 = on-curve. No repeat-count optimisation — this
	// subset is small enough that the straightforward one-flag-per-point
	// encoding costs nothing that matters, and it is far easier to read
	// and to get right than the repeat-run encoding.
	flags := make([]byte, len(allPoints))
	for i, p := range allPoints {
		if p.onCurve {
			flags[i] = 0x01
		}
	}
	b = append(b, flags...)

	// X coordinates: always stored as signed 16-bit deltas from the
	// previous point (flag bits that would allow the 8-bit/repeat
	// encodings are simply never set, matching the flags loop above).
	var prevX, prevY int16
	for _, p := range allPoints {
		putS16(p.x - prevX)
		prevX = p.x
	}
	for _, p := range allPoints {
		putS16(p.y - prevY)
		prevY = p.y
	}
	return b
}

func buildLocaLong(offsets []uint32) []byte {
	b := make([]byte, len(offsets)*4)
	for i, o := range offsets {
		binary.BigEndian.PutUint32(b[i*4:], o)
	}
	return b
}

func (sf *subsetFont) buildHead(glyfLength uint32) []byte {
	b := make([]byte, 54)
	binary.BigEndian.PutUint32(b[0:], 0x00010000)  // version 1.0
	binary.BigEndian.PutUint32(b[4:], 0x00010000)  // fontRevision 1.0
	binary.BigEndian.PutUint32(b[8:], 0)           // checkSumAdjustment, patched by assembleSFNT
	binary.BigEndian.PutUint32(b[12:], 0x5F0F3CF5) // magicNumber
	binary.BigEndian.PutUint16(b[16:], 0x0003)     // flags: baseline-at-0, left-sidebearing-at-0
	binary.BigEndian.PutUint16(b[18:], sf.unitsPerEm)
	binary.BigEndian.PutUint64(b[20:], sf.createdModified)
	binary.BigEndian.PutUint64(b[28:], sf.createdModified)
	minX, minY, maxX, maxY := sf.bbox()
	binary.BigEndian.PutUint16(b[36:], uint16(minX))
	binary.BigEndian.PutUint16(b[38:], uint16(minY))
	binary.BigEndian.PutUint16(b[40:], uint16(maxX))
	binary.BigEndian.PutUint16(b[42:], uint16(maxY))
	binary.BigEndian.PutUint16(b[44:], 0) // macStyle
	binary.BigEndian.PutUint16(b[46:], 8) // lowestRecPPEM
	binary.BigEndian.PutUint16(b[48:], 2) // fontDirectionHint (deprecated, 2 = "strongly left to right")
	binary.BigEndian.PutUint16(b[50:], 1) // indexToLocFormat: long (avoids the short format's "offset/2 must be exact" constraint)
	binary.BigEndian.PutUint16(b[52:], 0) // glyphDataFormat
	_ = glyfLength
	return b
}

func (sf *subsetFont) bbox() (minX, minY, maxX, maxY int16) {
	minX, minY, maxX, maxY = 0, 0, 0, 0
	first := true
	for _, g := range sf.glyphs {
		for _, c := range g.contours {
			for _, p := range c {
				if first {
					minX, minY, maxX, maxY = p.x, p.y, p.x, p.y
					first = false
					continue
				}
				if p.x < minX {
					minX = p.x
				}
				if p.y < minY {
					minY = p.y
				}
				if p.x > maxX {
					maxX = p.x
				}
				if p.y > maxY {
					maxY = p.y
				}
			}
		}
	}
	return
}

func (sf *subsetFont) buildHhea(numGlyphs uint16) []byte {
	b := make([]byte, 36)
	binary.BigEndian.PutUint32(b[0:], 0x00010000) // version 1.0
	binary.BigEndian.PutUint16(b[4:], uint16(sf.ascent))
	binary.BigEndian.PutUint16(b[6:], uint16(sf.descent))
	binary.BigEndian.PutUint16(b[8:], uint16(sf.lineGap))
	maxAdv := uint16(0)
	for _, g := range sf.glyphs {
		if uint16(g.advance) > maxAdv {
			maxAdv = uint16(g.advance)
		}
	}
	// The three horizontal-extent fields are what they say they are, now
	// that the bearings are real (buildHmtx): a reader that trusts them
	// and finds them contradicting hmtx has been handed a font that
	// disagrees with itself.
	minLSB, minRSB, xMaxExtent := int16(0x7FFF), int16(0x7FFF), int16(-0x8000)
	for _, g := range sf.glyphs {
		if len(g.contours) == 0 {
			continue
		}
		lsb := glyphXMin(g)
		xMax := glyphXMax(g)
		if lsb < minLSB {
			minLSB = lsb
		}
		if rsb := int16(g.advance) - xMax; rsb < minRSB {
			minRSB = rsb
		}
		if xMax > xMaxExtent {
			xMaxExtent = xMax
		}
	}
	binary.BigEndian.PutUint16(b[10:], maxAdv)         // advanceWidthMax
	binary.BigEndian.PutUint16(b[12:], uint16(minLSB)) // minLeftSideBearing
	binary.BigEndian.PutUint16(b[14:], uint16(minRSB)) // minRightSideBearing
	binary.BigEndian.PutUint16(b[16:], uint16(xMaxExtent))
	binary.BigEndian.PutUint16(b[18:], 1) // caretSlopeRise
	binary.BigEndian.PutUint16(b[20:], 0) // caretSlopeRun
	binary.BigEndian.PutUint16(b[22:], 0) // caretOffset
	// bytes 24..31: four reserved int16 fields, left zero
	binary.BigEndian.PutUint16(b[32:], 0)         // metricDataFormat
	binary.BigEndian.PutUint16(b[34:], numGlyphs) // numberOfHMetrics: one entry per glyph, no compaction
	return b
}

// buildHmtx writes each glyph's advance width and its real left side
// bearing.
//
// The bearing is not decoration and not optional. A TrueType rasteriser
// positions a glyph's outline by shifting it horizontally by
// (hmtx.leftSideBearing - glyf.xMin) — the outline is authored wherever
// the designer put it, and hmtx is what says where its origin actually
// is. Writing zero for every glyph therefore moves every glyph left by
// its own xMin, which is a different amount for every letter.
//
// Measured on the committed subset before this was fixed: Cyrillic Ј
// (U+0408) has xMin -78 and advance 273, so a written bearing of 0
// shifted it 78 units right, leaving its ink ending at 260 of its 273
// advance and crowding whatever followed. Cyrillic О has xMin 60 and
// was shifted 60 units left, opening a gap before it. Both appear in
// one word: "СТАНОЈЕВИЋ" rendered with a visible gap between О and Ј
// and none at all between Ј and Е, which is what the owner reported
// from a signed document.
//
// For a glyph with no outline (.notdef) the bearing is 0, which is what
// the spec asks for and what glyphXMin returns.
func (sf *subsetFont) buildHmtx() []byte {
	b := make([]byte, len(sf.glyphs)*4)
	for i, g := range sf.glyphs {
		binary.BigEndian.PutUint16(b[i*4:], uint16(g.advance))
		binary.BigEndian.PutUint16(b[i*4+2:], uint16(glyphXMin(g)))
	}
	return b
}

// glyphXMax is the rightmost x of a glyph's own outline. Zero for an
// outline-less glyph.
func glyphXMax(g glyphOutline) int16 {
	first := true
	var maxX int16
	for _, c := range g.contours {
		for _, pt := range c {
			if first || pt.x > maxX {
				maxX, first = pt.x, false
			}
		}
	}
	if first {
		return 0
	}
	return maxX
}

// glyphXMin is the leftmost x of a glyph's own outline, which is both
// the value glyf records as xMin and the left side bearing hmtx must
// carry for the two to agree. Zero for an outline-less glyph.
func glyphXMin(g glyphOutline) int16 {
	first := true
	var minX int16
	for _, c := range g.contours {
		for _, pt := range c {
			if first || pt.x < minX {
				minX, first = pt.x, false
			}
		}
	}
	if first {
		return 0
	}
	return minX
}

func (sf *subsetFont) buildMaxp(numGlyphs uint16) []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint32(b[0:], 0x00010000) // version 1.0
	binary.BigEndian.PutUint16(b[4:], numGlyphs)
	maxPoints, maxContours := 0, 0
	for _, g := range sf.glyphs {
		pts := 0
		for _, c := range g.contours {
			pts += len(c)
		}
		if pts > maxPoints {
			maxPoints = pts
		}
		if len(g.contours) > maxContours {
			maxContours = len(g.contours)
		}
	}
	binary.BigEndian.PutUint16(b[6:], uint16(maxPoints))
	binary.BigEndian.PutUint16(b[8:], uint16(maxContours))
	binary.BigEndian.PutUint16(b[10:], 0) // maxCompositePoints: this subset never emits a composite glyph
	binary.BigEndian.PutUint16(b[12:], 0) // maxCompositeContours
	binary.BigEndian.PutUint16(b[14:], 2) // maxZones: 2 is the conventional value even with no instructions
	binary.BigEndian.PutUint16(b[16:], 0) // maxTwilightPoints
	binary.BigEndian.PutUint16(b[18:], 0) // maxStorage
	binary.BigEndian.PutUint16(b[20:], 0) // maxFunctionDefs
	binary.BigEndian.PutUint16(b[22:], 0) // maxInstructionDefs
	binary.BigEndian.PutUint16(b[24:], 0) // maxStackElements
	binary.BigEndian.PutUint16(b[26:], 0) // maxSizeOfInstructions: no glyph in this subset carries instructions
	binary.BigEndian.PutUint16(b[28:], 0) // maxComponentElements
	binary.BigEndian.PutUint16(b[30:], 0) // maxComponentDepth
	return b
}

// buildCmap builds a single-subtable (format 4, platform 3/encoding 1 —
// Windows/Unicode BMP, the subtable every consumer is guaranteed to
// look for) cmap. F4's own PDF text-showing path never consults this —
// Identity-H addresses glyphs by CID/GID directly (§3.3) — but a
// well-formed embedded TrueType font is expected to carry one regardless,
// and building it is inexpensive once the subset's rune→GID assignment
// already exists.
func (sf *subsetFont) buildCmap() ([]byte, error) {
	runes := make([]rune, 0, len(sf.runeToNewGID))
	for r := range sf.runeToNewGID {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })

	type run struct{ start, end rune }
	var runs []run
	for _, r := range runes {
		if len(runs) > 0 && runs[len(runs)-1].end == r-1 {
			runs[len(runs)-1].end = r
			continue
		}
		runs = append(runs, run{r, r})
	}
	// Every run's runes were assigned sequentially increasing GIDs (the
	// caller sorts by rune and assigns GID = index+1), so each run is
	// also GID-contiguous — idRangeOffset 0 with a per-segment idDelta
	// is exact, with no need for the indirect glyphIdArray form.
	segCount := len(runs) + 1 // +1 for the required 0xFFFF terminator
	segCountX2 := uint16(segCount * 2)
	entrySelector := uint16(0)
	for (1 << (entrySelector + 1)) <= segCount {
		entrySelector++
	}
	searchRange := uint16(1<<entrySelector) * 2
	rangeShift := segCountX2 - searchRange

	// The whole subtable: seven uint16 header fields (format, length,
	// language, segCountX2, searchRange, entrySelector, rangeShift),
	// then endCode[segCount], one reservedPad, then startCode, idDelta
	// and idRangeOffset — four parallel arrays of segCount uint16s.
	// 14 + 2 + 8*segCount.
	subtableLen := 16 + 2*4*segCount
	var sub []byte
	put16 := func(v uint16) { sub = append(sub, byte(v>>8), byte(v)) }
	put16(4) // format
	// length is the subtable's own total, header included. It used to
	// be written as 14 + subtableLen, counting the header twice: the
	// committed asset declared 238 bytes and carried 224, and fontTools
	// refuses to parse it ("corrupt cmap table format 4"). Nothing in
	// this project's own rendering path reads this table — Identity-H
	// addresses glyphs by CID (§3.3) — which is exactly why a
	// self-contradicting length could sit in a shipped font asset
	// unnoticed.
	put16(uint16(subtableLen))
	put16(0) // language
	put16(segCountX2)
	put16(searchRange)
	put16(entrySelector)
	put16(rangeShift)
	for _, r := range runs {
		if r.end > 0xFFFF {
			return nil, fmt.Errorf("gensubsetfont: rune U+%04X is outside the BMP; format-4 cmap cannot represent it", r.end)
		}
		put16(uint16(r.end))
	}
	put16(0xFFFF)
	put16(0) // reservedPad
	for _, r := range runs {
		put16(uint16(r.start))
	}
	put16(0xFFFF)
	for _, r := range runs {
		startGID := sf.runeToNewGID[r.start]
		delta := int32(startGID) - int32(r.start)
		put16(uint16(int16(delta)))
	}
	put16(1) // terminator segment: idDelta 1 so 0xFFFF + 1 wraps to glyph 0 (.notdef)
	for range runs {
		put16(0) // idRangeOffset, unused (idDelta form throughout)
	}
	put16(0)

	var b []byte
	pb16 := func(v uint16) { b = append(b, byte(v>>8), byte(v)) }
	pb32 := func(v uint32) {
		b = append(b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
	}
	pb16(0)  // version
	pb16(1)  // numTables
	pb16(3)  // platformID: Windows
	pb16(1)  // encodingID: Unicode BMP
	pb32(12) // offset to the subtable, right after this one record
	b = append(b, sub...)
	return b, nil
}

// buildName writes a minimal, valid "name" table (format 0): just the
// handful of records every consumer expects to find (family, subfamily,
// unique identifier, full name, PostScript name), platform 3/encoding
// 1/language 0x0409 (Windows, Unicode BMP, en-US), UTF-16BE encoded.
func buildName() []byte {
	type rec struct {
		id  uint16
		val string
	}
	// subsetTag must match the six-uppercase-letter prefix this
	// project's /BaseFont uses (F4 §3.1/§10) — kept in sync with
	// internal/pades/appearance.subsetTag by scripts/gensubsetfont's own
	// TestGeneratedSubsetMatchesCharset-equivalent check (this
	// generator's self-verification step, main.go).
	const tag = "LIROBR"
	recs := []rec{
		{1, tag + "-NotoSans-Subset"},
		{2, "Regular"},
		{3, tag + "-NotoSans-Subset-2026"},
		{4, tag + "-NotoSans-Subset Regular"},
		{6, tag + "-NotoSans-Subset"},
	}
	var strings []byte
	var records []byte
	pr16 := func(v uint16) { records = append(records, byte(v>>8), byte(v)) }
	for _, r := range recs {
		u16 := utf16BE(r.val)
		pr16(3)      // platformID
		pr16(1)      // encodingID
		pr16(0x0409) // languageID
		pr16(r.id)
		pr16(uint16(len(u16)))
		pr16(uint16(len(strings)))
		strings = append(strings, u16...)
	}
	var b []byte
	p16 := func(v uint16) { b = append(b, byte(v>>8), byte(v)) }
	p16(0) // format 0
	p16(uint16(len(recs)))
	p16(uint16(6 + len(records))) // storageOffset: header(6) + records
	b = append(b, records...)
	b = append(b, strings...)
	return b
}

func utf16BE(s string) []byte {
	var out []byte
	for _, r := range s {
		if r <= 0xFFFF {
			out = append(out, byte(r>>8), byte(r))
			continue
		}
		r -= 0x10000
		hi := 0xD800 + (r >> 10)
		lo := 0xDC00 + (r & 0x3FF)
		out = append(out, byte(hi>>8), byte(hi), byte(lo>>8), byte(lo))
	}
	return out
}

// buildPost writes a minimal version 3.0 "post" table: no per-glyph
// names (this subset's glyphs are addressed only by GID/CID, never by
// PostScript name), just the fixed 32-byte header.
func buildPost() []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint32(b[0:], 0x00030000) // version 3.0
	return b
}

// assembleSFNT lays out tables in ascending tag order (required by the
// OpenType spec's table directory), computes every table checksum, then
// the whole-font checksum, and patches head's checkSumAdjustment
// (OpenType spec, "head" table / "Calculating the OpenType Font
// Checksum") — the one field in the whole font that depends on the
// font's own complete, final byte layout.
func assembleSFNT(tables map[string][]byte) ([]byte, error) {
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)

	numTables := len(tags)
	entrySelector := uint16(0)
	for (1 << (entrySelector + 1)) <= numTables {
		entrySelector++
	}
	searchRange := uint16(1<<entrySelector) * 16
	rangeShift := uint16(numTables*16) - searchRange

	headerLen := 12 + 16*numTables
	out := make([]byte, headerLen)
	binary.BigEndian.PutUint32(out[0:], 0x00010000) // sfnt version: TrueType
	binary.BigEndian.PutUint16(out[4:], uint16(numTables))
	binary.BigEndian.PutUint16(out[6:], searchRange)
	binary.BigEndian.PutUint16(out[8:], entrySelector)
	binary.BigEndian.PutUint16(out[10:], rangeShift)

	headOffset := -1
	for i, tag := range tags {
		data := tables[tag]
		offset := len(out)
		if tag == "head" {
			headOffset = offset
		}
		cksum := tableChecksum(data)
		entry := out[12+16*i:]
		copy(entry[0:4], tag)
		binary.BigEndian.PutUint32(entry[4:8], cksum)
		binary.BigEndian.PutUint32(entry[8:12], uint32(offset))
		binary.BigEndian.PutUint32(entry[12:16], uint32(len(data)))

		out = append(out, data...)
		for len(out)%4 != 0 {
			out = append(out, 0)
		}
	}
	if headOffset < 0 {
		return nil, fmt.Errorf("gensubsetfont: no head table")
	}

	fontChecksum := tableChecksum(out)
	adjustment := 0xB1B0AFBA - fontChecksum
	binary.BigEndian.PutUint32(out[headOffset+8:], adjustment)
	return out, nil
}

// tableChecksum implements the OpenType spec's checksum algorithm: sum
// every big-endian uint32 word, treating a trailing partial word as
// zero-padded (this matches how sfnt table data is itself padded to a
// 4-byte boundary in the file, so passing already-padded data — as
// assembleSFNT does for the whole-font checksum — and unpadded data — as
// it does for a single table's own directory entry — both produce the
// spec-defined result).
func tableChecksum(data []byte) uint32 {
	var sum uint32
	i := 0
	for ; i+4 <= len(data); i += 4 {
		sum += binary.BigEndian.Uint32(data[i : i+4])
	}
	if rem := len(data) - i; rem > 0 {
		var last [4]byte
		copy(last[:], data[i:])
		sum += binary.BigEndian.Uint32(last[:])
	}
	return sum
}
