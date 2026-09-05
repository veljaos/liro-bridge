package appearance

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"

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
	// Label is line 1, already localised by the caller (F4 §5.3):
	// "Digitally signed" / "Digitalno potpisano" / "Дигитално
	// потписано". It is the one line whose script follows the interface
	// language rather than the certificate.
	Label string

	// SignerName is never localised and never transliterated (F4
	// §5.3/SPEC §9.3): whatever script the certificate's
	// givenName/surname carry is the script it is drawn in, in a
	// Cyrillic, Latin or English interface alike. buildLines
	// upper-cases it, which is a change of case, not of script.
	SignerName string

	// Reference is the caller-supplied document reference line
	// (--stamp-reference), shown only when non-empty.
	Reference string

	// DocumentID is the identity document number line (--stamp-show-
	// document-id), shown only when non-empty — which the caller
	// guarantees is only when that flag was explicitly passed (F4 §5.2:
	// "opt-in... never the default").
	DocumentID string

	// SerialHex is the certificate serial, upper-case hexadecimal, and
	// SigningTime the already-formatted signing date. They are lines 3
	// and 4, one each; buildLines adds the "SN " prefix.
	SerialHex   string
	SigningTime string

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
		// F6 §6: explicit coordinates are clamped into the page box
		// less the margin, never rejected and never drawn hanging off
		// the edge. The margin is the same 24 pt every corner
		// placement keeps.
		box := pdf.ResolveMediaBox(doc, pageDict)
		x, y, moved := ClampToPageBox(box, opts.X, opts.Y, StampWidth, height, Margin)
		if moved {
			slog.Info("appearance: stamp coordinates moved inside the page margin",
				"requestedX", opts.X, "requestedY", opts.Y, "x", x, "y", y)
		}
		rect = [4]float64{x, y, x + StampWidth, y + height}
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

// serialPrefix labels the certificate serial on line 3. Not localised
// and not translated: it is a field name for a hexadecimal number, read
// the same way in every locale, and the three-character form is what
// fits beside a 36pt logo.
const serialPrefix = "SN "

// buildLines assembles the stamp's text lines, in the order they are
// drawn. Four are always present and always on their own line:
//
//	Дигитално потписано      the label, in the interface's language
//	ВЕЉКО СТАНОЈЕВИЋ         the signer, in the certificate's own script
//	SN 20F048A768F56F099E    the certificate serial
//	04.09.2026. 15:04:33     the signing date and time
//
// then the caller's reference line, then the identity document number,
// each only when supplied — six lines at most.
//
// The four base lines used to be three, with the serial and the time
// sharing one: two facts crowded onto one line, neither readable at a
// glance. Splitting them is why the height table (geometry.go) is sized
// the way it is rather than the other way round.
//
// Only the label follows the interface language. The signer's name is
// never transliterated — SPEC §9.3 makes certificate subject fields
// data, so a MUP certificate reads Cyrillic and a Halcom one Latin
// whatever language the window is in. The name is upper-cased, which
// changes its case and not its script: strings.ToUpper is
// Unicode-aware, so "Zoran Milovanović" becomes "ZORAN MILOVANOVIĆ"
// and "ВЕЉКО СТАНОЈЕВИЋ" is already what it will be.
func buildLines(opts Options) ([]string, error) {
	if opts.Label == "" || opts.SignerName == "" {
		return nil, fmt.Errorf("appearance: Label and SignerName are required")
	}
	lines := []string{
		opts.Label,
		strings.ToUpper(opts.SignerName),
		serialPrefix + opts.SerialHex,
		opts.SigningTime,
	}
	if opts.Reference != "" {
		lines = append(lines, opts.Reference)
	}
	if opts.DocumentID != "" {
		lines = append(lines, opts.DocumentID)
	}
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
