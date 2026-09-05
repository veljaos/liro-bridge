package appearance

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

func TestFitLineUsesNominalSizeWhenItFits(t *testing.T) {
	text, size, err := fitLine("short", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if text != "short" || size != nominalFontSize {
		t.Fatalf("fitLine = (%q, %g), want (%q, %g)", text, size, "short", nominalFontSize)
	}
}

func TestFitLineReducesOneStepBeforeTruncating(t *testing.T) {
	// A width that the nominal size overflows but the reduced size just
	// fits (F4 §5.4: "reduce the font size in one step" comes before
	// truncation, not instead of trying it).
	w1000, err := TextWidth1000("Redžvel Mešković")
	if err != nil {
		t.Fatal(err)
	}
	nominalWidth := float64(w1000) * nominalFontSize / 1000
	reducedWidth := float64(w1000) * reducedFontSize / 1000
	maxWidth := (nominalWidth + reducedWidth) / 2 // strictly between the two

	text, size, err := fitLine("Redžvel Mešković", maxWidth)
	if err != nil {
		t.Fatal(err)
	}
	if text != "Redžvel Mešković" {
		t.Fatalf("fitLine truncated %q when the reduced size alone should have fit", text)
	}
	if size != reducedFontSize {
		t.Fatalf("fitLine size = %g, want reducedFontSize %g", size, reducedFontSize)
	}
}

// TestFitLineTruncatesAndNeverOverflows is F4 §5.4/§8: "Long name is
// reduced then truncated, never overflowing." A name many times wider
// than the stamp must come back shorter than the original, marked with
// the truncation mark, and fit within maxWidth at the reduced size.
func TestFitLineTruncatesAndNeverOverflows(t *testing.T) {
	longName := strings.Repeat("Aleksandar Nikolić-Petrović ", 10)
	const maxWidth = 138.0 // this project's own text-column width (StampWidth - logo - padding)

	text, size, err := fitLine(longName, maxWidth)
	if err != nil {
		t.Fatal(err)
	}
	if size != reducedFontSize {
		t.Fatalf("fitLine size = %g, want reducedFontSize %g for an extreme overflow", size, reducedFontSize)
	}
	if !strings.HasSuffix(text, truncationMark) {
		t.Fatalf("fitLine result %q does not end with the truncation mark %q", text, truncationMark)
	}
	if text == longName {
		t.Fatal("fitLine did not actually shorten a name many times wider than the stamp")
	}
	w1000, err := TextWidth1000(text)
	if err != nil {
		t.Fatal(err)
	}
	gotWidth := float64(w1000) * size / 1000
	if gotWidth > maxWidth+0.01 {
		t.Fatalf("truncated text still overflows: width %g > maxWidth %g", gotWidth, maxWidth)
	}
}

func TestFitLineNeverWidensBeyondMaxWidth(t *testing.T) {
	// F4 §5.4: "Do not widen the stamp." No return value from fitLine,
	// at either size, may exceed maxWidth.
	texts := []string{"x", "Aleksandar Nikolić-Petrović", strings.Repeat("Ж", 40)}
	for _, in := range texts {
		for _, maxWidth := range []float64{10, 50, 138, 500} {
			text, size, err := fitLine(in, maxWidth)
			if err != nil {
				continue // a maxWidth too small even for one ellipsis is not this test's concern
			}
			w1000, err := TextWidth1000(text)
			if err != nil {
				t.Fatal(err)
			}
			gotWidth := float64(w1000) * size / 1000
			if gotWidth > maxWidth+0.01 {
				t.Errorf("fitLine(%q, maxWidth=%g) = %q at size %g: width %g exceeds maxWidth", in, maxWidth, text, size, gotWidth)
			}
		}
	}
}

// buildOnePagePDF returns a minimal, valid single-page PDF for Render's
// tests — the same shape internal/pades' own tests use, kept local
// here so this package's tests do not depend on internal/pades.
func buildOnePagePDF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := map[int]int{}
	write := func(num int, body string) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}
	write(1, "<< /Type /Catalog /Pages 2 0 R >>")
	write(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	write(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>")
	content := "BT /F1 12 Tf 72 712 Td (Hello) Tj ET"
	offsets[4] = buf.Len()
	fmt.Fprintf(&buf, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&buf, "%010d %05d n \n", offsets[i], 0)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xrefStart)
	return buf.Bytes()
}

// buildRotatedOnePagePDF is buildOnePagePDF with /Rotate rotate set on
// the page, for TestRenderOnRotatedPageStaysUprightInsidePage.
func buildRotatedOnePagePDF(t *testing.T, rotate int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := map[int]int{}
	write := func(num int, body string) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}
	write(1, "<< /Type /Catalog /Pages 2 0 R >>")
	write(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	write(3, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Rotate %d /Resources << >> /Contents 4 0 R >>", rotate))
	content := "BT /F1 12 Tf 72 712 Td (Hello) Tj ET"
	offsets[4] = buf.Len()
	fmt.Fprintf(&buf, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&buf, "%010d %05d n \n", offsets[i], 0)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xrefStart)
	return buf.Bytes()
}

// TestRenderOnRotatedPageStaysUprightInsidePage is F4 §8: "A rotated
// page places the stamp visibly correctly," exercised through the full
// Render pipeline (MediaBox/Rotate resolution included, not just
// geometry.go's pure PlaceCorner math, which geometry_test.go already
// covers directly) for every /Rotate value F4 §2.1 requires supporting.
func TestRenderOnRotatedPageStaysUprightInsidePage(t *testing.T) {
	for _, rotate := range []int{0, 90, 180, 270} {
		doc, err := pdf.Parse(buildRotatedOnePagePDF(t, rotate))
		if err != nil {
			t.Fatalf("rotate=%d: pdf.Parse: %v", rotate, err)
		}
		pageDict, ok := doc.ResolveDict(pdf.Reference{Num: 3})
		if !ok {
			t.Fatalf("rotate=%d: page does not resolve", rotate)
		}
		u := pdf.NewUpdate(doc)
		opts := baseOptions()
		opts.Corner = BottomRight

		result, err := Render(doc, pageDict, u, opts)
		if err != nil {
			t.Fatalf("rotate=%d: Render: %v", rotate, err)
		}
		box := pdf.ResolveMediaBox(doc, pageDict)
		r := result.Rect
		if r[0] < box[0] || r[1] < box[1] || r[2] > box[2] || r[3] > box[3] {
			t.Errorf("rotate=%d: Rect %v lands outside page box %v", rotate, r, box)
		}
		if _, err := u.Apply(); err != nil {
			t.Fatalf("rotate=%d: u.Apply: %v", rotate, err)
		}
	}
}

func mustParsePageOne(t *testing.T) (*pdf.Document, pdf.Dict) {
	t.Helper()
	doc, err := pdf.Parse(buildOnePagePDF(t))
	if err != nil {
		t.Fatalf("pdf.Parse: %v", err)
	}
	pageDict, ok := doc.ResolveDict(pdf.Reference{Num: 3})
	if !ok {
		t.Fatal("page 3 does not resolve")
	}
	return doc, pageDict
}

func baseOptions() Options {
	return Options{
		Label:       "Digitally signed",
		SignerName:  "Test Signer",
		SerialHex:   "DEADBEEF",
		SigningTime: "01.09.2026. 12:00:00",
	}
}

// TestRenderExplicitXYLandsWhereSpecified is F4 §8: "Explicit
// coordinates land where specified."
func TestRenderExplicitXYLandsWhereSpecified(t *testing.T) {
	doc, pageDict := mustParsePageOne(t)
	u := pdf.NewUpdate(doc)

	opts := baseOptions()
	opts.UseXY, opts.X, opts.Y = true, 123, 456

	appearanceResult, err := Render(doc, pageDict, u, opts)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	height, _ := HeightForLines(4) // label, name, serial, date
	want := [4]float64{123, 456, 123 + StampWidth, 456 + height}
	if appearanceResult.Rect != want {
		t.Fatalf("Rect = %v, want %v", appearanceResult.Rect, want)
	}
}

// TestRenderCornerPlacementStaysInsidePage exercises Render end to end
// (not just the pure geometry.go math) for every corner.
func TestRenderCornerPlacementStaysInsidePage(t *testing.T) {
	for _, corner := range []Corner{BottomRight, BottomLeft, TopRight, TopLeft} {
		doc, pageDict := mustParsePageOne(t)
		u := pdf.NewUpdate(doc)
		opts := baseOptions()
		opts.Corner = corner

		result, err := Render(doc, pageDict, u, opts)
		if err != nil {
			t.Fatalf("corner %d: Render: %v", corner, err)
		}
		box := pdf.ResolveMediaBox(doc, pageDict)
		r := result.Rect
		if r[0] < box[0] || r[1] < box[1] || r[2] > box[2] || r[3] > box[3] {
			t.Errorf("corner %d: Rect %v lands outside page box %v", corner, r, box)
		}
	}
}

// TestLogoXObjectDeclaresRealPixelDimensions is Task 2's confirmation
// that the rendered stamp's logo image XObject (and its /SMask) declare
// the real asset's actual pixel size (256×256, LogoImagePixels) — not
// the 36×36 placement size (LogoSize) the two constants shared before
// the real Liro logo replaced the placeholder mark (see
// docs/decisions.md). TestLogoAssetIs256x256WithAlphaApplied in
// logo_test.go checks the underlying pixel data directly; this test
// checks what Render actually writes into the PDF.
func TestLogoXObjectDeclaresRealPixelDimensions(t *testing.T) {
	doc, pageDict := mustParsePageOne(t)
	u := pdf.NewUpdate(doc)
	opts := baseOptions()

	if _, err := Render(doc, pageDict, u, opts); err != nil {
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

	var imageDicts int
	for num := 1; num <= 100; num++ {
		stream, ok := doc2.Get(num).(*pdf.Stream)
		if !ok || stream.Dict.Get(pdf.Name("Subtype")) != pdf.Name("Image") {
			continue
		}
		imageDicts++
		w, _ := stream.Dict.Get(pdf.Name("Width")).(int64)
		h, _ := stream.Dict.Get(pdf.Name("Height")).(int64)
		if w != LogoImagePixels || h != LogoImagePixels {
			t.Errorf("image object %d: /Width %v /Height %v, want %d/%d", num, w, h, LogoImagePixels, LogoImagePixels)
		}
	}
	if imageDicts != 2 {
		t.Fatalf("found %d /Image XObjects, want 2 (RGB + alpha /SMask)", imageDicts)
	}
}

// TestRenderRejectsMissingGlyphInReference proves the missing-glyph
// failure actually reaches Render's caller (F4 §3.3), not just
// EncodeCIDs in isolation.
func TestRenderRejectsMissingGlyphInReference(t *testing.T) {
	doc, pageDict := mustParsePageOne(t)
	u := pdf.NewUpdate(doc)
	opts := baseOptions()
	opts.Reference = "INV-中-0042"

	_, err := Render(doc, pageDict, u, opts)
	if err == nil {
		t.Fatal("Render: want an error for a reference containing an unsupported character, got none")
	}
	var mg *MissingGlyphError
	if !errors.As(err, &mg) {
		t.Fatalf("Render error = %v (%T), want *MissingGlyphError", err, err)
	}
}

func TestBuildLinesCountsMatchOptionalFields(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want int
	}{
		{"base", baseOptions(), 4},
		{"with reference", withReference(baseOptions(), "REF-1"), 5},
		{"with document id", withDocumentID(baseOptions(), "998877"), 5},
		{"with both", withDocumentID(withReference(baseOptions(), "REF-1"), "998877"), 6},
	}
	for _, tt := range tests {
		lines, err := buildLines(tt.opts)
		if err != nil {
			t.Fatalf("%s: buildLines: %v", tt.name, err)
		}
		if len(lines) != tt.want {
			t.Errorf("%s: len(lines) = %d, want %d (%v)", tt.name, len(lines), tt.want, lines)
		}
	}
}

func withReference(o Options, ref string) Options { o.Reference = ref; return o }
func withDocumentID(o Options, id string) Options { o.DocumentID = id; return o }
