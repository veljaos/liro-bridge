package appearance

import "testing"

// A4 in points, the page every fixture in this project uses.
var a4 = [4]float64{0, 0, 595, 842}

// TestClampToPageBoxKeepsTheMargin is F6 §6: explicit coordinates are
// clamped into the page box less the margin, never rejected and never
// drawn hanging off the edge.
func TestClampToPageBoxKeepsTheMargin(t *testing.T) {
	const h = 44 // the smallest stamp height (SPEC §13.1's table)

	cases := []struct {
		name         string
		x, y         float64
		wantX, wantY float64
		wantMoved    bool
	}{
		{
			name: "well inside is left alone",
			x:    200, y: 300,
			wantX: 200, wantY: 300,
			wantMoved: false,
		},
		{
			name: "exactly on the margin is left alone",
			x:    Margin, y: Margin,
			wantX: Margin, wantY: Margin,
			wantMoved: false,
		},
		{
			name: "negative coordinates come inside",
			x:    -500, y: -500,
			wantX: Margin, wantY: Margin,
			wantMoved: true,
		},
		{
			name: "past the right edge comes back to the margin",
			x:    900, y: 300,
			wantX: 595 - Margin - StampWidth, wantY: 300,
			wantMoved: true,
		},
		{
			name: "past the top edge comes back to the margin",
			x:    200, y: 2000,
			wantX: 200, wantY: 842 - Margin - h,
			wantMoved: true,
		},
		{
			name: "flush against the right edge is moved inside it",
			x:    595 - StampWidth, y: 300,
			wantX: 595 - Margin - StampWidth, wantY: 300,
			wantMoved: true,
		},
		{
			name: "flush against the bottom edge is moved inside it",
			x:    200, y: 0,
			wantX: 200, wantY: Margin,
			wantMoved: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotX, gotY, moved := ClampToPageBox(a4, tc.x, tc.y, StampWidth, h, Margin)
			if gotX != tc.wantX || gotY != tc.wantY {
				t.Fatalf("ClampToPageBox = (%v, %v), want (%v, %v)", gotX, gotY, tc.wantX, tc.wantY)
			}
			if moved != tc.wantMoved {
				t.Fatalf("moved = %t, want %t", moved, tc.wantMoved)
			}
			// Whatever went in, what comes out is inside the page with
			// the margin clear on every side. That is the property; the
			// exact numbers above are how it is reached.
			if gotX < a4[0]+Margin || gotY < a4[1]+Margin {
				t.Fatalf("(%v, %v) is inside the bottom-left margin", gotX, gotY)
			}
			if gotX+StampWidth > a4[2]-Margin || gotY+h > a4[3]-Margin {
				t.Fatalf("(%v, %v) plus the stamp crosses the top-right margin", gotX, gotY)
			}
		})
	}
}

// TestClampToPageBoxNeverRejects is the other half of F6 §6's rule: a
// stamp nudged inside is better than a refusal, so there is no input
// that produces an error — the function has no error to return.
func TestClampToPageBoxNeverRejects(t *testing.T) {
	for _, xy := range [][2]float64{
		{-1e9, -1e9}, {1e9, 1e9}, {0, 0}, {595, 842},
	} {
		x, y, _ := ClampToPageBox(a4, xy[0], xy[1], StampWidth, 44, Margin)
		if x < 0 || y < 0 || x > 595 || y > 842 {
			t.Fatalf("ClampToPageBox(%v) escaped the page: (%v, %v)", xy, x, y)
		}
	}
}

// TestClampToPageBoxOnAPageTooSmallForTheMargins pins the case where
// the margin cannot be honoured on both sides of an axis. The stamp's
// size is fixed (SPEC §13.1), so something has to give; that axis is
// pinned to its low inset rather than centred, because a predictable
// edge is easier to reason about than a stamp that silently changed
// size.
//
// Each axis is decided on its own. A 100x100 page cannot hold a 190pt
// stamp horizontally, so x pins to the margin — but 100 points is
// ample for a 44pt stamp vertically, so y is an ordinary clamp to
// 100-24-44. Treating "the page is too small" as one condition for
// both axes would have moved y somewhere nobody asked for.
func TestClampToPageBoxOnAPageTooSmallForTheMargins(t *testing.T) {
	const h = 44
	tiny := [4]float64{0, 0, 100, 100} // narrower than the 190pt stamp

	x, y, moved := ClampToPageBox(tiny, 50, 50, StampWidth, h, Margin)
	if !moved {
		t.Fatal("a stamp wider than the page was reported as not moved")
	}
	if x != Margin {
		t.Fatalf("x = %v, want the margin (%v): the page cannot hold the stamp's width", x, Margin)
	}
	if want := float64(100 - Margin - h); y != want {
		t.Fatalf("y = %v, want %v: the page holds the stamp's height, so y is an ordinary clamp", y, want)
	}

	// A page too small on both axes pins both.
	smaller := [4]float64{0, 0, 40, 40}
	x, y, _ = ClampToPageBox(smaller, 5, 5, StampWidth, h, Margin)
	if x != Margin || y != Margin {
		t.Fatalf("ClampToPageBox = (%v, %v), want both pinned to the margin (%v)", x, y, Margin)
	}
}
