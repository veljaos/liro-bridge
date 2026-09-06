package render

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// MaxRenderPixels bounds one rendered page. A4 at 400 percent is about
// eight megapixels; the ceiling is high enough that no zoom step this
// project offers on a page of a plausible size reaches it, and low
// enough that a document declaring a two-metre page cannot ask for
// several gigabytes of image.
const MaxRenderPixels = 40_000_000

// Document is a PDF opened for preview.
type Document struct {
	doc   *pdf.Document
	pages int
}

// Page describes one page's geometry, in PDF points.
type Page struct {
	// Number is the 1-based page number.
	Number int

	// Box is the page's /MediaBox in its own content-space coordinates.
	// It is the box the preview shows and the box a stamp's coordinates
	// are measured in, deliberately the same one: what is on screen is
	// exactly the area a placed stamp can occupy.
	Box [4]float64

	// Rotate is the page's /Rotate, normalised to 0, 90, 180 or 270.
	Rotate int

	// WidthPt and HeightPt are the page's size *as displayed*, so they
	// are the swapped dimensions on a page rotated a quarter turn.
	WidthPt, HeightPt float64
}

// Open parses data for previewing. An encrypted document is refused
// here exactly as it is for signing, with the same error code.
func Open(data []byte) (*Document, error) {
	doc, err := pdf.Parse(data)
	if err != nil {
		return nil, err
	}
	n, err := doc.PageCount()
	if err != nil {
		return nil, err
	}
	return &Document{doc: doc, pages: n}, nil
}

func (d *Document) PageCount() int { return d.pages }

// PageInfo reads one page's box and rotation. Both are read per page:
// a document may mix page sizes and orientations, and F6b §2.2 requires
// that the second page of a document is measured rather than assumed to
// match the first.
func (d *Document) PageInfo(n int) (Page, error) {
	dict, err := d.doc.PageDict(n)
	if err != nil {
		return Page{}, err
	}
	box := pdf.ResolveMediaBox(d.doc, dict)
	rotate := normaliseRotate(pdf.ResolveRotate(d.doc, dict))
	w, h := box[2]-box[0], box[3]-box[1]
	if rotate == 90 || rotate == 270 {
		w, h = h, w
	}
	return Page{Number: n, Box: box, Rotate: rotate, WidthPt: w, HeightPt: h}, nil
}

func normaliseRotate(r int) int {
	r %= 360
	if r < 0 {
		r += 360
	}
	switch r {
	case 90, 180, 270:
		return r
	}
	return 0
}

// PageMatrix is the transform from a page's own content space to device
// pixels, at scale pixels per point, with the page's /Rotate applied so
// that the result is oriented the way a reader shows it.
//
// This is the single place the screen and the document agree on where
// things are. Everything the placement window does — reading a drag,
// drawing the stamp, clamping to the margin — is this matrix or its
// inverse, so there is one derivation to get right rather than one per
// feature.
func PageMatrix(p Page, scale float64) [6]float64 {
	return pageMatrix(p, scale)
}

func pageMatrix(p Page, scale float64) matrix {
	x0, y0, x1, y1 := p.Box[0], p.Box[1], p.Box[2], p.Box[3]
	switch p.Rotate {
	case 90:
		return matrix{0, scale, scale, 0, -y0 * scale, -x0 * scale}
	case 180:
		return matrix{-scale, 0, 0, scale, x1 * scale, -y0 * scale}
	case 270:
		return matrix{0, -scale, -scale, 0, y1 * scale, x1 * scale}
	default:
		return matrix{scale, 0, 0, -scale, -x0 * scale, y1 * scale}
	}
}

// Result is a rendered page and what could not be drawn on it.
type Result struct {
	Image *image.RGBA
	Page  Page

	// Notes names anything the renderer could not draw faithfully, with
	// how many times each happened. It is diagnostic, English, and for
	// the log — never shown to a signer.
	Notes map[string]int
}

// RenderPage draws page n at scale pixels per point.
func (d *Document) RenderPage(n int, scale float64) (Result, error) {
	info, err := d.PageInfo(n)
	if err != nil {
		return Result{}, err
	}
	if scale <= 0 {
		return Result{}, fmt.Errorf("render: scale must be positive")
	}
	w := int(math.Round(info.WidthPt * scale))
	h := int(math.Round(info.HeightPt * scale))
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w*h > MaxRenderPixels {
		return Result{}, fmt.Errorf("render: page %d at %.2f px/pt would be %d pixels, over the %d limit", n, scale, w*h, MaxRenderPixels)
	}

	dict, err := d.doc.PageDict(n)
	if err != nil {
		return Result{}, err
	}
	base := pageMatrix(info, scale)

	cv := newCanvas(w, h)
	r := &renderer{doc: d.doc, cv: cv, baseCTM: base}

	content, err := d.doc.PageContent(dict)
	if err == nil && len(content) > 0 {
		res, _ := d.doc.Inherited(dict, pdf.Name("Resources")).(pdf.Dict)
		r.run(content, res, newGState(base, fullClip(w, h)))
	} else if err != nil {
		r.note("unreadable page content")
	}

	r.drawAnnotations(dict, base, w, h)

	return Result{Image: cv.img, Page: info, Notes: r.notes}, nil
}

// drawAnnotations paints each annotation's normal appearance stream.
//
// They are drawn because they are part of what the reader shows: a
// document already carrying a signature shows that signature's stamp,
// and someone placing a second one needs to see the first rather than
// discover the overlap afterwards.
func (r *renderer) drawAnnotations(pageDict pdf.Dict, base matrix, w, h int) {
	annots, _ := r.doc.Resolve(pageDict.Get(pdf.Name("Annots"))).(pdf.Array)
	for _, a := range annots {
		dict, ok := r.doc.ResolveDict(a)
		if !ok {
			continue
		}
		switch dict.GetName(pdf.Name("Subtype")) {
		case "Link", "Popup":
			continue
		}
		if flags, ok := asInt(r.doc.Resolve(dict.Get(pdf.Name("F")))); ok {
			const hidden, noView = 2, 32
			if flags&hidden != 0 || flags&noView != 0 {
				continue
			}
		}
		stream := r.appearanceStream(dict)
		if stream == nil {
			continue
		}
		rect := floatArray(r.doc, dict.Get(pdf.Name("Rect")))
		if len(rect) != 4 {
			continue
		}
		rx0, ry0 := math.Min(rect[0], rect[2]), math.Min(rect[1], rect[3])
		rx1, ry1 := math.Max(rect[0], rect[2]), math.Max(rect[1], rect[3])
		if rx1-rx0 <= 0 || ry1-ry0 <= 0 {
			continue
		}

		fm := identity
		if m := floatArray(r.doc, stream.Dict.Get(pdf.Name("Matrix"))); len(m) == 6 {
			fm = matrix{m[0], m[1], m[2], m[3], m[4], m[5]}
		}
		bbox := floatArray(r.doc, stream.Dict.Get(pdf.Name("BBox")))
		fit := identity
		if len(bbox) == 4 {
			// PDF 32000-1 §12.5.5: transform the form's bounding box by
			// its matrix, then map the result onto the annotation's
			// rectangle. Skipping this draws every stamp at its author's
			// original size and position rather than where the
			// annotation says it is.
			bx0, by0 := math.Inf(1), math.Inf(1)
			bx1, by1 := math.Inf(-1), math.Inf(-1)
			for _, c := range [][2]float64{{bbox[0], bbox[1]}, {bbox[2], bbox[1]}, {bbox[2], bbox[3]}, {bbox[0], bbox[3]}} {
				x, y := fm.apply(c[0], c[1])
				bx0, by0 = math.Min(bx0, x), math.Min(by0, y)
				bx1, by1 = math.Max(bx1, x), math.Max(by1, y)
			}
			sx, sy := 1.0, 1.0
			if bx1-bx0 > 1e-9 {
				sx = (rx1 - rx0) / (bx1 - bx0)
			}
			if by1-by0 > 1e-9 {
				sy = (ry1 - ry0) / (by1 - by0)
			}
			fit = matrix{sx, 0, 0, sy, rx0 - bx0*sx, ry0 - by0*sy}
		}

		gs := newGState(fm.mul(fit).mul(base), fullClip(w, h))
		if len(bbox) == 4 {
			gs.clip = clipToUserRect(r.cv, gs.clip, gs.ctm, bbox)
		}
		content, err := r.doc.DecodeStream(stream)
		if err != nil {
			continue
		}
		res, _ := r.doc.Resolve(stream.Dict.Get(pdf.Name("Resources"))).(pdf.Dict)
		r.depth++
		r.run(content, res, gs)
		r.depth--
	}
}

// appearanceStream picks an annotation's normal appearance, following
// /AS when /AP /N is a dictionary of states.
func (r *renderer) appearanceStream(dict pdf.Dict) *pdf.Stream {
	ap, ok := r.doc.Resolve(dict.Get(pdf.Name("AP"))).(pdf.Dict)
	if !ok {
		return nil
	}
	switch n := r.doc.Resolve(ap.Get(pdf.Name("N"))).(type) {
	case *pdf.Stream:
		return n
	case pdf.Dict:
		if as := dict.GetName(pdf.Name("AS")); as != "" {
			if s, ok := r.doc.Resolve(n.Get(as)).(*pdf.Stream); ok {
				return s
			}
		}
		// No /AS: take the one state there is, and be deterministic
		// about which when there are several.
		keys := make([]string, 0, len(n))
		for k := range n {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		for _, k := range keys {
			if s, ok := r.doc.Resolve(n.Get(pdf.Name(k))).(*pdf.Stream); ok {
				return s
			}
		}
	}
	return nil
}
