package pdf

import (
	"bytes"
	"fmt"
	"regexp"
)

// xrefEntryType is one of the three PDF cross-reference entry kinds
// (F3 §2.1): free, a direct object in the file, or an object compressed
// inside an object stream.
type xrefEntryType int

const (
	xrefFree xrefEntryType = iota
	xrefInFile
	xrefInStream
)

// xrefEntry locates one object number. For xrefInFile, offset is the
// absolute byte offset of "N G obj" in the file and gen is its
// generation. For xrefInStream, offset is the object number of the
// containing object stream and index is this object's position within
// it (F3 §2.1's "type 2" entry).
type xrefEntry struct {
	typ    xrefEntryType
	offset int64
	gen    int
	index  int
}

// parseXrefChain follows /Prev (and hybrid-file /XRefStm) from startxref
// to the oldest revision, merging entries and trailer keys with newer
// revisions taking precedence (F3 §2.1) — implemented simply by never
// overwriting an entry or trailer key that an earlier (newer) section
// already set, since sections are visited newest first.
func parseXrefChain(data []byte, startxref int64) (map[int]xrefEntry, Dict, error) {
	entries := map[int]xrefEntry{}
	trailer := Dict{}
	visited := map[int64]bool{}

	offset := startxref
	for offset != 0 {
		if offset < 0 || offset >= int64(len(data)) {
			return nil, nil, fmt.Errorf("pdf: xref offset %d out of range", offset)
		}
		if visited[offset] {
			break // cycle guard
		}
		visited[offset] = true

		sectionEntries, sectionTrailer, prev, xrefStm, err := parseXrefSectionAt(data, offset)
		if err != nil {
			return nil, nil, err
		}
		mergeEntries(entries, sectionEntries)
		mergeTrailer(trailer, sectionTrailer)

		if xrefStm != 0 && !visited[xrefStm] {
			visited[xrefStm] = true
			hybridEntries, hybridTrailer, _, _, err := parseXrefSectionAt(data, xrefStm)
			if err == nil {
				mergeEntries(entries, hybridEntries)
				mergeTrailer(trailer, hybridTrailer)
			}
		}

		offset = prev
	}
	return entries, trailer, nil
}

func mergeEntries(dst, src map[int]xrefEntry) {
	for num, e := range src {
		if _, exists := dst[num]; !exists {
			dst[num] = e
		}
	}
}

func mergeTrailer(dst, src Dict) {
	for k, v := range src {
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
}

// parseXrefSectionAt parses one cross-reference section — classic table
// or stream — at offset, returning its entries, its trailer keys, the
// /Prev offset (0 if absent), and the /XRefStm offset for a hybrid
// classic table (0 if absent).
func parseXrefSectionAt(data []byte, offset int64) (map[int]xrefEntry, Dict, int64, int64, error) {
	p := newParser(data, nil)
	p.pos = int(offset)
	p.skipWhitespaceAndComments()

	if p.peekKeyword("xref") {
		return parseClassicXrefAt(p)
	}
	return parseXrefStreamAt(data, offset)
}

// parseClassicXrefAt parses "xref" followed by one or more subsections
// and a trailer dictionary (F3 §2.1).
func parseClassicXrefAt(p *parser) (map[int]xrefEntry, Dict, int64, int64, error) {
	if err := p.consumeKeyword("xref"); err != nil {
		return nil, nil, 0, 0, err
	}
	entries := map[int]xrefEntry{}
	for {
		p.skipWhitespaceAndComments()
		if p.peekKeyword("trailer") {
			break
		}
		if p.eof() || p.data[p.pos] < '0' || p.data[p.pos] > '9' {
			break
		}
		startObj, isInt1, err := p.parseNumber()
		if err != nil || !isInt1 {
			return nil, nil, 0, 0, fmt.Errorf("pdf: malformed xref subsection header")
		}
		p.skipWhitespaceAndComments()
		count, isInt2, err := p.parseNumber()
		if err != nil || !isInt2 {
			return nil, nil, 0, 0, fmt.Errorf("pdf: malformed xref subsection header")
		}
		start := startObj.(int64)
		n := count.(int64)
		for i := int64(0); i < n; i++ {
			p.skipWhitespaceAndComments()
			offTok, ok1, err := p.parseNumber()
			if err != nil || !ok1 {
				return nil, nil, 0, 0, fmt.Errorf("pdf: malformed xref entry")
			}
			p.skipWhitespaceAndComments()
			genTok, ok2, err := p.parseNumber()
			if err != nil || !ok2 {
				return nil, nil, 0, 0, fmt.Errorf("pdf: malformed xref entry")
			}
			p.skipWhitespaceAndComments()
			if p.eof() {
				return nil, nil, 0, 0, fmt.Errorf("pdf: truncated xref entry")
			}
			kind := p.data[p.pos]
			p.pos++
			num := int(start + i)
			switch kind {
			case 'n':
				if _, exists := entries[num]; !exists {
					entries[num] = xrefEntry{typ: xrefInFile, offset: offTok.(int64), gen: int(genTok.(int64))}
				}
			case 'f':
				if _, exists := entries[num]; !exists {
					entries[num] = xrefEntry{typ: xrefFree}
				}
			default:
				return nil, nil, 0, 0, fmt.Errorf("pdf: unrecognised xref entry type %q", kind)
			}
		}
	}
	if err := p.consumeKeyword("trailer"); err != nil {
		return nil, nil, 0, 0, err
	}
	val, err := p.parseValue()
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("pdf: malformed trailer dictionary: %w", err)
	}
	trailer, ok := val.(Dict)
	if !ok {
		return nil, nil, 0, 0, fmt.Errorf("pdf: trailer is not a dictionary")
	}
	prev, _ := asInt64(trailer.Get(Name("Prev")))
	xrefStm, _ := asInt64(trailer.Get(Name("XRefStm")))
	return entries, trailer, prev, xrefStm, nil
}

// parseXrefStreamAt parses a cross-reference stream (F3 §2.1): an
// indirect stream object with /Type /XRef, /W field widths, and /Index
// subsections, whose decoded bytes are fixed-width binary rows.
func parseXrefStreamAt(data []byte, offset int64) (map[int]xrefEntry, Dict, int64, int64, error) {
	p := newParser(data, nil)
	_, _, obj, err := p.parseIndirectAt(int(offset))
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("pdf: xref stream: %w", err)
	}
	stream, ok := obj.(*Stream)
	if !ok {
		return nil, nil, 0, 0, fmt.Errorf("pdf: object at offset %d is not a stream", offset)
	}
	if stream.Dict.GetName(Name("Type")) != "XRef" {
		return nil, nil, 0, 0, fmt.Errorf("pdf: object at offset %d is not an XRef stream", offset)
	}
	decoded, err := decodeStream(stream.Dict, stream.Raw)
	if err != nil {
		return nil, nil, 0, 0, fmt.Errorf("pdf: decoding xref stream: %w", err)
	}

	wArr, ok := stream.Dict.Get(Name("W")).(Array)
	if !ok || len(wArr) != 3 {
		return nil, nil, 0, 0, fmt.Errorf("pdf: xref stream missing /W")
	}
	// Each field width comes straight from the (possibly fuzzed)
	// dictionary; real widths are 1-4 bytes and never more than 8
	// (beInt's accumulator only needs to hold a handful of bytes
	// meaningfully). Bounding them here — rather than only bounding
	// their sum — stops a single negative or oversized width from
	// producing a row slice whose bounds don't match rowWidth (F3
	// §16.5).
	const maxFieldWidth = 8
	w := [3]int{}
	for i := 0; i < 3; i++ {
		v, _ := asInt64(wArr[i])
		if v < 0 || v > maxFieldWidth {
			return nil, nil, 0, 0, fmt.Errorf("pdf: xref stream field width %d out of range", v)
		}
		w[i] = int(v)
	}
	size, _ := asInt64(stream.Dict.Get(Name("Size")))

	var index []int64
	if idxArr, ok := stream.Dict.Get(Name("Index")).(Array); ok {
		for _, v := range idxArr {
			n, _ := asInt64(v)
			index = append(index, n)
		}
	}
	if len(index) == 0 {
		index = []int64{0, size}
	}

	rowWidth := w[0] + w[1] + w[2]
	if rowWidth <= 0 {
		return nil, nil, 0, 0, fmt.Errorf("pdf: xref stream has zero-width rows")
	}

	entries := map[int]xrefEntry{}
	pos := 0
	for si := 0; si+1 < len(index); si += 2 {
		start := index[si]
		count := index[si+1]
		for i := int64(0); i < count; i++ {
			if pos+rowWidth > len(decoded) {
				return nil, nil, 0, 0, fmt.Errorf("pdf: xref stream truncated")
			}
			row := decoded[pos : pos+rowWidth]
			pos += rowWidth

			typ := int64(1)
			off := 0
			if w[0] > 0 {
				typ = beInt(row[:w[0]])
				off = w[0]
			}
			f2 := beInt(row[off : off+w[1]])
			off += w[1]
			f3 := beInt(row[off : off+w[2]])

			num := int(start + i)
			var e xrefEntry
			switch typ {
			case 0:
				e = xrefEntry{typ: xrefFree}
			case 1:
				e = xrefEntry{typ: xrefInFile, offset: f2, gen: int(f3)}
			case 2:
				e = xrefEntry{typ: xrefInStream, offset: f2, index: int(f3)}
			default:
				continue // reserved type: ignore per spec
			}
			if _, exists := entries[num]; !exists {
				entries[num] = e
			}
		}
	}

	prev, _ := asInt64(stream.Dict.Get(Name("Prev")))
	return entries, stream.Dict, prev, 0, nil
}

func beInt(b []byte) int64 {
	var v int64
	for _, c := range b {
		v = v<<8 | int64(c)
	}
	return v
}

// objRegexp finds "N G obj" markers anywhere in the file, used only by
// the startxref-rebuild fallback (F3 §2.2's "real-world documents need
// this more often than they should").
var objRegexp = regexp.MustCompile(`(\d+)[\x00\x09\x0A\x0C\x0D\x20]+(\d+)[\x00\x09\x0A\x0C\x0D\x20]+obj\b`)

// rebuildXref scans the whole file for "N G obj" markers instead of
// trusting startxref (F3 §2.2). Later matches (further into the file)
// win, mirroring how a correct /Prev chain lets the newest revision of
// an object shadow older ones. The trailer is recovered from the last
// classic "trailer" keyword in the file, or failing that, from the last
// object whose dictionary declares /Type /XRef.
func rebuildXref(data []byte) (map[int]xrefEntry, Dict, error) {
	entries := map[int]xrefEntry{}
	matches := objRegexp.FindAllSubmatchIndex(data, -1)
	var xrefStreamOffsets []int64
	for _, m := range matches {
		numStr := data[m[2]:m[3]]
		genStr := data[m[4]:m[5]]
		num, gen, ok := parseDecimalPair(numStr, genStr)
		if !ok {
			continue
		}
		offset := int64(m[0])
		entries[num] = xrefEntry{typ: xrefInFile, offset: offset, gen: gen}
		xrefStreamOffsets = append(xrefStreamOffsets, offset)
	}

	if trailer := findClassicTrailer(data); trailer != nil {
		return entries, trailer, nil
	}

	// No classic trailer: look for the last object that is itself an
	// XRef stream and use its dictionary.
	p := newParser(data, nil)
	for i := len(xrefStreamOffsets) - 1; i >= 0; i-- {
		_, _, obj, err := p.parseIndirectAt(int(xrefStreamOffsets[i]))
		if err != nil {
			continue
		}
		if s, ok := obj.(*Stream); ok && s.Dict.GetName(Name("Type")) == "XRef" {
			return entries, s.Dict, nil
		}
	}
	return nil, nil, fmt.Errorf("pdf: rebuild found no trailer")
}

func parseDecimalPair(a, b []byte) (int, int, bool) {
	n1, ok1 := parseDecimal(a)
	n2, ok2 := parseDecimal(b)
	return n1, n2, ok1 && ok2
}

func parseDecimal(b []byte) (int, bool) {
	if len(b) == 0 {
		return 0, false
	}
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func findClassicTrailer(data []byte) Dict {
	idx := bytes.LastIndex(data, []byte("trailer"))
	if idx < 0 {
		return nil
	}
	p := newParser(data, nil)
	p.pos = idx
	if err := p.consumeKeyword("trailer"); err != nil {
		return nil
	}
	val, err := p.parseValue()
	if err != nil {
		return nil
	}
	d, ok := val.(Dict)
	if !ok {
		return nil
	}
	return d
}
