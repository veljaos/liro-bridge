package render

import (
	"math"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// obliqueShear is the slant applied when a font is italic and the shapes
// had to be substituted: 12 degrees, the angle most oblique companions
// to an upright face use.
var obliqueShear = math.Tan(12 * math.Pi / 180)

// syntheticBoldEm is how much a substituted bold glyph is thickened, as
// a fraction of the em. Emboldening by stroking the outline is what
// every renderer does when it has an upright face and needs a bold one;
// three percent of the em is about the difference between a regular and
// a bold stem.
const syntheticBoldEm = 0.03

// showText draws one shown string and returns the text matrix after it.
func (r *renderer) showText(gs gstate, res pdf.Dict, s pdf.String, tm matrix) matrix {
	f := gs.font
	if f == nil {
		return tm
	}
	glyphs := f.decode(s)
	invisible := gs.renderMode == 3 || gs.renderMode == 7

	for _, g := range glyphs {
		w0 := g.width
		if f.type3 {
			// A Type 3 font's widths are in its own glyph space, which
			// /FontMatrix maps into text space — not the fixed
			// thousandth of an em every other font type uses.
			w0 = g.width * 1000 * f.fontMatrix[0]
		}

		if !invisible && gs.fontSize != 0 {
			trm := matrix{gs.fontSize * gs.hscale, 0, 0, gs.fontSize, 0, gs.rise}.mul(tm).mul(gs.ctm)
			r.drawGlyph(gs, res, f, g, trm)
		}

		tx := (w0*gs.fontSize + gs.charSpacing) * gs.hscale
		if g.nbytes == 1 && g.code == 32 {
			tx += gs.wordSpacing * gs.hscale
		}
		tm = translate(tx, 0).mul(tm)
	}
	return tm
}

func (r *renderer) drawGlyph(gs gstate, res pdf.Dict, f *pdfFont, g shownGlyph, trm matrix) {
	if f.type3 {
		r.drawType3Glyph(gs, res, f, g, trm)
		return
	}
	if f.prog == nil {
		return
	}
	gid := f.glyphFor(g)
	if gid <= 0 {
		return
	}

	// Font units to text space, then into place. A substituted italic is
	// sheared here rather than in the outline, so the shear composes
	// with whatever the page's own matrix already does.
	m := scaleMatrix(1/f.prog.upem, 1/f.prog.upem)
	if f.substitute && f.italic {
		m = m.mul(matrix{1, 0, obliqueShear, 1, 0, 0})
	}
	m = m.mul(trm)

	p := &pathBuilder{}
	p.reset(m)
	if !f.prog.appendGlyph(gid, p, identity, 0) {
		return
	}

	fr, fg, fb := gs.fillRGB()
	pt := solidPaint(fr, fg, fb)
	if gs.fillCS != nil && gs.fillCS.kind == csPattern {
		pt = r.patternPaint(gs.fillPattern, res, gs)
	}
	// Every text rendering mode that shows anything is drawn filled.
	// Outline-only text (mode 1) is a display effect; drawing it solid
	// puts the ink in the right place, which is what a page being
	// measured for a stamp needs.
	r.cv.fill(p, false, gs.clip, pt, gs.fillAlpha)

	if f.substitute && f.bold {
		r.cv.stroke(p, syntheticBoldEm*f.prog.upem, capRound, gs.clip, pt, gs.fillAlpha)
	}
}

// drawType3Glyph runs a Type 3 font's own content stream for one code.
// The glyph is a little page of its own, so this is the interpreter
// calling itself with the font matrix in place of the page matrix.
func (r *renderer) drawType3Glyph(gs gstate, res pdf.Dict, f *pdfFont, g shownGlyph, trm matrix) {
	if f.charProcs == nil || r.depth > maxFormDepth {
		return
	}
	name := f.t3Names[int(g.code)&0xff]
	if name == "" {
		return
	}
	stream, ok := r.doc.Resolve(f.charProcs.Get(name)).(*pdf.Stream)
	if !ok {
		return
	}
	content, err := r.doc.DecodeStream(stream)
	if err != nil {
		return
	}
	sub := gs
	sub.ctm = f.fontMatrix.mul(trm)
	sub.font = nil
	useRes := f.t3Res
	if useRes == nil {
		useRes = res
	}
	r.depth++
	r.run(content, useRes, sub)
	r.depth--
}
