package render

import (
	"math"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// shading is an axial or radial gradient, prepared for sampling: the
// colour function is evaluated once into a 256-entry ramp rather than
// per pixel, because a stitching function over sampled sub-functions
// costs more than the rest of the page put together if it is asked a
// million times.
type shading struct {
	kind   int
	coords []float64
	ext0   bool
	ext1   bool
	inv    matrix
	ramp   [256][3]float32
	// background is what lies outside the shading when it is not
	// extended; ok is false when nothing should be painted there.
	bgOK bool
	bg   [3]float32
}

const rampSize = 256

func (r *renderer) buildShading(obj pdf.Object, res pdf.Dict, ctm matrix) (*shading, bool) {
	var dict pdf.Dict
	switch v := r.doc.Resolve(obj).(type) {
	case pdf.Dict:
		dict = v
	case *pdf.Stream:
		dict = v.Dict
	default:
		return nil, false
	}

	kind, _ := asInt(r.doc.Resolve(dict.Get(pdf.Name("ShadingType"))))
	cs := parseColorSpace(r.doc, dict.Get(pdf.Name("ColorSpace")), res, 0)
	fn, err := parseFunction(r.doc, dict.Get(pdf.Name("Function")))

	sh := &shading{kind: kind, coords: floatArray(r.doc, dict.Get(pdf.Name("Coords")))}
	if bg := floatArray(r.doc, dict.Get(pdf.Name("Background"))); len(bg) > 0 {
		br, bgc, bb := cs.toRGB(bg)
		sh.bg = [3]float32{br, bgc, bb}
		sh.bgOK = true
	}
	if ext, ok := r.doc.Resolve(dict.Get(pdf.Name("Extend"))).(pdf.Array); ok && len(ext) == 2 {
		sh.ext0, _ = r.doc.Resolve(ext[0]).(bool)
		sh.ext1, _ = r.doc.Resolve(ext[1]).(bool)
	}

	dom := floatArray(r.doc, dict.Get(pdf.Name("Domain")))
	t0, t1 := 0.0, 1.0
	if len(dom) >= 2 {
		t0, t1 = dom[0], dom[1]
	}
	for i := 0; i < rampSize; i++ {
		t := t0 + (t1-t0)*float64(i)/float64(rampSize-1)
		var comps []float64
		if err == nil {
			comps = fn.eval([]float64{t})
		}
		if len(comps) == 0 {
			// No usable function: a flat mid grey, which at least keeps
			// the shape of the page rather than punching a hole in it.
			sh.ramp[i] = [3]float32{0.5, 0.5, 0.5}
			continue
		}
		cr, cg, cb := cs.toRGB(comps)
		sh.ramp[i] = [3]float32{cr, cg, cb}
	}

	inv, ok := ctm.invert()
	if !ok {
		return nil, false
	}
	sh.inv = inv

	switch kind {
	case 2:
		if len(sh.coords) < 4 {
			return nil, false
		}
	case 3:
		if len(sh.coords) < 6 {
			return nil, false
		}
	default:
		// Function-based (1) and the mesh types (4-7) are painted as
		// the middle of their own colour ramp. A mesh shading is a
		// picture in its own right; approximating it as flat colour
		// keeps the page readable and is honest about what it is.
		r.note("shading type approximated as flat colour")
		sh.kind = 0
	}
	return sh, true
}

func (sh *shading) at(x, y int) (float32, float32, float32, float32) {
	if sh.kind == 0 {
		c := sh.ramp[rampSize/2]
		return c[0], c[1], c[2], 1
	}
	ux, uy := sh.inv.apply(float64(x)+0.5, float64(y)+0.5)
	var s float64
	var ok bool
	if sh.kind == 2 {
		s, ok = sh.axialParam(ux, uy)
	} else {
		s, ok = sh.radialParam(ux, uy)
	}
	if !ok {
		if sh.bgOK {
			return sh.bg[0], sh.bg[1], sh.bg[2], 1
		}
		return 0, 0, 0, 0
	}
	i := int(clampFloat(s, 0, 1) * float64(rampSize-1))
	c := sh.ramp[i]
	return c[0], c[1], c[2], 1
}

func (sh *shading) axialParam(x, y float64) (float64, bool) {
	x0, y0, x1, y1 := sh.coords[0], sh.coords[1], sh.coords[2], sh.coords[3]
	dx, dy := x1-x0, y1-y0
	den := dx*dx + dy*dy
	if den == 0 {
		return 0, true
	}
	s := ((x-x0)*dx + (y-y0)*dy) / den
	if s < 0 && !sh.ext0 {
		return 0, false
	}
	if s > 1 && !sh.ext1 {
		return 0, false
	}
	return s, true
}

// radialParam solves for the largest s whose circle passes through the
// point, which is the definition PDF 32000-1 §8.7.4.5.4 gives.
func (sh *shading) radialParam(px, py float64) (float64, bool) {
	x0, y0, r0 := sh.coords[0], sh.coords[1], sh.coords[2]
	x1, y1, r1 := sh.coords[3], sh.coords[4], sh.coords[5]
	dx, dy, dr := x1-x0, y1-y0, r1-r0
	fx, fy := px-x0, py-y0

	a := dx*dx + dy*dy - dr*dr
	b := 2 * (fx*dx + fy*dy + r0*dr)
	c := fx*fx + fy*fy - r0*r0

	var candidates []float64
	if math.Abs(a) < 1e-9 {
		if math.Abs(b) < 1e-12 {
			return 0, false
		}
		candidates = []float64{c / b}
	} else {
		disc := b*b - 4*a*c
		if disc < 0 {
			return 0, false
		}
		sq := math.Sqrt(disc)
		candidates = []float64{(b + sq) / (2 * a), (b - sq) / (2 * a)}
	}
	best := math.Inf(-1)
	found := false
	for _, s := range candidates {
		if r0+s*dr < 0 {
			continue
		}
		t := s
		if t < 0 {
			if !sh.ext0 {
				continue
			}
			t = 0
		}
		if t > 1 {
			if !sh.ext1 {
				continue
			}
			t = 1
		}
		if s > best {
			best = t
			found = true
		}
	}
	if !found {
		return 0, false
	}
	return best, true
}

// drawShading paints the whole clip region with a shading — the "sh"
// operator, which is a fill of everything currently visible.
func (r *renderer) drawShading(gs gstate, obj pdf.Object, res pdf.Dict, clip *clipRegion) {
	if obj == nil || clip.empty() {
		return
	}
	sh, ok := r.buildShading(obj, res, gs.ctm)
	if !ok {
		return
	}
	p := &pathBuilder{}
	p.reset(identity)
	p.rect(float64(clip.x0), float64(clip.y0), float64(clip.x1-clip.x0), float64(clip.y1-clip.y0))
	r.cv.fill(p, false, clip, paint{sample: sh.at}, gs.fillAlpha)
}

// patternPaint turns a pattern colour into something a fill can use.
//
// A shading pattern becomes the gradient it names. A tiling pattern
// becomes a flat mid grey: drawing the tile properly means running its
// content stream once per cell across the fill, and a hatch or a
// texture rendered as the tone it averages to is enough to see that
// something is there and where its edges are, which is what a placement
// preview is for.
func (r *renderer) patternPaint(obj pdf.Object, res pdf.Dict, gs gstate) paint {
	if obj == nil {
		return solidPaint(0.5, 0.5, 0.5)
	}
	var dict pdf.Dict
	switch v := r.doc.Resolve(obj).(type) {
	case pdf.Dict:
		dict = v
	case *pdf.Stream:
		dict = v.Dict
	default:
		return solidPaint(0.5, 0.5, 0.5)
	}

	pm := identity
	if m := floatArray(r.doc, dict.Get(pdf.Name("Matrix"))); len(m) == 6 {
		pm = matrix{m[0], m[1], m[2], m[3], m[4], m[5]}
	}
	if pt, _ := asInt(r.doc.Resolve(dict.Get(pdf.Name("PatternType")))); pt == 2 {
		// A pattern's matrix is relative to the default space of the
		// page, not to the CTM in force where it is used.
		if sh, ok := r.buildShading(dict.Get(pdf.Name("Shading")), res, pm.mul(r.baseCTM)); ok {
			return paint{sample: sh.at}
		}
	}
	r.note("tiling pattern approximated as flat colour")
	return solidPaint(0.5, 0.5, 0.5)
}
