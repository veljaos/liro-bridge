// Command genicon produces the application icon from the same real Liro
// brand mark scripts/genlogo consumes for the PDF visual stamp:
// assets/signature-logo.js, a 256×256 turquoise (#038387) node graph
// exported as raw RGB and 8-bit alpha channels, already zlib-compressed
// (RFC 1950). That file is the only place the mark's pixels live.
//
// The icon is a solid turquoise rounded square with the mark in white
// inside it. It was previously the bare mark on a transparent ground,
// which failed in three measured ways: a transparent turquoise mark is
// invisible on a turquoise wallpaper; the mark's strokes are 1.9% of its
// own width, so on a light background the icon is a hairline; and at
// 16 px — the tray and Explorer's list view, where it is seen most —
// those strokes fall to a quarter of a pixel and the mark breaks into
// disconnected dots. A solid tile answers the first two, and the stroke
// ramp in frames answers the third. See docs/decisions.md.
//
// # The mark is redrawn, not resampled
//
// Every frame is drawn analytically at its own size from the geometry in
// markNodes/markEdges, supersampled and box-downsampled. It is not one
// master bitmap scaled eight ways, and that difference is the whole
// reason 16 px works: a stroke weight that is right at 256 px is a
// quarter of a pixel at 16, so each size is given its own. Dilating a
// master and scaling it was tried — it is what the owner's own mock-up
// did — and measured to hold together down to 32 px and turn to mush at
// 16.
//
// # One source of truth, kept honest by a check
//
// Hard-coding the geometry would put the mark in two places: the bitmap
// in the source file and the numbers here. verifyAgainstSource closes
// that by redrawing the mark from those numbers at the source's own scale
// and requiring the two to agree both ways round, so a change to
// assets/signature-logo.js that this generator no longer describes fails
// the build rather than silently producing an icon of a mark the product
// no longer uses.
//
// Usage:
//
//	go run ./scripts/genicon --out internal/ui/assets/icon.ico
package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"math"
	"os"
	"regexp"
)

// wantSize is the source asset's fixed geometry (assets/signature-logo.js:
// width: 256, height: 256) — checked against the source, not assumed,
// exactly like scripts/genlogo does.
const wantSize = 256

// iconSizes are every size Windows requests a tray/shell icon at across
// the DPI scalings this project's own window code already handles
// (internal/ui/win32_windows.go's ensureDPIAware, per-monitor-v2): the
// small-icon metric at 100%/125%/150%/200% DPI is 16/20/24/32, the
// large-icon (Alt+Tab, taskbar) metric at the same scalings is
// 32/40/48/64, and 256 is the size Explorer uses for large-icon views
// and what Windows' own icon format tops out at.
//
// It is derived from frames rather than written out separately, so the
// two cannot disagree about which sizes exist.
func iconSizes() []int {
	sizes := make([]int, len(frames))
	for i, f := range frames {
		sizes[i] = f.size
	}
	return sizes
}

// brandTurquoise is the mark's own colour, unchanged: the same #038387
// scripts/genlogo embeds in the PDF stamp and internal/ui/assets/tokens.css
// carries. The tile is filled with exactly this rather than with a
// darkened variant — see the note on the turquoise wallpaper below.
var brandTurquoise = color.NRGBA{R: 0x03, G: 0x83, B: 0x87, A: 0xFF}

// markColour is the mark, in white, inside the tile.
var markColour = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}

// markNodes are the node graph's nine junctions, normalised to the box
// their own centres span, x then y. Recovered from the source bitmap by
// eroding away the thin edges until only the fat junctions survived, and
// checked against that bitmap on every run (verifyAgainstSource).
//
// Note the alignment, which is the mark's own and is part of why it
// survives being drawn small: four nodes share the centre vertical
// (x≈0.493), two share the left (x≈0) and two the right (x≈1).
var markNodes = [9][2]float64{
	{0.49304, 0.00000}, // 0  top centre
	{0.76530, 0.18285}, // 1  upper right
	{0.00000, 0.33115}, // 2  upper left
	{0.49304, 0.35213}, // 3  centre upper
	{1.00000, 0.64802}, // 4  right
	{0.00055, 0.66290}, // 5  lower left
	{0.49304, 0.66225}, // 6  centre lower
	{0.49304, 0.99095}, // 7  bottom centre
	{0.99944, 1.00000}, // 8  bottom right
}

// markEdges are the eleven segments actually drawn.
//
// Five further pairs of nodes are joined by an unbroken line of ink in
// the source — 0-6, 0-7, 1-5, 2-8 and 3-7 — and every one of them is a
// composition that passes through an intermediate node rather than an
// edge in its own right. Drawing them would change no pixel and would
// hide that fact.
var markEdges = [11][2]int{
	{0, 1}, {0, 3}, {1, 3}, // the triangle, upper right
	{2, 5}, {2, 6}, {3, 5}, {3, 6}, // the bow tie, left
	{4, 7}, {4, 8}, {6, 7}, {6, 8}, // the bow tie, lower right
}

// The mark as it sits in the 256×256 source, measured. The node centres
// span this box; each node is a regular hexagon of this circumradius;
// each edge is this wide. These describe the source and are used only by
// verifyAgainstSource — the icon takes its own weights from frames.
const (
	srcOriginX    = 28.29
	srcOriginY    = 28.17
	srcSpanX      = 198.42
	srcSpanY      = 198.79
	srcNodeRadius = 7.2
	srcStroke     = 4.0
)

// frame is one icon size and the weights it is drawn with.
//
// markFrac and stroke ramp against one another deliberately, and the ramp
// is the design rather than a set of tweaks. A constant stroke-to-mark
// ratio cannot work across a 16→256 family: at the source's own 1.9% the
// mark is a hairline everywhere, and at a weight heavy enough to hold
// together at 16 px it is a blob at 256. So the stroke grows from 6.2% of
// the mark at 256 px to 10.2% at 16, and the mark gives a little of the
// tile back as it shrinks. Every number here was arrived at by rendering
// it and looking — at 1:1 and magnified, on white, dark and turquoise
// grounds — not by evaluating a formula.
type frame struct {
	size     int     // the frame's own size, in pixels
	markFrac float64 // the mark's ink bounding box, as a fraction of the tile
	stroke   float64 // edge width, in this frame's own pixels
}

var frames = []frame{
	{16, 0.800, 1.30},
	{20, 0.790, 1.52},
	{24, 0.785, 1.72},
	{32, 0.780, 2.10},
	{40, 0.775, 2.44},
	{48, 0.770, 2.78},
	{64, 0.760, 3.40},
	{256, 0.750, 12.00},
}

// nodeToStroke is a node's circumradius as a multiple of the edge width,
// held constant across the family so that the eight frames are one
// drawing at eight weights rather than eight drawings.
//
// It is 1.21 where the source mark's own ratio is 1.8 (a 7.2 radius on a
// 4.0 stroke). The strokes are thickened harder than the nodes on
// purpose: the strokes are what vanish at small sizes and the nodes are
// what survive, so keeping the source's ratio would have produced a mark
// of beads joined by threads. At 1.21 the junctions still read as the
// hexagons they are without bulging out of the lines they join.
const nodeToStroke = 1.21

// cornerFrac is the tile's corner radius as a fraction of its side.
const cornerFrac = 0.22

func main() {
	srcPath := flag.String("src", "assets/signature-logo.js", "path to the Liro logo source (assets/signature-logo.js)")
	outPath := flag.String("out", "internal/ui/assets/icon.ico", "output .ico path")
	flag.Parse()

	src, err := os.ReadFile(*srcPath)
	if err != nil {
		log.Fatal(err)
	}

	width := extractInt(src, "width")
	height := extractInt(src, "height")
	if width != wantSize || height != wantSize {
		log.Fatalf("genicon: %s declares %dx%d, want %dx%d", *srcPath, width, height, wantSize, wantSize)
	}

	alpha := inflate(extractBase64(src, "alphaFlateBase64"), wantSize*wantSize)
	if err := verifyAgainstSource(alpha); err != nil {
		log.Fatalf("genicon: the geometry no longer describes %s: %v", *srcPath, err)
	}

	pngFrames := make([][]byte, len(frames))
	for i, f := range frames {
		var buf bytes.Buffer
		if err := png.Encode(&buf, renderFrame(f)); err != nil {
			log.Fatalf("genicon: encoding %dx%d frame: %v", f.size, f.size, err)
		}
		pngFrames[i] = buf.Bytes()
	}

	sizes := iconSizes()
	icoBytes, err := encodeICO(sizes, pngFrames)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outPath, icoBytes, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("genicon: wrote %s (%d frames: %v, %d bytes)\n", *outPath, len(sizes), sizes, len(icoBytes))
}

// renderFrame draws one frame at its own size: the rounded tile filled
// with the brand turquoise, then the mark's edges as round-capped strokes
// and its nodes as hexagons, in white. Outside the tile the frame is left
// transparent, so the rounded corners are the icon's own shape.
//
// It is drawn at a multiple of the target size and box-downsampled, so
// the anti-aliasing comes from the same area-weighted average the rest of
// this generator uses rather than from a second, differently-behaved one.
func renderFrame(f frame) *image.NRGBA {
	ss := supersample(f.size)
	hi := f.size * ss
	side := float64(hi)

	radius := cornerFrac * side
	stroke := f.stroke * float64(ss)
	nodeR := nodeToStroke * stroke

	// The mark's ink bounding box is the span of its node centres plus one
	// node radius at each end. Centre that box in the tile.
	ink := f.markFrac * side
	span := ink - 2*nodeR
	if span < 0 {
		span = 0
	}
	origin := (side-ink)/2 + nodeR

	nodes := make([][2]float64, len(markNodes))
	for i, n := range markNodes {
		nodes[i] = [2]float64{origin + n[0]*span, origin + n[1]*span}
	}

	img := image.NewNRGBA(image.Rect(0, 0, hi, hi))
	for y := 0; y < hi; y++ {
		py := float64(y) + 0.5
		for x := 0; x < hi; x++ {
			px := float64(x) + 0.5
			if !inRoundedSquare(px, py, side, radius) {
				continue
			}
			if onMark(px, py, nodes, stroke/2, nodeR) {
				img.SetNRGBA(x, y, markColour)
			} else {
				img.SetNRGBA(x, y, brandTurquoise)
			}
		}
	}
	return boxResize(img, f.size)
}

// supersample picks how many samples per output pixel to draw with:
// enough that even the smallest frame is drawn on a canvas of roughly a
// thousand pixels, bounded so the 256 px frame stays a reasonable size.
func supersample(size int) int {
	ss := 1024 / size
	if ss < 8 {
		ss = 8
	}
	if ss > 64 {
		ss = 64
	}
	return ss
}

// onMark reports whether a point is on the white mark: within half a
// stroke of any edge, or inside any node's hexagon.
func onMark(x, y float64, nodes [][2]float64, halfStroke, nodeR float64) bool {
	for _, e := range markEdges {
		a, b := nodes[e[0]], nodes[e[1]]
		if distanceToSegment(x, y, a[0], a[1], b[0], b[1]) <= halfStroke {
			return true
		}
	}
	for _, n := range nodes {
		if inHexagon(x, y, n[0], n[1], nodeR) {
			return true
		}
	}
	return false
}

// inHexagon reports whether a point is inside a regular hexagon of the
// given circumradius with vertices on the horizontal axis — pointy left
// and right, flat top and bottom, which is the orientation measured off
// the source mark's own nodes: their radial extent peaks every 60°
// starting at 0°, at a peak-to-trough ratio of 1.16 against a regular
// hexagon's own 2/sqrt(3) = 1.155.
func inHexagon(x, y, cx, cy, r float64) bool {
	dx, dy := math.Abs(x-cx), math.Abs(y-cy)
	if dx > r || dy > r*math.Sqrt(3)/2 {
		return false
	}
	return dy <= math.Sqrt(3)*(r-dx)
}

// inRoundedSquare reports whether a point is inside a square of the given
// side with corners of the given radius, anchored at the origin.
func inRoundedSquare(x, y, side, radius float64) bool {
	if x < 0 || y < 0 || x > side || y > side {
		return false
	}
	// Inside either of the two straight bands that cross the square, the
	// point is in regardless of the corners.
	if (x >= radius && x <= side-radius) || (y >= radius && y <= side-radius) {
		return true
	}
	// Otherwise it is in one of the four corner squares, and has to be
	// inside that corner's own disc.
	cx, cy := radius, radius
	if x > side-radius {
		cx = side - radius
	}
	if y > side-radius {
		cy = side - radius
	}
	return (x-cx)*(x-cx)+(y-cy)*(y-cy) <= radius*radius
}

// distanceToSegment returns the distance from a point to a line segment,
// which is what gives the mark's edges their round caps.
func distanceToSegment(px, py, ax, ay, bx, by float64) float64 {
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / l2
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// verifyAgainstSource checks that markNodes/markEdges still describe the
// mark in assets/signature-logo.js, both ways round: everything the
// geometry draws is ink in the source, and everything that is ink in the
// source is drawn by the geometry.
//
// One direction alone would not be enough. Checking only that the drawn
// mark lands on ink would pass a geometry that had lost an edge; checking
// only that the source's ink is covered would pass one that had gained a
// spurious one.
func verifyAgainstSource(alpha []byte) error {
	if len(alpha) != wantSize*wantSize {
		return fmt.Errorf("alpha channel is %d bytes, want %d", len(alpha), wantSize*wantSize)
	}
	isInk := func(x, y int) bool {
		if x < 0 || y < 0 || x >= wantSize || y >= wantSize {
			return false
		}
		return alpha[y*wantSize+x] > 128
	}

	nodes := make([][2]float64, len(markNodes))
	for i, n := range markNodes {
		nodes[i] = [2]float64{srcOriginX + n[0]*srcSpanX, srcOriginY + n[1]*srcSpanY}
	}

	// Forward: every node centre is ink, and every edge runs along ink for
	// almost its whole length.
	//
	// Almost, rather than all: the source is a rasterisation, and where a
	// node's hexagon meets an edge's stroke it has a one-pixel notch that
	// the centreline passes through. Measured on edge 0-1, seven pixels out
	// from node 0: the centreline is not ink there and the nearest ink is
	// 2.75 px away, while all 65 of its other samples are directly on ink.
	// Requiring every sample would fail on that notch, which is a fact
	// about the bitmap and not about the geometry.
	//
	// The threshold is still nowhere near loose enough to pass a missing
	// edge: an edge that was not there at all would score close to zero,
	// not 98%.
	const minEdgeCoverage = 0.95
	for i, n := range nodes {
		if !isInk(int(n[0]+0.5), int(n[1]+0.5)) {
			return fmt.Errorf("node %d at (%.1f, %.1f) is not on ink", i, n[0], n[1])
		}
	}
	for _, e := range markEdges {
		a, b := nodes[e[0]], nodes[e[1]]
		steps := int(math.Hypot(b[0]-a[0], b[1]-a[1]))
		onInk := 0
		for s := 0; s <= steps; s++ {
			t := float64(s) / float64(steps)
			x, y := a[0]+(b[0]-a[0])*t, a[1]+(b[1]-a[1])*t
			if isInk(int(x+0.5), int(y+0.5)) {
				onInk++
			}
		}
		if got := float64(onInk) / float64(steps+1); got < minEdgeCoverage {
			return fmt.Errorf("edge %d-%d runs along ink for only %.0f%% of its length (%d of %d samples); "+
				"the mark has changed and markEdges no longer describes it",
				e[0], e[1], got*100, onInk, steps+1)
		}
	}

	// Back: every ink pixel in the source is on something the geometry
	// draws. A little slack for the source's own anti-aliased edges, which
	// are a bitmap's and not the geometry's.
	const slack = 1.5
	var inkTotal, uncovered int
	for y := 0; y < wantSize; y++ {
		for x := 0; x < wantSize; x++ {
			if !isInk(x, y) {
				continue
			}
			inkTotal++
			if !onMark(float64(x)+0.5, float64(y)+0.5, nodes, srcStroke/2+slack, srcNodeRadius+slack) {
				uncovered++
			}
		}
	}
	if inkTotal == 0 {
		return errors.New("the source has no ink in it at all")
	}
	if covered := 1 - float64(uncovered)/float64(inkTotal); covered < 0.97 {
		return fmt.Errorf("the geometry covers only %.1f%% of the source's ink (%d of %d pixels uncovered); "+
			"the mark has changed and markNodes/markEdges no longer describe it",
			covered*100, uncovered, inkTotal)
	}
	return nil
}

func extractInt(js []byte, key string) int {
	re := regexp.MustCompile(key + `:\s*(\d+)`)
	m := re.FindSubmatch(js)
	if m == nil {
		log.Fatalf("genicon: field %q not found in source", key)
	}
	var n int
	if _, err := fmt.Sscanf(string(m[1]), "%d", &n); err != nil {
		log.Fatalf("genicon: field %q is not an integer: %v", key, err)
	}
	return n
}

func extractBase64(js []byte, key string) []byte {
	re := regexp.MustCompile(key + `:\s*'([^']+)'`)
	m := re.FindSubmatch(js)
	if m == nil {
		log.Fatalf("genicon: field %q not found in source", key)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(m[1]))
	if err != nil {
		log.Fatalf("genicon: field %q is not valid base64: %v", key, err)
	}
	return decoded
}

// inflate decompresses flateBytes (RFC 1950 zlib) and checks the result
// is exactly wantBytes long — the same sanity check scripts/genlogo
// performs.
func inflate(flateBytes []byte, wantBytes int) []byte {
	r, err := zlib.NewReader(bytes.NewReader(flateBytes))
	if err != nil {
		log.Fatalf("genicon: not valid zlib data: %v", err)
	}
	defer func() { _ = r.Close() }()
	out, err := io.ReadAll(r)
	if err != nil {
		log.Fatalf("genicon: failed to decompress: %v", err)
	}
	if len(out) != wantBytes {
		log.Fatalf("genicon: decompresses to %d bytes, want %d", len(out), wantBytes)
	}
	return out
}

// boxResize downsamples src to a size×size image using an area-weighted
// box filter: each destination pixel is the average of every source pixel
// it overlaps, weighted by the overlap area. This is the standard correct
// approach for downscaling (as opposed to point sampling or bilinear,
// both of which alias on a flat-shaded mark with hard edges like this
// one) and needs nothing beyond image/color from the standard library.
//
// # Colour is averaged premultiplied, and that is not a detail
//
// image.NRGBA is straight (non-premultiplied) alpha, and averaging
// straight colour across a boundary where alpha changes is simply wrong:
// a fully transparent pixel still carries a colour, and it drags the
// average toward it in proportion to how much of the destination pixel it
// covers. Outside the tile that colour is (0,0,0), so averaging straight
// would tint the rounded corners toward black.
//
// It was, and it was measured rather than reasoned about: before this,
// the 16 px frame's 34%-covered corner pixels read (1,45,46) where
// straight alpha requires the tile's own (3,131,135), and composited on
// #1E1E1E they came out 29/255 too dark in the green channel — a dark
// fringe around every corner, on every background.
//
// So each sample's colour is weighted by its own alpha as well as by its
// overlap, and the result is divided back out by the averaged alpha. A
// partly covered pixel then carries the colour of whatever actually
// covered it, at a reduced alpha, which is what straight alpha means.
func boxResize(src *image.NRGBA, size int) *image.NRGBA {
	srcSize := src.Bounds().Dx()
	if srcSize == size {
		return src
	}
	scale := float64(srcSize) / float64(size)
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))

	for y := 0; y < size; y++ {
		sy0, sy1 := float64(y)*scale, float64(y+1)*scale
		for x := 0; x < size; x++ {
			sx0, sx1 := float64(x)*scale, float64(x+1)*scale

			// r, g and b accumulate premultiplied; a accumulates alpha.
			var r, g, b, a, weight float64
			for iy := int(sy0); iy < int(sy1)+1 && iy < srcSize; iy++ {
				wy := overlap(float64(iy), float64(iy+1), sy0, sy1)
				if wy <= 0 {
					continue
				}
				for ix := int(sx0); ix < int(sx1)+1 && ix < srcSize; ix++ {
					wx := overlap(float64(ix), float64(ix+1), sx0, sx1)
					if wx <= 0 {
						continue
					}
					w := wx * wy
					c := src.NRGBAAt(ix, iy)
					aw := w * float64(c.A) / 255
					r += float64(c.R) * aw
					g += float64(c.G) * aw
					b += float64(c.B) * aw
					a += float64(c.A) * w
					weight += w
				}
			}
			if weight == 0 {
				weight = 1
			}
			outA := a / weight
			if outA <= 0 {
				// Nothing covered this pixel at all. There is no colour to
				// recover and dividing by the alpha would be a division by
				// zero, so it is transparent, full stop.
				dst.SetNRGBA(x, y, color.NRGBA{})
				continue
			}
			// Back out of premultiplied: the accumulated colour is divided by
			// the accumulated alpha rather than by the area, so the result is
			// the colour that actually covered the pixel.
			scale := 255 / (outA * weight)
			dst.SetNRGBA(x, y, color.NRGBA{
				R: clamp8(r*scale + 0.5),
				G: clamp8(g*scale + 0.5),
				B: clamp8(b*scale + 0.5),
				A: uint8(outA + 0.5),
			})
		}
	}
	return dst
}

// clamp8 rounds a channel value into a byte. Backing out of premultiplied
// alpha is a division, so a value can land a fraction above 255 on a pixel
// that is very nearly opaque; without this that wraps to 0 and a corner
// pixel turns black.
func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}

// overlap returns the length of the overlap between [a0,a1) and [b0,b1).
func overlap(a0, a1, b0, b1 float64) float64 {
	lo := a0
	if lo < b0 {
		lo = b0
	}
	hi := a1
	if hi > b1 {
		hi = b1
	}
	if hi <= lo {
		return 0
	}
	return hi - lo
}

// icoDirEntry mirrors ICONDIRENTRY (Microsoft's ICO format, MS-ICO):
// each entry describes one image's dimensions and where its bytes sit
// in the file. Width/Height are bytes — 0 means 256, the format's own
// established convention for the maximum size a single byte cannot
// otherwise represent.
type icoDirEntry struct {
	Width, Height           uint8
	ColorCount, Reserved    uint8
	Planes, BitCount        uint16
	BytesInRes, ImageOffset uint32
}

// encodeICO assembles a standard multi-image .ico: a 6-byte ICONDIR
// header, one 16-byte ICONDIRENTRY per frame, then every frame's PNG
// bytes concatenated in order — exactly the layout MS-ICO specifies,
// and the one every Windows icon-loading API (this project's own
// LoadImageW call in internal/ui, Explorer, Shell_NotifyIconW) reads.
func encodeICO(sizes []int, pngFrames [][]byte) ([]byte, error) {
	var buf bytes.Buffer

	// ICONDIR: reserved(2)=0, type(2)=1 (icon, not cursor), count(2).
	if err := binary.Write(&buf, binary.LittleEndian, uint16(0)); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint16(1)); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.LittleEndian, uint16(len(sizes))); err != nil {
		return nil, err
	}

	headerLen := 6 + 16*len(sizes)
	offset := uint32(headerLen)
	entries := make([]icoDirEntry, len(sizes))
	for i, size := range sizes {
		b := uint8(size)
		if size >= 256 {
			b = 0
		}
		entries[i] = icoDirEntry{
			Width: b, Height: b,
			ColorCount: 0, Reserved: 0,
			Planes: 1, BitCount: 32,
			BytesInRes:  uint32(len(pngFrames[i])),
			ImageOffset: offset,
		}
		offset += uint32(len(pngFrames[i]))
	}
	for _, e := range entries {
		if err := binary.Write(&buf, binary.LittleEndian, e); err != nil {
			return nil, err
		}
	}
	for _, f := range pngFrames {
		buf.Write(f)
	}
	return buf.Bytes(), nil
}
