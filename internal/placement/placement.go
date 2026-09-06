// Package placement is the arithmetic behind the visual stamp
// placement window (F6b): converting between what a person sees on
// screen and where the stamp actually goes in the document, keeping it
// inside the page's margin, snapping it to a corner, nudging it a point
// at a time, and fitting a remembered position onto the next document.
//
// It is pure Go with no window in it, for the same reason
// internal/consent is (F5 §10, D-085): every one of these has a right
// answer that does not depend on pixels being on a screen, and F6b §7
// asks for that answer to be tested exhaustively — at every zoom step,
// for all four rotations, on three page shapes. None of that is
// practical to drive through a window, and all of it is where this
// feature will go wrong if it goes wrong.
//
// The one idea the whole package rests on: **the position is stored in
// the page's own coordinates, never in pixels.** Screen coordinates are
// derived from it for drawing and read back from a drag; they are never
// what is kept. That is what makes zoom unable to move a stamp by so
// much as a point, which F6b §2.3 requires and which a stored-in-pixels
// design cannot promise.
package placement

import (
	"math"

	"github.com/veljaos/liro-bridge/internal/pades/appearance"
)

// Margin is the clear space kept between the stamp and every page edge,
// in points — the same value appearance.Margin uses for a corner
// placement, so a stamp dragged into a corner lands exactly where
// choosing that corner would have put it.
const Margin = appearance.Margin

// SnapDistance is how close to a corner a drag has to get before it
// snaps there, in points (F6b §2.4). Ten points is about three
// millimetres: far enough that a corner takes no aim at all, near enough
// that a deliberate placement two centimetres in is left alone.
const SnapDistance = 10

// NudgeStep and NudgeStepLarge are what an arrow key and a shifted
// arrow key move the stamp by, in points (F6b §2.4). They are points
// rather than pixels on purpose: a nudge means the same thing at every
// zoom, which is what makes it useful for lining a stamp up under a
// printed initial.
const (
	NudgeStep      = 1
	NudgeStepLarge = 10
)

// View is one page shown at one zoom: everything needed to convert
// between the page's own coordinates and the pixels on screen.
type View struct {
	// Box is the page's /MediaBox, in its own content-space
	// coordinates.
	Box [4]float64

	// Rotate is the page's /Rotate, 0, 90, 180 or 270. The preview
	// shows the page the way a reader does, so a quarter-turned page is
	// displayed turned, and a drag on it has to be read accordingly.
	Rotate int

	// Scale is display pixels per point.
	Scale float64
}

// DisplaySize is the page's size on screen, in pixels.
func (v View) DisplaySize() (w, h float64) {
	pw, ph := v.Box[2]-v.Box[0], v.Box[3]-v.Box[1]
	if v.Rotate == 90 || v.Rotate == 270 {
		pw, ph = ph, pw
	}
	return pw * v.Scale, ph * v.Scale
}

// ToDisplay converts a point in the page's content space to display
// pixels, with the origin at the top-left of the displayed page and y
// increasing downwards, which is how a browser measures.
//
// This is the same derivation the renderer's own page matrix uses; the
// two have to agree exactly, and a test compares them directly rather
// than trusting that they were written from the same reasoning.
func (v View) ToDisplay(x, y float64) (float64, float64) {
	x0, y0, x1, y1 := v.Box[0], v.Box[1], v.Box[2], v.Box[3]
	s := v.Scale
	switch v.Rotate {
	case 90:
		return (y - y0) * s, (x - x0) * s
	case 180:
		return (x1 - x) * s, (y - y0) * s
	case 270:
		return (y1 - y) * s, (x1 - x) * s
	default:
		return (x - x0) * s, (y1 - y) * s
	}
}

// ToPDF is ToDisplay's inverse.
func (v View) ToPDF(dx, dy float64) (float64, float64) {
	x0, y0, x1, y1 := v.Box[0], v.Box[1], v.Box[2], v.Box[3]
	s := v.Scale
	if s == 0 {
		return x0, y0
	}
	switch v.Rotate {
	case 90:
		return x0 + dy/s, y0 + dx/s
	case 180:
		return x1 - dx/s, y0 + dy/s
	case 270:
		return x1 - dy/s, y1 - dx/s
	default:
		return x0 + dx/s, y1 - dy/s
	}
}

// Footprint is the area the stamp occupies in the page's own
// coordinates. A stamp is always drawn upright as the page is
// displayed, so on a quarter-turned page its width and height swap in
// content space — the same swap appearance.PlaceCorner makes for a
// corner placement.
func Footprint(rotate int, stampW, stampH float64) (w, h float64) {
	if rotate == 90 || rotate == 270 {
		return stampH, stampW
	}
	return stampW, stampH
}

// Rect is a rectangle on screen, in display pixels.
type Rect struct{ Left, Top, Width, Height float64 }

// DisplayRect is where a stamp whose content-space lower-left corner is
// (x, y) appears on screen.
//
// It maps all four corners and takes the extremes rather than
// case-analysing the rotation. Every rotation here is a multiple of a
// quarter turn, so the rectangle stays axis-aligned in both spaces and
// the extremes are exactly its corners — which means one derivation
// covers all four rotations instead of four that have to agree.
func (v View) DisplayRect(x, y, stampW, stampH float64) Rect {
	fw, fh := Footprint(v.Rotate, stampW, stampH)
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{x, y}, {x + fw, y}, {x + fw, y + fh}, {x, y + fh}} {
		dx, dy := v.ToDisplay(c[0], c[1])
		minX, minY = math.Min(minX, dx), math.Min(minY, dy)
		maxX, maxY = math.Max(maxX, dx), math.Max(maxY, dy)
	}
	return Rect{Left: minX, Top: minY, Width: maxX - minX, Height: maxY - minY}
}

// OriginFromDisplay is DisplayRect's inverse: given where the stamp's
// rectangle sits on screen, the content-space coordinates of its
// lower-left corner.
func (v View) OriginFromDisplay(left, top, stampW, stampH float64) (x, y float64) {
	fw, fh := Footprint(v.Rotate, stampW, stampH)
	dw, dh := fw, fh
	if v.Rotate == 90 || v.Rotate == 270 {
		dw, dh = fh, fw
	}
	minX, minY := math.Inf(1), math.Inf(1)
	for _, c := range [4][2]float64{{left, top}, {left + dw*v.Scale, top}, {left + dw*v.Scale, top + dh*v.Scale}, {left, top + dh*v.Scale}} {
		px, py := v.ToPDF(c[0], c[1])
		minX, minY = math.Min(minX, px), math.Min(minY, py)
	}
	return minX, minY
}

// Bounds is the range a stamp's lower-left corner may take on this
// page: the box less the margin on every side, less the stamp's own
// footprint.
func Bounds(box [4]float64, rotate int, stampW, stampH float64) (minX, minY, maxX, maxY float64) {
	fw, fh := Footprint(rotate, stampW, stampH)
	minX, minY = box[0]+Margin, box[1]+Margin
	maxX, maxY = box[2]-Margin-fw, box[3]-Margin-fh
	return minX, minY, maxX, maxY
}

// Clamp moves a stamp so it sits inside the page's margin, and says
// whether it had to move.
//
// A page too small to hold the stamp and both margins is the one case
// where the margin cannot be kept on both sides; the stamp is pinned to
// the lower-left inset, which is what appearance.ClampToPageBox already
// does for an explicit coordinate and for the same reason — the stamp's
// size is fixed, so something has to give, and a predictable corner is
// easier to reason about than a stamp that silently shrank.
func Clamp(box [4]float64, rotate int, x, y, stampW, stampH float64) (cx, cy float64, moved bool) {
	minX, minY, maxX, maxY := Bounds(box, rotate, stampW, stampH)
	if maxX < minX {
		cx = minX
	} else {
		cx = clamp(x, minX, maxX)
	}
	if maxY < minY {
		cy = minY
	} else {
		cy = clamp(y, minY, maxY)
	}
	return cx, cy, cx != x || cy != y
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Corner is one of the four positions a stamp snaps to.
type Corner struct {
	// Name is the value the rest of the project uses for a corner
	// placement — "bottom-right" and its three companions.
	Name string
	X, Y float64
}

// Corners lists where the four corner placements put a stamp on this
// page, in content-space coordinates, named the way the configuration
// names them.
//
// The names are the *visual* corners, so on a quarter-turned page
// "bottom-right" is the corner a person sees at the bottom right, not
// the one the content-space arithmetic would call that. It is the same
// mapping appearance.PlaceCorner makes, and a test compares the two
// directly.
func Corners(box [4]float64, rotate int, stampW, stampH float64) []Corner {
	out := make([]Corner, 0, 4)
	for _, name := range []string{"bottom-right", "bottom-left", "top-right", "top-left"} {
		c, ok := cornerFor(box, rotate, name, stampW, stampH)
		if ok {
			out = append(out, c)
		}
	}
	return out
}

func cornerFor(box [4]float64, rotate int, name string, stampW, stampH float64) (Corner, bool) {
	corner, ok := appearanceCorner(name)
	if !ok {
		return Corner{}, false
	}
	rect, _, err := appearance.PlaceCorner(box, rotate, corner, Margin, stampW, stampH)
	if err != nil {
		return Corner{}, false
	}
	return Corner{Name: name, X: rect[0], Y: rect[1]}, true
}

func appearanceCorner(name string) (appearance.Corner, bool) {
	switch name {
	case "bottom-right":
		return appearance.BottomRight, true
	case "bottom-left":
		return appearance.BottomLeft, true
	case "top-right":
		return appearance.TopRight, true
	case "top-left":
		return appearance.TopLeft, true
	}
	return 0, false
}

// Snap pulls a position to the nearest corner when it is within
// SnapDistance of one, and reports which corner it snapped to.
func Snap(box [4]float64, rotate int, x, y, stampW, stampH float64) (sx, sy float64, name string) {
	best := math.Inf(1)
	sx, sy = x, y
	for _, c := range Corners(box, rotate, stampW, stampH) {
		d := math.Hypot(c.X-x, c.Y-y)
		if d <= SnapDistance && d < best {
			best, sx, sy, name = d, c.X, c.Y, c.Name
		}
	}
	return sx, sy, name
}

// Nudge moves a stamp by whole points *as the person sees it*: dx to the
// right on screen and dy downwards on screen, whatever the page's
// rotation happens to be. The result is clamped, so holding an arrow key
// against the margin stops there rather than running off the page.
func (v View) Nudge(x, y, dxDisplay, dyDisplay, stampW, stampH float64) (nx, ny float64) {
	// The conversion is done on a direction rather than a point, so it
	// picks up the rotation and drops the translation — and it is done
	// in points, not pixels, so the step is the same at every zoom.
	unit := View{Box: v.Box, Rotate: v.Rotate, Scale: 1}
	ox, oy := unit.ToPDF(0, 0)
	px, py := unit.ToPDF(dxDisplay, dyDisplay)
	nx, ny, _ = Clamp(v.Box, v.Rotate, x+(px-ox), y+(py-oy), stampW, stampH)
	return nx, ny
}
