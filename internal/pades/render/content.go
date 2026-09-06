package render

import (
	"math"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// gstate is the PDF graphics state, reduced to what this renderer acts
// on. Everything omitted (dash patterns, miter limit, rendering intent,
// blend modes, soft masks) is either invisible at preview scale or
// deliberately not implemented; see the package comment.
type gstate struct {
	ctm  matrix
	clip *clipRegion

	fillCS    *colorSpace
	strokeCS  *colorSpace
	fillCol   []float64
	strokeCol []float64

	fillPattern   pdf.Object
	strokePattern pdf.Object

	lineWidth   float64
	lineCap     int
	fillAlpha   float64
	strokeAlpha float64

	font        *pdfFont
	fontSize    float64
	charSpacing float64
	wordSpacing float64
	hscale      float64
	leading     float64
	rise        float64
	renderMode  int
}

func newGState(ctm matrix, clip *clipRegion) gstate {
	return gstate{
		ctm:         ctm,
		clip:        clip,
		fillCS:      deviceGray,
		strokeCS:    deviceGray,
		fillCol:     []float64{0},
		strokeCol:   []float64{0},
		lineWidth:   1,
		fillAlpha:   1,
		strokeAlpha: 1,
		hscale:      1,
	}
}

func (g gstate) fillRGB() (float32, float32, float32)   { return g.fillCS.toRGB(g.fillCol) }
func (g gstate) strokeRGB() (float32, float32, float32) { return g.strokeCS.toRGB(g.strokeCol) }

// maxOperators bounds how much work one page may cost. A preview that
// takes a minute is a window that looks hung, and a malformed or hostile
// document must not be able to hold the agent's UI thread. Reaching the
// limit stops drawing and keeps whatever was drawn so far — a partly
// drawn page is still a page a stamp can be placed on.
const maxOperators = 4_000_000

// maxFormDepth bounds nesting of form XObjects and Type 3 glyphs
// against a document whose forms refer to each other.
const maxFormDepth = 12

type renderer struct {
	doc   *pdf.Document
	cv    *canvas
	fonts map[int]*pdfFont
	ops   int
	depth int
	notes map[string]int // what could not be drawn, counted by reason

	// baseCTM is the page's own default transform. A pattern's matrix
	// is defined against it rather than against whatever CTM happens to
	// be in force where the pattern is used (PDF 32000-1 §8.7.3.1).
	baseCTM matrix
}

func (r *renderer) note(what string) {
	if r.notes == nil {
		r.notes = map[string]int{}
	}
	r.notes[what]++
}

func (r *renderer) run(content []byte, res pdf.Dict, gs gstate) {
	if r.depth > maxFormDepth {
		return
	}
	l := &clexer{data: content}
	var stack []pdf.Object
	var stateStack []gstate

	path := &pathBuilder{}
	path.reset(gs.ctm)
	pendingClip := 0 // 0 none, 1 nonzero, 2 even-odd

	// Text object state. BT/ET cannot nest, so one copy is enough.
	var tm, tlm matrix

	push := func(o pdf.Object) {
		if len(stack) < 64 {
			stack = append(stack, o)
		}
	}
	num := func(i int) float64 {
		// i counts back from the end: num(0) is the last operand.
		if i >= len(stack) {
			return 0
		}
		v, _ := asFloat(stack[len(stack)-1-i])
		return v
	}
	nameArg := func(i int) pdf.Name {
		if i >= len(stack) {
			return ""
		}
		n, _ := stack[len(stack)-1-i].(pdf.Name)
		return n
	}
	numbers := func() []float64 {
		out := make([]float64, 0, len(stack))
		for _, o := range stack {
			if v, ok := asFloat(o); ok {
				out = append(out, v)
			}
		}
		return out
	}

	endPath := func() {
		if pendingClip != 0 {
			gs.clip = r.cv.clipTo(gs.clip, path, pendingClip == 2)
			pendingClip = 0
		}
		path.reset(gs.ctm)
	}

	fillPaint := func() paint {
		if gs.fillCS != nil && gs.fillCS.kind == csPattern {
			return r.patternPaint(gs.fillPattern, res, gs)
		}
		fr, fg, fb := gs.fillRGB()
		return solidPaint(fr, fg, fb)
	}
	strokePaint := func() paint {
		if gs.strokeCS != nil && gs.strokeCS.kind == csPattern {
			return r.patternPaint(gs.strokePattern, res, gs)
		}
		sr, sg, sb := gs.strokeRGB()
		return solidPaint(sr, sg, sb)
	}

	for {
		if r.ops++; r.ops > maxOperators {
			r.note("operator budget exhausted")
			return
		}
		t := l.next()
		if t.kind == tokEOF {
			return
		}
		if t.kind == tokObject {
			push(t.obj)
			continue
		}

		switch t.op {
		case "q":
			stateStack = append(stateStack, gs)
		case "Q":
			if n := len(stateStack); n > 0 {
				gs = stateStack[n-1]
				stateStack = stateStack[:n-1]
				if path.empty() {
					path.ctm = gs.ctm
				}
			}
		case "cm":
			m := matrix{num(5), num(4), num(3), num(2), num(1), num(0)}
			gs.ctm = m.mul(gs.ctm)
			if path.empty() {
				path.ctm = gs.ctm
			}
		case "w":
			gs.lineWidth = num(0)
		case "J":
			gs.lineCap = int(num(0))
		case "gs":
			r.applyExtGState(&gs, res, nameArg(0))

		// --- path construction ---
		case "m":
			path.moveTo(num(1), num(0))
		case "l":
			path.lineTo(num(1), num(0))
		case "c":
			path.curveTo(num(5), num(4), num(3), num(2), num(1), num(0))
		case "v":
			path.curveTo(path.curX, path.curY, num(3), num(2), num(1), num(0))
		case "y":
			path.curveTo(num(3), num(2), num(1), num(0), num(1), num(0))
		case "h":
			path.closeSubpath()
		case "re":
			path.rect(num(3), num(2), num(1), num(0))

		// --- path painting ---
		case "n":
			endPath()
		case "f", "F", "f*":
			r.cv.fill(path, t.op == "f*", gs.clip, fillPaint(), gs.fillAlpha)
			endPath()
		case "S":
			r.cv.stroke(path, gs.lineWidth, gs.lineCap, gs.clip, strokePaint(), gs.strokeAlpha)
			endPath()
		case "s":
			path.closeSubpath()
			r.cv.stroke(path, gs.lineWidth, gs.lineCap, gs.clip, strokePaint(), gs.strokeAlpha)
			endPath()
		case "B", "B*":
			r.cv.fill(path, t.op == "B*", gs.clip, fillPaint(), gs.fillAlpha)
			r.cv.stroke(path, gs.lineWidth, gs.lineCap, gs.clip, strokePaint(), gs.strokeAlpha)
			endPath()
		case "b", "b*":
			path.closeSubpath()
			r.cv.fill(path, t.op == "b*", gs.clip, fillPaint(), gs.fillAlpha)
			r.cv.stroke(path, gs.lineWidth, gs.lineCap, gs.clip, strokePaint(), gs.strokeAlpha)
			endPath()
		case "W":
			pendingClip = 1
		case "W*":
			pendingClip = 2

		// --- colour ---
		case "g":
			gs.fillCS, gs.fillCol = deviceGray, []float64{num(0)}
		case "G":
			gs.strokeCS, gs.strokeCol = deviceGray, []float64{num(0)}
		case "rg":
			gs.fillCS, gs.fillCol = deviceRGB, []float64{num(2), num(1), num(0)}
		case "RG":
			gs.strokeCS, gs.strokeCol = deviceRGB, []float64{num(2), num(1), num(0)}
		case "k":
			gs.fillCS, gs.fillCol = deviceCMYK, []float64{num(3), num(2), num(1), num(0)}
		case "K":
			gs.strokeCS, gs.strokeCol = deviceCMYK, []float64{num(3), num(2), num(1), num(0)}
		case "cs":
			gs.fillCS = parseColorSpace(r.doc, nameArg(0), res, 0)
			gs.fillCol = gs.fillCS.initial()
			gs.fillPattern = nil
		case "CS":
			gs.strokeCS = parseColorSpace(r.doc, nameArg(0), res, 0)
			gs.strokeCol = gs.strokeCS.initial()
			gs.strokePattern = nil
		case "sc", "scn":
			if n := nameArg(0); n != "" && gs.fillCS != nil && gs.fillCS.kind == csPattern {
				gs.fillPattern = lookupRes(r.doc, res, "Pattern", n)
			}
			if v := numbers(); len(v) > 0 {
				gs.fillCol = v
			}
		case "SC", "SCN":
			if n := nameArg(0); n != "" && gs.strokeCS != nil && gs.strokeCS.kind == csPattern {
				gs.strokePattern = lookupRes(r.doc, res, "Pattern", n)
			}
			if v := numbers(); len(v) > 0 {
				gs.strokeCol = v
			}

		// --- text ---
		case "BT":
			tm, tlm = identity, identity
		case "ET":
		case "Tf":
			gs.fontSize = num(0)
			gs.font = r.loadFont(res, nameArg(1))
		case "Td":
			tlm = translate(num(1), num(0)).mul(tlm)
			tm = tlm
		case "TD":
			gs.leading = -num(0)
			tlm = translate(num(1), num(0)).mul(tlm)
			tm = tlm
		case "Tm":
			tlm = matrix{num(5), num(4), num(3), num(2), num(1), num(0)}
			tm = tlm
		case "T*":
			tlm = translate(0, -gs.leading).mul(tlm)
			tm = tlm
		case "TL":
			gs.leading = num(0)
		case "Tc":
			gs.charSpacing = num(0)
		case "Tw":
			gs.wordSpacing = num(0)
		case "Tz":
			gs.hscale = num(0) / 100
		case "Ts":
			gs.rise = num(0)
		case "Tr":
			gs.renderMode = int(num(0))
		case "Tj":
			if s, ok := lastString(stack); ok {
				tm = r.showText(gs, res, s, tm)
			}
		case "'":
			tlm = translate(0, -gs.leading).mul(tlm)
			tm = tlm
			if s, ok := lastString(stack); ok {
				tm = r.showText(gs, res, s, tm)
			}
		case "\"":
			gs.wordSpacing = num(2)
			gs.charSpacing = num(1)
			tlm = translate(0, -gs.leading).mul(tlm)
			tm = tlm
			if s, ok := lastString(stack); ok {
				tm = r.showText(gs, res, s, tm)
			}
		case "TJ":
			if len(stack) > 0 {
				if arr, ok := stack[len(stack)-1].(pdf.Array); ok {
					for _, e := range arr {
						switch v := e.(type) {
						case pdf.String:
							tm = r.showText(gs, res, v, tm)
						default:
							if adj, ok := asFloat(v); ok {
								tx := -adj / 1000 * gs.fontSize * gs.hscale
								tm = translate(tx, 0).mul(tm)
							}
						}
					}
				}
			}

		// --- XObjects, images, shadings ---
		case "Do":
			r.doXObject(gs, res, nameArg(0))
		case "BI":
			r.inlineImage(l, gs, res)
		case "sh":
			r.drawShading(gs, lookupRes(r.doc, res, "Shading", nameArg(0)), res, gs.clip)

		// --- deliberately ignored ---
		case "BMC", "BDC", "EMC", "MP", "DP", "BX", "EX", "d", "i", "j", "M", "ri", "d0", "d1":
		}
		stack = stack[:0]
	}
}

func lastString(stack []pdf.Object) (pdf.String, bool) {
	for i := len(stack) - 1; i >= 0; i-- {
		if s, ok := stack[i].(pdf.String); ok {
			return s, true
		}
	}
	return nil, false
}

// lookupRes finds name in one category of a resource dictionary.
func lookupRes(doc *pdf.Document, res pdf.Dict, category, name pdf.Name) pdf.Object {
	if res == nil || name == "" {
		return nil
	}
	table, _ := doc.Resolve(res.Get(category)).(pdf.Dict)
	if table == nil {
		return nil
	}
	return table.Get(name)
}

func (r *renderer) applyExtGState(gs *gstate, res pdf.Dict, name pdf.Name) {
	d, ok := r.doc.Resolve(lookupRes(r.doc, res, "ExtGState", name)).(pdf.Dict)
	if !ok {
		return
	}
	if v, ok := asFloat(r.doc.Resolve(d.Get(pdf.Name("LW")))); ok {
		gs.lineWidth = v
	}
	if v, ok := asFloat(r.doc.Resolve(d.Get(pdf.Name("CA")))); ok {
		gs.strokeAlpha = v
	}
	if v, ok := asFloat(r.doc.Resolve(d.Get(pdf.Name("ca")))); ok {
		gs.fillAlpha = v
	}
	if v, ok := asInt(r.doc.Resolve(d.Get(pdf.Name("LC")))); ok {
		gs.lineCap = v
	}
	if fnt, ok := r.doc.Resolve(d.Get(pdf.Name("Font"))).(pdf.Array); ok && len(fnt) == 2 {
		if f := r.loadFontObject(fnt[0]); f != nil {
			gs.font = f
		}
		if v, ok := asFloat(r.doc.Resolve(fnt[1])); ok {
			gs.fontSize = v
		}
	}
	// /SMask is not honoured: a luminosity soft mask needs the group to
	// be rendered off-screen first, and leaving it out draws the shape
	// at full strength rather than not at all. Recorded rather than
	// silently ignored.
	if sm := r.doc.Resolve(d.Get(pdf.Name("SMask"))); sm != nil {
		if n, ok := sm.(pdf.Name); !ok || n != "None" {
			r.note("soft mask ignored")
		}
	}
}

// doXObject draws a form or an image.
func (r *renderer) doXObject(gs gstate, res pdf.Dict, name pdf.Name) {
	obj := lookupRes(r.doc, res, "XObject", name)
	stream, ok := r.doc.Resolve(obj).(*pdf.Stream)
	if !ok {
		return
	}
	switch stream.Dict.GetName(pdf.Name("Subtype")) {
	case "Image":
		r.drawImage(gs, stream, res)
	case "Form":
		r.drawForm(gs, stream, res)
	}
}

func (r *renderer) drawForm(gs gstate, stream *pdf.Stream, parentRes pdf.Dict) {
	if r.depth > maxFormDepth {
		return
	}
	content, err := r.doc.DecodeStream(stream)
	if err != nil {
		r.note("unreadable form")
		return
	}
	sub := gs
	if m := floatArray(r.doc, stream.Dict.Get(pdf.Name("Matrix"))); len(m) == 6 {
		sub.ctm = matrix{m[0], m[1], m[2], m[3], m[4], m[5]}.mul(gs.ctm)
	}
	if bbox := floatArray(r.doc, stream.Dict.Get(pdf.Name("BBox"))); len(bbox) == 4 {
		sub.clip = clipToUserRect(r.cv, sub.clip, sub.ctm, bbox)
	}
	res, _ := r.doc.Resolve(stream.Dict.Get(pdf.Name("Resources"))).(pdf.Dict)
	if res == nil {
		res = parentRes
	}
	r.depth++
	r.run(content, res, sub)
	r.depth--
}

// clipToUserRect intersects a clip with a rectangle given in user space.
func clipToUserRect(cv *canvas, clip *clipRegion, ctm matrix, box []float64) *clipRegion {
	p := &pathBuilder{}
	p.reset(ctm)
	p.rect(math.Min(box[0], box[2]), math.Min(box[1], box[3]),
		math.Abs(box[2]-box[0]), math.Abs(box[3]-box[1]))
	return cv.clipTo(clip, p, false)
}
