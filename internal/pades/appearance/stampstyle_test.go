package appearance

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// textColumnWidth is the width one line of stamp text has to fit in:
// the fixed 190 points, less the logo and the three paddings around it
// (SPEC §13.1). Written as the arithmetic rather than as a number so
// that it cannot drift from the constants it comes from.
const textColumnWidth = StampWidth - (Padding + LogoSize + Padding) - Padding

// mupSerialHex is the real serial of the MUP signing certificate this
// change was verified against, upper-cased as buildAppearanceOptions
// upper-cases it — eighteen hexadecimal digits, the longest of the two
// issuers this project has certificates from (Halcom's is eight).
const mupSerialHex = "20F048A768F56F099E"

// TestGroupSerialGroupsInFoursFromTheLeft pins the shape D-209 chose,
// including the case that decides which end the short group falls on: a
// serial whose length is not a multiple of four.
func TestGroupSerialGroupsInFoursFromTheLeft(t *testing.T) {
	tests := []struct{ in, want string }{
		{mupSerialHex, "20F0 48A7 68F5 6F09 9E"},
		{"11E4DCC7", "11E4 DCC7"},
		{"ABCD", "ABCD"},
		{"ABC", "ABC"},
		{"", ""},
		{"ABCDE", "ABCD E"},
	}
	for _, tt := range tests {
		if got := groupSerial(tt.in); got != tt.want {
			t.Errorf("groupSerial(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestGroupedSerialFitsTheTextColumnAtNominalSize is the measurement
// D-209 promised: grouping makes the SN line longer, not shorter — 25
// characters against 21 — and the stamp's width is fixed, so the line
// has to be measured rather than assumed to fit.
//
// It is measured through fitLine, not by arithmetic beside it, so that
// what is asserted is what will actually be drawn: the real serial, at
// the nominal size, untruncated, inside the column.
func TestGroupedSerialFitsTheTextColumnAtNominalSize(t *testing.T) {
	line := serialPrefix + groupSerial(mupSerialHex)

	fitted, size, err := fitLine(line, textColumnWidth, Regular)
	if err != nil {
		t.Fatal(err)
	}
	if fitted != line {
		t.Fatalf("fitLine shortened the SN line to %q; the real MUP serial must fit whole", fitted)
	}
	if size != nominalFontSize {
		t.Fatalf("the SN line was reduced to %g pt; it must fit at the nominal %g pt", size, nominalFontSize)
	}
	w1000, err := TextWidth1000(fitted, Regular)
	if err != nil {
		t.Fatal(err)
	}
	width := float64(w1000) * size / 1000
	if width > textColumnWidth {
		t.Fatalf("the SN line is %.2f pt wide, the text column is %g pt", width, float64(textColumnWidth))
	}
	t.Logf("%q: %.2f pt at %g pt type, in a %g pt column of a %d pt stamp",
		fitted, width, size, float64(textColumnWidth), StampWidth)
}

// TestSignerNameIsTheOnlyLineDrawnBold is the whole point of the second
// embedded face: one line carries the weight, and it is the line a
// person looks at the stamp to find. A label or a date creeping into
// bold would spend the same 13.5 KB per document on emphasis that
// emphasises nothing.
func TestSignerNameIsTheOnlyLineDrawnBold(t *testing.T) {
	opts := withDocumentID(withReference(baseOptions(), "REF-1"), "998877")
	lines, err := buildLines(opts)
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range lines {
		want := Regular
		if i == 1 {
			want = Bold
		}
		if line.weight != want {
			t.Errorf("line %d (%q) is drawn in weight %d, want %d", i, line.text, line.weight, want)
		}
	}
	if got := lines[1].text; got != strings.ToUpper(opts.SignerName) {
		t.Errorf("the bold line is %q, want the signer's name %q", got, strings.ToUpper(opts.SignerName))
	}
}

// TestContentStreamDrawsTheNameInTheBoldFaceAndNothingElse checks the
// bytes that reach the document rather than the intent behind them: the
// bold resource name appears exactly once, and it is on the line
// carrying the signer's name.
func TestContentStreamDrawsTheNameInTheBoldFaceAndNothingElse(t *testing.T) {
	opts := baseOptions()
	lines, err := buildLines(opts)
	if err != nil {
		t.Fatal(err)
	}
	height, err := HeightForLines(len(lines))
	if err != nil {
		t.Fatal(err)
	}
	content, err := buildContentStream(lines, height)
	if err != nil {
		t.Fatal(err)
	}

	if n := strings.Count(string(content), fontResourceName(Bold)+" "); n != 1 {
		t.Fatalf("the bold face is selected %d times in the content stream, want 1", n)
	}
	nameCIDs, err := EncodeCIDs(strings.ToUpper(opts.SignerName))
	if err != nil {
		t.Fatal(err)
	}
	want := fontResourceName(Bold)
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.Contains(line, cidsToHex(nameCIDs)) {
			continue
		}
		if !strings.HasPrefix(line, want) {
			t.Fatalf("the signer's name is drawn by %q, which does not start with %q", line, want)
		}
		return
	}
	t.Fatal("the signer's name does not appear in the content stream at all")
}

// TestStampInkIsTheDesignTokensTextPrimary is why the stamp's text
// colour is a token rather than a preference: the number lives in one
// place, and this test is what makes the copy in this package a copy
// rather than a second opinion. The logo's #038387 is held to the same
// standard against the logo asset (logo_test.go).
//
// Reading internal/ui/assets/tokens.css directly, rather than importing
// anything, is deliberate: internal/pades must not depend on the window
// layer (scripts/checkdeps), and a test file is the one place the two
// can be compared without creating that dependency.
func TestStampInkIsTheDesignTokensTextPrimary(t *testing.T) {
	css, err := os.ReadFile("../../ui/assets/tokens.css")
	if err != nil {
		t.Fatalf("reading the design tokens: %v", err)
	}
	m := regexp.MustCompile(`--liro-color-text-primary:\s*#([0-9a-fA-F]{6});`).FindSubmatch(css)
	if m == nil {
		t.Fatal("tokens.css has no --liro-color-text-primary")
	}
	hex := string(m[1])

	got := fmt.Sprintf("%02x%02x%02x",
		componentByte(t, stampInkR), componentByte(t, stampInkG), componentByte(t, stampInkB))
	if !strings.EqualFold(got, hex) {
		t.Fatalf("the stamp's ink is #%s, --liro-color-text-primary is #%s", got, hex)
	}
}

// componentByte turns a PDF DeviceRGB component back into the 0..255
// value the token was written as.
func componentByte(t *testing.T, v float64) int {
	t.Helper()
	n, err := strconv.Atoi(strconv.FormatFloat(v*255, 'f', 0, 64))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestContentStreamSetsTheInkOnceBeforeAnyText is the other half of the
// colour change: the fill colour is set, and it is set outside the text
// object, so it governs every line rather than the first.
//
// Before this the stamp set no colour at all and drew in whatever the
// graphics state held — black, in every viewer anyone had tried, but
// black by default rather than by decision.
func TestContentStreamSetsTheInkOnceBeforeAnyText(t *testing.T) {
	lines, err := buildLines(baseOptions())
	if err != nil {
		t.Fatal(err)
	}
	height, err := HeightForLines(len(lines))
	if err != nil {
		t.Fatal(err)
	}
	content := string(mustContentStream(t, lines, height))

	rg := regexp.MustCompile(`(?m)^([0-9.]+) ([0-9.]+) ([0-9.]+) rg$`)
	all := rg.FindAllStringSubmatch(content, -1)
	if len(all) != 1 {
		t.Fatalf("the content stream sets a fill colour %d times, want exactly 1", len(all))
	}
	for i, want := range []float64{stampInkR, stampInkG, stampInkB} {
		got, err := strconv.ParseFloat(all[0][i+1], 64)
		if err != nil {
			t.Fatal(err)
		}
		if diff := got - want; diff > 0.0001 || diff < -0.0001 {
			t.Errorf("fill colour component %d is %g, want %g", i, got, want)
		}
	}
	if strings.Index(content, all[0][0]) > strings.Index(content, "BT") {
		t.Error("the fill colour is set inside the text object, not before it")
	}
}

func mustContentStream(t *testing.T, lines []stampLine, height float64) []byte {
	t.Helper()
	content, err := buildContentStream(lines, height)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
