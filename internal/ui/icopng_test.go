package ui

import (
	"bytes"
	"image/png"
	"testing"
)

// The status-bar icon is a real PNG from the project's own icon, at the
// size nearest the one asked for.
func TestIcoPNGEntryFindsTheNearestSize(t *testing.T) {
	blob := icoPNGEntry(trayIconICO, 36)
	if blob == nil {
		t.Fatal("no PNG entry found in the project's icon")
	}
	img, err := png.Decode(bytes.NewReader(blob))
	if err != nil {
		t.Fatalf("the entry does not decode: %v", err)
	}
	if w := img.Bounds().Dx(); w < 24 || w > 64 {
		t.Fatalf("the entry nearest 36 is %d wide", w)
	}
	if icoPNGEntry([]byte("not an icon"), 36) != nil {
		t.Fatal("garbage gave an entry")
	}
}
