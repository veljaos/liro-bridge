package pades

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"image"
	"strconv"
	"time"

	"github.com/veljaos/liro-bridge/internal/pades/appearance"
	"github.com/veljaos/liro-bridge/internal/pades/pdf"
	"github.com/veljaos/liro-bridge/internal/pades/render"
)

// StampPreview is a picture of the stamp exactly as it will be drawn,
// and the size in points it will occupy.
type StampPreview struct {
	Image      *image.RGBA
	WidthPt    float64
	HeightPt   float64
	SignerName string
}

// RenderStampPreview draws the stamp on its own, at scale pixels per
// point, for the placement window to show under the cursor (F6b §2.4:
// "The stamp is drawn as it will actually appear — logo, four lines,
// real text from the certificate — not as an empty box").
//
// It builds a one-page document the size of the stamp plus its margins,
// puts the real stamp on it through the same appearance.Render the
// signing path uses, and rasterises the result. Nothing about the stamp
// is redrawn or approximated for the preview: what the person places is
// the object that ends up in the document, which is the only way "what
// you place is what you get" can be true rather than nearly true.
func RenderStampPreview(signerCert *x509.Certificate, signingDate time.Time, stamp *StampOptions, scale float64) (StampPreview, error) {
	if stamp == nil {
		return StampPreview{}, fmt.Errorf("pades: no stamp to preview")
	}
	if scale <= 0 {
		return StampPreview{}, fmt.Errorf("pades: preview scale must be positive")
	}
	opts := buildAppearanceOptions(signerCert, signingDate, stamp)
	height, err := appearance.HeightFor(opts)
	if err != nil {
		return StampPreview{}, err
	}

	// The page is the stamp plus a margin on every side, and the stamp
	// is asked for at exactly the margin — which is where it lands,
	// because that is the one position the clamp does not move.
	const m = appearance.Margin
	pageW := float64(appearance.StampWidth) + 2*m
	pageH := height + 2*m

	doc, err := pdf.Parse(minimalPage(pageW, pageH))
	if err != nil {
		return StampPreview{}, err
	}
	pageDict, err := doc.PageDict(1)
	if err != nil {
		return StampPreview{}, err
	}

	u := pdf.NewUpdate(doc)
	place := opts
	place.UseXY = true
	place.X, place.Y = m, m
	place.Corner = appearance.BottomRight
	ap, err := appearance.Render(doc, pageDict, u, place)
	if err != nil {
		return StampPreview{}, wrapStampError(err)
	}

	// A widget annotation is how the stamp reaches a page in a signed
	// document, so it is how it reaches this one: the preview then goes
	// through the renderer's annotation path, the same one that draws
	// a stamp already on somebody else's document.
	annotNum := u.NewObjectNumber()
	u.Set(annotNum, pdf.Dict{
		pdf.Name("Type"):    pdf.Name("Annot"),
		pdf.Name("Subtype"): pdf.Name("Widget"),
		pdf.Name("Rect"):    pdf.Array{ap.Rect[0], ap.Rect[1], ap.Rect[2], ap.Rect[3]},
		pdf.Name("F"):       int64(4),
		pdf.Name("AP"):      pdf.Dict{pdf.Name("N"): ap.FormXObject},
	})
	page := pdf.Dict{}
	for k, v := range pageDict {
		page[k] = v
	}
	page[pdf.Name("Annots")] = pdf.Array{pdf.Reference{Num: annotNum}}
	u.Set(previewPageObject, page)

	out, err := u.Apply()
	if err != nil {
		return StampPreview{}, err
	}

	rdoc, err := render.Open(out)
	if err != nil {
		return StampPreview{}, err
	}
	res, err := rdoc.RenderPage(1, scale)
	if err != nil {
		return StampPreview{}, err
	}

	// Crop the margin back off, so the image is the stamp and nothing
	// else and the window can position it by its own corner.
	crop := image.Rect(
		int(m*scale), int(m*scale),
		int((m+float64(appearance.StampWidth))*scale), int((m+height)*scale),
	).Intersect(res.Image.Bounds())
	cropped := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	for y := 0; y < crop.Dy(); y++ {
		src := res.Image.PixOffset(crop.Min.X, crop.Min.Y+y)
		dst := cropped.PixOffset(0, y)
		copy(cropped.Pix[dst:dst+crop.Dx()*4], res.Image.Pix[src:src+crop.Dx()*4])
	}

	return StampPreview{
		Image:      cropped,
		WidthPt:    float64(appearance.StampWidth),
		HeightPt:   height,
		SignerName: opts.SignerName,
	}, nil
}

// previewPageObject is the page object's number in minimalPage below.
const previewPageObject = 3

// minimalPage writes the smallest complete PDF that has one page of the
// given size: a catalog, a page tree node and an empty page. It exists
// so the stamp preview has something to be drawn onto that carries
// nothing else at all.
func minimalPage(w, h float64) []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.7\n")
	obj("<</Type /Catalog /Pages 2 0 R>>")
	obj("<</Type /Pages /Count 1 /Kids [3 0 R]>>")
	obj(fmt.Sprintf("<</Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources <<>>>>",
		formatNumber(w), formatNumber(h)))
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<</Size %d /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

func formatNumber(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 4, 64)
}
