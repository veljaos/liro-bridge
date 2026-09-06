package render

import "math"

// rasterizer turns closed polygons into per-pixel coverage, by the
// signed-area method: every edge deposits its signed vertical extent
// into the cells it crosses, split so that a running sum along a row
// yields, at each pixel, the (fractional) winding number there. One
// pass, no supersampling, and the antialiasing is exact area coverage
// rather than a sampled approximation of it — which matters because
// almost everything this renderer draws is small text.
//
// Coordinates are device pixels with y increasing downwards. Edges
// outside the buffer are clipped, except on the left, where an edge at
// x < 0 is folded onto column 0 so that a shape starting off-screen
// still fills the part of the row that is on-screen.
type rasterizer struct {
	w, h   int
	stride int // w+2: an edge at x == w deposits into column w+1
	acc    []float32

	// The bounding box of what the current path has touched, so that
	// accumulating and clearing cost the size of the shape rather than
	// the size of the page. A glyph is a few hundred pixels on a page
	// of eight million.
	minRow, maxRow int
	minCol, maxCol int
	dirty          bool

	// span is the reusable one-row coverage buffer accumulate fills.
	span []float32
}

func newRasterizer(w, h int) *rasterizer {
	r := &rasterizer{w: w, h: h, stride: w + 2}
	r.acc = make([]float32, r.stride*h)
	r.reset()
	return r
}

func (r *rasterizer) reset() {
	if r.dirty {
		for row := r.minRow; row <= r.maxRow; row++ {
			line := r.acc[row*r.stride : (row+1)*r.stride]
			for i := r.minCol; i <= r.maxCol+1 && i < len(line); i++ {
				line[i] = 0
			}
		}
	}
	r.minRow, r.maxRow = r.h, -1
	r.minCol, r.maxCol = r.w, -1
	r.dirty = false
}

// bounds returns the rows and columns the current path touched.
// ok is false when it touched nothing.
func (r *rasterizer) bounds() (row0, row1, col0, col1 int, ok bool) {
	if !r.dirty || r.maxRow < r.minRow || r.maxCol < r.minCol {
		return 0, 0, 0, 0, false
	}
	col1 = r.maxCol
	if col1 >= r.w {
		col1 = r.w - 1
	}
	if col1 < r.minCol {
		return 0, 0, 0, 0, false
	}
	return r.minRow, r.maxRow, r.minCol, col1, true
}

func (r *rasterizer) mark(row, col int) {
	if row < r.minRow {
		r.minRow = row
	}
	if row > r.maxRow {
		r.maxRow = row
	}
	if col < r.minCol {
		r.minCol = col
	}
	if col > r.maxCol {
		r.maxCol = col
	}
	r.dirty = true
}

// line accumulates one edge of a closed polygon.
func (r *rasterizer) line(ax, ay, bx, by float64) {
	if ay == by || math.IsNaN(ax) || math.IsNaN(ay) || math.IsNaN(bx) || math.IsNaN(by) {
		return
	}
	dir := float32(1)
	if ay > by {
		ax, ay, bx, by = bx, by, ax, ay
		dir = -1
	}
	h := float64(r.h)
	if by <= 0 || ay >= h {
		return
	}
	dxdy := (bx - ax) / (by - ay)
	if ay < 0 {
		ax += dxdy * -ay
		ay = 0
	}
	if by > h {
		// Only the end of the y range is clipped: the walk below starts
		// from (ax, ay) and steps by dxdy, so the far end's x never
		// needs computing.
		by = h
	}
	if ay >= by {
		return
	}

	x := ax
	y := ay
	row := int(y)
	if row >= r.h {
		row = r.h - 1
	}
	for y < by {
		rowEnd := float64(row + 1)
		if rowEnd > by {
			rowEnd = by
		}
		dy := rowEnd - y
		xEnd := x + dxdy*dy
		r.rowSpan(row, x, xEnd, float32(dy)*dir)
		y = rowEnd
		x = xEnd
		row++
		if row >= r.h {
			return
		}
	}
}

// rowSpan deposits one row's worth of an edge: the piece runs from x0 to
// x1 within row, and carries dy of signed vertical extent between them.
func (r *rasterizer) rowSpan(row int, x0, x1 float64, dy float32) {
	if dy == 0 {
		return
	}
	limit := float64(r.w)
	x0 = clampFloat(x0, 0, limit)
	x1 = clampFloat(x1, 0, limit)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	base := row * r.stride
	line := r.acc[base : base+r.stride]

	c0 := int(x0)
	c1 := int(x1)
	if c0 >= r.stride-1 {
		c0 = r.stride - 2
	}
	if c1 >= r.stride-1 {
		c1 = r.stride - 2
	}

	if c0 == c1 {
		// Wholly inside one pixel column.
		xm := (x0+x1)/2 - float64(c0)
		line[c0] += dy * float32(1-xm)
		line[c0+1] += dy * float32(xm)
		r.mark(row, c0)
		return
	}

	// Spanning several columns: split at the column boundaries and give
	// each piece its share of dy, proportional to its horizontal extent.
	invTotal := 1 / (x1 - x0)
	for c := c0; c <= c1; c++ {
		px0 := math.Max(x0, float64(c))
		px1 := math.Min(x1, float64(c+1))
		if px1 <= px0 {
			continue
		}
		share := dy * float32((px1-px0)*invTotal)
		xm := (px0+px1)/2 - float64(c)
		line[c] += share * float32(1-xm)
		line[c+1] += share * float32(xm)
	}
	r.mark(row, c0)
	r.mark(row, c1)
}

// spanFunc receives one row's coverage: cov[i] is the coverage of pixel
// (col0+i, row), already in 0..1.
type spanFunc func(row, col0 int, cov []float32)

// accumulate runs the prefix sum over every touched row and hands the
// resulting coverage to fn. evenOdd selects the even-odd fill rule
// rather than the nonzero winding rule.
func (r *rasterizer) accumulate(evenOdd bool, fn spanFunc) {
	row0, row1, col0, col1, ok := r.bounds()
	if !ok {
		return
	}
	n := col1 - col0 + 1
	if cap(r.span) < n {
		r.span = make([]float32, n)
	}
	buf := r.span[:n]

	for row := row0; row <= row1; row++ {
		line := r.acc[row*r.stride : (row+1)*r.stride]
		// The running sum starts at col0, not at column 0: nothing is
		// ever deposited to the left of the path's own leftmost
		// column, and an edge that runs off the left of the buffer is
		// folded onto column 0 by rowSpan precisely so that this holds.
		var sum float32
		any := false
		for i := 0; i < n; i++ {
			sum += line[col0+i]
			c := coverageOf(sum, evenOdd)
			buf[i] = c
			if c > 0 {
				any = true
			}
		}
		if any {
			fn(row, col0, buf)
		}
	}
}

// coverageOf turns a fractional winding number into coverage.
//
// Nonzero: anything at or past ±1 is fully inside, and the fractional
// part at an edge is the area coverage. Even-odd: the winding number
// folded into a triangle wave, so 0→0, 1→1, 2→0, which is what makes a
// hole punched by a second contour actually a hole.
func coverageOf(w float32, evenOdd bool) float32 {
	if w < 0 {
		w = -w
	}
	if evenOdd {
		w = float32(math.Mod(float64(w), 2))
		if w > 1 {
			w = 2 - w
		}
		return w
	}
	if w > 1 {
		return 1
	}
	return w
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	if math.IsNaN(v) {
		return lo
	}
	return v
}
