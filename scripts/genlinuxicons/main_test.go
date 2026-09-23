package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestEveryHicolorSizeIsTheIcoFrameByteForByte runs against the
// committed icon, so it fails if genicon ever stops writing a size the
// package installs — and it decodes each file, so a frame of the right
// name and the wrong dimensions cannot pass either.
func TestEveryHicolorSizeIsTheIcoFrameByteForByte(t *testing.T) {
	ico, err := os.ReadFile(filepath.Join("..", "..", "internal", "ui", "assets", "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	frames, err := icoFrames(ico)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	written, err := writeHicolor(frames, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != len(hicolorSizes) {
		t.Fatalf("wrote %d files, want %d", len(written), len(hicolorSizes))
	}
	for _, n := range hicolorSizes {
		p := filepath.Join(out, fmt.Sprintf("%dx%d", n, n), "apps", "liro-bridge.png")
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, frames[n]) {
			t.Errorf("%s differs from the .ico's %d px frame", p, n)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(got))
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if cfg.Width != n || cfg.Height != n {
			t.Errorf("%s is %dx%d, want %dx%d", p, cfg.Width, cfg.Height, n, n)
		}
	}
}

// TestAMissingSizeIsRefused is the control: without it the test above
// could pass on a writer that skips sizes it cannot find.
func TestAMissingSizeIsRefused(t *testing.T) {
	ico, err := os.ReadFile(filepath.Join("..", "..", "internal", "ui", "assets", "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	frames, err := icoFrames(ico)
	if err != nil {
		t.Fatal(err)
	}
	delete(frames, 48)
	if _, err := writeHicolor(frames, t.TempDir()); err == nil {
		t.Fatal("a missing 48 px frame was not refused")
	}
}

func TestANonPNGFrameIsRefused(t *testing.T) {
	// One directory entry, 16 px, eight bytes at offset 22 that are not
	// a PNG signature.
	ico := []byte{0, 0, 1, 0, 1, 0,
		16, 16, 0, 0, 1, 0, 32, 0, 8, 0, 0, 0, 22, 0, 0, 0,
		'B', 'M', 0, 0, 0, 0, 0, 0}
	if _, err := icoFrames(ico); err == nil {
		t.Fatal("a BMP frame was accepted")
	}
}
