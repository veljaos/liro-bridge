package appearance

import (
	"bytes"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// TestBoldSubsetIsTheSameCharactersAtTheSameGIDs is the property the
// whole two-face arrangement rests on (D-209): both subsets are built
// from the same sorted charset(), so a rune's glyph index is the same
// number in either face, and one runeToGID table plus one /ToUnicode
// CMap can serve both.
//
// If that ever stopped being true the stamp would not fail — it would
// draw the signer's name in the wrong letters, silently, which is the
// worst outcome this package has. So it is checked the way
// TestEncodeCIDsCyrillicMatchesIndependentlyParsedFont checks the
// regular face: against the committed .ttf asset parsed cold by
// golang.org/x/image/font/sfnt, a decoder no runtime code here imports.
func TestBoldSubsetIsTheSameCharactersAtTheSameGIDs(t *testing.T) {
	regular, err := sfnt.Parse(subsetFontTTF)
	if err != nil {
		t.Fatalf("independently parsing notosans-subset.ttf: %v", err)
	}
	bold, err := sfnt.Parse(boldSubsetFontTTF)
	if err != nil {
		t.Fatalf("independently parsing notosans-bold-subset.ttf: %v", err)
	}
	if got, want := bold.NumGlyphs(), regular.NumGlyphs(); got != want {
		t.Fatalf("bold face has %d glyphs, regular has %d", got, want)
	}
	if got, want := len(gidWidthsBold), len(gidWidths); got != want {
		t.Fatalf("len(gidWidthsBold) = %d, len(gidWidths) = %d", got, want)
	}

	var rbuf, bbuf sfnt.Buffer
	for r, gid := range runeToGID {
		boldGID, err := bold.GlyphIndex(&bbuf, r)
		if err != nil {
			t.Fatalf("bold GlyphIndex(%q): %v", r, err)
		}
		if uint16(boldGID) != gid {
			t.Errorf("bold face maps %q to GID %d, runeToGID says %d", r, boldGID, gid)
		}
		regularGID, err := regular.GlyphIndex(&rbuf, r)
		if err != nil {
			t.Fatalf("regular GlyphIndex(%q): %v", r, err)
		}
		if uint16(regularGID) != gid {
			t.Errorf("regular face maps %q to GID %d, runeToGID says %d", r, regularGID, gid)
		}
	}
}

// TestBoldFaceIsAGenuineBoldNotTheRegularFaceRenamed is the check that
// the cost recorded in D-209 — a second embedded font, 31 184 bytes in
// the binary and 13 514 in every stamped document — bought something. A
// second copy of the regular subset under a different name would
// satisfy every other test in this file (same characters, same GIDs,
// same glyph count) and draw the signer's name in exactly the weight it
// was drawn in before; so would a synthetic stroke, which changes no
// metric at all.
//
// The decisive measurement is the thickness of a stem, and the Latin
// capital I is a stem and nothing else: its outline's own width, read
// off the committed asset by an independent decoder, is how heavy the
// face is. NotoSans Bold's is a quarter again NotoSans Regular's.
// Aggregate advance width over every capital a name can contain is
// checked too — bold is wider overall, even though a few individual
// letters (G among them) are not.
func TestBoldFaceIsAGenuineBoldNotTheRegularFaceRenamed(t *testing.T) {
	if bytes.Equal(boldSubsetFontTTF, subsetFontTTF) {
		t.Fatal("notosans-bold-subset.ttf is byte-identical to notosans-subset.ttf")
	}

	regular, err := sfnt.Parse(subsetFontTTF)
	if err != nil {
		t.Fatal(err)
	}
	bold, err := sfnt.Parse(boldSubsetFontTTF)
	if err != nil {
		t.Fatal(err)
	}
	regularStem := stemWidth(t, regular, 'I')
	boldStem := stemWidth(t, bold, 'I')
	if boldStem <= regularStem {
		t.Errorf("the bold face's I is %d design units wide, the regular face's is %d — that is not a heavier face", boldStem, regularStem)
	}
	t.Logf("stem width of I: regular %d, bold %d design units (%.0f%% heavier)",
		regularStem, boldStem, 100*(float64(boldStem)/float64(regularStem)-1))

	// Every letter a signer's name can be upper-cased into, Latin and
	// Cyrillic.
	const capitals = "ABCDEFGHIJKLMNOPQRSTUVWXYZČĆĐŠŽАБВГДЂЕЖЗИЈКЛЉМНЊОПРСТЋУФХЦЧЏШ"
	var regularTotal, boldTotal int
	for _, r := range capitals {
		gid, ok := runeToGID[r]
		if !ok {
			t.Fatalf("%q is not in the subset at all", r)
		}
		regularTotal += int(gidWidths[gid])
		boldTotal += int(gidWidthsBold[gid])
	}
	if boldTotal <= regularTotal {
		t.Errorf("the capitals total %d units bold and %d regular — bold is not the wider face", boldTotal, regularTotal)
	}
}

// stemWidth is the width, in design units, of one glyph's outline as an
// independent decoder reads it out of a committed font asset.
func stemWidth(t *testing.T, f *sfnt.Font, r rune) int {
	t.Helper()
	var buf sfnt.Buffer
	gid, err := f.GlyphIndex(&buf, r)
	if err != nil {
		t.Fatalf("GlyphIndex(%q): %v", r, err)
	}
	bounds, _, err := f.GlyphBounds(&buf, gid, fixed.I(int(f.UnitsPerEm())), font.HintingNone)
	if err != nil {
		t.Fatalf("GlyphBounds(%q): %v", r, err)
	}
	return (bounds.Max.X - bounds.Min.X).Round()
}

// TestBothFacesAreEmbeddedOverOneSharedToUnicodeCMap checks what
// actually reaches the document: two /FontFile2 streams, one per face,
// each carrying its own asset verbatim — and exactly one /ToUnicode
// CMap, referenced by both Type0 fonts (D-209). The shared CMap is
// worth asserting rather than assuming: two copies would be ~2.7 KB of
// duplicated stream in every signed document, and — worse — two places
// for the GID-to-Unicode answer to differ.
func TestBothFacesAreEmbeddedOverOneSharedToUnicodeCMap(t *testing.T) {
	doc, pageDict := mustParsePageOne(t)
	u := pdf.NewUpdate(doc)
	if _, err := Render(doc, pageDict, u, baseOptions()); err != nil {
		t.Fatalf("Render: %v", err)
	}
	out, err := u.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	doc2, err := pdf.Parse(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}

	lengths := map[int64]int{}
	toUnicodeRefs := map[int]int{}
	type0s := 0
	for num := 1; num <= 200; num++ {
		switch obj := doc2.Get(num).(type) {
		case *pdf.Stream:
			if l1, ok := obj.Dict.Get(pdf.Name("Length1")).(int64); ok {
				lengths[l1]++
			}
		case pdf.Dict:
			if obj.Get(pdf.Name("Subtype")) != pdf.Name("Type0") {
				continue
			}
			type0s++
			ref, ok := obj.Get(pdf.Name("ToUnicode")).(pdf.Reference)
			if !ok {
				t.Fatalf("Type0 font %d has no /ToUnicode reference", num)
			}
			toUnicodeRefs[ref.Num]++
		}
	}

	if type0s != 2 {
		t.Fatalf("found %d Type0 fonts, want 2 (regular and bold)", type0s)
	}
	if len(toUnicodeRefs) != 1 {
		t.Fatalf("the two Type0 fonts point at %d different /ToUnicode streams, want 1 shared", len(toUnicodeRefs))
	}
	for _, want := range []int64{int64(len(subsetFontTTF)), int64(len(boldSubsetFontTTF))} {
		if lengths[want] != 1 {
			t.Errorf("found %d /FontFile2 streams with /Length1 %d, want exactly 1", lengths[want], want)
		}
	}
}
