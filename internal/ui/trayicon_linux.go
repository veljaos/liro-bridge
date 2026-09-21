//go:build linux

package ui

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"sort"
)

// The tray icon, as pixels.
//
// StatusNotifierItem offers two ways to say what an item looks like: a
// themed icon *name*, and raw pixels. A name is the better of the two
// and this program cannot use it yet — a name is resolved against the
// icon theme, which means an installed icon file, which is F12 §8's
// packaging and does not exist while the agent is being run out of a
// build directory. Pixels work with nothing installed at all.
//
// The source is the same icon.ico every other platform draws from, so
// there is one icon in this program rather than one per host (D-285,
// D-286). It happens to hold PNGs, which the standard library decodes,
// so nothing here needs a second image format or a second dependency.

// trayPixmap is one size of the icon in the form
// org.kde.StatusNotifierItem's IconPixmap wants: width, height, and
// ARGB32 in **network byte order**, which the specification is explicit
// about and which is the one thing here that a little-endian machine
// gets wrong by doing nothing.
type trayPixmap struct {
	Width  int32
	Height int32
	Pixels []byte
}

// trayIconPixmaps decodes the embedded icon into the sizes a host might
// pick from, smallest first.
//
// Every size in the file is offered rather than one chosen here: a
// panel at 200% scaling wants 48 where the same panel at 100% wants 24,
// and the host is the only thing that knows which. The 256 is dropped —
// it is four times the pixels of anything a tray draws and it would
// travel over the bus on every registration.
func trayIconPixmaps(ico []byte) ([]trayPixmap, error) {
	images, err := decodeICO(ico)
	if err != nil {
		return nil, err
	}
	out := make([]trayPixmap, 0, len(images))
	for _, img := range images {
		b := img.Bounds()
		if b.Dx() > maxTrayIconSize {
			continue
		}
		out = append(out, trayPixmap{
			Width:  int32(b.Dx()),
			Height: int32(b.Dy()),
			Pixels: argb32(img),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ui: the icon holds no image at or below %dpx", maxTrayIconSize)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Width < out[j].Width })
	return out, nil
}

// maxTrayIconSize is the largest size worth sending to a panel.
const maxTrayIconSize = 64

// argb32 converts an image to the packed ARGB the specification asks
// for: one 32-bit value per pixel, alpha in the most significant byte,
// big-endian.
func argb32(img image.Image) []byte {
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)

	out := make([]byte, 0, b.Dx()*b.Dy()*4)
	var px [4]byte
	for i := 0; i < len(rgba.Pix); i += 4 {
		r, g, bl, a := rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2], rgba.Pix[i+3]
		binary.BigEndian.PutUint32(px[:], uint32(a)<<24|uint32(r)<<16|uint32(g)<<8|uint32(bl))
		out = append(out, px[:]...)
	}
	return out
}

// decodeICO reads the images out of a Windows .ico.
//
// It reads the directory rather than guessing: six bytes of header, then
// sixteen per entry, of which this needs only the offset and the length.
// Every entry in this project's own icon is a PNG (scripts/genicon
// writes them that way), and an entry that is not is skipped rather than
// guessed at — a BMP entry in an .ico carries an AND mask and an
// upside-down bitmap, which is a decoder this program has no reason to
// own.
func decodeICO(data []byte) ([]image.Image, error) {
	const headerLen, entryLen = 6, 16
	if len(data) < headerLen {
		return nil, fmt.Errorf("ui: the icon is %d bytes, too short to hold a directory", len(data))
	}
	if binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil, fmt.Errorf("ui: the icon is not an .ico (type %d)", binary.LittleEndian.Uint16(data[2:4]))
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))

	var out []image.Image
	for i := 0; i < count; i++ {
		off := headerLen + i*entryLen
		if off+entryLen > len(data) {
			return nil, fmt.Errorf("ui: the icon's directory claims %d entries and the file ends after %d", count, i)
		}
		size := int(binary.LittleEndian.Uint32(data[off+8 : off+12]))
		at := int(binary.LittleEndian.Uint32(data[off+12 : off+16]))
		if at < 0 || size < 0 || at+size > len(data) {
			return nil, fmt.Errorf("ui: the icon's entry %d points outside the file", i)
		}
		blob := data[at : at+size]
		if !bytes.HasPrefix(blob, []byte("\x89PNG\r\n\x1a\n")) {
			continue
		}
		img, err := png.Decode(bytes.NewReader(blob))
		if err != nil {
			return nil, fmt.Errorf("ui: the icon's entry %d does not decode: %w", i, err)
		}
		out = append(out, img)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ui: the icon holds no PNG entries")
	}
	return out, nil
}
