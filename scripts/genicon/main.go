// Command genicon produces the tray icon (Task 5, F5 review) from the
// same real Liro brand mark scripts/genlogo already consumes for the
// PDF visual stamp: assets/signature-logo.js, a 256×256 turquoise
// (#038387) mark exported as raw RGB and 8-bit alpha channels, already
// zlib-compressed (RFC 1950). The tray icon was a placeholder
// (IDI_APPLICATION, the generic Windows application icon) because F5
// never produced a real one; this generator is the missing step,
// reproducible from the same source scripts/genlogo already uses so
// there is exactly one place the mark's pixels live.
//
// It decodes the source once into a 256×256 image, then writes a
// standard multi-image .ico containing that image downsampled to every
// size Windows actually asks a tray icon for — 16, 20, 24, 32, 40, 48,
// 64 and 256 px — each frame encoded as PNG, which every icon-capable
// Windows API (LoadImage, Shell_NotifyIconW, Explorer) has accepted
// inside .ico containers since Windows Vista. Downsampling uses a
// simple box filter (area-weighted average of overlapping source
// pixels): correct, alias-free downscaling for a flat-shaded mark like
// this one, and simple enough to keep dependency-free per SPEC §8.6 —
// nothing here justifies pulling in an image-resizing library.
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
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
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
var iconSizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

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

	rgb := inflate(extractBase64(src, "rgbFlateBase64"), wantSize*wantSize*3)
	alpha := inflate(extractBase64(src, "alphaFlateBase64"), wantSize*wantSize)

	base := image.NewNRGBA(image.Rect(0, 0, wantSize, wantSize))
	for y := 0; y < wantSize; y++ {
		for x := 0; x < wantSize; x++ {
			i := y*wantSize + x
			base.SetNRGBA(x, y, color.NRGBA{
				R: rgb[i*3], G: rgb[i*3+1], B: rgb[i*3+2], A: alpha[i],
			})
		}
	}

	frames := make([][]byte, len(iconSizes))
	for i, size := range iconSizes {
		img := base
		if size != wantSize {
			img = boxResize(base, size)
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			log.Fatalf("genicon: encoding %dx%d frame: %v", size, size, err)
		}
		frames[i] = buf.Bytes()
	}

	icoBytes, err := encodeICO(iconSizes, frames)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outPath, icoBytes, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("genicon: wrote %s (%d frames: %v, %d bytes)\n", *outPath, len(iconSizes), iconSizes, len(icoBytes))
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
// performs, except genicon actually needs the decompressed pixels
// (genlogo only re-embeds the still-compressed source).
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

// boxResize downsamples src (always wantSize×wantSize here) to a
// size×size image using an area-weighted box filter: each destination
// pixel is the average of every source pixel it overlaps, weighted by
// the overlap area. This is the standard correct approach for
// downscaling (as opposed to point sampling or bilinear, both of which
// alias on a flat-shaded mark with hard edges like this one) and needs
// nothing beyond image/color from the standard library.
func boxResize(src *image.NRGBA, size int) *image.NRGBA {
	srcSize := src.Bounds().Dx()
	scale := float64(srcSize) / float64(size)
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))

	for y := 0; y < size; y++ {
		sy0, sy1 := float64(y)*scale, float64(y+1)*scale
		for x := 0; x < size; x++ {
			sx0, sx1 := float64(x)*scale, float64(x+1)*scale

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
					r += float64(c.R) * w
					g += float64(c.G) * w
					b += float64(c.B) * w
					a += float64(c.A) * w
					weight += w
				}
			}
			if weight == 0 {
				weight = 1
			}
			dst.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r/weight + 0.5),
				G: uint8(g/weight + 0.5),
				B: uint8(b/weight + 0.5),
				A: uint8(a/weight + 0.5),
			})
		}
	}
	return dst
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
func encodeICO(sizes []int, frames [][]byte) ([]byte, error) {
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
			BytesInRes:  uint32(len(frames[i])),
			ImageOffset: offset,
		}
		offset += uint32(len(frames[i]))
	}
	for _, e := range entries {
		if err := binary.Write(&buf, binary.LittleEndian, e); err != nil {
			return nil, err
		}
	}
	for _, f := range frames {
		buf.Write(f)
	}
	return buf.Bytes(), nil
}
