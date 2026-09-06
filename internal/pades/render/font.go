package render

import (
	"strconv"
	"strings"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// pdfFont is one /Font resource, reduced to what drawing it needs.
//
// The division that matters here is between *metrics* and *shapes*.
// Metrics — which codes a string decodes into and how wide each one is —
// always come from the document, never from a font program, because the
// document's own layout was computed against them. Shapes come from the
// embedded font program when there is one this package can read, and
// from the substitute font when there is not. A page drawn with
// substituted shapes is a page whose every word begins and ends exactly
// where the real one does; only the letterforms differ.
type pdfFont struct {
	composite bool
	type3     bool

	// simple fonts
	firstChar    int
	widths       []float64
	missingWidth float64
	codeRunes    [256]rune
	// encGIDs holds a glyph *index* for the codes whose /Differences
	// entry names one — "g12", "cid7", "index42", which subsetters emit
	// when they have thrown the character map away and there is no
	// character left to name. Zero means the code names a character
	// instead, which is every ordinary case.
	encGIDs  [256]int
	symbolic bool

	// composite fonts
	enc      *cmapEnc
	cidW     map[int]float64
	defaultW float64
	cidToGID []byte

	// what a code means as text, when the shapes have to be substituted
	toUnicode map[uint32]rune

	prog       *ttfFont
	substitute bool
	bold       bool
	italic     bool
	monospaced bool

	// Type 3
	fontMatrix matrix
	charProcs  pdf.Dict
	t3Res      pdf.Dict
	t3Names    [256]pdf.Name
}

func (r *renderer) loadFont(res pdf.Dict, name pdf.Name) *pdfFont {
	return r.loadFontObject(lookupRes(r.doc, res, "Font", name))
}

func (r *renderer) loadFontObject(obj pdf.Object) *pdfFont {
	if obj == nil {
		return nil
	}
	if ref, ok := obj.(pdf.Reference); ok {
		if f, ok := r.fonts[ref.Num]; ok {
			return f
		}
		f := r.buildFont(obj)
		if r.fonts == nil {
			r.fonts = map[int]*pdfFont{}
		}
		r.fonts[ref.Num] = f
		return f
	}
	return r.buildFont(obj)
}

func (r *renderer) buildFont(obj pdf.Object) *pdfFont {
	dict, ok := r.doc.Resolve(obj).(pdf.Dict)
	if !ok {
		return nil
	}
	doc := r.doc
	f := &pdfFont{defaultW: 1000, fontMatrix: matrix{0.001, 0, 0, 0.001, 0, 0}}

	subtype := dict.GetName(pdf.Name("Subtype"))
	switch subtype {
	case "Type0":
		f.composite = true
		f.enc = parseCMapEncoding(doc, dict.Get(pdf.Name("Encoding")))
		desc, _ := doc.Resolve(dict.Get(pdf.Name("DescendantFonts"))).(pdf.Array)
		var df pdf.Dict
		if len(desc) > 0 {
			df, _ = doc.Resolve(desc[0]).(pdf.Dict)
		}
		if df != nil {
			if v, ok := asFloat(doc.Resolve(df.Get(pdf.Name("DW")))); ok {
				f.defaultW = v
			}
			f.cidW = parseCIDWidths(doc, df.Get(pdf.Name("W")))
			f.loadProgram(r, doc, df)
			switch m := doc.Resolve(df.Get(pdf.Name("CIDToGIDMap"))).(type) {
			case *pdf.Stream:
				if b, err := doc.DecodeStream(m); err == nil {
					f.cidToGID = b
				}
			}
		}
	case "Type3":
		f.type3 = true
		if m := floatArray(doc, dict.Get(pdf.Name("FontMatrix"))); len(m) == 6 {
			f.fontMatrix = matrix{m[0], m[1], m[2], m[3], m[4], m[5]}
		}
		f.charProcs, _ = doc.Resolve(dict.Get(pdf.Name("CharProcs"))).(pdf.Dict)
		f.t3Res, _ = doc.Resolve(dict.Get(pdf.Name("Resources"))).(pdf.Dict)
		f.readSimpleWidths(doc, dict)
		f.readSimpleEncoding(doc, dict, true)
	default:
		f.readSimpleWidths(doc, dict)
		f.loadProgram(r, doc, dict)
		f.readSimpleEncoding(doc, dict, false)
	}

	f.toUnicode = parseToUnicode(doc, dict.Get(pdf.Name("ToUnicode")))
	return f
}

// loadProgram finds the embedded font program, or decides to substitute.
func (f *pdfFont) loadProgram(r *renderer, doc *pdf.Document, dict pdf.Dict) {
	desc, _ := doc.Resolve(dict.Get(pdf.Name("FontDescriptor"))).(pdf.Dict)
	base := string(dict.GetName(pdf.Name("BaseFont")))

	if desc != nil {
		if flags, ok := asInt(doc.Resolve(desc.Get(pdf.Name("Flags")))); ok {
			f.symbolic = flags&4 != 0 && flags&32 == 0
			f.italic = flags&64 != 0
			// A "serif" flag exists too, and is set wrongly often
			// enough by real producers that the base font name is the
			// better signal; see substituteFor.
		}
		if v, ok := asFloat(doc.Resolve(desc.Get(pdf.Name("MissingWidth")))); ok {
			f.missingWidth = v
		}
		if sw, ok := asFloat(doc.Resolve(desc.Get(pdf.Name("StemV")))); ok && sw >= 120 {
			f.bold = true
		}
		for _, key := range []pdf.Name{"FontFile2", "FontFile3", "FontFile"} {
			s, ok := doc.Resolve(desc.Get(key)).(*pdf.Stream)
			if !ok {
				continue
			}
			data, err := doc.DecodeStream(s)
			if err != nil {
				continue
			}
			prog, err := parseTTF(data)
			if err == nil {
				f.prog = prog
				return
			}
			// A font program this package cannot read is not an error:
			// it is the substitute font's whole reason for existing.
			r.note("substituted a " + string(key) + " font program")
			break
		}
	}

	f.substitute = true
	if lower := strings.ToLower(base); lower != "" {
		if strings.Contains(lower, "courier") || strings.Contains(lower, "mono") {
			f.monospaced = true
		}
		if strings.Contains(lower, "bold") || strings.Contains(lower, "black") || strings.Contains(lower, "heavy") {
			f.bold = true
		}
		if strings.Contains(lower, "italic") || strings.Contains(lower, "oblique") {
			f.italic = true
		}
	}
	f.prog = substituteFor(base)
}

func (f *pdfFont) readSimpleWidths(doc *pdf.Document, dict pdf.Dict) {
	f.firstChar, _ = asInt(doc.Resolve(dict.Get(pdf.Name("FirstChar"))))
	f.widths = floatArray(doc, dict.Get(pdf.Name("Widths")))
}

// readSimpleEncoding resolves the code -> character mapping of a simple
// font: a base encoding, then the /Differences array over it.
func (f *pdfFont) readSimpleEncoding(doc *pdf.Document, dict pdf.Dict, isType3 bool) {
	base := standardEncoding
	// A symbolic font's built-in encoding is the font program's own, and
	// is reached through the (3,0) cmap rather than through any table
	// here; leaving the base at Standard costs nothing because
	// glyphFor tries the code directly first for such a font.
	switch enc := doc.Resolve(dict.Get(pdf.Name("Encoding"))).(type) {
	case pdf.Name:
		base = namedEncoding(enc, base)
	case pdf.Dict:
		if n := enc.GetName(pdf.Name("BaseEncoding")); n != "" {
			base = namedEncoding(n, base)
		}
		f.codeRunes = base
		diffs, _ := doc.Resolve(enc.Get(pdf.Name("Differences"))).(pdf.Array)
		code := 0
		for _, e := range diffs {
			switch v := doc.Resolve(e).(type) {
			case int64:
				code = int(v)
			case float64:
				code = int(v)
			case pdf.Name:
				if code >= 0 && code < 256 {
					if isType3 {
						f.t3Names[code] = v
					}
					if r := glyphNameToRune(string(v)); r != 0 {
						f.codeRunes[code] = r
					} else if gid, ok := glyphIndexName(string(v)); ok {
						f.encGIDs[code] = gid
					}
				}
				code++
			}
		}
		return
	}
	f.codeRunes = base
}

func namedEncoding(n pdf.Name, fallback [256]rune) [256]rune {
	switch n {
	case "WinAnsiEncoding":
		return winAnsiEncoding
	case "MacRomanEncoding":
		return macRomanEncoding
	case "StandardEncoding", "MacExpertEncoding":
		return standardEncoding
	}
	return fallback
}

// glyphNameToRune resolves an Adobe glyph name. The composed
// "base + accent" names are built rather than tabulated (see
// encoding_tables.go); "uniXXXX" and "uXXXX" are parsed; a single
// character is itself.
func glyphNameToRune(name string) rune {
	if name == "" {
		return 0
	}
	if r, ok := glyphNames[name]; ok {
		return r
	}
	// A name may carry a suffix after a period ("a.sc", "one.oldstyle").
	if i := strings.IndexByte(name, '.'); i > 0 {
		return glyphNameToRune(name[:i])
	}
	if strings.HasPrefix(name, "uni") && len(name) >= 7 {
		if v, err := strconv.ParseUint(name[3:7], 16, 32); err == nil {
			return rune(v)
		}
	}
	if strings.HasPrefix(name, "u") && len(name) >= 5 && len(name) <= 7 {
		if v, err := strconv.ParseUint(name[1:], 16, 32); err == nil {
			return rune(v)
		}
	}
	if r := []rune(name); len(r) == 1 {
		return r[0]
	}
	return 0
}

// glyphIndexName recognises the names that address a glyph by index
// rather than by meaning — "g12", "cid7", "index42" — which subsetters
// emit when they have thrown the character map away. The longer
// prefixes are tried first, so "glyph7" is not read as "g" followed by
// something that is not a number.
func glyphIndexName(name string) (int, bool) {
	for _, prefix := range []string{"index", "glyph", "cid", "g", "G"} {
		if strings.HasPrefix(name, prefix) {
			if v, err := strconv.Atoi(name[len(prefix):]); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}

// shownGlyph is one character code out of a shown string.
type shownGlyph struct {
	code   uint32
	cid    int
	nbytes int
	width  float64 // text-space units, i.e. already divided by 1000
}

func (f *pdfFont) decode(s []byte) []shownGlyph {
	var out []shownGlyph
	if f == nil {
		return out
	}
	if !f.composite {
		for _, b := range s {
			g := shownGlyph{code: uint32(b), cid: int(b), nbytes: 1}
			g.width = f.widthOf(int(b), g)
			out = append(out, g)
		}
		return out
	}
	for i := 0; i < len(s); {
		code, n := f.enc.next(s[i:])
		cid := f.enc.cid(code, n)
		g := shownGlyph{code: code, cid: cid, nbytes: n}
		g.width = f.widthOf(cid, g)
		out = append(out, g)
		i += n
	}
	return out
}

// widthOf is the advance for one code, in text space units.
//
// The document's own numbers are what count, and they are what is
// looked at first, because they are what its layout was computed
// against. What follows is only for the case where the document does
// not supply any: a font named but not described, which is what a PDF
// written against the fourteen standard fonts is entitled to be.
func (f *pdfFont) widthOf(key int, g shownGlyph) float64 {
	if f.composite {
		if w, ok := f.cidW[key]; ok {
			return w / 1000
		}
		return f.defaultW / 1000
	}
	if i := key - f.firstChar; i >= 0 && i < len(f.widths) {
		return f.widths[i] / 1000
	}
	if f.missingWidth != 0 {
		return f.missingWidth / 1000
	}
	if f.monospaced {
		// Courier and its relatives: every glyph six tenths of an em,
		// which is the one standard font metric that is a single number
		// rather than a table.
		return 0.6
	}
	// Nothing left but the substitute font's own advances. They are not
	// the standard font's metrics — Noto Sans is a few percent wider
	// than Helvetica — so a line set this way is a few percent long.
	// It is proportional text at the right size in the right place,
	// which is what a page being measured for a stamp needs; and it
	// only ever happens for a document that declined to say how wide
	// its own characters are.
	if f.prog != nil {
		if gid := f.glyphFor(g); gid > 0 {
			if adv := f.prog.advance(gid); adv > 0 {
				return adv / f.prog.upem
			}
		}
	}
	return 0.5
}

// glyphFor turns one shown code into a glyph index in f.prog.
func (f *pdfFont) glyphFor(g shownGlyph) int {
	if f.prog == nil {
		return 0
	}
	if f.composite {
		if f.substitute {
			return f.prog.gidForRune(f.runeFor(g))
		}
		if len(f.cidToGID) > 0 {
			i := g.cid * 2
			if i+1 < len(f.cidToGID) {
				return int(f.cidToGID[i])<<8 | int(f.cidToGID[i+1])
			}
			return 0
		}
		return g.cid
	}

	code := int(g.code) & 0xff
	if f.substitute {
		return f.prog.gidForRune(f.runeFor(g))
	}
	// A /Differences entry that names a glyph index rather than a
	// character means exactly what it says, and says it about this
	// font program specifically.
	if gid := f.encGIDs[code]; gid != 0 {
		return gid
	}
	// PDF 32000-1 §9.6.6.4, in the order it gives: a symbolic font is
	// addressed by its own code through the (3,0) or (1,0) subtable;
	// otherwise the code names a character, and the character is looked
	// up in the Unicode subtable.
	if f.symbolic {
		if gid := f.prog.gidForCode(code); gid != 0 {
			return gid
		}
	}
	if r := f.codeRunes[code]; r != 0 {
		if gid := f.prog.gidForRune(r); gid != 0 {
			return gid
		}
	}
	if gid := f.prog.gidForCode(code); gid != 0 {
		return gid
	}
	if r := f.runeFor(g); r != 0 {
		if gid := f.prog.gidForRune(r); gid != 0 {
			return gid
		}
	}
	if !f.prog.hasCmap() {
		// A subset font with no character map at all addresses glyphs
		// by code directly, which is what its producer intended.
		return code
	}
	return 0
}

// runeFor is what a code means as text: the /ToUnicode map first, since
// it is the producer's own statement, then the encoding table.
func (f *pdfFont) runeFor(g shownGlyph) rune {
	if r, ok := f.toUnicode[g.code]; ok && r != 0 {
		return r
	}
	if !f.composite {
		if r := f.codeRunes[int(g.code)&0xff]; r != 0 {
			return r
		}
	}
	if f.composite && f.enc != nil && f.enc.identity {
		// Identity encoding with no /ToUnicode: the CID is a glyph
		// index into a font this package could not read, so there is
		// nothing that says what the character is.
		return 0
	}
	return 0
}
