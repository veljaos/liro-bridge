package appearance

// Byte-level checks on the committed font subset itself, rather than on
// what any decoder makes of it.
//
// The defect these exist for was reported from a signed document, in
// words: in "ВЕЉКО СТАНОЈЕВИЋ", the Ј and the Е were touching. Every
// test in this package was green. They could all be green, because
// every one of them asks a question about advance widths or glyph
// indices, and the fault was in neither: scripts/gensubsetfont wrote
// zero as every glyph's left side bearing, with a comment saying the
// field was "not used by this project's rendering path". It is used. A
// TrueType rasteriser positions a glyph by shifting its outline
// horizontally by (hmtx.leftSideBearing - glyf.xMin), so a bearing of
// zero moves every glyph left by its own xMin — a different amount for
// every letter. Cyrillic Ј has xMin -78, so it moved 78 units the other
// way and ran into whatever followed it.
//
// So these read the font's own tables and check it does not contradict
// itself. Nothing here goes through EncodeCIDs, TextWidth1000 or
// gidWidths: those are the layer that was right while the asset was
// wrong.

import (
	"encoding/binary"
	"testing"
)

// sfntTables splits the committed subset into its tables by tag.
func sfntTables(t *testing.T) map[string][]byte {
	t.Helper()
	d := subsetFontTTF
	if len(d) < 12 {
		t.Fatalf("the embedded subset is %d bytes; that is not a font", len(d))
	}
	n := int(binary.BigEndian.Uint16(d[4:6]))
	out := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		if rec+16 > len(d) {
			t.Fatalf("table record %d runs past the end of the font", i)
		}
		tag := string(d[rec : rec+4])
		off := binary.BigEndian.Uint32(d[rec+8:])
		length := binary.BigEndian.Uint32(d[rec+12:])
		if int(off)+int(length) > len(d) {
			t.Fatalf("table %q claims bytes %d..%d of a %d-byte font", tag, off, int(off)+int(length), len(d))
		}
		out[tag] = d[off : off+length]
	}
	return out
}

// glyfEntries returns each glyph's glyf bytes, in GID order, via loca.
func glyfEntries(t *testing.T, tables map[string][]byte) [][]byte {
	t.Helper()
	head, glyf, loca := tables["head"], tables["glyf"], tables["loca"]
	maxp := tables["maxp"]
	for tag, b := range map[string][]byte{"head": head, "glyf": glyf, "loca": loca, "maxp": maxp} {
		if b == nil {
			t.Fatalf("the subset has no %q table", tag)
		}
	}
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:]))
	longLoca := binary.BigEndian.Uint16(head[50:]) == 1

	offset := func(i int) uint32 {
		if longLoca {
			return binary.BigEndian.Uint32(loca[i*4:])
		}
		return uint32(binary.BigEndian.Uint16(loca[i*2:])) * 2
	}

	out := make([][]byte, numGlyphs)
	for i := 0; i < numGlyphs; i++ {
		start, end := offset(i), offset(i+1)
		if start > end || int(end) > len(glyf) {
			t.Fatalf("glyph %d has loca offsets %d..%d in a %d-byte glyf table", i, start, end, len(glyf))
		}
		out[i] = glyf[start:end]
	}
	return out
}

// TestEveryGlyphsBearingMatchesItsOwnOutline is the check that would
// have caught the reported defect, stated as the invariant it broke:
// for a glyph with an outline, hmtx's left side bearing and glyf's own
// xMin are two recordings of the same number, and a rasteriser that
// finds them disagreeing moves the glyph by the difference.
func TestEveryGlyphsBearingMatchesItsOwnOutline(t *testing.T) {
	tables := sfntTables(t)
	hmtx, hhea := tables["hmtx"], tables["hhea"]
	if hmtx == nil || hhea == nil {
		t.Fatal("the subset has no hmtx or hhea table")
	}
	entries := glyfEntries(t, tables)

	numberOfHMetrics := int(binary.BigEndian.Uint16(hhea[34:]))
	if numberOfHMetrics != len(entries) {
		t.Fatalf("hhea.numberOfHMetrics = %d for %d glyphs; this subset writes one full metric per glyph",
			numberOfHMetrics, len(entries))
	}
	if len(hmtx) < numberOfHMetrics*4 {
		t.Fatalf("hmtx is %d bytes for %d metrics", len(hmtx), numberOfHMetrics)
	}

	for gid, entry := range entries {
		lsb := int16(binary.BigEndian.Uint16(hmtx[gid*4+2:])) //nolint:gosec // the field is a signed FWORD
		if len(entry) == 0 {
			// No outline: the spec's bearing is zero, and .notdef is the
			// only such glyph here (F4 §3.3 makes a missing character a
			// hard error, so .notdef is never drawn).
			if lsb != 0 {
				t.Errorf("glyph %d has no outline but a bearing of %d", gid, lsb)
			}
			continue
		}
		if len(entry) < 10 {
			t.Fatalf("glyph %d has a %d-byte glyf entry; the header alone is 10", gid, len(entry))
		}
		xMin := int16(binary.BigEndian.Uint16(entry[2:])) //nolint:gosec // FWORD
		if lsb != xMin {
			t.Errorf("glyph %d: hmtx left side bearing %d, glyf xMin %d — a rasteriser shifts this glyph by %d units",
				gid, lsb, xMin, lsb-xMin)
		}
	}
}

// TestTheReportedGlyphHasItsLeftOverhang names the letter the defect
// was reported in and says why it showed there first.
//
// Cyrillic Ј is drawn with its hook reaching left of its own origin —
// a negative left side bearing — and it is one of the few letters in
// the Serbian alphabet that is. A subset that wrote zero for every
// bearing displaced every glyph, but it displaced Ј the most and in the
// opposite direction from its neighbours, which is why "СТАНОЈЕВИЋ" was
// where a person saw it.
//
// Asserting the sign rather than the number: a future Noto Sans release
// may redraw the letter, and this should fail only if it stops
// overhanging or if the two tables stop agreeing.
func TestTheReportedGlyphHasItsLeftOverhang(t *testing.T) {
	tables := sfntTables(t)
	hmtx := tables["hmtx"]
	entries := glyfEntries(t, tables)

	const je = 'Ј' // U+0408 CYRILLIC CAPITAL LETTER JE
	gid, ok := runeToGID[je]
	if !ok {
		t.Fatalf("the subset cannot draw %q (U+%04X)", je, je)
	}
	entry := entries[gid]
	if len(entry) < 10 {
		t.Fatalf("glyph for %q has no outline", je)
	}
	lsb := int16(binary.BigEndian.Uint16(hmtx[int(gid)*4+2:])) //nolint:gosec // FWORD
	xMin := int16(binary.BigEndian.Uint16(entry[2:]))          //nolint:gosec // FWORD

	if xMin >= 0 {
		t.Errorf("%q has xMin %d; this letter is expected to overhang to the left of its origin", je, xMin)
	}
	if lsb != xMin {
		t.Fatalf("%q: bearing %d, xMin %d — drawn %d units from where it was designed", je, lsb, xMin, lsb-xMin)
	}
}
