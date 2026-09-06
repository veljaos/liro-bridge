package render

import "math"

type point struct{ x, y float64 }

type subpath struct {
	pts    []point
	closed bool
}

// pathBuilder collects a path in *user* space, together with the matrix
// in force while it was built.
//
// User space rather than device space, because stroking has to happen
// before the transform: a line width is a user-space quantity, and a
// page that scales x and y differently draws a stroked circle as an
// ellipse with a varying pen. Stroke the path where the width means
// what the document says it means, then transform the resulting
// outline.
type pathBuilder struct {
	subs []subpath
	ctm  matrix

	startX, startY float64
	curX, curY     float64
	open           bool
}

func (p *pathBuilder) reset(ctm matrix) {
	p.subs = p.subs[:0]
	p.ctm = ctm
	p.open = false
}

func (p *pathBuilder) empty() bool { return len(p.subs) == 0 }

func (p *pathBuilder) moveTo(x, y float64) {
	p.subs = append(p.subs, subpath{pts: []point{{x, y}}})
	p.startX, p.startY = x, y
	p.curX, p.curY = x, y
	p.open = true
}

func (p *pathBuilder) lineTo(x, y float64) {
	if !p.open {
		p.moveTo(x, y)
		return
	}
	s := &p.subs[len(p.subs)-1]
	s.pts = append(s.pts, point{x, y})
	p.curX, p.curY = x, y
}

// curveTo flattens a cubic Bézier into line segments, finely enough that
// the error is under half a device pixel.
func (p *pathBuilder) curveTo(x1, y1, x2, y2, x3, y3 float64) {
	if !p.open {
		p.moveTo(x1, y1)
	}
	x0, y0 := p.curX, p.curY
	n := bezierSegments(p.ctm, x0, y0, x1, y1, x2, y2, x3, y3)
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		u := 1 - t
		a := u * u * u
		b := 3 * u * u * t
		c := 3 * u * t * t
		d := t * t * t
		p.lineTo(a*x0+b*x1+c*x2+d*x3, a*y0+b*y1+c*y2+d*y3)
	}
}

// bezierSegments picks how many straight pieces a curve becomes, from
// the length of its control polygon *in device pixels* — so a curve
// zoomed to 400 percent gets four times the segments and stays smooth,
// and one drawn a millimetre across does not cost anything.
func bezierSegments(ctm matrix, x0, y0, x1, y1, x2, y2, x3, y3 float64) int {
	s := ctm.expansion()
	l := (dist(x0, y0, x1, y1) + dist(x1, y1, x2, y2) + dist(x2, y2, x3, y3)) * s
	n := int(math.Ceil(l / 2.5))
	if n < 1 {
		n = 1
	}
	if n > 600 {
		n = 600
	}
	return n
}

func dist(x0, y0, x1, y1 float64) float64 { return math.Hypot(x1-x0, y1-y0) }

func (p *pathBuilder) closeSubpath() {
	if len(p.subs) == 0 {
		return
	}
	s := &p.subs[len(p.subs)-1]
	s.closed = true
	p.curX, p.curY = p.startX, p.startY
}

func (p *pathBuilder) rect(x, y, w, h float64) {
	p.moveTo(x, y)
	p.lineTo(x+w, y)
	p.lineTo(x+w, y+h)
	p.lineTo(x, y+h)
	p.closeSubpath()
	// PDF 32000-1 §8.5.2.1: "re" leaves the current point at the
	// rectangle's origin, as if a new subpath had been started there.
	p.curX, p.curY = x, y
	p.startX, p.startY = x, y
}

// fillEdges feeds the path to the rasteriser as closed polygons, in
// device space. An unclosed subpath is closed implicitly, which is what
// filling means (PDF 32000-1 §8.5.3.3.2).
func (p *pathBuilder) fillEdges(r *rasterizer, extra matrix) {
	m := p.ctm.mul(extra)
	for _, s := range p.subs {
		if len(s.pts) < 2 {
			continue
		}
		px, py := m.apply(s.pts[0].x, s.pts[0].y)
		fx, fy := px, py
		for _, q := range s.pts[1:] {
			qx, qy := m.apply(q.x, q.y)
			r.line(px, py, qx, qy)
			px, py = qx, qy
		}
		r.line(px, py, fx, fy)
	}
}

// strokeOutline turns the path into the filled region a pen of width w
// sweeps along it, as a set of consistently-oriented polygons: one quad
// per segment, a disc at every join, and the requested cap at each end.
//
// Consistent orientation is what makes this correct rather than merely
// convenient: the pieces overlap, and the rasteriser unions overlapping
// polygons under the nonzero winding rule only when they all wind the
// same way. Two pieces wound oppositely would cancel and leave a hole
// down the middle of the stroke.
//
// Joins are always round. PDF's default is a miter join, and at the
// widths this renderer is asked to draw — table rules, underlines, box
// borders — a round join and a miter join differ by less than the pixel
// they are drawn into. A preview is allowed that; a signed document
// would not be.
func (p *pathBuilder) strokeOutline(r *rasterizer, w float64, capStyle int) {
	m := p.ctm
	half := w / 2
	// A pen thinner than about one device pixel is drawn as one device
	// pixel: PDF 32000-1 §8.4.3.2 gives width 0 that meaning explicitly,
	// and a 0.05pt table rule that vanished at Fit would be read as a
	// missing line rather than a thin one.
	if minHalf := 0.5 / m.expansion(); half < minHalf {
		half = minHalf
	}

	for _, s := range p.subs {
		pts := dedup(s.pts)
		if len(pts) == 1 {
			// A degenerate subpath draws a dot under a round cap and
			// nothing at all otherwise (PDF 32000-1 §8.4.3.3).
			if capStyle == capRound {
				emitDisc(r, m, pts[0], half)
			}
			continue
		}
		if s.closed && len(pts) > 1 {
			pts = append(pts, pts[0])
		}
		for i := 0; i+1 < len(pts); i++ {
			emitSegment(r, m, pts[i], pts[i+1], half)
		}
		// A disc at every interior vertex is the join; on a closed
		// subpath the meeting point of first and last is interior too.
		for i := 1; i+1 < len(pts); i++ {
			emitDisc(r, m, pts[i], half)
		}
		if s.closed {
			emitDisc(r, m, pts[0], half)
			continue
		}
		switch capStyle {
		case capRound:
			emitDisc(r, m, pts[0], half)
			emitDisc(r, m, pts[len(pts)-1], half)
		case capSquare:
			emitSquareCap(r, m, pts[1], pts[0], half)
			emitSquareCap(r, m, pts[len(pts)-2], pts[len(pts)-1], half)
		}
	}
}

const (
	capButt = iota
	capRound
	capSquare
)

func dedup(pts []point) []point {
	out := pts[:0:0]
	for i, q := range pts {
		if i > 0 && math.Abs(q.x-out[len(out)-1].x) < 1e-9 && math.Abs(q.y-out[len(out)-1].y) < 1e-9 {
			continue
		}
		out = append(out, q)
	}
	if len(out) == 0 {
		out = append(out, pts[0])
	}
	return out
}

// emitPolygon transforms a user-space polygon to device space and feeds
// it to the rasteriser, reversing it if necessary so that every polygon
// this file emits winds the same way.
func emitPolygon(r *rasterizer, m matrix, poly []point) {
	if len(poly) < 3 {
		return
	}
	dev := make([]point, len(poly))
	var area float64
	for i, q := range poly {
		x, y := m.apply(q.x, q.y)
		dev[i] = point{x, y}
	}
	for i := range dev {
		j := (i + 1) % len(dev)
		area += dev[i].x*dev[j].y - dev[j].x*dev[i].y
	}
	if area < 0 {
		for i, j := 0, len(dev)-1; i < j; i, j = i+1, j-1 {
			dev[i], dev[j] = dev[j], dev[i]
		}
	}
	for i := range dev {
		j := (i + 1) % len(dev)
		r.line(dev[i].x, dev[i].y, dev[j].x, dev[j].y)
	}
}

func emitSegment(r *rasterizer, m matrix, a, b point, half float64) {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	nx, ny := -dy/l*half, dx/l*half
	emitPolygon(r, m, []point{
		{a.x + nx, a.y + ny}, {b.x + nx, b.y + ny},
		{b.x - nx, b.y - ny}, {a.x - nx, a.y - ny},
	})
}

func emitSquareCap(r *rasterizer, m matrix, from, end point, half float64) {
	dx, dy := end.x-from.x, end.y-from.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return
	}
	ux, uy := dx/l*half, dy/l*half
	emitSegment(r, m, end, point{end.x + ux, end.y + uy}, half)
}

// emitDisc approximates a round join or cap. The number of sides is
// chosen from the radius in device pixels, so a hairline join costs a
// triangle and a thick one is smooth.
func emitDisc(r *rasterizer, m matrix, c point, radius float64) {
	steps := int(math.Ceil(radius * m.expansion() * 2))
	if steps < 6 {
		steps = 6
	}
	if steps > 64 {
		steps = 64
	}
	poly := make([]point, steps)
	for i := 0; i < steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		poly[i] = point{c.x + radius*math.Cos(a), c.y + radius*math.Sin(a)}
	}
	emitPolygon(r, m, poly)
}

// deviceBounds is the path's device-space bounding box, used to decide
// whether a clip is rectangular and to size an image's sampling loop.
func (p *pathBuilder) deviceBounds() (x0, y0, x1, y1 float64, ok bool) {
	first := true
	for _, s := range p.subs {
		for _, q := range s.pts {
			x, y := p.ctm.apply(q.x, q.y)
			if first {
				x0, y0, x1, y1 = x, y, x, y
				first = false
				continue
			}
			x0, y0 = math.Min(x0, x), math.Min(y0, y)
			x1, y1 = math.Max(x1, x), math.Max(y1, y)
		}
	}
	return x0, y0, x1, y1, !first
}

// isDeviceRect reports whether the path is a single axis-aligned
// rectangle in device space — the shape almost every clip actually is,
// and the one that needs no mask.
func (p *pathBuilder) isDeviceRect() bool {
	if len(p.subs) != 1 {
		return false
	}
	pts := dedup(p.subs[0].pts)
	if len(pts) != 4 {
		return false
	}
	dev := make([]point, 4)
	for i, q := range pts {
		x, y := p.ctm.apply(q.x, q.y)
		dev[i] = point{x, y}
	}
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		dx, dy := math.Abs(dev[i].x-dev[j].x), math.Abs(dev[i].y-dev[j].y)
		if dx > 1e-6 && dy > 1e-6 {
			return false
		}
	}
	return true
}
