package render

import (
	"encoding/binary"
	"fmt"
)

// ttfFont reads glyph outlines out of a TrueType font program — the
// /FontFile2 of an embedded font, and the substitute font this package
// embeds for the ones that are not embedded.
//
// It reads exactly what drawing a glyph needs: the header, the glyph
// index bounds, the outlines, and the character map. Advance widths are
// deliberately *not* read from the font: PDF carries its own /Widths (or
// /W) array, and that array is what the document's own layout was
// computed against. Taking the width from the font instead is how a
// preview drifts a word at a time away from where the text really is.
type ttfFont struct {
	data      []byte
	upem      float64
	numGlyphs int
	loca      []uint32
	glyf      []byte

	cmapUnicode map[rune]int
	cmapSymbol  map[int]int
	cmapMac     map[int]int

	// advances are the font's own advance widths, in font units. They
	// are used for exactly one thing: a document that names a standard
	// font and supplies no /Widths array of its own, leaving nothing
	// else to measure with. Everywhere else the document's widths win —
	// see pdfFont.
	advances    []uint16
	lastAdvance uint16
}

func parseTTF(data []byte) (*ttfFont, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("render: font program too short")
	}
	off := 0
	if string(data[0:4]) == "ttcf" {
		if len(data) < 16 {
			return nil, fmt.Errorf("render: truncated font collection")
		}
		off = int(be32(data, 12))
		if off+12 > len(data) {
			return nil, fmt.Errorf("render: font collection offset out of range")
		}
	}
	tag := be32(data, off)
	if tag == 0x4f54544f { // 'OTTO': PostScript (CFF) outlines in an sfnt wrapper
		return nil, errCFFOutlines
	}
	if tag != 0x00010000 && tag != 0x74727565 { // 'true'
		return nil, fmt.Errorf("render: not a TrueType font (tag %08x)", tag)
	}
	n := int(be16(data, off+4))
	tables := map[string][]byte{}
	for i := 0; i < n; i++ {
		rec := off + 12 + i*16
		if rec+16 > len(data) {
			break
		}
		name := string(data[rec : rec+4])
		o := int(be32(data, rec+8))
		l := int(be32(data, rec+12))
		if o < 0 || l < 0 || o > len(data) {
			continue
		}
		if o+l > len(data) {
			l = len(data) - o
		}
		tables[name] = data[o : o+l]
	}

	head := tables["head"]
	if len(head) < 54 {
		return nil, fmt.Errorf("render: font has no usable head table")
	}
	f := &ttfFont{data: data}
	f.upem = float64(be16(head, 18))
	if f.upem == 0 {
		f.upem = 1000
	}
	locFormat := int(int16(be16(head, 50)))

	maxp := tables["maxp"]
	if len(maxp) >= 6 {
		f.numGlyphs = int(be16(maxp, 4))
	}

	f.glyf = tables["glyf"]
	loca := tables["loca"]
	if f.glyf == nil || loca == nil {
		if tables["CFF "] != nil {
			return nil, errCFFOutlines
		}
		return nil, fmt.Errorf("render: font has no glyph outlines")
	}
	f.loca = make([]uint32, 0, f.numGlyphs+1)
	if locFormat == 0 {
		for i := 0; i+1 < len(loca) && i/2 <= f.numGlyphs; i += 2 {
			f.loca = append(f.loca, uint32(be16(loca, i))*2)
		}
	} else {
		for i := 0; i+3 < len(loca) && i/4 <= f.numGlyphs; i += 4 {
			f.loca = append(f.loca, be32(loca, i))
		}
	}

	f.parseCmap(tables["cmap"])
	f.parseHmtx(tables["hhea"], tables["hmtx"])
	return f, nil
}

func (f *ttfFont) parseHmtx(hhea, hmtx []byte) {
	if len(hhea) < 36 || len(hmtx) < 4 {
		return
	}
	n := int(be16(hhea, 34))
	if n <= 0 {
		return
	}
	f.advances = make([]uint16, 0, n)
	for i := 0; i < n && i*4+2 <= len(hmtx); i++ {
		f.advances = append(f.advances, be16(hmtx, i*4))
	}
	if len(f.advances) > 0 {
		f.lastAdvance = f.advances[len(f.advances)-1]
	}
}

// advance is glyph gid's own advance width in font units, or zero when
// the font does not say.
func (f *ttfFont) advance(gid int) float64 {
	if gid < 0 || len(f.advances) == 0 {
		return 0
	}
	if gid < len(f.advances) {
		return float64(f.advances[gid])
	}
	// Beyond the last entry every glyph shares the last advance, which
	// is how a monospaced or CJK font stores its metrics compactly.
	return float64(f.lastAdvance)
}

// errCFFOutlines says the font program is real but its outlines are
// PostScript, which this reader does not draw. The caller substitutes,
// keeping the document's own widths — see pdfFont.
var errCFFOutlines = fmt.Errorf("render: CFF outlines")

func be16(b []byte, i int) uint16 {
	if i+2 > len(b) || i < 0 {
		return 0
	}
	return binary.BigEndian.Uint16(b[i:])
}

func be32(b []byte, i int) uint32 {
	if i+4 > len(b) || i < 0 {
		return 0
	}
	return binary.BigEndian.Uint32(b[i:])
}

func (f *ttfFont) parseCmap(cmap []byte) {
	if len(cmap) < 4 {
		return
	}
	n := int(be16(cmap, 2))
	for i := 0; i < n; i++ {
		rec := 4 + i*8
		if rec+8 > len(cmap) {
			break
		}
		plat := int(be16(cmap, rec))
		enc := int(be16(cmap, rec+2))
		off := int(be32(cmap, rec+4))
		if off >= len(cmap) {
			continue
		}
		table := cmap[off:]
		switch {
		case plat == 3 && (enc == 1 || enc == 10), plat == 0:
			if f.cmapUnicode == nil {
				f.cmapUnicode = map[rune]int{}
			}
			readCmapSubtable(table, func(code, gid int) {
				if _, seen := f.cmapUnicode[rune(code)]; !seen {
					f.cmapUnicode[rune(code)] = gid
				}
			})
		case plat == 3 && enc == 0:
			if f.cmapSymbol == nil {
				f.cmapSymbol = map[int]int{}
			}
			readCmapSubtable(table, func(code, gid int) { f.cmapSymbol[code] = gid })
		case plat == 1 && enc == 0:
			if f.cmapMac == nil {
				f.cmapMac = map[int]int{}
			}
			readCmapSubtable(table, func(code, gid int) { f.cmapMac[code] = gid })
		}
	}
}

func readCmapSubtable(t []byte, emit func(code, gid int)) {
	switch be16(t, 0) {
	case 0:
		for c := 0; c < 256 && 6+c < len(t); c++ {
			if g := int(t[6+c]); g != 0 {
				emit(c, g)
			}
		}
	case 4:
		segX2 := int(be16(t, 6))
		if segX2 < 2 {
			return
		}
		segs := segX2 / 2
		endBase := 14
		startBase := endBase + segX2 + 2
		deltaBase := startBase + segX2
		rangeBase := deltaBase + segX2
		for s := 0; s < segs; s++ {
			end := int(be16(t, endBase+s*2))
			start := int(be16(t, startBase+s*2))
			delta := int(int16(be16(t, deltaBase+s*2)))
			ro := int(be16(t, rangeBase+s*2))
			if start > end {
				continue
			}
			for c := start; c <= end && c != 0x10000; c++ {
				var g int
				if ro == 0 {
					g = (c + delta) & 0xffff
				} else {
					idx := rangeBase + s*2 + ro + (c-start)*2
					g = int(be16(t, idx))
					if g != 0 {
						g = (g + delta) & 0xffff
					}
				}
				if g != 0 {
					emit(c, g)
				}
			}
		}
	case 6:
		first := int(be16(t, 6))
		count := int(be16(t, 8))
		for i := 0; i < count; i++ {
			if g := int(be16(t, 10+i*2)); g != 0 {
				emit(first+i, g)
			}
		}
	case 12:
		groups := int(be32(t, 12))
		for i := 0; i < groups; i++ {
			rec := 16 + i*12
			if rec+12 > len(t) {
				break
			}
			start := int(be32(t, rec))
			end := int(be32(t, rec+4))
			gid := int(be32(t, rec+8))
			if end-start > 0x10000 {
				end = start + 0x10000
			}
			for c := start; c <= end; c++ {
				emit(c, gid+(c-start))
			}
		}
	}
}

// gidForRune maps a Unicode code point through the font's own character
// map. Zero means the font does not have that character.
func (f *ttfFont) gidForRune(r rune) int {
	if f.cmapUnicode != nil {
		if g, ok := f.cmapUnicode[r]; ok {
			return g
		}
	}
	return 0
}

// gidForCode is the lookup a *symbolic* TrueType font wants: the byte
// code itself, through the (3,0) subtable, which producers key at
// 0xF000+code as often as at the bare code, or through a (1,0) Mac
// subtable when that is all there is (PDF 32000-1 §9.6.6.4).
func (f *ttfFont) gidForCode(code int) int {
	if f.cmapSymbol != nil {
		if g, ok := f.cmapSymbol[0xF000|code&0xff]; ok {
			return g
		}
		if g, ok := f.cmapSymbol[code]; ok {
			return g
		}
	}
	if f.cmapMac != nil {
		if g, ok := f.cmapMac[code]; ok {
			return g
		}
	}
	return 0
}

func (f *ttfFont) hasCmap() bool {
	return len(f.cmapUnicode) > 0 || len(f.cmapSymbol) > 0 || len(f.cmapMac) > 0
}

// appendGlyph adds glyph gid's outline to p, with points transformed
// from font units by m. Returns false when there is no outline to draw,
// which includes the entirely ordinary case of a space.
func (f *ttfFont) appendGlyph(gid int, p *pathBuilder, m matrix, depth int) bool {
	if depth > 6 || gid < 0 || gid+1 >= len(f.loca) {
		return false
	}
	start, end := f.loca[gid], f.loca[gid+1]
	if end <= start || int(end) > len(f.glyf) {
		return false
	}
	g := f.glyf[start:end]
	if len(g) < 10 {
		return false
	}
	nc := int(int16(be16(g, 0)))
	if nc < 0 {
		return f.appendComposite(g[10:], p, m, depth)
	}
	return f.appendSimple(g, nc, p, m)
}

func (f *ttfFont) appendSimple(g []byte, nc int, p *pathBuilder, m matrix) bool {
	pos := 10
	ends := make([]int, nc)
	for i := 0; i < nc; i++ {
		ends[i] = int(be16(g, pos))
		pos += 2
	}
	if nc == 0 {
		return false
	}
	npts := ends[nc-1] + 1
	if npts <= 0 || npts > 10000 {
		return false
	}
	insLen := int(be16(g, pos))
	pos += 2 + insLen

	flags := make([]byte, 0, npts)
	for len(flags) < npts && pos < len(g) {
		fl := g[pos]
		pos++
		flags = append(flags, fl)
		if fl&8 != 0 && pos < len(g) {
			rep := int(g[pos])
			pos++
			for i := 0; i < rep && len(flags) < npts; i++ {
				flags = append(flags, fl)
			}
		}
	}
	if len(flags) < npts {
		return false
	}

	xs := make([]int, npts)
	v := 0
	for i := 0; i < npts; i++ {
		fl := flags[i]
		switch {
		case fl&2 != 0:
			if pos >= len(g) {
				return false
			}
			d := int(g[pos])
			pos++
			if fl&16 == 0 {
				d = -d
			}
			v += d
		case fl&16 == 0:
			v += int(int16(be16(g, pos)))
			pos += 2
		}
		xs[i] = v
	}
	ys := make([]int, npts)
	v = 0
	for i := 0; i < npts; i++ {
		fl := flags[i]
		switch {
		case fl&4 != 0:
			if pos >= len(g) {
				return false
			}
			d := int(g[pos])
			pos++
			if fl&32 == 0 {
				d = -d
			}
			v += d
		case fl&32 == 0:
			v += int(int16(be16(g, pos)))
			pos += 2
		}
		ys[i] = v
	}

	drew := false
	first := 0
	for c := 0; c < nc; c++ {
		last := ends[c]
		if last < first || last >= npts {
			break
		}
		if emitContour(p, m, flags[first:last+1], xs[first:last+1], ys[first:last+1]) {
			drew = true
		}
		first = last + 1
	}
	return drew
}

// emitContour turns one TrueType contour — a ring of on- and off-curve
// points, where two consecutive off-curve points imply an on-curve point
// halfway between them — into moves, lines and quadratic curves.
func emitContour(p *pathBuilder, m matrix, flags []byte, xs, ys []int) bool {
	n := len(flags)
	if n == 0 {
		return false
	}
	on := func(i int) bool { return flags[i%n]&1 != 0 }
	px := func(i int) (float64, float64) {
		return m.apply(float64(xs[i%n]), float64(ys[i%n]))
	}
	mid := func(i, j int) (float64, float64) {
		ax, ay := px(i)
		bx, by := px(j)
		return (ax + bx) / 2, (ay + by) / 2
	}

	// Find a starting on-curve point. A contour made entirely of
	// control points — legal, and what a circle drawn as four
	// quadratics looks like — starts at the implied on-curve point
	// halfway between the last control point and the first, and its
	// first control point to process is point 0.
	start := -1
	for i := 0; i < n; i++ {
		if on(i) {
			start = i
			break
		}
	}
	var sx, sy float64
	if start < 0 {
		start = n - 1
		sx, sy = mid(n-1, 0)
	} else {
		sx, sy = px(start)
	}
	p.moveTo(sx, sy)

	i := start
	for k := 0; k < n; {
		i++
		k++
		if on(i) {
			x, y := px(i)
			p.lineTo(x, y)
			continue
		}
		cx, cy := px(i)
		var ex, ey float64
		if on(i + 1) {
			ex, ey = px(i + 1)
			i++
			k++
		} else {
			ex, ey = mid(i, i+1)
		}
		quadTo(p, cx, cy, ex, ey)
	}
	p.lineTo(sx, sy)
	p.closeSubpath()
	return true
}

// quadTo appends a quadratic curve as the cubic with the same shape.
func quadTo(p *pathBuilder, cx, cy, x, y float64) {
	x0, y0 := p.curX, p.curY
	p.curveTo(x0+2.0/3*(cx-x0), y0+2.0/3*(cy-y0), x+2.0/3*(cx-x), y+2.0/3*(cy-y), x, y)
}

func (f *ttfFont) appendComposite(g []byte, p *pathBuilder, m matrix, depth int) bool {
	pos := 0
	drew := false
	for {
		if pos+4 > len(g) {
			return drew
		}
		flags := int(be16(g, pos))
		gi := int(be16(g, pos+2))
		pos += 4

		var dx, dy float64
		if flags&1 != 0 { // ARG_1_AND_2_ARE_WORDS
			dx = float64(int16(be16(g, pos)))
			dy = float64(int16(be16(g, pos+2)))
			pos += 4
		} else {
			if pos+2 > len(g) {
				return drew
			}
			dx = float64(int8(g[pos]))
			dy = float64(int8(g[pos+1]))
			pos += 2
		}
		if flags&2 == 0 {
			// Point-matching placement rather than an offset. Rare
			// enough that treating it as no offset is better than not
			// drawing the component at all.
			dx, dy = 0, 0
		}

		sub := matrix{1, 0, 0, 1, dx, dy}
		switch {
		case flags&8 != 0: // WE_HAVE_A_SCALE
			s := f2dot14(be16(g, pos))
			pos += 2
			sub = matrix{s, 0, 0, s, dx, dy}
		case flags&0x40 != 0: // X_AND_Y_SCALE
			sx := f2dot14(be16(g, pos))
			sy := f2dot14(be16(g, pos+2))
			pos += 4
			sub = matrix{sx, 0, 0, sy, dx, dy}
		case flags&0x80 != 0: // TWO_BY_TWO
			a := f2dot14(be16(g, pos))
			b := f2dot14(be16(g, pos+2))
			c := f2dot14(be16(g, pos+4))
			d := f2dot14(be16(g, pos+6))
			pos += 8
			sub = matrix{a, b, c, d, dx, dy}
		}
		if f.appendGlyph(gi, p, sub.mul(m), depth+1) {
			drew = true
		}
		if flags&0x20 == 0 { // MORE_COMPONENTS
			return drew
		}
	}
}

func f2dot14(v uint16) float64 { return float64(int16(v)) / 16384 }
