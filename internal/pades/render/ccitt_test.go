package render

import (
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// TestCCITTGroup4MatchesAnIndependentEncoder decodes a Group 4 image
// produced elsewhere and compares every pixel against the bitmap that
// encoder was given. It is the only check that says the code tables in
// ccitt.go are the tables in T.4 and T.6 rather than something that
// merely decodes this decoder's own output.
func TestCCITTGroup4MatchesAnIndependentEncoder(t *testing.T) {
	doc := &pdf.Document{}
	parms := pdf.Dict{
		pdf.Name("K"):       int64(-1),
		pdf.Name("Columns"): int64(ccittFixtureWidth),
		pdf.Name("Rows"):    int64(ccittFixtureHeight),
	}
	got, err := ccittDecode(ccittGroup4Encoded, doc, parms, ccittFixtureWidth, ccittFixtureHeight)
	if err != nil {
		t.Fatalf("ccittDecode: %v", err)
	}
	if len(got) != len(ccittGroup4Expected) {
		t.Fatalf("decoded %d bytes, want %d", len(got), len(ccittGroup4Expected))
	}
	rowBytes := (ccittFixtureWidth + 7) / 8
	for row := 0; row < ccittFixtureHeight; row++ {
		for x := 0; x < ccittFixtureWidth; x++ {
			i := row*rowBytes + x/8
			mask := byte(1) << uint(7-x%8)
			gotBit := got[i]&mask != 0
			wantBit := ccittGroup4Expected[i]&mask != 0
			if gotBit != wantBit {
				t.Fatalf("pixel (%d,%d): got white=%v, want white=%v", x, row, gotBit, wantBit)
			}
		}
	}
}

// TestCCITTGroup3OneDimensionalMatchesTheSameImage decodes the same
// picture encoded without the two-dimensional modes, so the run-length
// tables are exercised on their own.
func TestCCITTGroup3OneDimensionalMatchesTheSameImage(t *testing.T) {
	doc := &pdf.Document{}
	parms := pdf.Dict{
		pdf.Name("K"):       int64(0),
		pdf.Name("Columns"): int64(ccittFixtureWidth),
	}
	got, err := ccittDecode(ccittGroup3Encoded, doc, parms, ccittFixtureWidth, ccittFixtureHeight)
	if err != nil {
		t.Fatalf("ccittDecode: %v", err)
	}
	comparePixels(t, got, ccittGroup4Expected)
}

// TestCCITTBlackIs1InvertsTheOutput: /BlackIs1 says which bit value the
// filter emits for black, so setting it must invert every byte and
// nothing else.
func TestCCITTBlackIs1InvertsTheOutput(t *testing.T) {
	doc := &pdf.Document{}
	base := pdf.Dict{pdf.Name("K"): int64(-1), pdf.Name("Columns"): int64(ccittFixtureWidth)}
	plain, err := ccittDecode(ccittGroup4Encoded, doc, base, ccittFixtureWidth, ccittFixtureHeight)
	if err != nil {
		t.Fatalf("ccittDecode: %v", err)
	}
	base[pdf.Name("BlackIs1")] = true
	inverted, err := ccittDecode(ccittGroup4Encoded, doc, base, ccittFixtureWidth, ccittFixtureHeight)
	if err != nil {
		t.Fatalf("ccittDecode with BlackIs1: %v", err)
	}
	if len(plain) != len(inverted) {
		t.Fatalf("lengths differ: %d and %d", len(plain), len(inverted))
	}
	for i := range plain {
		if plain[i] != ^inverted[i] {
			t.Fatalf("byte %d: %08b and %08b are not complements", i, plain[i], inverted[i])
		}
	}
}

func comparePixels(t *testing.T, got, want []byte) {
	t.Helper()
	rowBytes := (ccittFixtureWidth + 7) / 8
	for row := 0; row < ccittFixtureHeight; row++ {
		for x := 0; x < ccittFixtureWidth; x++ {
			i := row*rowBytes + x/8
			if i >= len(got) || i >= len(want) {
				t.Fatalf("pixel (%d,%d): past the end of the decoded data", x, row)
			}
			mask := byte(1) << uint(7-x%8)
			if (got[i]&mask != 0) != (want[i]&mask != 0) {
				t.Fatalf("pixel (%d,%d): got white=%v, want white=%v", x, row, got[i]&mask != 0, want[i]&mask != 0)
			}
		}
	}
}
