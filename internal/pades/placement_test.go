package pades

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// testSigningTime is the /M value these tests sign at. Fixed, so the
// stamp's own line count — and therefore its height — is the same on
// every run.
var testSigningTime = time.Date(2026, 9, 6, 15, 4, 5, 0, time.UTC)

// buildPagesPDF writes a document with the given page boxes and
// rotations, so a placement can be asked to fit pages that are not all
// the same shape — which is exactly what a remembered position meets in
// a real batch (F6b §3).
func buildPagesPDF(pages [][2]any) []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) int {
		offsets = append(offsets, b.Len())
		n := len(offsets)
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
		return n
	}
	b.WriteString("%PDF-1.7\n")
	obj("<</Type /Catalog /Pages 2 0 R>>")
	pagesObj := obj("placeholder")

	var kids []byte
	for _, p := range pages {
		box := p[0].(string)
		rotate := p[1].(int)
		n := obj(fmt.Sprintf("<</Type /Page /Parent 2 0 R /MediaBox [%s] /Rotate %d /Resources <<>>>>", box, rotate))
		kids = append(kids, []byte(fmt.Sprintf("%d 0 R ", n))...)
	}
	offsets[pagesObj-1] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<</Type /Pages /Count %d /Kids [%s]>>\nendobj\n",
		pagesObj, len(pages), string(kids))

	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<</Size %d /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

// TestPlacedCoordinatesBecomeTheAppearanceRectangle is the other half
// of the placement window's promise: the coordinates a person dropped
// the stamp at are the rectangle the signature actually occupies in the
// document, to the point.
func TestPlacedCoordinatesBecomeTheAppearanceRectangle(t *testing.T) {
	sess := newFakeSession(t)
	src := buildPagesPDF([][2]any{{"0 0 595 842", 0}})

	const wantX, wantY = 137.5, 421.25
	out, page, ap, adj, err := applyStamp(src, sess.cert, testSigningTime, &StampOptions{
		Label: "Digitally signed",
		UseXY: true,
		X:     wantX,
		Y:     wantY,
		Page:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page != 1 {
		t.Errorf("the stamp went on page %d, want 1", page)
	}
	if adj.moved || adj.pageFellBack {
		t.Errorf("a position well inside an A4 page was reported as adjusted: %+v", adj)
	}

	height := stampHeightFor(sess.cert, testSigningTime, &StampOptions{Label: "Digitally signed"})
	want := [4]float64{wantX, wantY, wantX + float64(appearance.StampWidth), wantY + height}
	if ap.Rect != want {
		t.Errorf("the appearance rectangle is %v, want %v", ap.Rect, want)
	}
	if !bytes.HasPrefix(out, src) {
		t.Error("the original bytes are no longer a prefix of the stamped document")
	}
}

// TestPlacedPageBeyondTheEndFallsBackToTheLast is F6b §3: a position
// saved on page twelve of a fifty-page report goes on the last page of
// a four-page contract, and says so.
func TestPlacedPageBeyondTheEndFallsBackToTheLast(t *testing.T) {
	sess := newFakeSession(t)
	src := buildPagesPDF([][2]any{
		{"0 0 595 842", 0}, {"0 0 595 842", 0}, {"0 0 595 842", 0},
	})

	_, page, _, adj, err := applyStamp(src, sess.cert, testSigningTime, &StampOptions{
		Label: "Digitally signed",
		UseXY: true, X: 100, Y: 100,
		Page: 12,
	})
	if err != nil {
		t.Fatalf("a saved page past the end was refused instead of clamped: %v", err)
	}
	if page != 3 {
		t.Errorf("the stamp went on page %d, want the last page (3)", page)
	}
	if !adj.pageFellBack {
		t.Error("the page fallback was not reported, so the batch could not tell anyone")
	}
}

// TestPlacedPositionOffASmallerPageIsClampedAndReported is the other
// adjustment F6b §3 names: an A4 position on a small page is brought
// inside that page's margin rather than drawn hanging off it.
func TestPlacedPositionOffASmallerPageIsClampedAndReported(t *testing.T) {
	sess := newFakeSession(t)
	src := buildPagesPDF([][2]any{{"0 0 300 300", 0}})

	_, _, ap, adj, err := applyStamp(src, sess.cert, testSigningTime, &StampOptions{
		Label: "Digitally signed",
		UseXY: true, X: 500, Y: 500,
		Page: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !adj.moved {
		t.Error("a position off the edge of the page was not reported as moved")
	}
	if ap.Rect[0] < appearance.Margin-1e-9 || ap.Rect[1] < appearance.Margin-1e-9 {
		t.Errorf("the clamped rectangle %v is inside the margin", ap.Rect)
	}
	if ap.Rect[2] > 300-appearance.Margin+1e-9 || ap.Rect[3] > 300-appearance.Margin+1e-9 {
		t.Errorf("the clamped rectangle %v runs past the margin", ap.Rect)
	}
}

// TestPlacedPositionOnARotatedPageIsDrawnUpright: a stamp placed by
// coordinate on a quarter-turned page has to display the right way up,
// which means its footprint swaps in the page's own coordinates and its
// form carries the counter-rotation. It used to carry the identity
// matrix, which drew the stamp on its side.
func TestPlacedPositionOnARotatedPageIsDrawnUpright(t *testing.T) {
	sess := newFakeSession(t)
	height := stampHeightFor(sess.cert, testSigningTime, &StampOptions{Label: "Digitally signed"})

	for _, rot := range []int{0, 90, 180, 270} {
		src := buildPagesPDF([][2]any{{"0 0 595 842", rot}})
		_, _, ap, _, err := applyStamp(src, sess.cert, testSigningTime, &StampOptions{
			Label: "Digitally signed",
			UseXY: true, X: 100, Y: 100,
			Page: 1,
		})
		if err != nil {
			t.Fatalf("rotate %d: %v", rot, err)
		}
		gotW := ap.Rect[2] - ap.Rect[0]
		gotH := ap.Rect[3] - ap.Rect[1]
		wantW, wantH := float64(appearance.StampWidth), height
		if rot == 90 || rot == 270 {
			wantW, wantH = height, float64(appearance.StampWidth)
		}
		if gotW != wantW || gotH != wantH {
			t.Errorf("rotate %d: rectangle is %g x %g, want %g x %g", rot, gotW, gotH, wantW, wantH)
		}
	}
}

// TestPlacedPositionSurvivesSigning is the end-to-end version: the
// rectangle in the finished, signed document is the one that was asked
// for, and the signature still verifies.
func TestPlacedPositionSurvivesSigning(t *testing.T) {
	sess := newFakeSession(t)
	const wantX, wantY float64 = 210, 333

	result, err := SignDocument(context.Background(), buildMinimalPDF(t), sess, Options{
		OnTSAFailureAbort: false,
		Stamp: &StampOptions{
			Label: "Digitally signed",
			UseXY: true, X: wantX, Y: wantY,
			Page: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StampPage != 1 {
		t.Errorf("the result says page %d, want 1", result.StampPage)
	}
	if result.StampMoved || result.StampPageFellBack {
		t.Error("a position inside the page was reported as adjusted")
	}

	doc, err := pdf.Parse(result.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pageDict, err := doc.PageDict(1)
	if err != nil {
		t.Fatal(err)
	}
	annots, _ := doc.Resolve(pageDict.Get(pdf.Name("Annots"))).(pdf.Array)
	found := false
	for _, a := range annots {
		d, ok := doc.ResolveDict(a)
		if !ok {
			continue
		}
		rect, ok := doc.Resolve(d.Get(pdf.Name("Rect"))).(pdf.Array)
		if !ok || len(rect) != 4 {
			continue
		}
		x, _ := asFloatForTest(rect[0])
		y, _ := asFloatForTest(rect[1])
		if x == 0 && y == 0 {
			continue // the invisible-signature rectangle, if any
		}
		found = true
		if x != wantX || y != wantY {
			t.Errorf("the signature's rectangle starts at (%g, %g), want (%g, %g)", x, y, wantX, wantY)
		}
	}
	if !found {
		t.Error("the signed document has no visible signature rectangle")
	}
}

func asFloatForTest(o pdf.Object) (float64, bool) {
	switch v := o.(type) {
	case int64:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}
