package render

import (
	"bytes"
	"image"
	_ "image/jpeg" // DCTDecode
	"math"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// maxImagePixels bounds how large an image this renderer will decode.
// A page of a scanned contract is a few tens of megapixels at most; a
// document claiming more is either broken or trying to exhaust memory,
// and either way skipping the image is better than falling over with
// the window half drawn.
const maxImagePixels = 80_000_000

// rawImage is one decoded image XObject, kept in its source form and
// converted a pixel at a time. A scan is thirty megapixels; expanding it
// to floating-point RGBA up front would cost half a gigabyte to draw
// something six hundred pixels wide.
type rawImage struct {
	w, h   int
	bpc    int
	ncomp  int
	data   []byte
	rowLen int
	cs     *colorSpace
	decode []float64

	// goimg is set instead of data when a codec (JPEG) produced a
	// decoded picture directly.
	goimg image.Image

	isMask bool // an /ImageMask stencil: one bit, painted in the fill colour
}

func (im *rawImage) componentAt(col, row, c int) float64 {
	idx := (col*im.ncomp + c)
	switch im.bpc {
	case 8:
		p := row*im.rowLen + idx
		if p >= len(im.data) {
			return 0
		}
		return float64(im.data[p])
	case 16:
		p := row*im.rowLen + idx*2
		if p+1 >= len(im.data) {
			return 0
		}
		return float64(int(im.data[p])<<8 | int(im.data[p+1]))
	default: // 1, 2, 4
		bit := idx * im.bpc
		p := row*im.rowLen + bit/8
		if p >= len(im.data) {
			return 0
		}
		shift := 8 - im.bpc - bit%8
		mask := byte(1<<uint(im.bpc)) - 1
		return float64((im.data[p] >> uint(shift)) & mask)
	}
}

// rgbAt converts one source pixel to a screen colour.
func (im *rawImage) rgbAt(col, row int) (float32, float32, float32) {
	if im.goimg != nil {
		r, g, b, _ := im.goimg.At(col+im.goimg.Bounds().Min.X, row+im.goimg.Bounds().Min.Y).RGBA()
		return float32(r) / 65535, float32(g) / 65535, float32(b) / 65535
	}
	maxV := float64(int(1)<<uint(im.bpc) - 1)
	comps := make([]float64, im.ncomp)
	for c := 0; c < im.ncomp; c++ {
		v := im.componentAt(col, row, c)
		dmin, dmax := 0.0, 1.0
		if im.cs != nil && im.cs.kind == csIndexed {
			dmin, dmax = 0, maxV
		} else if im.cs != nil && im.cs.kind == csLab {
			switch c {
			case 0:
				dmin, dmax = 0, 100
			default:
				dmin, dmax = im.cs.labRange[(c-1)*2], im.cs.labRange[(c-1)*2+1]
			}
		}
		if len(im.decode) >= 2*(c+1) {
			dmin, dmax = im.decode[2*c], im.decode[2*c+1]
		}
		comps[c] = dmin + v*(dmax-dmin)/maxV
	}
	return im.cs.toRGB(comps)
}

// alphaAt is the stencil value of an /ImageMask: 1 where the mask paints.
func (im *rawImage) alphaAt(col, row int) float32 {
	v := im.componentAt(col, row, 0)
	on := v == 0 // sample 0 paints, per PDF 32000-1 §8.9.6.2
	if len(im.decode) >= 2 && im.decode[0] == 1 {
		on = !on
	}
	if on {
		return 1
	}
	return 0
}

func (r *renderer) decodeImage(stream *pdf.Stream, res pdf.Dict) (*rawImage, bool) {
	doc := r.doc
	d := stream.Dict
	get := func(long, short pdf.Name) pdf.Object {
		if v := d.Get(long); v != nil {
			return v
		}
		return d.Get(short)
	}

	w, _ := asInt(doc.Resolve(get("Width", "W")))
	h, _ := asInt(doc.Resolve(get("Height", "H")))
	if w <= 0 || h <= 0 || w*h > maxImagePixels {
		return nil, false
	}

	im := &rawImage{w: w, h: h, bpc: 8, ncomp: 1, cs: deviceGray}
	if v, ok := asInt(doc.Resolve(get("BitsPerComponent", "BPC"))); ok {
		im.bpc = v
	}
	im.decode = floatArray(doc, get("Decode", "D"))

	maskFlag := doc.Resolve(get("ImageMask", "IM"))
	if b, ok := maskFlag.(bool); ok && b {
		im.isMask = true
		im.bpc = 1
		im.ncomp = 1
	} else if csObj := get("ColorSpace", "CS"); csObj != nil {
		im.cs = parseColorSpace(doc, csObj, res, 0)
		im.ncomp = im.cs.n
		if im.cs.kind == csIndexed {
			im.ncomp = 1
		}
	}

	data, codec, _, err := doc.DecodeImageStream(stream)
	if err != nil {
		r.note("unreadable image")
		return nil, false
	}
	switch codec {
	case "":
	case "DCTDecode", "DCT":
		img, _, decErr := image.Decode(bytes.NewReader(data))
		if decErr != nil {
			r.note("undecodable JPEG image")
			return nil, false
		}
		im.goimg = img
		if cm, ok := img.(*image.CMYK); ok && len(im.decode) >= 8 && im.decode[0] == 1 {
			// An Adobe-inverted CMYK JPEG carries /Decode [1 0 1 0 1 0 1 0];
			// image/jpeg has already undone the APP14 inversion, so
			// applying the array again would invert it back.
			im.decode = nil
			_ = cm
		}
		im.w, im.h = img.Bounds().Dx(), img.Bounds().Dy()
		return im, true
	case "CCITTFaxDecode", "CCF":
		parms, _ := doc.Resolve(d.Get(pdf.Name("DecodeParms"))).(pdf.Dict)
		if parms == nil {
			parms, _ = doc.Resolve(d.Get(pdf.Name("DP"))).(pdf.Dict)
		}
		bits, ccittErr := ccittDecode(data, doc, parms, w, h)
		if ccittErr != nil {
			r.note("undecodable CCITT image")
			return nil, false
		}
		data = bits
		im.bpc = 1
		im.ncomp = 1
		if !im.isMask {
			// ccittDecode produces the filter's own output, honouring
			// /BlackIs1 — so the image's colour space and /Decode
			// interpret it exactly as they would any other one-bit
			// stream, which is what makes a scan with /Decode [1 0]
			// come out the right way up.
			im.cs = deviceGray
		}
	default:
		r.note("image codec " + string(codec) + " not supported")
		return nil, false
	}

	im.data = data
	im.rowLen = (im.w*im.ncomp*im.bpc + 7) / 8
	if len(im.data) < im.rowLen*im.h {
		// A truncated image still draws the rows it has; componentAt
		// returns zero past the end.
		r.note("truncated image data")
	}
	return im, true
}

// drawImage paints an image XObject into the unit square under the CTM.
func (r *renderer) drawImage(gs gstate, stream *pdf.Stream, res pdf.Dict) {
	im, ok := r.decodeImage(stream, res)
	if !ok {
		return
	}
	smask := r.softMaskFor(stream, res)
	colourKey := colourKeyRanges(r.doc, stream.Dict)
	stencil := r.stencilMaskFor(stream, res)

	inv, ok := gs.ctm.invert()
	if !ok {
		return
	}

	// How many source pixels one device pixel covers, which decides how
	// finely to sample: a scan shrunk to fit a window has to be
	// averaged or it turns into moiré.
	sx := float64(im.w) / math.Max(1, math.Hypot(gs.ctm[0], gs.ctm[1]))
	sy := float64(im.h) / math.Max(1, math.Hypot(gs.ctm[2], gs.ctm[3]))
	k := int(math.Ceil(math.Max(sx, sy)))
	if k < 1 {
		k = 1
	}
	if k > 4 {
		k = 4
	}

	fr, fg, fb := gs.fillRGB()
	sample := func(x, y int) (float32, float32, float32, float32) {
		var rs, gsum, bs, as float32
		var n float32
		for j := 0; j < k; j++ {
			for i := 0; i < k; i++ {
				dx := (float64(i) + 0.5) / float64(k)
				dy := (float64(j) + 0.5) / float64(k)
				ux, uy := inv.apply(float64(x)+dx, float64(y)+dy)
				if ux < 0 || ux >= 1 || uy < 0 || uy >= 1 {
					continue
				}
				col := int(ux * float64(im.w))
				row := int((1 - uy) * float64(im.h))
				if col >= im.w {
					col = im.w - 1
				}
				if row >= im.h {
					row = im.h - 1
				}
				var pr, pg, pb, pa float32
				if im.isMask {
					pr, pg, pb = fr, fg, fb
					pa = im.alphaAt(col, row)
				} else {
					pr, pg, pb = im.rgbAt(col, row)
					pa = 1
					if colourKey != nil && colourKey.masks(im, col, row) {
						pa = 0
					}
				}
				if pa > 0 && smask != nil {
					pa *= smask.alphaFor(ux, uy)
				}
				if pa > 0 && stencil != nil {
					pa *= stencil.alphaFor(ux, uy)
				}
				rs += pr * pa
				gsum += pg * pa
				bs += pb * pa
				as += pa
				n++
			}
		}
		if n == 0 || as == 0 {
			return 0, 0, 0, 0
		}
		return rs / as, gsum / as, bs / as, as / n
	}

	p := &pathBuilder{}
	p.reset(gs.ctm)
	p.rect(0, 0, 1, 1)
	r.cv.fill(p, false, gs.clip, paint{sample: sample}, gs.fillAlpha)
}

// alphaSource samples a mask image in the drawn image's own unit square,
// which is what lets a soft mask of a different size line up with it.
type alphaSource struct {
	im     *rawImage
	invert bool
}

func (a *alphaSource) alphaFor(ux, uy float64) float32 {
	col := int(ux * float64(a.im.w))
	row := int((1 - uy) * float64(a.im.h))
	if col < 0 {
		col = 0
	}
	if row < 0 {
		row = 0
	}
	if col >= a.im.w {
		col = a.im.w - 1
	}
	if row >= a.im.h {
		row = a.im.h - 1
	}
	if a.invert {
		return 1 - a.im.alphaAt(col, row)
	}
	g, _, _ := a.im.rgbAt(col, row)
	return g
}

func (r *renderer) softMaskFor(stream *pdf.Stream, res pdf.Dict) *alphaSource {
	s, ok := r.doc.Resolve(stream.Dict.Get(pdf.Name("SMask"))).(*pdf.Stream)
	if !ok {
		return nil
	}
	im, ok := r.decodeImage(s, res)
	if !ok {
		return nil
	}
	return &alphaSource{im: im}
}

// stencilMaskFor handles /Mask given as a stencil image: its painted
// area is what is *hidden*, the opposite sense of a soft mask.
func (r *renderer) stencilMaskFor(stream *pdf.Stream, res pdf.Dict) *alphaSource {
	s, ok := r.doc.Resolve(stream.Dict.Get(pdf.Name("Mask"))).(*pdf.Stream)
	if !ok {
		return nil
	}
	im, ok := r.decodeImage(s, res)
	if !ok || !im.isMask {
		return nil
	}
	return &alphaSource{im: im, invert: true}
}

// colourKey is /Mask given as an array of component ranges: every pixel
// whose components all fall inside is transparent.
type colourKey struct{ ranges []float64 }

func colourKeyRanges(doc *pdf.Document, d pdf.Dict) *colourKey {
	arr := floatArray(doc, d.Get(pdf.Name("Mask")))
	if len(arr) < 2 {
		return nil
	}
	return &colourKey{ranges: arr}
}

func (c *colourKey) masks(im *rawImage, col, row int) bool {
	if im.goimg != nil {
		return false
	}
	for i := 0; i < im.ncomp && 2*i+1 < len(c.ranges); i++ {
		v := im.componentAt(col, row, i)
		if v < c.ranges[2*i] || v > c.ranges[2*i+1] {
			return false
		}
	}
	return true
}

// inlineImage handles BI ... ID ... EI, which carries its dictionary and
// its data inline in the content stream.
func (r *renderer) inlineImage(l *clexer, gs gstate, res pdf.Dict) {
	dict := pdf.Dict{}
	for {
		t := l.next()
		if t.kind == tokEOF {
			return
		}
		if t.kind == tokOperator {
			if t.op == "ID" {
				break
			}
			continue
		}
		key, ok := t.obj.(pdf.Name)
		if !ok {
			continue
		}
		v := l.next()
		if v.kind != tokObject {
			return
		}
		dict[key] = v.obj
	}
	// One whitespace byte separates ID from the data.
	if l.pos < len(l.data) && isWhite(l.data[l.pos]) {
		l.pos++
	}
	start := l.pos
	end := findInlineImageEnd(l.data, start)
	data := l.data[start:end]
	l.pos = end

	// Skip past the EI operator.
	for l.pos < len(l.data) {
		t := l.next()
		if t.kind == tokEOF || (t.kind == tokOperator && t.op == "EI") {
			break
		}
	}

	stream := &pdf.Stream{Dict: dict, Raw: data}
	r.drawImage(gs, stream, res)
}

// findInlineImageEnd looks for the EI that closes an inline image: a
// whitespace byte, "EI", and then whitespace or the end of the stream.
// Binary image data can contain those two letters, so the surrounding
// whitespace is what makes the match trustworthy.
func findInlineImageEnd(data []byte, start int) int {
	for i := start; i+2 < len(data); i++ {
		if !isWhite(data[i]) || data[i+1] != 'E' || data[i+2] != 'I' {
			continue
		}
		if i+3 >= len(data) || isWhite(data[i+3]) || isDelim(data[i+3]) {
			return i
		}
	}
	return len(data)
}
