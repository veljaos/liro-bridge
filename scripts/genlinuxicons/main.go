// Command genlinuxicons copies the application icon's frames out of
// internal/ui/assets/icon.ico into the freedesktop hicolor layout the
// Linux package installs (F12 §8):
//
//	<out>/<n>x<n>/apps/liro-bridge.png
//
// # Copied, not drawn
//
// scripts/genicon already draws every frame analytically at its own
// size and stores each one in the .ico as a complete PNG. Drawing them a
// second time for Linux would put the mark in two generated files that
// could disagree, which is the drift D-285 and D-286 spent two entries
// on. So the bytes written here are the .ico's own frames, byte for
// byte, and the test checks exactly that.
//
// # Which sizes
//
// hicolorSizes is the intersection of the .ico's frames with the sizes
// the hicolor theme declares a directory for (index.theme's
// Directories=, read on Ubuntu 24.04): 16, 24, 32, 48, 64 and 256. The
// .ico's 20 and 40 are Windows DPI steps with no hicolor directory, and
// an icon in a directory the theme does not list is never looked up.
//
// Usage:
//
//	go run ./scripts/genlinuxicons --ico internal/ui/assets/icon.ico --out dist/linux/stage/icons
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// iconName is the icon's theme name, the value of Icon= in both desktop
// entries under build/linux.
const iconName = "liro-bridge"

var hicolorSizes = []int{16, 24, 32, 48, 64, 256}

// pngMagic is the eight-byte PNG signature. Every frame genicon writes
// is one; a frame that is not would be a BMP-in-ICO, which no hicolor
// consumer reads, so it is refused rather than written with a .png name.
var pngMagic = []byte("\x89PNG\r\n\x1a\n")

func main() {
	icoPath := flag.String("ico", "internal/ui/assets/icon.ico", "the application icon")
	outDir := flag.String("out", "", "hicolor root to write into (required)")
	flag.Parse()
	if *outDir == "" {
		log.Fatal("genlinuxicons: --out is required")
	}

	ico, err := os.ReadFile(*icoPath)
	if err != nil {
		log.Fatal(err)
	}
	frames, err := icoFrames(ico)
	if err != nil {
		log.Fatalf("genlinuxicons: %s: %v", *icoPath, err)
	}
	written, err := writeHicolor(frames, *outDir)
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range written {
		fmt.Println(p)
	}
}

// icoFrames returns each frame's bytes keyed by its pixel size.
func icoFrames(ico []byte) (map[int][]byte, error) {
	if len(ico) < 6 {
		return nil, errors.New("too short to be an .ico")
	}
	if binary.LittleEndian.Uint16(ico[0:2]) != 0 || binary.LittleEndian.Uint16(ico[2:4]) != 1 {
		return nil, errors.New("not an .ico: reserved/type header mismatch")
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	frames := make(map[int][]byte, count)
	for i := 0; i < count; i++ {
		entry := 6 + 16*i
		if entry+16 > len(ico) {
			return nil, fmt.Errorf("directory entry %d runs past the file", i)
		}
		size := int(ico[entry])
		if size == 0 {
			size = 256 // the format's own encoding of 256 in one byte
		}
		length := int(binary.LittleEndian.Uint32(ico[entry+8 : entry+12]))
		offset := int(binary.LittleEndian.Uint32(ico[entry+12 : entry+16]))
		if offset < 0 || length < 0 || offset+length > len(ico) {
			return nil, fmt.Errorf("frame %d (%d px) runs past the file", i, size)
		}
		data := ico[offset : offset+length]
		if !bytes.HasPrefix(data, pngMagic) {
			return nil, fmt.Errorf("frame %d (%d px) is not PNG-encoded", i, size)
		}
		frames[size] = data
	}
	return frames, nil
}

// writeHicolor writes every hicolor size, and fails if the .ico lacks
// one: a package that silently ships five of six icons is the kind of
// thing nobody notices until a panel draws a blurry one.
func writeHicolor(frames map[int][]byte, outDir string) ([]string, error) {
	var written []string
	for _, n := range hicolorSizes {
		data, ok := frames[n]
		if !ok {
			return nil, fmt.Errorf("genlinuxicons: the .ico has no %d px frame", n)
		}
		dir := filepath.Join(outDir, fmt.Sprintf("%dx%d", n, n), "apps")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		p := filepath.Join(dir, iconName+".png")
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, err
		}
		written = append(written, p)
	}
	return written, nil
}
