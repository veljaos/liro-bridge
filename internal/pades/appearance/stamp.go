package appearance

import (
	"bytes"
	"fmt"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// nominalFontSize and reducedFontSize implement F4 §5.4's overflow rule
// ("reduce the font size in one step, then truncate with an ellipsis").
// Neither value is specified by SPEC or F4; these are this project's
// own choice for a stamp whose fixed width is 190pt minus a 36pt logo
// and padding, documented in docs/decisions.md.
const (
	nominalFontSize = 7.0
	reducedFontSize = 6.0
)

// truncationMark replaces SPEC §13's literal "…" with three ASCII
// periods: U+2026 is not in this project's font subset (F4 §3.2 lists
// exactly ASCII, the Serbian Latin extras and the Serbian Cyrillic
// alphabet — a typographic ellipsis is none of those), and three
// ASCII periods need no extension to the subset for a purely cosmetic
// choice. Recorded in docs/decisions.md.
const truncationMark = "..."

// Options configures Render (F4 §5/§7). None of it is read from a
// certificate directly — the caller (internal/pades) extracts
// SignerName from givenName/surname (SPEC §11.7) and DocumentID from
// the certificate's IDCRS-prefixed identifier (F4 §5.2) before this
// package ever sees a string, so that the national identity number
// (PNORS, discarded long before either of those extraction steps) has
// no code path into this package at all — "unreachable from this
// package" is true by construction, not by convention.
type Options struct {
	// Label is already localised (F4 §5.3): "Digitally signed by" /
	// "Digitalno potpisao" / "Дигитално потписао".
	Label string

	// SignerName is never localised (F4 §5.3/SPEC §9.3): whatever script
	// the certificate's givenName/surname carry, rendered as-is.
	SignerName string

	// Reference is the caller-supplied document reference line
	// (--stamp-reference), shown only when non-empty.
	Reference string

	// DocumentID is the identity document number line (--stamp-show-
	// document-id), shown only when non-empty — which the caller
	// guarantees is only when that flag was explicitly passed (F4 §5.2:
	// "opt-in... never the default").
	DocumentID string

	// SerialHex and SigningTime build the mandatory fourth line.
	SerialHex   string
	SigningTime string // already formatted by the caller

	// Corner anchors the stamp when UseXY is false (the default path).
	Corner Corner

	// UseXY, X, Y select --stamp-xy's explicit placement instead of a
	// corner (F4 §7: the two are mutually exclusive, enforced by the
	// caller before Options ever reaches this package).
	UseXY bool
	X, Y  float64
}

// Render builds every object this stamp needs (font, logo, content
// stream, Form XObject — F4 §3/§4) as new objects in u, and returns the
// signature widget's placement, ready for
// pdf.PlaceholderOptions.Appearance (F4 §6). doc and pageDict identify
// the target page, used only to resolve /MediaBox and /Rotate
// (F4 §2.1).
func Render(doc *pdf.Document, pageDict pdf.Dict, u *pdf.Update, opts Options) (pdf.Appearance, error) {
	lines, err := buildLines(opts)
	if err != nil {
		return pdf.Appearance{}, err
	}
	height, err := HeightForLines(len(lines))
	if err != nil {
		return pdf.Appearance{}, err
	}

	var rect [4]float64
	var matrix [6]float64
	if opts.UseXY {
		rect = [4]float64{opts.X, opts.Y, opts.X + StampWidth, opts.Y + height}
		matrix = [6]float64{1, 0, 0, 1, 0, 0}
	} else {
		box := pdf.ResolveMediaBox(doc, pageDict)
		rotate := NormaliseRotate(pdf.ResolveRotate(doc, pageDict))
		rect, matrix, err = PlaceCorner(box, rotate, opts.Corner, Margin, StampWidth, height)
		if err != nil {
			return pdf.Appearance{}, err
		}
	}

	fonts := addFontObjects(u)
	logoNum := addLogoObjects(u)

	content, err := buildContentStream(lines, height)
	if err != nil {
		return pdf.Appearance{}, err
	}

	resources := pdf.Dict{
		pdf.Name("Font"):    pdf.Dict{pdf.Name("F1"): pdf.Reference{Num: fonts.Type0Num}},
		pdf.Name("XObject"): pdf.Dict{pdf.Name("Logo"): pdf.Reference{Num: logoNum}},
	}
	formNum := u.NewObjectNumber()
	u.Set(formNum, &pdf.Stream{
		Dict: pdf.Dict{
			pdf.Name("Type"):      pdf.Name("XObject"),
			pdf.Name("Subtype"):   pdf.Name("Form"),
			pdf.Name("BBox"):      pdf.Array{int64(0), int64(0), float64(StampWidth), height},
			pdf.Name("Matrix"):    pdf.Array{matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5]},
			pdf.Name("Resources"): resources,
		},
		Raw: content,
	})

	return pdf.Appearance{Rect: rect, FormXObject: pdf.Reference{Num: formNum}}, nil
}

// buildLines assembles the stamp's text lines in order (F4 §5): label
// and signer name are always present, reference and the identity
// document number are each independently optional. Up to five lines
// results (heightsByLineCount's fifth entry exists for exactly the case
// where both optional lines are present at once).
func buildLines(opts Options) ([]string, error) {
	if opts.Label == "" || opts.SignerName == "" {
		return nil, fmt.Errorf("appearance: Label and SignerName are required")
	}
	lines := []string{opts.Label, opts.SignerName}
	if opts.Reference != "" {
		lines = append(lines, opts.Reference)
	}
	if opts.DocumentID != "" {
		lines = append(lines, opts.DocumentID)
	}
	lines = append(lines, fmt.Sprintf("SN %s  %s", opts.SerialHex, opts.SigningTime))
	return lines, nil
}

// buildContentStream draws the logo (bottom-left, F4 §4) and the text
// lines (right of the logo, vertically distributed to fill the box) in
// the stamp's own local, always-upright coordinate space — Render's
// Form XObject /Matrix is what makes this display correctly regardless
// of the target page's /Rotate (see geometry.go's rotationPlan doc
// comment).
func buildContentStream(lines []string, height float64) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "q %g 0 0 %g %g %g cm /Logo Do Q\n", float64(LogoSize), float64(LogoSize), float64(Padding), float64(Padding))

	textX := float64(Padding + LogoSize + Padding)
	maxTextWidth := StampWidth - textX - Padding

	n := len(lines)
	slot := (height - 2*Padding) / float64(n)

	b.WriteString("BT\n")
	for i, line := range lines {
		fitted, size, err := fitLine(line, maxTextWidth)
		if err != nil {
			return nil, err
		}
		cids, err := EncodeCIDs(fitted)
		if err != nil {
			return nil, err
		}
		top := height - Padding - float64(i)*slot
		baseline := top - slot*0.72 // roughly centres the glyph body within its slot
		fmt.Fprintf(&b, "/F1 %.2f Tf 1 0 0 1 %.2f %.2f Tm %s Tj\n", size, textX, baseline, cidsToHex(cids))
	}
	b.WriteString("ET\n")
	return b.Bytes(), nil
}

// fitLine implements F4 §5.4 exactly: try the nominal size, then the
// (single-step) reduced size, then truncate with an ellipsis at the
// reduced size — never widening the stamp, never wrapping.
func fitLine(text string, maxWidth float64) (string, float64, error) {
	widthAt := func(s string, size float64) (float64, error) {
		w1000, err := TextWidth1000(s)
		if err != nil {
			return 0, err
		}
		return float64(w1000) * size / 1000, nil
	}

	if w, err := widthAt(text, nominalFontSize); err != nil {
		return "", 0, err
	} else if w <= maxWidth {
		return text, nominalFontSize, nil
	}
	if w, err := widthAt(text, reducedFontSize); err != nil {
		return "", 0, err
	} else if w <= maxWidth {
		return text, reducedFontSize, nil
	}

	ellipsisWidth, err := widthAt(truncationMark, reducedFontSize)
	if err != nil {
		return "", 0, err
	}
	runes := []rune(text)
	for len(runes) > 0 {
		w, err := widthAt(string(runes), reducedFontSize)
		if err != nil {
			return "", 0, err
		}
		if w+ellipsisWidth <= maxWidth {
			break
		}
		runes = runes[:len(runes)-1]
	}
	return string(runes) + truncationMark, reducedFontSize, nil
}
