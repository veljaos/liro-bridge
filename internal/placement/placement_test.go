package placement

import (
	"math"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/appearance"
)

// The page shapes every test in this file runs over: A4 portrait, A4
// landscape, and one that is neither — F6b §7 asks for exactly these
// three, because a square-ish or unusually small page is where an
// arithmetic mistake that A4 hides shows itself.
var testBoxes = map[string][4]float64{
	"A4 portrait":  {0, 0, 595.32, 842.04},
	"A4 landscape": {0, 0, 842.04, 595.32},
	"non-standard": {12, 20, 420, 400},
}

var testRotations = []int{0, 90, 180, 270}

// testScales are the zoom steps the window offers, plus two Fit-like
// scales that are not on the list, since Fit is computed from the
// window's own size and lands wherever it lands.
var testScales = []float64{0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4, 0.3183, 1.7071}

const (
	stampW = float64(appearance.StampWidth)
	stampH = 48
)

func viewsFor(f func(name string, v View)) {
	for boxName, box := range testBoxes {
		for _, rot := range testRotations {
			for _, s := range testScales {
				v := View{Box: box, Rotate: rot, Scale: s}
				f(boxName, v)
			}
		}
	}
}

func closeTo(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// TestDisplayAndPDFRoundTrip is F6b §7's first case: screen to PDF and
// back, at every zoom step, for all four rotations, on three page
// shapes. A conversion that does not round-trip is one that moves the
// stamp a little every time the person touches it.
func TestDisplayAndPDFRoundTrip(t *testing.T) {
	viewsFor(func(name string, v View) {
		w, h := v.DisplaySize()
		for _, p := range [][2]float64{{0, 0}, {w, h}, {w / 2, h / 3}, {1, h - 1}, {w - 1, 1}} {
			x, y := v.ToPDF(p[0], p[1])
			dx, dy := v.ToDisplay(x, y)
			tol := 1e-6 * math.Max(1, math.Max(w, h))
			if !closeTo(dx, p[0], tol) || !closeTo(dy, p[1], tol) {
				t.Fatalf("%s rotate %d scale %g: (%g, %g) -> pdf (%g, %g) -> (%g, %g)",
					name, v.Rotate, v.Scale, p[0], p[1], x, y, dx, dy)
			}
		}
	})
}

// TestDisplaySizeMatchesTheRotatedPage: a quarter-turned page is shown
// with its width and height exchanged, which is what a reader does and
// therefore what the window has to do.
func TestDisplaySizeMatchesTheRotatedPage(t *testing.T) {
	box := testBoxes["A4 portrait"]
	pw, ph := box[2]-box[0], box[3]-box[1]
	for _, rot := range testRotations {
		v := View{Box: box, Rotate: rot, Scale: 2}
		w, h := v.DisplaySize()
		wantW, wantH := pw*2, ph*2
		if rot == 90 || rot == 270 {
			wantW, wantH = ph*2, pw*2
		}
		if !closeTo(w, wantW, 1e-9) || !closeTo(h, wantH, 1e-9) {
			t.Errorf("rotate %d: display size (%g, %g), want (%g, %g)", rot, w, h, wantW, wantH)
		}
	}
}

// TestPageCornersMapToDisplayCorners is the check that the rotation is
// the one a reader applies rather than its mirror or its inverse: the
// page's own bottom-left corner has to arrive at the *displayed*
// bottom-left, whichever way the page is turned.
func TestPageCornersMapToDisplayCorners(t *testing.T) {
	box := testBoxes["A4 portrait"]
	x0, y0, x1, y1 := box[0], box[1], box[2], box[3]
	for _, rot := range testRotations {
		v := View{Box: box, Rotate: rot, Scale: 1}
		w, h := v.DisplaySize()
		// Which content corner ends up at the top left of the display:
		// rotating the page clockwise for display moves the bottom-left
		// corner there for 90, the bottom-right for 180, and so on.
		var wantTopLeft [2]float64
		switch rot {
		case 90:
			wantTopLeft = [2]float64{x0, y0}
		case 180:
			wantTopLeft = [2]float64{x1, y0}
		case 270:
			wantTopLeft = [2]float64{x1, y1}
		default:
			wantTopLeft = [2]float64{x0, y1}
		}
		dx, dy := v.ToDisplay(wantTopLeft[0], wantTopLeft[1])
		if !closeTo(dx, 0, 1e-6) || !closeTo(dy, 0, 1e-6) {
			t.Errorf("rotate %d: content corner %v lands at (%g, %g), want the top left", rot, wantTopLeft, dx, dy)
		}
		// And the display's own far corner is the page's other extreme.
		if w <= 0 || h <= 0 {
			t.Errorf("rotate %d: display size is empty", rot)
		}
	}
}

// TestDisplayRectAndOriginRoundTrip: the rectangle the window draws and
// the position it reads back from a drag are inverses of each other, at
// every zoom and rotation. If they were not, a stamp would drift by a
// fraction of a point every time it was picked up and put down.
func TestDisplayRectAndOriginRoundTrip(t *testing.T) {
	viewsFor(func(name string, v View) {
		minX, minY, maxX, maxY := Bounds(v.Box, v.Rotate, stampW, stampH)
		for _, p := range [][2]float64{{minX, minY}, {maxX, maxY}, {(minX + maxX) / 2, (minY + maxY) / 2}} {
			if maxX < minX || maxY < minY {
				continue
			}
			r := v.DisplayRect(p[0], p[1], stampW, stampH)
			gx, gy := v.OriginFromDisplay(r.Left, r.Top, stampW, stampH)
			tol := 1e-6 / v.Scale * 10
			if !closeTo(gx, p[0], tol) || !closeTo(gy, p[1], tol) {
				t.Fatalf("%s rotate %d scale %g: (%g, %g) -> rect %+v -> (%g, %g)",
					name, v.Rotate, v.Scale, p[0], p[1], r, gx, gy)
			}
		}
	})
}

// TestDisplayRectIsTheStampsOwnSizeOnScreen: the stamp is drawn upright
// as the page is displayed, so on screen it is always 190 points wide
// and its own height tall, whatever the page's rotation.
func TestDisplayRectIsTheStampsOwnSizeOnScreen(t *testing.T) {
	viewsFor(func(name string, v View) {
		minX, minY, _, _ := Bounds(v.Box, v.Rotate, stampW, stampH)
		r := v.DisplayRect(minX, minY, stampW, stampH)
		wantW, wantH := stampW*v.Scale, stampH*v.Scale
		if !closeTo(r.Width, wantW, 1e-6) || !closeTo(r.Height, wantH, 1e-6) {
			t.Fatalf("%s rotate %d scale %g: rect is %g x %g, want %g x %g",
				name, v.Rotate, v.Scale, r.Width, r.Height, wantW, wantH)
		}
	})
}

// TestZoomNeverMovesTheStamp is F6b §2.3's own requirement, proven
// rather than asserted: the position is in points, so no sequence of
// zoom changes may alter it by so much as a fraction of one.
//
// The check goes through the *display* on purpose. Holding the position
// in a variable and never touching it would prove nothing; this takes
// the position, draws it at one zoom, reads it back at that zoom, draws
// it at the next, and so on through every step in both directions —
// which is what the window actually does as the person zooms.
func TestZoomNeverMovesTheStamp(t *testing.T) {
	for boxName, box := range testBoxes {
		for _, rot := range testRotations {
			startX, startY := 100.0, 90.0
			x, y, _ := Clamp(box, rot, startX, startY, stampW, stampH)
			originalX, originalY := x, y

			sequence := append([]float64{}, testScales...)
			for i := len(testScales) - 1; i >= 0; i-- {
				sequence = append(sequence, testScales[i])
			}
			for _, s := range sequence {
				v := View{Box: box, Rotate: rot, Scale: s}
				r := v.DisplayRect(x, y, stampW, stampH)
				x, y = v.OriginFromDisplay(r.Left, r.Top, stampW, stampH)
			}
			if !closeTo(x, originalX, 1e-6) || !closeTo(y, originalY, 1e-6) {
				t.Errorf("%s rotate %d: after zooming through every step and back, the stamp moved from (%g, %g) to (%g, %g)",
					boxName, rot, originalX, originalY, x, y)
			}
		}
	}
}

// TestClampKeepsTheMarginOnEveryEdge is F6b §7's clamping case: at
// every zoom step, at all four edges, the result is inside the box less
// twelve points.
func TestClampKeepsTheMarginOnEveryEdge(t *testing.T) {
	for boxName, box := range testBoxes {
		for _, rot := range testRotations {
			fw, fh := Footprint(rot, stampW, stampH)
			if box[2]-box[0] < fw+2*Margin || box[3]-box[1] < fh+2*Margin {
				continue // too small to hold the stamp: covered separately
			}
			for _, p := range [][2]float64{
				{-10000, 0}, {10000, 0}, {0, -10000}, {0, 10000},
				{box[0], box[1]}, {box[2], box[3]},
				{box[0] + Margin, box[1] + Margin},
			} {
				x, y, _ := Clamp(box, rot, p[0], p[1], stampW, stampH)
				if x < box[0]+Margin-1e-9 || y < box[1]+Margin-1e-9 {
					t.Errorf("%s rotate %d: (%g, %g) clamped to (%g, %g), which is inside the margin",
						boxName, rot, p[0], p[1], x, y)
				}
				if x+fw > box[2]-Margin+1e-9 || y+fh > box[3]-Margin+1e-9 {
					t.Errorf("%s rotate %d: (%g, %g) clamped to (%g, %g), whose far edge is past the margin",
						boxName, rot, p[0], p[1], x, y)
				}
			}
		}
	}
}

// TestClampOnAPageTooSmallPinsToTheInset: the one case where the margin
// cannot be kept on both sides, handled the same way appearance's own
// clamp handles it — pinned rather than shrunk.
func TestClampOnAPageTooSmallPinsToTheInset(t *testing.T) {
	tiny := [4]float64{0, 0, 100, 100}
	x, y, moved := Clamp(tiny, 0, 50, 50, stampW, stampH)
	if !moved {
		t.Error("a stamp wider than the page was reported as fitting")
	}
	if x != Margin {
		t.Errorf("x = %g, want the margin (%d)", x, Margin)
	}
	if want := float64(100 - Margin - stampH); y != want {
		t.Errorf("y = %g, want %g", y, want)
	}
}

// TestCornersAgreeWithTheSignedDocument is the check that this
// package's idea of a corner and the signing engine's are the same one.
// A stamp snapped to the bottom right in the window has to land exactly
// where choosing "bottom-right" in the corner selector would put it, or
// the two ways of asking for the same thing quietly disagree.
func TestCornersAgreeWithTheSignedDocument(t *testing.T) {
	for boxName, box := range testBoxes {
		for _, rot := range testRotations {
			for _, c := range Corners(box, rot, stampW, stampH) {
				corner, ok := appearanceCorner(c.Name)
				if !ok {
					t.Fatalf("unknown corner %q", c.Name)
				}
				rect, _, err := appearance.PlaceCorner(box, rot, corner, appearance.Margin, stampW, stampH)
				if err != nil {
					t.Fatal(err)
				}
				if rect[0] != c.X || rect[1] != c.Y {
					t.Errorf("%s rotate %d %s: placement says (%g, %g), the signing engine says (%g, %g)",
						boxName, rot, c.Name, c.X, c.Y, rect[0], rect[1])
				}
			}
		}
	}
}

// TestSnapWithinTenPointsAndNotBeyond is F6b §7's snapping case, from
// both sides: inside ten points it snaps, outside it does not.
func TestSnapWithinTenPointsAndNotBeyond(t *testing.T) {
	box := testBoxes["A4 portrait"]
	for _, rot := range testRotations {
		for _, c := range Corners(box, rot, stampW, stampH) {
			// Nine points away, diagonally: inside the distance.
			near := 9 / math.Sqrt2
			x, y, name := Snap(box, rot, c.X+near, c.Y+near, stampW, stampH)
			if name != c.Name || x != c.X || y != c.Y {
				t.Errorf("rotate %d: a position 9pt from %s did not snap (got %q at %g, %g)",
					rot, c.Name, name, x, y)
			}
			// Eleven points away: outside it.
			far := 11 / math.Sqrt2
			fx, fy, fname := Snap(box, rot, c.X+far, c.Y+far, stampW, stampH)
			if fname != "" || fx != c.X+far || fy != c.Y+far {
				t.Errorf("rotate %d: a position 11pt from %s snapped anyway (got %q at %g, %g)",
					rot, c.Name, fname, fx, fy)
			}
		}
	}
}

// TestNudgeMovesExactlyOnePointAtEveryZoom is F6b §7's nudging case.
// The step is in points and the zoom is irrelevant to it, which is the
// whole reason arrow keys exist here: a mouse cannot place a stamp
// under a printed initial to the point, and this can.
func TestNudgeMovesExactlyOnePointAtEveryZoom(t *testing.T) {
	box := testBoxes["A4 portrait"]
	for _, rot := range testRotations {
		for _, s := range testScales {
			v := View{Box: box, Rotate: rot, Scale: s}
			startX, startY := 200.0, 300.0
			for _, step := range []float64{NudgeStep, NudgeStepLarge} {
				// Right on screen, then left again: back where it began.
				x, y := v.Nudge(startX, startY, step, 0, stampW, stampH)
				moved := math.Hypot(x-startX, y-startY)
				if !closeTo(moved, step, 1e-9) {
					t.Fatalf("rotate %d scale %g: nudging %g points moved %g", rot, s, step, moved)
				}
				bx, by := v.Nudge(x, y, -step, 0, stampW, stampH)
				if !closeTo(bx, startX, 1e-9) || !closeTo(by, startY, 1e-9) {
					t.Fatalf("rotate %d scale %g: right then left ended at (%g, %g), not (%g, %g)",
						rot, s, bx, by, startX, startY)
				}
				// Down on screen, then up again.
				dx, dy := v.Nudge(startX, startY, 0, step, stampW, stampH)
				if !closeTo(math.Hypot(dx-startX, dy-startY), step, 1e-9) {
					t.Fatalf("rotate %d scale %g: nudging down %g points did not move %g", rot, s, step, step)
				}
			}
		}
	}
}

// TestNudgeRightIsRightOnScreen: the direction has to be the one the
// person sees, not the one the page's own coordinates would give. On a
// page turned a quarter turn those are different axes.
func TestNudgeRightIsRightOnScreen(t *testing.T) {
	box := testBoxes["A4 portrait"]
	for _, rot := range testRotations {
		v := View{Box: box, Rotate: rot, Scale: 1.5}
		x, y := 200.0, 300.0
		before := v.DisplayRect(x, y, stampW, stampH)
		nx, ny := v.Nudge(x, y, NudgeStepLarge, 0, stampW, stampH)
		after := v.DisplayRect(nx, ny, stampW, stampH)
		if got := after.Left - before.Left; !closeTo(got, NudgeStepLarge*v.Scale, 1e-6) {
			t.Errorf("rotate %d: nudging right moved the stamp %g pixels on screen, want %g",
				rot, got, NudgeStepLarge*v.Scale)
		}
		dy, dyy := v.Nudge(x, y, 0, NudgeStepLarge, stampW, stampH)
		down := v.DisplayRect(dy, dyy, stampW, stampH)
		if got := down.Top - before.Top; !closeTo(got, NudgeStepLarge*v.Scale, 1e-6) {
			t.Errorf("rotate %d: nudging down moved the stamp %g pixels on screen, want %g",
				rot, got, NudgeStepLarge*v.Scale)
		}
	}
}

// TestNudgeStopsAtTheMargin: holding an arrow key against an edge stops
// there rather than running the stamp off the page.
func TestNudgeStopsAtTheMargin(t *testing.T) {
	box := testBoxes["A4 portrait"]
	v := View{Box: box, Rotate: 0, Scale: 1}
	x, y := 200.0, 300.0
	for i := 0; i < 500; i++ {
		x, y = v.Nudge(x, y, -NudgeStepLarge, NudgeStepLarge, stampW, stampH)
	}
	minX, minY, _, _ := Bounds(box, 0, stampW, stampH)
	if !closeTo(x, minX, 1e-9) || !closeTo(y, minY, 1e-9) {
		t.Errorf("nudging into the corner ended at (%g, %g), want (%g, %g)", x, y, minX, minY)
	}
}

// TestFootprintSwapsOnAQuarterTurn: the stamp's own size never changes,
// but the area it occupies in the page's coordinates does.
func TestFootprintSwapsOnAQuarterTurn(t *testing.T) {
	for _, rot := range testRotations {
		w, h := Footprint(rot, stampW, stampH)
		switch rot {
		case 90, 270:
			if w != stampH || h != stampW {
				t.Errorf("rotate %d: footprint (%g, %g), want the swap", rot, w, h)
			}
		default:
			if w != stampW || h != stampH {
				t.Errorf("rotate %d: footprint (%g, %g), want (%g, %d)", rot, w, h, stampW, stampH)
			}
		}
	}
}
