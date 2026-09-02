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

// TextWidth1000 returns text's total advance width in 1000-unit glyph
// space (the space /W and font size arithmetic both use), or a
// MissingGlyphError if any character is not in the subset.
func TextWidth1000(text string) (int, error) {
	cids, err := EncodeCIDs(text)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, cid := range cids {
		total += int(gidWidths[cid])
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

// fontObjects is every indirect object this stamp's embedded font
// contributes to the incremental revision, plus the Type0 font's own
// object number (the one /Resources /Font entries point at).
type fontObjects struct {
	Type0Num int
}

// addFontObjects allocates and writes (via u.Set) the complete Type0
// font: the Type0 dict, its CIDFontType2 descendant, the
// FontDescriptor, the FontFile2 stream and the ToUnicode CMap stream
// (F4 §3.1/§3.4) — every table F4's structure diagram requires, built
// from the data scripts/gensubsetfont embedded.
func addFontObjects(u *pdf.Update) fontObjects {
	type0Num := u.NewObjectNumber()
	cidFontNum := u.NewObjectNumber()
	descriptorNum := u.NewObjectNumber()
	fontFileNum := u.NewObjectNumber()
	toUnicodeNum := u.NewObjectNumber()

	baseFont := pdf.Name(subsetTag + "+NotoSans")

	compressed := flateCompress(subsetFontTTF)
	u.Set(fontFileNum, &pdf.Stream{
		Dict: pdf.Dict{
			pdf.Name("Length1"): int64(len(subsetFontTTF)),
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
		pdf.Name("FontBBox"):    pdf.Array{int64(fontBBoxMinX), int64(fontBBoxMinY), int64(fontBBoxMaxX), int64(fontBBoxMaxY)},
		pdf.Name("ItalicAngle"): int64(0),
		pdf.Name("Ascent"):      int64(fontAscent),
		pdf.Name("Descent"):     int64(fontDescent),
		pdf.Name("CapHeight"):   int64(fontCapHeight),
		// StemV has no measured value here (this subset carries no PANOSE/
		// OS/2 data — see scripts/gensubsetfont's package doc comment for
		// why OS/2 is omitted); 80 is the conventional placeholder several
		// real-world PDF producers use for a medium-weight sans-serif, and
		// PDF viewers use it only for font-substitution decisions, which
		// never apply here since the font is embedded (PDF 32000-1 §9.8.1).
		pdf.Name("StemV"):     int64(80),
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
		pdf.Name("W"):              widthsArray(),
		pdf.Name("CIDToGIDMap"):    pdf.Name("Identity"),
	})

	u.Set(toUnicodeNum, &pdf.Stream{
		Dict: pdf.Dict{},
		Raw:  buildToUnicodeCMap(),
	})

	u.Set(type0Num, pdf.Dict{
		pdf.Name("Type"):            pdf.Name("Font"),
		pdf.Name("Subtype"):         pdf.Name("Type0"),
		pdf.Name("BaseFont"):        baseFont,
		pdf.Name("Encoding"):        pdf.Name("Identity-H"),
		pdf.Name("DescendantFonts"): pdf.Array{pdf.Reference{Num: cidFontNum}},
		pdf.Name("ToUnicode"):       pdf.Reference{Num: toUnicodeNum},
	})

	return fontObjects{Type0Num: type0Num}
}

// widthsArray builds /W in the compact "c [w1 w2 ... wn]" form, one
// entry for every glyph in the subset (F4 §3.5: "the real advance width
// of every glyph in the subset," not only the glyphs one particular
// stamp happens to draw — the font is embedded once and may be reused,
// within the same document, by a later revision this project does not
// control).
func widthsArray() pdf.Array {
	ws := make(pdf.Array, len(gidWidths))
	for i, w := range gidWidths {
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
