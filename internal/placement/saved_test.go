package placement

import (
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/render"
)

// TestSavedPageFallsBackToTheLastPage is F6b §3: a position saved on
// page twelve means nothing in a four-page document, so it goes on that
// document's last page and says so.
func TestSavedPageFallsBackToTheLastPage(t *testing.T) {
	cases := []struct {
		want      int
		pageCount int
		gotPage   int
		gotFall   bool
	}{
		{want: 12, pageCount: 4, gotPage: 4, gotFall: true},
		{want: 12, pageCount: 12, gotPage: 12, gotFall: false},
		{want: 1, pageCount: 1, gotPage: 1, gotFall: false},
		{want: 3, pageCount: 50, gotPage: 3, gotFall: false},
		{want: 0, pageCount: 5, gotPage: 1, gotFall: false},
	}
	for _, tc := range cases {
		page, fell := ResolvePage(tc.want, tc.pageCount)
		if page != tc.gotPage || fell != tc.gotFall {
			t.Errorf("ResolvePage(%d, %d) = (%d, %v), want (%d, %v)",
				tc.want, tc.pageCount, page, fell, tc.gotPage, tc.gotFall)
		}
	}
}

// TestSavedPositionOnASmallerPageIsClampedAndReported is the other half
// of F6b §3: a position chosen on A4 does not fit an A5 page, so it is
// brought inside that page's margin — and the fact that it moved is
// what the batch reports afterwards.
func TestSavedPositionOnASmallerPageIsClampedAndReported(t *testing.T) {
	a4 := [4]float64{0, 0, 595.32, 842.04}
	a5 := [4]float64{0, 0, 419.53, 595.28}

	// A position at the bottom right of A4 — which is off the right edge
	// of A5 entirely.
	saved := Saved{Page: 1, X: 393.32, Y: 12}

	x, y, moved := Fit(saved, a4, 0, stampW, stampH)
	if moved {
		t.Errorf("the position moved on the page it was chosen on: (%g, %g)", x, y)
	}

	x, y, moved = Fit(saved, a5, 0, stampW, stampH)
	if !moved {
		t.Fatal("a position off the right edge of a smaller page was not reported as moved")
	}
	maxX := a5[2] - Margin - stampW
	if !closeTo(x, maxX, 1e-9) {
		t.Errorf("x = %g, want %g (the right edge of the smaller page, less the margin)", x, maxX)
	}
	if !closeTo(y, 12, 1e-9) {
		t.Errorf("y = %g, want 12: the height fitted, so it should not have moved", y)
	}
}

// TestAdjustmentReportsOnlyWhenSomethingChanged: a batch of documents
// that all took the position as it stood reports nothing, which is the
// ordinary case and the one that must not produce noise.
func TestAdjustmentReportsOnlyWhenSomethingChanged(t *testing.T) {
	if (Adjustment{Name: "a.pdf", Page: 3}).Adjusted() {
		t.Error("an untouched placement reported itself as adjusted")
	}
	if !(Adjustment{Name: "a.pdf", Page: 3, Moved: true}).Adjusted() {
		t.Error("a moved placement did not report itself")
	}
	if !(Adjustment{Name: "a.pdf", Page: 1, PageFellBack: true}).Adjusted() {
		t.Error("a page fallback did not report itself")
	}
}

// TestPlacementAgreesWithTheRenderersPageMatrix is the check that the
// window's arithmetic and the renderer's are one derivation rather than
// two.
//
// They have to be: the page image is drawn with the renderer's matrix
// and the stamp is positioned over it with this package's, so a
// disagreement of a single point would put the stamp a point away from
// where the person dropped it — and would be invisible in every test
// that only looked at one of them.
func TestPlacementAgreesWithTheRenderersPageMatrix(t *testing.T) {
	for boxName, box := range testBoxes {
		for _, rot := range testRotations {
			for _, s := range testScales {
				p := render.Page{Box: box, Rotate: rot}
				m := render.PageMatrix(p, s)
				v := View{Box: box, Rotate: rot, Scale: s}
				for _, pt := range [][2]float64{
					{box[0], box[1]}, {box[2], box[3]},
					{(box[0] + box[2]) / 2, (box[1] + box[3]) / 2},
					{box[0] + 37, box[1] + 211},
				} {
					wantX := m[0]*pt[0] + m[2]*pt[1] + m[4]
					wantY := m[1]*pt[0] + m[3]*pt[1] + m[5]
					gotX, gotY := v.ToDisplay(pt[0], pt[1])
					if !closeTo(gotX, wantX, 1e-6) || !closeTo(gotY, wantY, 1e-6) {
						t.Fatalf("%s rotate %d scale %g: point %v is at (%g, %g) for the window and (%g, %g) for the renderer",
							boxName, rot, s, pt, gotX, gotY, wantX, wantY)
					}
				}
			}
		}
	}
}

// TestPlacementAgreesWithTheRenderersPageSize: the image the window
// draws and the coordinate space it draws over have to be the same
// size, or the stamp is offset by the difference.
func TestPlacementAgreesWithTheRenderersPageSize(t *testing.T) {
	for boxName, box := range testBoxes {
		for _, rot := range testRotations {
			p := render.Page{Box: box, Rotate: rot}
			w, h := box[2]-box[0], box[3]-box[1]
			if rot == 90 || rot == 270 {
				w, h = h, w
			}
			p.WidthPt, p.HeightPt = w, h
			v := View{Box: box, Rotate: rot, Scale: 1.5}
			vw, vh := v.DisplaySize()
			if !closeTo(vw, p.WidthPt*1.5, 1e-9) || !closeTo(vh, p.HeightPt*1.5, 1e-9) {
				t.Errorf("%s rotate %d: window says %g x %g, renderer says %g x %g",
					boxName, rot, vw, vh, p.WidthPt*1.5, p.HeightPt*1.5)
			}
		}
	}
}
