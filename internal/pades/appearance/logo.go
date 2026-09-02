package appearance

import (
	_ "embed"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// logoRGBFlate and logoAlphaFlate are the stamp's logo — the real
// 256×256 Liro mark (LogoImagePixels) from assets/signature-logo.js,
// placed at a fixed 36×36pt (LogoSize) — extracted at build time by
// scripts/genlogo, still zlib-compressed exactly as supplied. The
// runtime path never encodes or decodes image data, only copies these
// bytes into the PDF's /XObject and /SMask streams directly.
//
//go:embed logo-rgb.flate
var logoRGBFlate []byte

//go:embed logo-alpha.flate
var logoAlphaFlate []byte

// addLogoObjects allocates and writes the logo's two image objects (the
// RGB XObject and its alpha mask, referenced through /SMask — F4 §4)
// and returns the RGB XObject's object number, the one /Resources
// /XObject entries and the content stream's Do operator reference.
func addLogoObjects(u *pdf.Update) int {
	alphaNum := u.NewObjectNumber()
	rgbNum := u.NewObjectNumber()

	u.Set(alphaNum, &pdf.Stream{
		Dict: pdf.Dict{
			pdf.Name("Type"):             pdf.Name("XObject"),
			pdf.Name("Subtype"):          pdf.Name("Image"),
			pdf.Name("Width"):            int64(LogoImagePixels),
			pdf.Name("Height"):           int64(LogoImagePixels),
			pdf.Name("ColorSpace"):       pdf.Name("DeviceGray"),
			pdf.Name("BitsPerComponent"): int64(8),
			pdf.Name("Filter"):           pdf.Name("FlateDecode"),
		},
		Raw: logoAlphaFlate,
	})

	u.Set(rgbNum, &pdf.Stream{
		Dict: pdf.Dict{
			pdf.Name("Type"):             pdf.Name("XObject"),
			pdf.Name("Subtype"):          pdf.Name("Image"),
			pdf.Name("Width"):            int64(LogoImagePixels),
			pdf.Name("Height"):           int64(LogoImagePixels),
			pdf.Name("ColorSpace"):       pdf.Name("DeviceRGB"),
			pdf.Name("BitsPerComponent"): int64(8),
			pdf.Name("Filter"):           pdf.Name("FlateDecode"),
			pdf.Name("SMask"):            pdf.Reference{Num: alphaNum},
		},
		Raw: logoRGBFlate,
	})

	return rgbNum
}
