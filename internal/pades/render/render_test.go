package render

import (
	"bytes"
	"fmt"
	"image"
	"strings"
	"testing"
)

// testPage is one page of a synthetic document: its box, its rotation
// and its content stream.
//
// Synthetic rather than real, for the reason D-021 and D-038 already
// give: a real signed PDF carries a real person's name and national
// identifier, and this project does not commit those. The real
// fixtures in testdata/pdfs/local are what the manual pass uses; these
// are what CI can run.
type testPage struct {
	box     [4]float64
	rotate  int
	content string
	extra   string // extra entries for the page dictionary
	objects []string
}

// buildTestPDF writes a complete, classic-xref PDF with the given
// pages. Every offset is computed rather than typed, so the fixture
// cannot drift out of agreement with itself.
func buildTestPDF(pages []testPage) []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) int {
		offsets = append(offsets, b.Len())
		n := len(offsets)
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
		return n
	}
	b.WriteString("%PDF-1.7\n")

	// 1 catalog, 2 pages node, 3 the standard font every fixture uses.
	obj("<</Type /Catalog /Pages 2 0 R>>")
	pagesObj := obj("placeholder")
	fontObj := obj("<</Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding>>")

	var kids []string
	for _, p := range pages {
		for _, extra := range p.objects {
			obj(extra)
		}
		contentObj := obj(fmt.Sprintf("<</Length %d>>\nstream\n%s\nendstream", len(p.content), p.content))
		pageNum := obj(fmt.Sprintf(
			"<</Type /Page /Parent 2 0 R /MediaBox [%g %g %g %g] /Rotate %d /Resources <</Font <</F1 %d 0 R>>>> /Contents %d 0 R%s>>",
			p.box[0], p.box[1], p.box[2], p.box[3], p.rotate, fontObj, contentObj, p.extra))
		kids = append(kids, fmt.Sprintf("%d 0 R", pageNum))
	}

	// Rewriting the pages node in place would move every offset after
	// it, so it is written last, as a shadowing definition the xref
	// points at.
	pagesBody := fmt.Sprintf("<</Type /Pages /Count %d /Kids [%s]>>", len(pages), strings.Join(kids, " "))
	offsets[pagesObj-1] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", pagesObj, pagesBody)

	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<</Size %d /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

// TestPageBoxesAndRotationAreReadPerPage is F6b §2.2's requirement:
// pages of one document may differ in size and orientation, and the
// second page is measured rather than assumed to match the first.
func TestPageBoxesAndRotationAreReadPerPage(t *testing.T) {
	want := []testPage{
		{box: [4]float64{0, 0, 595, 842}, rotate: 0},
		{box: [4]float64{0, 0, 842, 595}, rotate: 0},
		{box: [4]float64{0, 0, 595, 842}, rotate: 90},
		{box: [4]float64{20, 30, 440, 420}, rotate: 270},
	}
	doc, err := Open(buildTestPDF(want))
	if err != nil {
		t.Fatal(err)
	}
	if doc.PageCount() != len(want) {
		t.Fatalf("PageCount = %d, want %d", doc.PageCount(), len(want))
	}
	for i, w := range want {
		got, err := doc.PageInfo(i + 1)
		if err != nil {
			t.Fatal(err)
		}
		if got.Box != w.box {
			t.Errorf("page %d: box %v, want %v", i+1, got.Box, w.box)
		}
		if got.Rotate != w.rotate {
			t.Errorf("page %d: rotate %d, want %d", i+1, got.Rotate, w.rotate)
		}
		wantW, wantH := w.box[2]-w.box[0], w.box[3]-w.box[1]
		if w.rotate == 90 || w.rotate == 270 {
			wantW, wantH = wantH, wantW
		}
		if got.WidthPt != wantW || got.HeightPt != wantH {
			t.Errorf("page %d: displayed size %g x %g, want %g x %g",
				i+1, got.WidthPt, got.HeightPt, wantW, wantH)
		}
	}
}

// TestRenderedImageIsTheDisplayedPageAtTheScale: the image the window
// draws over has to be exactly the size the placement arithmetic thinks
// it is, or the stamp lands somewhere other than where it was dropped.
func TestRenderedImageIsTheDisplayedPageAtTheScale(t *testing.T) {
	pages := []testPage{
		{box: [4]float64{0, 0, 200, 100}, rotate: 0},
		{box: [4]float64{0, 0, 200, 100}, rotate: 90},
		{box: [4]float64{0, 0, 200, 100}, rotate: 180},
		{box: [4]float64{0, 0, 200, 100}, rotate: 270},
	}
	doc, err := Open(buildTestPDF(pages))
	if err != nil {
		t.Fatal(err)
	}
	for i := range pages {
		for _, scale := range []float64{0.5, 1, 2.5} {
			res, err := doc.RenderPage(i+1, scale)
			if err != nil {
				t.Fatal(err)
			}
			wantW := int(res.Page.WidthPt*scale + 0.5)
			wantH := int(res.Page.HeightPt*scale + 0.5)
			if res.Image.Bounds().Dx() != wantW || res.Image.Bounds().Dy() != wantH {
				t.Errorf("page %d at %g: image %v, want %dx%d",
					i+1, scale, res.Image.Bounds(), wantW, wantH)
			}
		}
	}
}

func pixelAt(img *image.RGBA, x, y int) (uint8, uint8, uint8) {
	i := img.PixOffset(x, y)
	return img.Pix[i], img.Pix[i+1], img.Pix[i+2]
}

func pixelIsBlack(img *image.RGBA, x, y int) bool {
	r, g, b := pixelAt(img, x, y)
	return r < 40 && g < 40 && b < 40
}

func pixelIsWhite(img *image.RGBA, x, y int) bool {
	r, g, b := pixelAt(img, x, y)
	return r > 230 && g > 230 && b > 230
}

// TestFilledRectangleLandsWhereTheContentStreamSaid is the rasteriser's
// own correctness check: a rectangle filled at known coordinates covers
// exactly the pixels those coordinates name, with the page's y axis the
// right way up.
func TestFilledRectangleLandsWhereTheContentStreamSaid(t *testing.T) {
	// A 100 x 100 page with a black square from (10,10) to (40,40) in
	// PDF coordinates — which is the *bottom* left, because PDF's y axis
	// increases upwards and the image's does not.
	doc, err := Open(buildTestPDF([]testPage{{
		box:     [4]float64{0, 0, 100, 100},
		content: "0 0 0 rg 10 10 30 30 re f",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	res, err := doc.RenderPage(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	img := res.Image

	// Inside the square: rows 60..89 (100 - 40 .. 100 - 10), columns
	// 10..39.
	for _, p := range [][2]int{{12, 62}, {25, 75}, {38, 88}} {
		if !pixelIsBlack(img, p[0], p[1]) {
			r, g, b := pixelAt(img, p[0], p[1])
			t.Errorf("pixel (%d,%d) is (%d,%d,%d), want black", p[0], p[1], r, g, b)
		}
	}
	// Outside it, including the mirror position a flipped y axis would
	// have put it in.
	for _, p := range [][2]int{{5, 62}, {45, 75}, {25, 55}, {25, 95}, {25, 25}} {
		if !pixelIsWhite(img, p[0], p[1]) {
			r, g, b := pixelAt(img, p[0], p[1])
			t.Errorf("pixel (%d,%d) is (%d,%d,%d), want white", p[0], p[1], r, g, b)
		}
	}
}

// TestEvenOddFillPunchesAHoleAndNonzeroDoesNot: the two fill rules
// differ exactly where a shape overlaps itself, and a renderer that
// implements one of them twice draws a solid blob where a ring belongs.
func TestEvenOddFillPunchesAHoleAndNonzeroDoesNot(t *testing.T) {
	// Two nested rectangles wound the same way. Even-odd leaves the
	// middle empty; nonzero fills it.
	both := "0 0 0 rg 10 10 80 80 re 30 30 40 40 re "
	for _, tc := range []struct {
		op       string
		wantHole bool
	}{
		{op: "f*", wantHole: true},
		{op: "f", wantHole: false},
	} {
		doc, err := Open(buildTestPDF([]testPage{{
			box:     [4]float64{0, 0, 100, 100},
			content: both + tc.op,
		}}))
		if err != nil {
			t.Fatal(err)
		}
		res, err := doc.RenderPage(1, 1)
		if err != nil {
			t.Fatal(err)
		}
		centreWhite := pixelIsWhite(res.Image, 50, 50)
		if centreWhite != tc.wantHole {
			t.Errorf("%q: centre white = %v, want %v", tc.op, centreWhite, tc.wantHole)
		}
		if !pixelIsBlack(res.Image, 20, 50) {
			t.Errorf("%q: the ring itself is not filled", tc.op)
		}
	}
}

// TestStrokeIsContinuousThroughItsJoins: a stroked path is drawn as
// overlapping pieces unioned under the nonzero rule, and pieces wound
// opposite ways would cancel and leave a gap down the middle of the
// line. This is that gap, checked for.
func TestStrokeIsContinuousThroughItsJoins(t *testing.T) {
	doc, err := Open(buildTestPDF([]testPage{{
		box:     [4]float64{0, 0, 100, 100},
		content: "0 0 0 RG 6 w 20 20 m 80 20 l 80 80 l S",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	res, err := doc.RenderPage(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Along the horizontal run (y = 20 in PDF is row 80), and through
	// the join at (80, 20).
	for _, p := range [][2]int{{30, 80}, {50, 80}, {70, 80}, {80, 80}, {80, 50}, {80, 30}} {
		if !pixelIsBlack(res.Image, p[0], p[1]) {
			r, g, b := pixelAt(res.Image, p[0], p[1])
			t.Errorf("pixel (%d,%d) is (%d,%d,%d), want the stroke", p[0], p[1], r, g, b)
		}
	}
	if !pixelIsWhite(res.Image, 50, 50) {
		t.Error("the inside of the corner was filled: a stroke is not a fill")
	}
}

// TestTextIsDrawnWhereTheDocumentPutIt: the substitute font's shapes
// are not the document's, but the position is — so ink has to appear in
// the band the text matrix names, and nowhere else.
func TestTextIsDrawnWhereTheDocumentPutIt(t *testing.T) {
	doc, err := Open(buildTestPDF([]testPage{{
		box:     [4]float64{0, 0, 200, 100},
		content: "BT /F1 24 Tf 20 40 Td (Hello) Tj ET",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	res, err := doc.RenderPage(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	img := res.Image

	inkRows := map[int]bool{}
	inkCols := map[int]bool{}
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if !pixelIsWhite(res.Image, x, y) {
				inkRows[y] = true
				inkCols[x] = true
			}
		}
	}
	if len(inkRows) == 0 {
		t.Fatal("no text was drawn at all")
	}
	minRow, maxRow := 1<<30, -1
	for r := range inkRows {
		if r < minRow {
			minRow = r
		}
		if r > maxRow {
			maxRow = r
		}
	}
	minCol, maxCol := 1<<30, -1
	for c := range inkCols {
		if c < minCol {
			minCol = c
		}
		if c > maxCol {
			maxCol = c
		}
	}
	// The baseline is at y = 40 in PDF, which is row 60. Capitals of a
	// 24pt face reach roughly 17 points above it and descenders a few
	// below, so the ink belongs between rows 40 and 66 — not a tight
	// box, but tight enough that a flipped axis or a lost text matrix
	// fails it.
	if minRow < 38 || maxRow > 66 {
		t.Errorf("text ink spans rows %d..%d, want it around the baseline at row 60", minRow, maxRow)
	}
	if minCol < 18 || minCol > 30 {
		t.Errorf("text starts at column %d, want it at the 20-point left edge", minCol)
	}
	if maxCol > 100 {
		t.Errorf("text runs to column %d, which is wider than five 24pt letters", maxCol)
	}
}

// TestAnnotationAppearanceIsDrawn: a document that already carries a
// signature shows that signature's stamp, which is what stops someone
// placing a second one on top of it.
func TestAnnotationAppearanceIsDrawn(t *testing.T) {
	// A form XObject that fills its own bounding box, referenced by a
	// widget annotation over the top-right quarter of the page.
	form := "<</Type /XObject /Subtype /Form /BBox [0 0 10 10] /Length 26>>\nstream\n0 0 0 rg 0 0 10 10 re f\nendstream"
	annot := "<</Type /Annot /Subtype /Widget /Rect [50 50 100 100] /F 4 /AP <</N 4 0 R>>>>"
	doc, err := Open(buildTestPDF([]testPage{{
		box:     [4]float64{0, 0, 100, 100},
		objects: []string{form, annot},
		extra:   " /Annots [5 0 R]",
	}}))
	if err != nil {
		t.Fatal(err)
	}
	res, err := doc.RenderPage(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	// PDF (50..100, 50..100) is the image's top-right quarter.
	if !pixelIsBlack(res.Image, 75, 25) {
		r, g, b := pixelAt(res.Image, 75, 25)
		t.Errorf("the annotation's appearance was not drawn: pixel is (%d,%d,%d)", r, g, b)
	}
	if !pixelIsWhite(res.Image, 25, 75) {
		t.Error("the annotation was drawn outside its own rectangle")
	}
}

// TestEncryptedDocumentIsRefused: F6b §5 — the preview refuses what the
// signing path refuses, with the same code, so the window can fall back
// to the corner selector rather than showing half a page.
func TestEncryptedDocumentIsRefused(t *testing.T) {
	src := buildTestPDF([]testPage{{box: [4]float64{0, 0, 100, 100}}})
	encrypted := bytes.Replace(src, []byte("<</Size "), []byte("<</Encrypt 9 0 R /Size "), 1)
	if _, err := Open(encrypted); err == nil {
		t.Fatal("an encrypted document was opened for previewing")
	}
}

// TestOversizedPageIsRefusedRatherThanAllocated: a document declaring a
// page the size of a football pitch must not be able to ask this
// process for several gigabytes of image.
func TestOversizedPageIsRefusedRatherThanAllocated(t *testing.T) {
	doc, err := Open(buildTestPDF([]testPage{{box: [4]float64{0, 0, 20000, 20000}}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := doc.RenderPage(1, 4); err == nil {
		t.Fatal("a 6.4-gigapixel render was accepted")
	}
	if _, err := doc.RenderPage(1, 0.05); err != nil {
		t.Fatalf("the same page at a sane scale was refused: %v", err)
	}
}
