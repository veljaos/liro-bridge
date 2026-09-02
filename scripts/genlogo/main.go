// Command genlogo produces the visual stamp's logo asset (F4 §4) from
// the real Liro brand mark supplied in assets/signature-logo.js: a
// 256×256 turquoise (#038387) mark, exported as raw RGB and 8-bit alpha
// channels, already zlib-compressed (RFC 1950) in exactly the form a
// PDF /FlateDecode image stream and its /SMask require.
//
// This generator does not draw anything or touch a pixel: it extracts
// the two base64 fields from the JS source, base64-decodes them, and
// writes the resulting bytes straight to internal/pades/appearance —
// no re-encoding, no resampling, no colour conversion. The runtime path
// (internal/pades/appearance/logo.go) never encodes or decodes image
// data either; it copies these bytes directly into the PDF's /XObject
// and /SMask streams. Decompressing here is only a sanity check (the
// decompressed sizes must match width×height×3 and width×height) — the
// bytes written to disk are the still-compressed source bytes,
// unmodified.
//
// Usage:
//
//	go run ./scripts/genlogo --out internal/pades/appearance
//
// See docs/decisions.md for why this replaced the earlier
// programmatically-drawn placeholder mark.
package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
)

// wantSize is the source asset's fixed geometry (assets/signature-logo.js:
// width: 256, height: 256). Checked, not assumed, against both the JS
// source's own declared fields and the decompressed pixel data.
const wantSize = 256

func main() {
	srcPath := flag.String("src", "assets/signature-logo.js", "path to the Liro logo source (assets/signature-logo.js)")
	outDir := flag.String("out", "internal/pades/appearance", "output directory")
	flag.Parse()

	src, err := os.ReadFile(*srcPath)
	if err != nil {
		log.Fatal(err)
	}

	width := extractInt(src, "width")
	height := extractInt(src, "height")
	if width != wantSize || height != wantSize {
		log.Fatalf("genlogo: %s declares %dx%d, want %dx%d", *srcPath, width, height, wantSize, wantSize)
	}

	rgbFlate := extractBase64(src, "rgbFlateBase64")
	alphaFlate := extractBase64(src, "alphaFlateBase64")

	checkDecompressedSize(rgbFlate, "rgbFlateBase64", wantSize*wantSize*3)
	checkDecompressedSize(alphaFlate, "alphaFlateBase64", wantSize*wantSize)

	if err := os.WriteFile(*outDir+"/logo-rgb.flate", rgbFlate, 0o644); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outDir+"/logo-alpha.flate", alphaFlate, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("genlogo: wrote %dx%d logo (rgb %d bytes, alpha %d bytes, compressed, from %s) to %s\n",
		wantSize, wantSize, len(rgbFlate), len(alphaFlate), *srcPath, *outDir)
}

// extractInt reads a bare numeric field, e.g. "width: 256,", from the JS
// source.
func extractInt(js []byte, key string) int {
	re := regexp.MustCompile(key + `:\s*(\d+)`)
	m := re.FindSubmatch(js)
	if m == nil {
		log.Fatalf("genlogo: field %q not found in source", key)
	}
	var n int
	if _, err := fmt.Sscanf(string(m[1]), "%d", &n); err != nil {
		log.Fatalf("genlogo: field %q is not an integer: %v", key, err)
	}
	return n
}

// extractBase64 reads a single-quoted base64 string field, e.g.
// "rgbFlateBase64: '...',", and decodes it. The decoded bytes are
// returned exactly as they were embedded — already zlib-compressed —
// with no further processing.
func extractBase64(js []byte, key string) []byte {
	re := regexp.MustCompile(key + `:\s*'([^']+)'`)
	m := re.FindSubmatch(js)
	if m == nil {
		log.Fatalf("genlogo: field %q not found in source", key)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(m[1]))
	if err != nil {
		log.Fatalf("genlogo: field %q is not valid base64: %v", key, err)
	}
	return decoded
}

// checkDecompressedSize is a read-only sanity check: it confirms
// flateBytes decompresses (RFC 1950 zlib, what PDF's /FlateDecode
// filter requires) to exactly wantBytes, without altering flateBytes
// itself — the file written to disk is still the original compressed
// data.
func checkDecompressedSize(flateBytes []byte, field string, wantBytes int) {
	r, err := zlib.NewReader(bytes.NewReader(flateBytes))
	if err != nil {
		log.Fatalf("genlogo: field %q is not valid zlib data: %v", field, err)
	}
	defer func() { _ = r.Close() }()
	n, err := io.Copy(io.Discard, r)
	if err != nil {
		log.Fatalf("genlogo: field %q failed to decompress: %v", field, err)
	}
	if n != int64(wantBytes) {
		log.Fatalf("genlogo: field %q decompresses to %d bytes, want %d", field, n, wantBytes)
	}
}
