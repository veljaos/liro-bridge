package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

const (
	sourcePath = "../../assets/signature-logo.js"
	assetPath  = "../../internal/ui/assets/icon.ico"
)

func sourceAlpha(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the logo source: %v", err)
	}
	return inflate(extractBase64(src, "alphaFlateBase64"), wantSize*wantSize)
}

// TestTheGeometryStillDescribesTheSourceMark is the drift check that keeps
// this generator honest about having one source of truth. markNodes and
// markEdges are numbers in this file; the mark itself is a bitmap in
// assets/signature-logo.js. If somebody replaces the mark, this fails
// rather than the icon quietly becoming a picture of a mark the product
// no longer uses.
func TestTheGeometryStillDescribesTheSourceMark(t *testing.T) {
	if err := verifyAgainstSource(sourceAlpha(t)); err != nil {
		t.Fatalf("the geometry no longer describes %s: %v", sourcePath, err)
	}
}

// TestTheDriftCheckWouldActuallyFire is the other half, because a check
// that has never been seen to fail is not a check. Each case is a way the
// mark could change that the icon must not survive silently.
func TestTheDriftCheckWouldActuallyFire(t *testing.T) {
	alpha := sourceAlpha(t)

	t.Run("an edge that is no longer there", func(t *testing.T) {
		// Erase the ink along edge 0-1 and nowhere else.
		damaged := append([]byte(nil), alpha...)
		a := [2]float64{srcOriginX + markNodes[0][0]*srcSpanX, srcOriginY + markNodes[0][1]*srcSpanY}
		b := [2]float64{srcOriginX + markNodes[1][0]*srcSpanX, srcOriginY + markNodes[1][1]*srcSpanY}
		for y := 0; y < wantSize; y++ {
			for x := 0; x < wantSize; x++ {
				if distanceToSegment(float64(x)+0.5, float64(y)+0.5, a[0], a[1], b[0], b[1]) <= srcNodeRadius {
					damaged[y*wantSize+x] = 0
				}
			}
		}
		if err := verifyAgainstSource(damaged); err == nil {
			t.Fatal("an erased edge passed the check")
		} else {
			t.Logf("correctly refused: %v", err)
		}
	})

	t.Run("ink the geometry does not draw", func(t *testing.T) {
		// A blot in the empty upper-left quadrant, well away from the mark.
		damaged := append([]byte(nil), alpha...)
		for y := 30; y < 75; y++ {
			for x := 55; x < 100; x++ {
				damaged[y*wantSize+x] = 255
			}
		}
		if err := verifyAgainstSource(damaged); err == nil {
			t.Fatal("ink that the geometry does not account for passed the check")
		} else {
			t.Logf("correctly refused: %v", err)
		}
	})

	t.Run("no mark at all", func(t *testing.T) {
		if err := verifyAgainstSource(make([]byte, wantSize*wantSize)); err == nil {
			t.Fatal("an empty source passed the check")
		}
	})
}

// TestEveryFrameIsDrawnAtItsOwnSize guards the thing that makes the small
// sizes work: the frames are eight drawings at eight weights, not one
// drawing scaled eight ways. If frames ever collapsed to a single set of
// weights, the stroke ratios would go flat and 16 px would go back to
// being a quarter-pixel hairline.
func TestEveryFrameIsDrawnAtItsOwnSize(t *testing.T) {
	seen := map[int]bool{}
	prevRatio := 0.0
	for i, f := range frames {
		if seen[f.size] {
			t.Fatalf("size %d appears twice in frames", f.size)
		}
		seen[f.size] = true

		ratio := f.stroke / (f.markFrac * float64(f.size))
		t.Logf("%3dpx  mark %6.2fpx  stroke %5.2fpx  = %.1f%% of the mark",
			f.size, f.markFrac*float64(f.size), f.stroke, ratio*100)

		// The stroke is a larger share of the mark the smaller the frame is;
		// that ramp is what the small sizes are legible because of.
		if i > 0 && ratio >= prevRatio {
			t.Errorf("size %d has a stroke ratio of %.3f, not less than the next size down's %.3f: "+
				"the ramp has gone flat or backwards", f.size, ratio, prevRatio)
		}
		prevRatio = ratio

		// Below about a pixel and a quarter a stroke is grey mush rather
		// than a line. This is the floor the original icon fell through.
		if f.stroke < 1.25 {
			t.Errorf("size %d has a stroke of %.2f px, which will not hold together", f.size, f.stroke)
		}
	}
}

// TestTheMarkIsOneConnectedShapeAtEverySize is the defect, as a check.
//
// The icon this replaced was the bare mark with strokes 1.9% of its own
// width. At 16 px those fell to a quarter of a pixel and the mark broke
// into a scatter of disconnected blobs — the owner's report, and
// reproduced before any of this was changed. A mark that is one connected
// shape is the property that was lost, so it is the property pinned here.
func TestTheMarkIsOneConnectedShapeAtEverySize(t *testing.T) {
	for _, f := range frames {
		img := renderFrame(f)
		n, largest, total := markComponents(img)
		if total == 0 {
			t.Errorf("%dpx: there is no mark at all", f.size)
			continue
		}
		t.Logf("%3dpx  mark is %d component(s), %d px, largest %d", f.size, n, total, largest)
		if n != 1 {
			t.Errorf("%dpx: the mark is %d disconnected pieces, want 1 (largest holds %d of %d pixels)",
				f.size, n, largest, total)
		}
	}
}

// TestTheTileIsTheBrandTurquoiseWithNothingBlendedIntoIt pins the answer
// to the turquoise-wallpaper question: the fill is exactly #038387, the
// same value scripts/genlogo puts in the PDF stamp, with no darkening and
// no light border. A darkened fill would make the icon's turquoise
// disagree with the stamp's, and it could not buy an edge anybody can see
// in any case — against a wallpaper of the brand colour the best a
// recognisably-brand darkening reaches is 1.24:1, where 3:1 is the
// threshold at which a boundary reads as deliberate.
func TestTheTileIsTheBrandTurquoiseWithNothingBlendedIntoIt(t *testing.T) {
	// First, tie the constant to something outside this file. Comparing the
	// rendered fill against brandTurquoise alone would pass a darkened
	// constant, because the same constant does the filling and the
	// asserting. The mark's own colour in assets/signature-logo.js is the
	// independent copy, and it is the one that has to agree: the tile is
	// filled with the colour the mark is drawn in, so the icon and the PDF
	// stamp cannot come to disagree about what the brand turquoise is.
	src, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("reading the logo source: %v", err)
	}
	rgb := inflate(extractBase64(src, "rgbFlateBase64"), wantSize*wantSize*3)
	alpha := sourceAlpha(t)
	srcCounts := map[color.NRGBA]int{}
	for i := 0; i < wantSize*wantSize; i++ {
		if alpha[i] > 200 {
			srcCounts[color.NRGBA{R: rgb[i*3], G: rgb[i*3+1], B: rgb[i*3+2], A: 0xFF}]++
		}
	}
	var srcModal color.NRGBA
	srcBest := 0
	for c, n := range srcCounts {
		if n > srcBest {
			srcModal, srcBest = c, n
		}
	}
	if srcModal != brandTurquoise {
		t.Fatalf("the mark in %s is drawn in #%02X%02X%02X (%d px) and the tile is filled with "+
			"#%02X%02X%02X; the icon's turquoise and the mark's own have diverged",
			sourcePath, srcModal.R, srcModal.G, srcModal.B, srcBest,
			brandTurquoise.R, brandTurquoise.G, brandTurquoise.B)
	}
	t.Logf("the mark's own colour in %s is #%02X%02X%02X (%d opaque px), and the tile is filled with it",
		sourcePath, srcModal.R, srcModal.G, srcModal.B, srcBest)

	for _, f := range frames {
		img := renderFrame(f)

		// The commonest opaque colour in the frame is the fill. Asserting on
		// the mode rather than on a chosen pixel is what makes this hold at
		// 16 px, where the mark comes within a pixel and a half of the tile's
		// edge and there is no spot that is reliably clear of its
		// anti-aliasing.
		counts := map[color.NRGBA]int{}
		for y := 0; y < f.size; y++ {
			for x := 0; x < f.size; x++ {
				if c := img.NRGBAAt(x, y); c.A == 0xFF {
					counts[c]++
				}
			}
		}
		var modal color.NRGBA
		best := 0
		for c, n := range counts {
			if n > best {
				modal, best = c, n
			}
		}
		if modal != brandTurquoise {
			t.Errorf("%dpx: the commonest opaque colour is #%02X%02X%02X (%d px), want the brand #038387; "+
				"the fill has been darkened or something has been blended into it",
				f.size, modal.R, modal.G, modal.B, best)
		}
	}

	// And at 256 px, where the mark is 32 px clear of every edge, the band
	// just inside the tile's boundary is the fill exactly — which is what
	// says there is no light border in there.
	big := renderFrame(frames[len(frames)-1])
	mid := 256 / 2
	for _, p := range []image.Point{{mid, 1}, {mid, 254}, {1, mid}, {254, mid}} {
		if c := big.NRGBAAt(p.X, p.Y); c != brandTurquoise {
			t.Errorf("256px: the tile just inside its edge at (%d,%d) is #%02X%02X%02X alpha %d, "+
				"want the brand #038387 opaque; there is a border in there",
				p.X, p.Y, c.R, c.G, c.B, c.A)
		}
	}
}

// TestTheCornersAreRoundedAndTransparent keeps the tile a rounded square
// rather than a full one: the extreme corner pixel must be clear, or the
// icon is a rectangle and the shape decision has been lost.
func TestTheCornersAreRoundedAndTransparent(t *testing.T) {
	for _, f := range frames {
		img := renderFrame(f)
		for _, p := range []image.Point{{0, 0}, {f.size - 1, 0}, {0, f.size - 1}, {f.size - 1, f.size - 1}} {
			if a := img.NRGBAAt(p.X, p.Y).A; a != 0 {
				t.Errorf("%dpx: corner (%d,%d) has alpha %d, want 0", f.size, p.X, p.Y, a)
			}
		}
	}
}

// TestAPartlyCoveredPixelCarriesTheColourThatCoveredIt is the fringe, as a
// check.
//
// .ico frames are straight (non-premultiplied) alpha, so a pixel the tile
// covers a third of must carry the tile's own colour at a third of the
// alpha — not a third of the way from the tile's colour toward whatever
// happens to sit outside it. Averaging straight colour across the tile's
// boundary gets this wrong in a way that is invisible on the frame itself
// and shows up only once something composites it: measured before this was
// fixed, the 16px frame's 34%-covered corner pixels carried (1,45,46)
// instead of (3,131,135), and came out 29/255 too dark in the green
// channel over #1E1E1E — a dark fringe on every background.
//
// The rounded corners are the only place in this icon where alpha varies,
// so every partly covered pixel is on the tile's own edge and must be the
// tile's own colour.
func TestAPartlyCoveredPixelCarriesTheColourThatCoveredIt(t *testing.T) {
	// A dark ground makes a colour dragged toward black show up as a real
	// error rather than as rounding; white would hide it.
	const groundR, groundG, groundB = 0x1E, 0x1E, 0x1E

	for _, f := range frames {
		img := renderFrame(f)
		partial, worst := 0, 0
		for y := 0; y < f.size; y++ {
			for x := 0; x < f.size; x++ {
				c := img.NRGBAAt(x, y)
				if c.A == 0 || c.A == 0xFF {
					continue
				}
				partial++
				if c != (color.NRGBA{R: brandTurquoise.R, G: brandTurquoise.G, B: brandTurquoise.B, A: c.A}) {
					t.Errorf("%dpx: (%d,%d) is %d%% covered and carries #%02X%02X%02X, want the tile's own #%02X%02X%02X; "+
						"the colour was averaged across the tile's edge without being premultiplied",
						f.size, x, y, int(float64(c.A)/255*100), c.R, c.G, c.B,
						brandTurquoise.R, brandTurquoise.G, brandTurquoise.B)
				}
				// And say what it would cost on screen, in the channel with
				// the most to lose.
				a := float64(c.A) / 255
				got := float64(groundG)*(1-a) + float64(c.G)*a
				want := float64(groundG)*(1-a) + float64(brandTurquoise.G)*a
				if d := int(want - got + 0.5); d > worst {
					worst = d
				}
			}
		}
		if partial == 0 {
			t.Errorf("%dpx: no partly covered pixels at all — the corners are not anti-aliased", f.size)
		}
		t.Logf("%3dpx  %4d partly covered, worst green error over #1E1E1E: %d/255", f.size, partial, worst)
	}
}

// TestTheCommittedAssetIsWhatThisGeneratorProduces is this project's
// standing rule for a generated artefact, applied here: a committed file
// nobody regenerates is a committed file in name only, and running the
// generator has to be safe rather than a surprise (D-183).
func TestTheCommittedAssetIsWhatThisGeneratorProduces(t *testing.T) {
	committed, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("reading the committed icon: %v", err)
	}

	pngFrames := make([][]byte, len(frames))
	for i, f := range frames {
		var buf bytes.Buffer
		if err := png.Encode(&buf, renderFrame(f)); err != nil {
			t.Fatalf("encoding the %dpx frame: %v", f.size, err)
		}
		pngFrames[i] = buf.Bytes()
	}
	generated, err := encodeICO(iconSizes(), pngFrames)
	if err != nil {
		t.Fatalf("encoding the icon: %v", err)
	}

	if !bytes.Equal(committed, generated) {
		dir := t.TempDir()
		_ = os.WriteFile(filepath.Join(dir, "generated.ico"), generated, 0o644)
		t.Fatalf("%s is %d bytes and this generator produces %d; "+
			"run: go run ./scripts/genicon --out internal/ui/assets/icon.ico",
			assetPath, len(committed), len(generated))
	}
}

// TestTheCommittedAssetHoldsEveryFrameWindowsAsksFor reads the committed
// file the way Windows does rather than the way this generator wrote it.
func TestTheCommittedAssetHoldsEveryFrameWindowsAsksFor(t *testing.T) {
	raw, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("reading the committed icon: %v", err)
	}
	if got := binary.LittleEndian.Uint16(raw[2:4]); got != 1 {
		t.Fatalf("ICONDIR type is %d, want 1 (an icon, not a cursor)", got)
	}
	n := int(binary.LittleEndian.Uint16(raw[4:6]))
	want := iconSizes()
	if n != len(want) {
		t.Fatalf("the committed icon holds %d frames, want %d", n, len(want))
	}
	for i := 0; i < n; i++ {
		e := raw[6+16*i : 6+16*(i+1)]
		size := int(e[0])
		if size == 0 {
			size = 256
		}
		if size != want[i] {
			t.Errorf("frame %d is %dx%d, want %dx%d", i, size, size, want[i], want[i])
		}
		off := binary.LittleEndian.Uint32(e[12:16])
		length := binary.LittleEndian.Uint32(e[8:12])
		img, err := png.Decode(bytes.NewReader(raw[off : off+length]))
		if err != nil {
			t.Errorf("frame %d (%dx%d) is not decodable PNG: %v", i, size, size, err)
			continue
		}
		if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
			t.Errorf("frame %d says %dx%d and holds %dx%d", i, size, size, b.Dx(), b.Dy())
		}
	}
}

// markComponents counts the connected components of the white mark in a
// rendered frame, returning the count, the largest component's size and
// the mark's total size. A pixel belongs to the mark when it is nearer to
// white than to the tile's turquoise, which is the same judgement an eye
// makes at these sizes.
func markComponents(img *image.NRGBA) (count, largest, total int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	isMark := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(b.Min.X+x, b.Min.Y+y)
			if c.A < 128 {
				continue
			}
			if dist(c, markColour) < dist(c, brandTurquoise) {
				isMark[y*w+x] = true
				total++
			}
		}
	}

	seen := make([]bool, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !isMark[y*w+x] || seen[y*w+x] {
				continue
			}
			count++
			size := 0
			stack := [][2]int{{x, y}}
			seen[y*w+x] = true
			for len(stack) > 0 {
				p := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				size++
				// Four-connected: a mark that holds together only through
				// pixels touching at their corners does not hold together.
				for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					nx, ny := p[0]+d[0], p[1]+d[1]
					if nx < 0 || ny < 0 || nx >= w || ny >= h {
						continue
					}
					if isMark[ny*w+nx] && !seen[ny*w+nx] {
						seen[ny*w+nx] = true
						stack = append(stack, [2]int{nx, ny})
					}
				}
			}
			if size > largest {
				largest = size
			}
		}
	}
	return count, largest, total
}

func dist(a, b color.NRGBA) float64 {
	dr := float64(a.R) - float64(b.R)
	dg := float64(a.G) - float64(b.G)
	db := float64(a.B) - float64(b.B)
	return math.Sqrt(dr*dr + dg*dg + db*db)
}
