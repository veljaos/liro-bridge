// Package appearance builds the visual signature stamp (SPEC §13, F4):
// its geometry, its embedded font, and its content stream. Nothing here
// is invoked unless a caller explicitly asks for a stamp (F4 §6) — the
// default, invisible signature path (internal/pades/pdf.BuildPlaceholder
// with no Appearance) never imports or executes any code in this
// package.
package appearance

import "fmt"

// StampWidth is the stamp's fixed width, in points (SPEC §13.1/F4 §2).
const StampWidth = 190

// Margin is the stamp's distance from the page edge, in points.
//
// Twelve, superseding F6's twenty-four. The owner's instruction with the
// placement window (F6b §2.5): not too large, because someone may
// genuinely want the stamp close to the edge — but never flush against
// it, which the original Bridge did not allow either. Twelve points is
// about four millimetres, enough that no printer's unprintable border
// and no binding clips it, and small enough that a stamp deliberately
// tucked into a corner looks tucked in rather than floated.
//
// It applies to every placement, corner and coordinate alike: a stamp
// dragged to the bottom right in the placement window has to land where
// choosing "bottom-right" puts it, and two margins would make that
// false.
const Margin = 12

// LogoSize is the logo's fixed placement width and height, in points
// (SPEC §13.1). This is independent of the source image's pixel
// dimensions (LogoImagePixels): a PDF image XObject is always mapped
// into the unit square by the content stream's `cm` matrix, so the
// asset's actual resolution never needs to match the points it is
// drawn into.
const LogoSize = 36

// LogoImagePixels is the logo image's fixed width and height, in
// pixels — the real Liro mark supplied in assets/signature-logo.js
// (256×256, turquoise #038387), regenerated via scripts/genlogo (see
// docs/decisions.md, superseding the earlier 36×36 placeholder mark).
const LogoImagePixels = 256

// Padding is the internal padding between the stamp's edge and its
// content (logo, text), in points.
const Padding = 4

// stampLineHeight is the vertical space one line of stamp text gets.
// 7pt type (nominalFontSize) in a 10pt slot is ordinary tight
// typesetting — 1.4× — and it is what makes four lines fit beside a
// 36pt logo without either the ascenders of one line or the descenders
// of the one above it reaching into their neighbour.
const stampLineHeight = 10

// heightsByLineCount is SPEC §13.1/F4 §2's height table, indexed by
// (line count - 1) — "height grows with the number of lines actually
// drawn, so there is no empty space".
//
// SPEC §13.1 gives it as [44, 44, 46, 56, 72] and the table is sized to
// the content, not the other way round: the stamp now always draws four
// lines (label, signer, serial, date — stamp.go's buildLines) and can
// draw six, so entries the original table did not have are needed and
// two it did have are no longer reachable.
//
// The first three entries are SPEC's own, untouched. Four lines and up
// are 2*Padding + n*stampLineHeight, floored at 44 — the height the
// logo itself occupies, 36pt plus its own padding above and below,
// below which the box would be shorter than the mark inside it. That
// gives 48, 58, 68, which is why a four-line stamp is 48pt tall rather
// than SPEC's 56: 56 was sized for four lines of which one was the
// optional reference, and spreading four *mandatory* lines over it
// leaves 12pt gaps that read as a paragraph rather than a stamp.
var heightsByLineCount = [6]float64{
	44, 44, 46,
	heightForTextLines(4), heightForTextLines(5), heightForTextLines(6),
}

// logoBlockHeight is what the logo alone occupies: its own 36 points
// plus the padding above and below it. No stamp is shorter than the
// mark inside it.
const logoBlockHeight = LogoSize + 2*Padding

// logoY is the bottom edge of the logo's 36pt box in a stamp of height
// h: vertically centred, not pinned to the bottom padding.
//
// It used to be pinned — drawn at Padding, which in a 48pt four-line
// stamp left 8 points of white above the mark and 4 below and put the
// mark's centre 2 points below the box's. That is the gap the text
// block was asked to be centred against, so both are now centred
// against the same thing: the box.
func logoY(h float64) float64 {
	return (h - LogoSize) / 2
}

// textBlockTop is the top edge of an n-line text block in a stamp of
// height h — the y the first line's slot starts at.
//
// The block is n whole line heights tall and is centred on the logo,
// rather than starting at the top of the box and running down from
// there. Four lines beside a 36pt mark is the case that matters and the
// case that made this necessary: the block and the mark are now
// concentric, so neither reads as sitting higher than the other.
//
// Centring is then clamped into the box's own padding, bottom edge
// first so that the top edge wins if a block is taller than the box can
// hold: a stamp with every optional line present is the taller thing,
// and running out of room at the bottom is more legible than running
// out of it at the top.
func textBlockTop(n int, h float64) float64 {
	blockHeight := float64(n) * stampLineHeight
	top := logoY(h) + LogoSize/2 + blockHeight/2
	if top-blockHeight < Padding {
		top = Padding + blockHeight
	}
	if top > h-Padding {
		top = h - Padding
	}
	return top
}

// heightForTextLines is the derivation behind heightsByLineCount's
// fourth entry onward, written out rather than commented so it cannot
// drift from the numbers it produced: the text column's own height,
// floored at the logo's.
func heightForTextLines(n int) float64 {
	h := float64(2*Padding + n*stampLineHeight)
	if h < logoBlockHeight {
		return logoBlockHeight
	}
	return h
}

// HeightForLines returns the stamp's total height for a stamp drawing
// exactly n lines (1..5) — "height grows with the number of lines
// actually drawn, so there is no empty space" (F4 §2).
func HeightForLines(n int) (float64, error) {
	if n < 1 || n > len(heightsByLineCount) {
		return 0, fmt.Errorf("appearance: %d lines has no defined height (valid range 1..%d)", n, len(heightsByLineCount))
	}
	return heightsByLineCount[n-1], nil
}

// Corner is one of the four page corners a stamp can be anchored to
// (F4 §2/§7). BottomRight is the default (SPEC §13.1).
type Corner int

const (
	BottomRight Corner = iota
	BottomLeft
	TopRight
	TopLeft
)

// contentCorner names one of a MediaBox's four physical corners, in the
// page's own (unrotated) content-stream coordinate space.
type contentCorner int

const (
	ccBottomLeft contentCorner = iota
	ccBottomRight
	ccTopLeft
	ccTopRight
)

// rotationPlan says, for one /Rotate value and one requested visual
// corner, which physical content-space corner to anchor the stamp at,
// whether the stamp's width/height swap in content space, and the Form
// XObject /Matrix that counter-rotates the stamp's own (always drawn
// upright, natural reading orientation) content so it displays upright
// after the viewer applies /Rotate (F4 §2.1: "Respect /Rotate... A
// stamp placed in the bottom-right of an unrotated coordinate system
// lands somewhere else on a rotated page").
//
// Derivation (also recorded in docs/decisions.md): /Rotate N is defined
// as the clockwise rotation applied when *displaying* the page (PDF
// 32000-1 §7.7.3.3). Physically rotating a rectangle 90° clockwise
// moves its top-left corner to its top-right, its top-right to its
// bottom-right, its bottom-right to its bottom-left, and its
// bottom-left to its top-left — so the corner that ends up displayed at
// a given visual position, before that display rotation is applied, is
// found by walking that same cycle backwards. To make the stamp's own
// content appear upright after the viewer's clockwise rotation, this
// package pre-rotates it counter-clockwise by the same angle; the
// Matrix values below are exactly that rotation (PDF `cm` operator
// convention, columns are where the unit basis vectors map to).
type rotationPlan struct {
	corner contentCorner
	swap   bool
	matrix [6]float64
}

var rotationTable = map[int]map[Corner]rotationPlan{
	0: {
		BottomRight: {ccBottomRight, false, [6]float64{1, 0, 0, 1, 0, 0}},
		BottomLeft:  {ccBottomLeft, false, [6]float64{1, 0, 0, 1, 0, 0}},
		TopRight:    {ccTopRight, false, [6]float64{1, 0, 0, 1, 0, 0}},
		TopLeft:     {ccTopLeft, false, [6]float64{1, 0, 0, 1, 0, 0}},
	},
	90: {
		BottomRight: {ccTopRight, true, [6]float64{0, 1, -1, 0, 0, 0}},
		BottomLeft:  {ccBottomRight, true, [6]float64{0, 1, -1, 0, 0, 0}},
		TopRight:    {ccTopLeft, true, [6]float64{0, 1, -1, 0, 0, 0}},
		TopLeft:     {ccBottomLeft, true, [6]float64{0, 1, -1, 0, 0, 0}},
	},
	180: {
		BottomRight: {ccTopLeft, false, [6]float64{-1, 0, 0, -1, 0, 0}},
		BottomLeft:  {ccTopRight, false, [6]float64{-1, 0, 0, -1, 0, 0}},
		TopRight:    {ccBottomLeft, false, [6]float64{-1, 0, 0, -1, 0, 0}},
		TopLeft:     {ccBottomRight, false, [6]float64{-1, 0, 0, -1, 0, 0}},
	},
	270: {
		BottomRight: {ccBottomLeft, true, [6]float64{0, -1, 1, 0, 0, 0}},
		BottomLeft:  {ccTopLeft, true, [6]float64{0, -1, 1, 0, 0, 0}},
		TopRight:    {ccBottomRight, true, [6]float64{0, -1, 1, 0, 0, 0}},
		TopLeft:     {ccTopRight, true, [6]float64{0, -1, 1, 0, 0, 0}},
	},
}

// PlaceCorner computes the widget /Rect (in the page's own content-space
// coordinates, box) and the Form XObject /Matrix for a stamp of
// stampW×stampH anchored at corner, inset by margin, on a page whose
// /Rotate is rotate (normalised to 0/90/180/270 by the caller). Both
// results are ready to use as-is: Rect always lands inside box, and
// Matrix is what makes the stamp display upright regardless of rotate.
func PlaceCorner(box [4]float64, rotate int, corner Corner, margin, stampW, stampH float64) (rect [4]float64, matrix [6]float64, err error) {
	plan, ok := rotationTable[rotate][corner]
	if !ok {
		return rect, matrix, fmt.Errorf("appearance: unsupported /Rotate value %d (must be 0, 90, 180 or 270)", rotate)
	}
	w, h := stampW, stampH
	if plan.swap {
		w, h = stampH, stampW
	}

	x0, y0, x1, y1 := box[0], box[1], box[2], box[3]
	switch plan.corner {
	case ccBottomLeft:
		rect = [4]float64{x0 + margin, y0 + margin, x0 + margin + w, y0 + margin + h}
	case ccBottomRight:
		rect = [4]float64{x1 - margin - w, y0 + margin, x1 - margin, y0 + margin + h}
	case ccTopLeft:
		rect = [4]float64{x0 + margin, y1 - margin - h, x0 + margin + w, y1 - margin}
	case ccTopRight:
		rect = [4]float64{x1 - margin - w, y1 - margin - h, x1 - margin, y1 - margin}
	}
	return rect, plan.matrix, nil
}

// RotationMatrix is the Form XObject /Matrix that counter-rotates a
// stamp's own content so it displays upright on a page whose /Rotate is
// rotate. It is what PlaceCorner returns alongside a corner placement,
// exposed on its own for the explicit-coordinate path, which needs the
// same counter-rotation and used to be given the identity — drawing the
// stamp on its side on any rotated page.
func RotationMatrix(rotate int) [6]float64 {
	if plan, ok := rotationTable[NormaliseRotate(rotate)][BottomRight]; ok {
		return plan.matrix
	}
	return [6]float64{1, 0, 0, 1, 0, 0}
}

// SwapsFootprint reports whether a stamp's width and height exchange
// places in the page's own coordinates — which they do on a page
// displayed a quarter turn from its content, because the stamp is drawn
// upright as displayed.
func SwapsFootprint(rotate int) bool {
	r := NormaliseRotate(rotate)
	return r == 90 || r == 270
}

// NormaliseRotate reduces an arbitrary /Rotate value (PDF permits any
// multiple of 90, including negative ones) to 0/90/180/270.
func NormaliseRotate(rotate int) int {
	r := rotate % 360
	if r < 0 {
		r += 360
	}
	return r
}

// ClampToPageBox moves a stamp of stampW×stampH, requested at (x, y),
// so that it sits entirely inside box with at least margin clear of
// every edge — and returns whether it had to move.
//
// F6 §6 states the rule and the reason: "clamp them into the page box
// less the margin rather than rejecting them — a stamp nudged inside is
// better than a refusal, and a stamp hanging off the page is not
// acceptable output." Refusing would turn a slightly-wrong coordinate
// into a failed batch; drawing it where it was asked would produce a
// document with a signature appearance half over the edge, which is
// what the original Bridge would not allow either.
//
// A page too small to hold the stamp and both margins is the one case
// where the margin cannot be honoured on both sides. The stamp is then
// pinned to the bottom-left inset rather than centred or shrunk: the
// stamp's own size is fixed (SPEC §13.1), so something has to give, and
// a predictable corner is easier to reason about than a stamp that
// silently changed size.
func ClampToPageBox(box [4]float64, x, y, stampW, stampH, margin float64) (cx, cy float64, moved bool) {
	minX, minY := box[0]+margin, box[1]+margin
	maxX, maxY := box[2]-margin-stampW, box[3]-margin-stampH

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
