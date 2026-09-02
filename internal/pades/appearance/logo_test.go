package appearance

import (
	"bytes"
	"compress/zlib"
	"io"
	"testing"
)

// inflate decompresses a zlib (RFC 1950) stream, mirroring how
// internal/pades/pdf.filters.go reads a /FlateDecode stream — used here
// only to check the embedded logo asset, never at signing time.
func inflate(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("zlib.NewReader: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	return out
}

// TestLogoAssetIs256x256WithAlphaApplied is Task 2's confirmation (see
// docs/decisions.md): the placeholder disc-and-checkmark mark was
// replaced by the real Liro logo from assets/signature-logo.js, a
// 256×256 turquoise (#038387) image with an alpha mask, decoded via
// scripts/genlogo with no re-encoding, resampling or colour conversion.
// This test decompresses the two committed .flate assets exactly the
// way the runtime path (addLogoObjects) embeds them into the PDF, and
// checks the pixel data itself — not just the declared /Width /Height,
// which TestLogoXObjectDeclaresRealPixelDimensions in stamp_test.go
// already covers separately.
func TestLogoAssetIs256x256WithAlphaApplied(t *testing.T) {
	rgb := inflate(t, logoRGBFlate)
	alpha := inflate(t, logoAlphaFlate)

	wantRGBLen := LogoImagePixels * LogoImagePixels * 3
	if len(rgb) != wantRGBLen {
		t.Fatalf("decompressed RGB length = %d, want %d (%dx%d x 3 bytes/pixel)", len(rgb), wantRGBLen, LogoImagePixels, LogoImagePixels)
	}
	wantAlphaLen := LogoImagePixels * LogoImagePixels
	if len(alpha) != wantAlphaLen {
		t.Fatalf("decompressed alpha length = %d, want %d (%dx%d x 1 byte/pixel)", len(alpha), wantAlphaLen, LogoImagePixels, LogoImagePixels)
	}

	// The alpha mask must actually mask something: a logo with no
	// transparent pixels at all (alpha uniformly 255) would not exercise
	// /SMask in any observable way, and a logo with no opaque pixels at
	// all would not be visible. A real, applied alpha mask has both.
	var sawOpaque, sawTransparent bool
	for _, a := range alpha {
		if a == 255 {
			sawOpaque = true
		}
		if a == 0 {
			sawTransparent = true
		}
		if sawOpaque && sawTransparent {
			break
		}
	}
	if !sawOpaque {
		t.Fatal("alpha channel has no fully-opaque pixel")
	}
	if !sawTransparent {
		t.Fatal("alpha channel has no fully-transparent pixel — /SMask would have no visible effect")
	}

	// The mark's own colour, #038387 (SPEC brand turquoise), must appear
	// among fully-opaque pixels — confirming this is the real asset, not
	// a solid fill of some other colour that happens to be the right
	// size.
	const wantR, wantG, wantB = 0x03, 0x83, 0x87
	foundBrandColour := false
	for i := 0; i < LogoImagePixels*LogoImagePixels; i++ {
		if alpha[i] != 255 {
			continue
		}
		r, g, b := rgb[i*3], rgb[i*3+1], rgb[i*3+2]
		if r == wantR && g == wantG && b == wantB {
			foundBrandColour = true
			break
		}
	}
	if !foundBrandColour {
		t.Fatalf("no fully-opaque pixel has the brand colour #%02X%02X%02X", wantR, wantG, wantB)
	}
}
