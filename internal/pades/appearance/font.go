package appearance

import (
	"bytes"
	"compress/zlib"
	_ "embed"
	"fmt"
	"sort"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// subsetFontTTF is the subsetted NotoSans, Type0/CIDFontType2-ready,
// generated at build time by scripts/gensubsetfont and committed as an
// asset (F4 §3.2). Nothing in this package parses it — it is copied
// into the PDF's /FontFile2 stream verbatim (after generic Flate
// compression, which is encoding, not parsing).
//
//go:embed notosans-subset.ttf
var subsetFontTTF []byte

// boldSubsetFontTTF is the same subset of NotoSans Bold: the same
// characters, in the same sorted order, so it shares runeToGID and the
// /ToUnicode CMap with the regular face and differs only in outlines
// and advance widths (D-209). It exists for one line of the stamp — the
// signer's name — and costs 31 184 bytes in the binary and 13 514 bytes
// of /FontFile2 in every stamped document.
//
//go:embed notosans-bold-subset.ttf
var boldSubsetFontTTF []byte

// Weight selects which of the two embedded faces a line of stamp text
// is drawn in. The two are interchangeable everywhere a CID is
// concerned — same characters, same GIDs — and differ only in shape and
// advance width, which is why nothing outside widthsFor and addFace has
// to know that there are two of them at all.
type Weight int

const (
	Regular Weight = iota
	Bold
)

// widthsFor is one weight's advance-width table, in 1000-unit glyph
// space.
func widthsFor(weight Weight) []uint16 {
	if weight == Bold {
		return gidWidthsBold
	}
	return gidWidths
}

// MissingGlyphError is returned by EncodeCIDs for a character the
// embedded subset cannot draw (F4 §3.3): "If a character is not in the
// subset, return a clear error naming the character and its code
// point... Do not draw nothing, do not substitute, do not fall back to
// a question mark."
type MissingGlyphError struct{ Rune rune }

func (e *MissingGlyphError) Error() string {
	return fmt.Sprintf("appearance: character %q (U+%04X) is not in the embedded font subset", e.Rune, e.Rune)
}

// EncodeCIDs converts text to the CID sequence Identity-H addressing
// requires: this font's /CIDToGIDMap is /Identity (F4 §3.1), so each
// rune's CID is simply its glyph index in the subset.
func EncodeCIDs(text string) ([]uint16, error) {
	cids := make([]uint16, 0, len(text))
	for _, r := range text {
		gid, ok := runeToGID[r]
		if !ok {
			return nil, &MissingGlyphError{Rune: r}
		}
		cids = append(cids, gid)
	}
	return cids, nil
}

// TextWidth1000 returns text's total advance width in weight's face, in
// 1000-unit glyph space (the space /W and font size arithmetic both
// use), or a MissingGlyphError if any character is not in the subset.
//
// The weight is a parameter rather than an assumption because bold is
// measurably wider than regular for the same string, and the one line
// drawn bold — the signer's name — is also the stamp's longest and the
// one most likely to need reducing or truncating to fit.
func TextWidth1000(text string, weight Weight) (int, error) {
	cids, err := EncodeCIDs(text)
	if err != nil {
		return 0, err
	}
	widths := widthsFor(weight)
	total := 0
	for _, cid := range cids {
		total += int(widths[cid])
	}
	return total, nil
}

// cidsToHex renders cids as the big-endian, two-byte-per-glyph hex
// string Identity-H requires for Tj (F4 §3.3: "<00480065...> Tj").
func cidsToHex(cids []uint16) string {
	var b bytes.Buffer
	b.WriteByte('<')
	for _, c := range cids {
		fmt.Fprintf(&b, "%04X", c)
	}
	b.WriteByte('>')
	return b.String()
}

// fontObjects is the two Type0 font object numbers the stamp's
// /Resources /Font entries point at: the regular face every line but
// one is drawn in, and the bold face the signer's name is drawn in.
type fontObjects struct {
	Type0Num     int
	Type0BoldNum int
}

// addFontObjects allocates and writes (via u.Set) both embedded faces —
// each a Type0 dict, a CIDFontType2 descendant, a FontDescriptor and a
// FontFile2 stream (F4 §3.1) — over one shared /ToUnicode CMap stream
// (F4 §3.4), built from the data scripts/gensubsetfont embedded.
func addFontObjects(u *pdf.Update) fontObjects {
	// One /ToUnicode CMap, shared by both faces. Both carry the same
	// characters at the same GIDs (D-209), so the GID-to-Unicode map is
	// the same table twice over — and text a reader selects across both
	// faces comes back as one answer rather than two that have to agree.
	toUnicodeNum := u.NewObjectNumber()
	u.Set(toUnicodeNum, &pdf.Stream{
		Dict: pdf.Dict{},
		Raw:  buildToUnicodeCMap(),
	})

	return fontObjects{
		Type0Num:     addFace(u, Regular, toUnicodeNum),
		Type0BoldNum: addFace(u, Bold, toUnicodeNum),
	}
}

// addFace writes one complete embedded face — the Type0 dict, its
// CIDFontType2 descendant, the FontDescriptor and the FontFile2 stream
// (F4 §3.1) — and returns the Type0 dict's object number. The
// /ToUnicode CMap is passed in rather than built here because the two
// faces share one (addFontObjects).
func addFace(u *pdf.Update, weight Weight, toUnicodeNum int) int {
	type0Num := u.NewObjectNumber()
	cidFontNum := u.NewObjectNumber()
	descriptorNum := u.NewObjectNumber()
	fontFileNum := u.NewObjectNumber()

	ttf := subsetFontTTF
	baseFont := pdf.Name(subsetTag + "+NotoSans")
	ascent, descent, capHeight := int64(fontAscent), int64(fontDescent), int64(fontCapHeight)
	bbox := pdf.Array{int64(fontBBoxMinX), int64(fontBBoxMinY), int64(fontBBoxMaxX), int64(fontBBoxMaxY)}
	// StemV is the one /FontDescriptor entry that is supposed to differ
	// with weight — it is the vertical stem width, and a bold face's
	// stems really are thicker. Both numbers are still the conventional
	// placeholders the comment below describes, since no PANOSE/OS-2
	// data survives this subset; but a bold face declaring the regular
	// face's stem width would be a statement this project knows to be
	// false, so the two are not the same placeholder.
	stemV := int64(80)
	if weight == Bold {
		ttf = boldSubsetFontTTF
		baseFont = pdf.Name(subsetTag + "+NotoSans-Bold")
		ascent, descent, capHeight = int64(boldFontAscent), int64(boldFontDescent), int64(boldFontCapHeight)
		bbox = pdf.Array{int64(boldFontBBoxMinX), int64(boldFontBBoxMinY), int64(boldFontBBoxMaxX), int64(boldFontBBoxMaxY)}
		stemV = 160
	}

	compressed := flateCompress(ttf)
	u.Set(fontFileNum, &pdf.Stream{
		Dict: pdf.Dict{
			pdf.Name("Length1"): int64(len(ttf)),
			pdf.Name("Filter"):  pdf.Name("FlateDecode"),
		},
		Raw: compressed,
	})

	u.Set(descriptorNum, pdf.Dict{
		pdf.Name("Type"):     pdf.Name("FontDescriptor"),
		pdf.Name("FontName"): baseFont,
		// Symbolic (bit 3, value 4): this font's glyphs are addressed by
		// CID/GID via Identity-H, not through a StandardEncoding-style
		// named-glyph encoding (PDF 32000-1 §9.8.2, Table 123).
		pdf.Name("Flags"):       int64(4),
		pdf.Name("FontBBox"):    bbox,
		pdf.Name("ItalicAngle"): int64(0),
		pdf.Name("Ascent"):      ascent,
		pdf.Name("Descent"):     descent,
		pdf.Name("CapHeight"):   capHeight,
		// StemV has no measured value here (this subset carries no PANOSE/
		// OS/2 data — see scripts/gensubsetfont's package doc comment for
		// why OS/2 is omitted); 80 is the conventional placeholder several
		// real-world PDF producers use for a medium-weight sans-serif, and
		// PDF viewers use it only for font-substitution decisions, which
		// never apply here since the font is embedded (PDF 32000-1 §9.8.1).
		pdf.Name("StemV"):     stemV,
		pdf.Name("FontFile2"): pdf.Reference{Num: fontFileNum},
	})

	u.Set(cidFontNum, pdf.Dict{
		pdf.Name("Type"):     pdf.Name("Font"),
		pdf.Name("Subtype"):  pdf.Name("CIDFontType2"),
		pdf.Name("BaseFont"): baseFont,
		pdf.Name("CIDSystemInfo"): pdf.Dict{
			pdf.Name("Registry"):   pdf.String("Adobe"),
			pdf.Name("Ordering"):   pdf.String("Identity"),
			pdf.Name("Supplement"): int64(0),
		},
		pdf.Name("FontDescriptor"): pdf.Reference{Num: descriptorNum},
		pdf.Name("DW"):             int64(1000),
		pdf.Name("W"):              widthsArray(weight),
		pdf.Name("CIDToGIDMap"):    pdf.Name("Identity"),
	})

	u.Set(type0Num, pdf.Dict{
		pdf.Name("Type"):            pdf.Name("Font"),
		pdf.Name("Subtype"):         pdf.Name("Type0"),
		pdf.Name("BaseFont"):        baseFont,
		pdf.Name("Encoding"):        pdf.Name("Identity-H"),
		pdf.Name("DescendantFonts"): pdf.Array{pdf.Reference{Num: cidFontNum}},
		pdf.Name("ToUnicode"):       pdf.Reference{Num: toUnicodeNum},
	})

	return type0Num
}

// widthsArray builds /W in the compact "c [w1 w2 ... wn]" form, one
// entry for every glyph in the subset (F4 §3.5: "the real advance width
// of every glyph in the subset," not only the glyphs one particular
// stamp happens to draw — the font is embedded once and may be reused,
// within the same document, by a later revision this project does not
// control).
func widthsArray(weight Weight) pdf.Array {
	widths := widthsFor(weight)
	ws := make(pdf.Array, len(widths))
	for i, w := range widths {
		ws[i] = int64(w)
	}
	return pdf.Array{int64(0), ws}
}

// buildToUnicodeCMap emits a CMap stream mapping every GID in the
// subset back to its Unicode code point (F4 §3.4: required, not
// optional — "without it the stamp text cannot be selected, copied or
// searched, and accessibility tools read nothing").
func buildToUnicodeCMap() []byte {
	type entry struct {
		gid uint16
		r   rune
	}
	entries := make([]entry, 0, len(runeToGID))
	for r, gid := range runeToGID {
		entries = append(entries, entry{gid, r})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].gid < entries[j].gid })

	var b bytes.Buffer
	b.WriteString("/CIDInit /ProcSet findresource begin\n")
	b.WriteString("12 dict begin\n")
	b.WriteString("begincmap\n")
	b.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	b.WriteString("/CMapName /Adobe-Identity-UCS def\n")
	b.WriteString("/CMapType 2 def\n")
	b.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	fmt.Fprintf(&b, "%d beginbfchar\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(&b, "<%04X> <%s>\n", e.gid, utf16BEHex(e.r))
	}
	b.WriteString("endbfchar\n")
	b.WriteString("endcmap\n")
	b.WriteString("CMapName currentdict /CMap defineresource pop\n")
	b.WriteString("end\nend\n")
	return b.Bytes()
}

// utf16BEHex renders r as the hex-encoded UTF-16BE bytes a ToUnicode
// CMap's bfchar entries require (surrogate pairs for r > U+FFFF, though
// this subset's own charset never needs one).
func utf16BEHex(r rune) string {
	if r <= 0xFFFF {
		return fmt.Sprintf("%04X", r)
	}
	r -= 0x10000
	hi := 0xD800 + (r >> 10)
	lo := 0xDC00 + (r & 0x3FF)
	return fmt.Sprintf("%04X%04X", hi, lo)
}

// flateCompress zlib-wraps data (RFC 1950), matching what PDF's
// /FlateDecode filter requires and what internal/pades/pdf's own
// decoder expects (internal/pades/pdf/filters.go). This is generic
// encoding of already-trusted, project-authored bytes, not the kind of
// format *parsing* F4 §3.2 forbids at runtime.
func flateCompress(data []byte) []byte {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}
