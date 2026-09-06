// Package render rasterises a PDF page to an image, for the visual
// stamp placement window (F6b §2.1).
//
// It is a *preview* renderer and nothing else. Nothing it produces is
// written to a signed document, compared against a golden file, or
// trusted by any validator — it exists so that a person can see the
// page they are putting a stamp on. That is what makes writing it from
// scratch a reasonable thing to do at all: unlike every other
// hand-written layer in this project, a bug here cannot make an invalid
// signature look valid. The worst it can do is draw a page wrongly,
// which is visible on sight to the one person who is looking at it.
//
// What it supports, and what it does instead when it cannot, is
// documented on each piece. The one rule it holds to everywhere: it
// never guesses geometry. Text may be drawn in a substituted typeface,
// but always at the position and the advance width the document itself
// specifies, because position is the whole point of the window this
// serves.
package render

import "math"

// matrix is a PDF transformation matrix [a b c d e f], the affine
// transform PDF 32000-1 §8.3.3 writes as
//
//	| a b 0 |
//	| c d 0 |
//	| e f 1 |
//
// with row vectors on the left, so composing m then n is m.mul(n).
type matrix [6]float64

var identity = matrix{1, 0, 0, 1, 0, 0}

// mul returns the transform that applies m first and then n.
func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2],
		m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2],
		m[2]*n[1] + m[3]*n[3],
		m[4]*n[0] + m[5]*n[2] + n[4],
		m[4]*n[1] + m[5]*n[3] + n[5],
	}
}

// apply transforms the point (x, y).
func (m matrix) apply(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

func translate(tx, ty float64) matrix { return matrix{1, 0, 0, 1, tx, ty} }
func scaleMatrix(sx, sy float64) matrix {
	return matrix{sx, 0, 0, sy, 0, 0}
}

// det is the determinant, whose absolute value is the area scale factor.
func (m matrix) det() float64 { return m[0]*m[3] - m[1]*m[2] }

// invert returns the inverse transform, and whether one exists. Used to
// map a device pixel back into an image's own unit square.
func (m matrix) invert() (matrix, bool) {
	d := m.det()
	if math.Abs(d) < 1e-12 {
		return identity, false
	}
	inv := 1 / d
	return matrix{
		m[3] * inv,
		-m[1] * inv,
		-m[2] * inv,
		m[0] * inv,
		(m[2]*m[5] - m[3]*m[4]) * inv,
		(m[1]*m[4] - m[0]*m[5]) * inv,
	}, true
}

// expansion is the average scale factor the matrix applies to a length,
// used to pick a curve-flattening tolerance and a minimum stroke width
// in device space.
func (m matrix) expansion() float64 {
	return math.Sqrt(math.Abs(m.det()))
}
