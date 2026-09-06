package render

import (
	"image"
	"math"
)

// clipRegion is the area painting is confined to. Almost every clip in a
// real document is an axis-aligned rectangle — a page box, a table cell,
// a picture frame — so the rectangle is the representation, and a mask
// is allocated only for the paths that are not one. The mask covers the
// clip's own bounding box rather than the whole page, so a clipped logo
// costs the logo's area and not eight megapixels.
type clipRegion struct {
	x0, y0, x1, y1 int // device pixels, x1/y1 exclusive
	mask           []float32
	stride         int
}

func fullClip(w, h int) *clipRegion { return &clipRegion{x0: 0, y0: 0, x1: w, y1: h} }

func (c *clipRegion) empty() bool { return c.x1 <= c.x0 || c.y1 <= c.y0 }

// at returns how much of pixel (x, y) is inside the clip, 0..1.
func (c *clipRegion) at(x, y int) float32 {
	if x < c.x0 || x >= c.x1 || y < c.y0 || y >= c.y1 {
		return 0
	}
	if c.mask == nil {
		return 1
	}
	return c.mask[(y-c.y0)*c.stride+(x-c.x0)]
}

// intersectRect narrows the clip to a device rectangle.
func (c *clipRegion) intersectRect(x0, y0, x1, y1 int) *clipRegion {
	n := &clipRegion{
		x0: maxInt(c.x0, x0), y0: maxInt(c.y0, y0),
		x1: minInt(c.x1, x1), y1: minInt(c.y1, y1),
	}
	if n.empty() {
		return &clipRegion{}
	}
	if c.mask == nil {
		return n
	}
	n.stride = n.x1 - n.x0
	n.mask = make([]float32, n.stride*(n.y1-n.y0))
	for y := n.y0; y < n.y1; y++ {
		for x := n.x0; x < n.x1; x++ {
			n.mask[(y-n.y0)*n.stride+(x-n.x0)] = c.at(x, y)
		}
	}
	return n
}

// canvas is the page image being drawn into, plus the one rasteriser
// every fill runs through.
type canvas struct {
	img  *image.RGBA
	ras  *rasterizer
	w, h int
}

func newCanvas(w, h int) *canvas {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// A page is white before anything is drawn on it. PDF has no
	// concept of a page colour, and every reader shows one.
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	return &canvas{img: img, ras: newRasterizer(w, h), w: w, h: h}
}

// paint says what colour to put down. A solid fill is the overwhelming
// majority and skips the per-pixel call; sampled is what an image or a
// shading uses.
type paint struct {
	r, g, b float32
	sample  func(x, y int) (r, g, b, a float32)
}

func solidPaint(r, g, b float32) paint { return paint{r: r, g: g, b: b} }

// fill rasterises the current path and composites it.
func (c *canvas) fill(p *pathBuilder, evenOdd bool, clip *clipRegion, pt paint, alpha float64) {
	if clip.empty() || alpha <= 0 {
		return
	}
	c.ras.reset()
	p.fillEdges(c.ras, identity)
	c.composite(evenOdd, clip, pt, alpha)
}

// stroke rasterises the pen sweep of the current path and composites it.
func (c *canvas) stroke(p *pathBuilder, width float64, capStyle int, clip *clipRegion, pt paint, alpha float64) {
	if clip.empty() || alpha <= 0 {
		return
	}
	c.ras.reset()
	p.strokeOutline(c.ras, width, capStyle)
	// A stroke's own overlapping pieces are unioned, never subtracted:
	// the nonzero rule is what makes that true (see strokeOutline).
	c.composite(false, clip, pt, alpha)
}

func (c *canvas) composite(evenOdd bool, clip *clipRegion, pt paint, alpha float64) {
	a0 := float32(clampFloat(alpha, 0, 1))
	c.ras.accumulate(evenOdd, func(row, col0 int, cov []float32) {
		if row < clip.y0 || row >= clip.y1 {
			return
		}
		for i, cv := range cov {
			if cv <= 0 {
				continue
			}
			x := col0 + i
			cl := clip.at(x, row)
			if cl <= 0 {
				continue
			}
			r, g, b := pt.r, pt.g, pt.b
			sa := float32(1)
			if pt.sample != nil {
				r, g, b, sa = pt.sample(x, row)
				if sa <= 0 {
					continue
				}
			}
			c.blend(x, row, r, g, b, cv*cl*a0*sa)
		}
	})
}

func (c *canvas) blend(x, y int, r, g, b, a float32) {
	if a <= 0 {
		return
	}
	if a > 1 {
		a = 1
	}
	i := c.img.PixOffset(x, y)
	pix := c.img.Pix
	inv := 1 - a
	pix[i+0] = clampByte(float32(pix[i+0])*inv + r*255*a)
	pix[i+1] = clampByte(float32(pix[i+1])*inv + g*255*a)
	pix[i+2] = clampByte(float32(pix[i+2])*inv + b*255*a)
	pix[i+3] = 0xff
}

func clampByte(v float32) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// clipTo intersects clip with the current path. A path that is an
// axis-aligned device rectangle narrows the rectangle; anything else
// builds a mask over its own bounding box.
func (c *canvas) clipTo(clip *clipRegion, p *pathBuilder, evenOdd bool) *clipRegion {
	x0f, y0f, x1f, y1f, ok := p.deviceBounds()
	if !ok {
		return &clipRegion{}
	}
	bx0, by0 := int(math.Floor(x0f)), int(math.Floor(y0f))
	bx1, by1 := int(math.Ceil(x1f)), int(math.Ceil(y1f))

	if p.isDeviceRect() {
		return clip.intersectRect(bx0, by0, bx1, by1)
	}

	n := &clipRegion{
		x0: maxInt(clip.x0, bx0), y0: maxInt(clip.y0, by0),
		x1: minInt(clip.x1, bx1), y1: minInt(clip.y1, by1),
	}
	if n.empty() {
		return &clipRegion{}
	}
	n.stride = n.x1 - n.x0
	n.mask = make([]float32, n.stride*(n.y1-n.y0))

	c.ras.reset()
	p.fillEdges(c.ras, identity)
	c.ras.accumulate(evenOdd, func(row, col0 int, cov []float32) {
		if row < n.y0 || row >= n.y1 {
			return
		}
		for i, cv := range cov {
			x := col0 + i
			if x < n.x0 || x >= n.x1 {
				continue
			}
			n.mask[(row-n.y0)*n.stride+(x-n.x0)] = cv * clip.at(x, row)
		}
	})
	return n
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
