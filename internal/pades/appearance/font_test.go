package appearance

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

// TestEncodeCIDsCyrillicMatchesIndependentlyParsedFont is F4 §8's
// central font test: "Cyrillic name from the MUP certificate renders
// correctly — assert the CID sequence matches the expected glyph
// indices." SPEC §11.7's real, measured MUP givenName + surname is
// "ВЕЉКО СТАНОЈЕВИЋ".
//
// The "expected glyph indices" are not re-derived from this package's
// own runeToGID map — that would only prove EncodeCIDs agrees with
// itself. Instead notosans-subset.ttf (the committed asset) is parsed
// cold by golang.org/x/image/font/sfnt, a decoder scripts/gensubsetfont
// also uses but this package's own runtime code never imports, and its
// glyph indices are compared against EncodeCIDs' output — the same
// "independent second implementation" discipline internal/pades/verify
// applies to signatures (F3 §8/D-044), applied here to font bytes.
func TestEncodeCIDsCyrillicMatchesIndependentlyParsedFont(t *testing.T) {
	const mupName = "ВЕЉКО СТАНОЈЕВИЋ" // SPEC §11.7

	f, err := sfnt.Parse(subsetFontTTF)
	if err != nil {
		t.Fatalf("independently parsing notosans-subset.ttf: %v", err)
	}
	var buf sfnt.Buffer

	cids, err := EncodeCIDs(mupName)
	if err != nil {
		t.Fatalf("EncodeCIDs(%q): %v", mupName, err)
	}
	runes := []rune(mupName)
	if len(cids) != len(runes) {
		t.Fatalf("len(cids) = %d, want %d (one per rune)", len(cids), len(runes))
	}
	for i, r := range runes {
		wantGID, err := f.GlyphIndex(&buf, r)
		if err != nil {
			t.Fatalf("independent GlyphIndex(%q): %v", r, err)
		}
		if cids[i] != uint16(wantGID) {
			t.Errorf("EncodeCIDs(%q)[%d] (rune %q) = %d, want %d (independently parsed)", mupName, i, r, cids[i], wantGID)
		}
	}
}

// TestEncodeCIDsLatinExtrasMatchIndependentlyParsedFont is F4 §8's
// "Latin name with č ć đ š ž renders correctly," exercised against
// SPEC §11.7's other two real examples in one string plus every
// Serbian Latin diacritic and its capital.
func TestEncodeCIDsLatinExtrasMatchIndependentlyParsedFont(t *testing.T) {
	const name = "Zoran Milovanović Redžvel Mešković čćđšž ČĆĐŠŽ"

	f, err := sfnt.Parse(subsetFontTTF)
	if err != nil {
		t.Fatal(err)
	}
	var buf sfnt.Buffer

	cids, err := EncodeCIDs(name)
	if err != nil {
		t.Fatalf("EncodeCIDs(%q): %v", name, err)
	}
	for i, r := range []rune(name) {
		wantGID, err := f.GlyphIndex(&buf, r)
		if err != nil {
			t.Fatalf("independent GlyphIndex(%q): %v", r, err)
		}
		if cids[i] != uint16(wantGID) {
			t.Errorf("EncodeCIDs(%q)[%d] (rune %q) = %d, want %d", name, i, r, cids[i], wantGID)
		}
	}
}

// TestEncodeCIDsMissingGlyphFailsLoudly is F4 §3.3/§8's hard
// requirement: "A character outside the subset produces a clear error
// naming the character" — the test that makes this specific discipline
// fail if it regresses (a security-relevant check per this project's
// own stated policy: a silently missing character in a legal document's
// stamp is worse than a failed signing operation).
func TestEncodeCIDsMissingGlyphFailsLoudly(t *testing.T) {
	// U+4E2D ("中", CJK), Greek 'Ω' and an emoji are all, by construction,
	// outside the subset (charset.go's charset() covers ASCII, the
	// Serbian Latin/Cyrillic alphabets and a fixed list of Serbian
	// punctuation — Task 3 — never CJK, Greek or general Unicode
	// symbols). U+2026 (the typographic ellipsis) is no longer in this
	// list: Task 3 added it to the subset, superseding D-057's earlier
	// decision to spell the stamp's own truncation mark as three ASCII
	// periods instead of embedding it.
	for _, r := range []rune{'中', 'Ω', '\U0001F600'} {
		_, err := EncodeCIDs(string(r))
		if err == nil {
			t.Fatalf("EncodeCIDs(%q): want an error, got none", r)
		}
		var mg *MissingGlyphError
		if !errors.As(err, &mg) {
			t.Fatalf("EncodeCIDs(%q): error is %T, want *MissingGlyphError", r, err)
		}
		if mg.Rune != r {
			t.Fatalf("MissingGlyphError.Rune = %q, want %q", mg.Rune, r)
		}
		msg := err.Error()
		if !strings.ContainsRune(msg, r) {
			t.Errorf("error message %q does not name the character %q", msg, r)
		}
		wantCodePoint := runeCodePointHex(r)
		if !strings.Contains(msg, wantCodePoint) {
			t.Errorf("error message %q does not name the code point %q", msg, wantCodePoint)
		}
	}
}

// TestEncodeCIDsMissingGlyphInsideLongerString confirms the failure
// fires even when only one character in a longer, mostly-valid string
// is unsupported — the realistic case for a caller-supplied
// --stamp-reference string.
func TestEncodeCIDsMissingGlyphInsideLongerString(t *testing.T) {
	_, err := EncodeCIDs("INV-2026-中-0042")
	var mg *MissingGlyphError
	if !errors.As(err, &mg) {
		t.Fatalf("EncodeCIDs: error is %T, want *MissingGlyphError", err)
	}
	if mg.Rune != '中' {
		t.Fatalf("MissingGlyphError.Rune = %q, want '中'", mg.Rune)
	}
}

// TestToUnicodeCoversEveryGID is F4 §3.4/§8: "/ToUnicode is present and
// maps every drawn glyph." Every GID this subset defines (not only
// GIDs one particular stamp happens to draw) must have a bfchar entry,
// since the font — and therefore its ToUnicode CMap — is embedded once
// per document and may in principle be consulted for any glyph in it.
func TestToUnicodeCoversEveryGID(t *testing.T) {
	cmap := string(buildToUnicodeCMap())

	if !strings.Contains(cmap, "beginbfchar") || !strings.Contains(cmap, "endbfchar") {
		t.Fatal("ToUnicode CMap is missing beginbfchar/endbfchar")
	}
	for r, gid := range runeToGID {
		hex := gidHex(gid)
		if !strings.Contains(cmap, "<"+hex+"> <") {
			t.Errorf("ToUnicode CMap has no bfchar entry for GID %d (rune %q)", gid, r)
		}
	}
}

func gidHex(gid uint16) string {
	const hexDigits = "0123456789ABCDEF"
	b := [4]byte{
		hexDigits[(gid>>12)&0xF],
		hexDigits[(gid>>8)&0xF],
		hexDigits[(gid>>4)&0xF],
		hexDigits[gid&0xF],
	}
	return string(b[:])
}

func runeCodePointHex(r rune) string {
	return fmt.Sprintf("U+%04X", r)
}
