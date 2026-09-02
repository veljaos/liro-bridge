package pdf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// DefaultReservedBytes is the default /Contents reservation (F3 §4.3).
// Evidence, from the real fixtures the specification was written
// against: their CMS blobs were 11 917, 10 052 and 14 039 bytes,
// carrying the signer certificate, up to two chain certificates and a
// ~7 KB signature timestamp token. 32 KB leaves the largest of those
// more than 2x headroom, at a file-size cost that is irrelevant.
const DefaultReservedBytes = 32768

// byteRangeFieldWidth is the fixed character width of each /ByteRange
// number (F3 §4.4). 10 digits supports files up to 9.9 GB, comfortably
// above MaxInputSize (512 MB), so the length-neutral rewrite in step 3
// of BuildPlaceholder never needs a wider field than was reserved.
const byteRangeFieldWidth = 10

// PlaceholderOptions configures BuildPlaceholder (F3 §4).
type PlaceholderOptions struct {
	// ReservedBytes is the /Contents reservation, in raw (not
	// hex-encoded) bytes. Zero means DefaultReservedBytes.
	ReservedBytes int

	// SubFilter is the signature dictionary's /SubFilter (F3 §12.2):
	// "ETSI.CAdES.detached" for a signature revision, "ETSI.RFC3161"
	// for a document-timestamp revision.
	SubFilter Name

	// SigningDate becomes the dictionary's /M entry — a PDF metadata
	// field. This is unrelated to, and never a substitute for, the CMS
	// signingTime attribute, which F3 §5.2 forbids outright: PAdES
	// takes the time from the RFC 3161 timestamp, not the signer's
	// clock.
	SigningDate time.Time

	// FieldName is the AcroForm field's /T value. Empty means
	// BuildPlaceholder chooses one itself: the first of
	// "Liro-Signature-1", "Liro-Signature-2", ... not already present
	// anywhere in the document's existing /Fields tree (Task 1 fix —
	// see docs/decisions.md). /T must be unique in the AcroForm field
	// tree; two fields sharing a name are one field with two widgets to
	// any reader that assembles the tree, which is exactly what made a
	// hard-coded "Signature1" collide with a document's own signature
	// field of the same name.
	FieldName string

	// PageNumber selects the target page for the signature widget
	// (F4 §2.1: "works on any page, not only the first"). Zero means
	// the first page — F3's original, unchanged default. -1 means the
	// last page.
	PageNumber int

	// Appearance, when non-nil, makes the widget visible: Rect is its
	// placement rectangle (already computed, in the page's own
	// content-space coordinates) and FormXObject is the indirect
	// reference to an already-appended Form XObject that draws the
	// stamp. This package never builds that content itself — F4 §6
	// requires the default, invisible path (Appearance == nil, Rect
	// [0 0 0 0], no /AP) to stay byte-for-byte unchanged, which is
	// easiest to guarantee when this package has no stamp-drawing code
	// of its own to accidentally invoke.
	Appearance *Appearance
}

// Appearance is a visible signature widget's placement and content,
// computed by internal/pades/appearance and passed in here (F4 §6).
type Appearance struct {
	Rect        [4]float64
	FormXObject Reference
}

// Placeholder is BuildPlaceholder's result: a complete file with a
// reserved, digestable signature slot. The caller computes Digest(),
// builds the CMS over it, and calls InjectSignature.
type Placeholder struct {
	// Bytes is the complete file, including the real /ByteRange values
	// but still-zero /Contents.
	Bytes []byte

	// ByteRange is the real [0 b c d] array now written into Bytes.
	ByteRange [4]int64

	SigObjectNum int

	contentsStart  int64 // absolute offset of the first hex digit
	contentsHexLen int   // reserved hex-digit count (2 * ReservedBytes)
}

// Digest returns SHA-256 over exactly the byte spans /ByteRange
// names — [0,b) and [c,c+d) — recomputed from ph.Bytes and ph.ByteRange
// themselves (F3 §4.2 step 4). Nothing here trusts an offset variable
// computed earlier in BuildPlaceholder; that separation is what lets the
// F3 §4.5 test recompute the same digest independently, from offsets it
// reads back out of the file.
func (ph *Placeholder) Digest() [32]byte {
	b, c, d := ph.ByteRange[1], ph.ByteRange[2], ph.ByteRange[3]
	h := sha256.New()
	h.Write(ph.Bytes[:b])
	h.Write(ph.Bytes[c : c+d])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// InjectSignature hex-encodes cms into the reserved /Contents slot,
// zero-padding the remainder, and fails loudly instead of truncating if
// cms does not fit the reservation (F3 §4.3).
func (ph *Placeholder) InjectSignature(cms []byte) error {
	if len(cms)*2 > ph.contentsHexLen {
		return errs.WithDetails(errs.CodeSignFailed,
			fmt.Errorf("CMS is %d bytes, reservation only fits %d", len(cms), ph.contentsHexLen/2),
			map[string]any{"cmsBytes": len(cms), "reservedBytes": ph.contentsHexLen / 2})
	}
	field := make([]byte, ph.contentsHexLen)
	encoded := strings.ToUpper(hex.EncodeToString(cms))
	copy(field, encoded)
	for i := len(encoded); i < len(field); i++ {
		field[i] = '0'
	}
	copy(ph.Bytes[ph.contentsStart:ph.contentsStart+int64(ph.contentsHexLen)], field)
	return nil
}

// BuildPlaceholder appends one incremental revision (F3 §3.3) that adds
// an invisible signature field to doc's first page — the signature
// dictionary itself (with reserved /Contents and /ByteRange), a widget
// annotation doubling as the AcroForm field, the AcroForm dictionary
// (created if the document had none), the modified page, and the
// modified catalog if /AcroForm had to be added to it — and rewrites
// /ByteRange with the real offsets (F3 §4.2).
func BuildPlaceholder(doc *Document, opts PlaceholderOptions) (*Placeholder, error) {
	reserved := opts.ReservedBytes
	if reserved <= 0 {
		reserved = DefaultReservedBytes
	}

	rootRef, ok := doc.Trailer().Get(Name("Root")).(Reference)
	if !ok {
		return nil, fmt.Errorf("pdf: trailer /Root is not an indirect reference")
	}
	catalog, ok := doc.ResolveDict(rootRef)
	if !ok {
		return nil, fmt.Errorf("pdf: /Root does not resolve to a dictionary")
	}
	targetPage := opts.PageNumber
	if targetPage == 0 {
		targetPage = 1
	}
	pageNum, err := FindPage(doc, catalog, targetPage)
	if err != nil {
		return nil, err
	}
	pageDict, ok := doc.ResolveDict(Reference{Num: pageNum})
	if !ok {
		return nil, fmt.Errorf("pdf: page %d does not resolve to a dictionary", pageNum)
	}

	u := NewUpdate(doc)
	sigNum := u.NewObjectNumber()
	widgetNum := u.NewObjectNumber()

	catalogCopy := copyDict(catalog)
	var acroFormNum int
	var acroFormDict Dict
	var fields Array
	acroFormInline := false
	catalogModified := false
	switch af := catalog.Get(Name("AcroForm")).(type) {
	case Reference:
		acroFormNum = af.Num
		existing, _ := doc.ResolveDict(af)
		acroFormDict = copyDict(existing)
		fields, _ = doc.Resolve(existing.Get(Name("Fields"))).(Array)
	case Dict:
		// An inline (non-indirect) /AcroForm is unusual but legal — and,
		// measured directly against halcom.pdf, mup.pdf and posta.pdf, is
		// exactly how all three real fixtures carry it. Promoting it to a
		// new indirect object (the previous behaviour here) changes
		// /AcroForm's identity: inline in the catalog before, a separate
		// object after. Acrobat's incremental-update analysis compares
		// revisions, not just resolved final state, and reads that change
		// as the entire form being replaced rather than one field being
		// added — the field tree then fails to assemble and the Signature
		// Panel renders empty, including the document's own earlier
		// signatures. So /AcroForm stays inline here, with /Fields
		// extended in place inside the rewritten catalog — the same rule
		// /Annots below already follows correctly. See docs/decisions.md.
		acroFormDict = copyDict(af)
		fields, _ = af.Get(Name("Fields")).(Array)
		acroFormInline = true
		catalogModified = true
	default:
		acroFormNum = u.NewObjectNumber()
		acroFormDict = Dict{}
		catalogCopy[Name("AcroForm")] = Reference{Num: acroFormNum}
		catalogModified = true
	}
	fieldName := opts.FieldName
	if fieldName == "" {
		fieldName = uniqueFieldName(doc, fields)
	}

	fields = append(append(Array{}, fields...), Reference{Num: widgetNum})
	acroFormDict[Name("Fields")] = fields
	acroFormDict[Name("SigFlags")] = int64(3)
	if acroFormInline {
		catalogCopy[Name("AcroForm")] = acroFormDict
	}

	pageCopy := copyDict(pageDict)
	annots, _ := doc.Resolve(pageDict.Get(Name("Annots"))).(Array)
	annots = append(append(Array{}, annots...), Reference{Num: widgetNum})
	pageCopy[Name("Annots")] = annots

	rect := Array{int64(0), int64(0), int64(0), int64(0)}
	widgetDict := Dict{
		Name("Type"):    Name("Annot"),
		Name("Subtype"): Name("Widget"),
		Name("FT"):      Name("Sig"),
		Name("Rect"):    rect,
		Name("F"):       int64(4), // Print — conventional for a signature widget even when invisible (Rect [0 0 0 0] draws nothing regardless)
		Name("T"):       String(fieldName),
		Name("V"):       Reference{Num: sigNum},
		Name("P"):       Reference{Num: pageNum},
	}
	if opts.Appearance != nil {
		// F4 §6: this branch is the only thing separating a visible stamp
		// from the default invisible path, and it only runs when a
		// caller explicitly supplies Appearance.
		r := opts.Appearance.Rect
		widgetDict[Name("Rect")] = Array{r[0], r[1], r[2], r[3]}
		widgetDict[Name("AP")] = Dict{Name("N"): opts.Appearance.FormXObject}
	}

	u.Set(widgetNum, widgetDict)
	if !acroFormInline {
		u.Set(acroFormNum, acroFormDict)
	}
	u.Set(pageNum, pageCopy)
	if catalogModified {
		u.Set(rootRef.Num, catalogCopy)
	}

	dictBytes, relByteRangeStart, relContentsOpenAngle := buildSignatureDictBytes(reserved, opts.SubFilter, opts.SigningDate)
	u.Set(sigNum, Raw(dictBytes))

	out, err := u.Apply()
	if err != nil {
		return nil, err
	}

	sigStart, ok := u.OffsetOf(sigNum)
	if !ok {
		return nil, fmt.Errorf("pdf: internal error: signature object offset not recorded")
	}
	header := fmt.Sprintf("%d 0 obj\n", sigNum)
	base := int64(sigStart + len(header))
	b := base + int64(relContentsOpenAngle)
	contentsHexStart := b + 1
	hexLen := int64(reserved * 2)
	c := contentsHexStart + hexLen + 1 // +1 for the closing '>'
	d := int64(len(out)) - c

	byteRange := [4]int64{0, b, c, d}
	writeByteRangeValues(out, base+int64(relByteRangeStart), byteRange)

	return &Placeholder{
		Bytes:          out,
		ByteRange:      byteRange,
		SigObjectNum:   sigNum,
		contentsStart:  contentsHexStart,
		contentsHexLen: int(hexLen),
	}, nil
}

// buildSignatureDictBytes hand-builds the signature dictionary's exact
// bytes (F3 §4.1: this is too dangerous to build through a generic,
// map-iteration-order dictionary writer). It returns the dictionary
// bytes plus the byte offsets, relative to the start of those bytes, of
// the first /ByteRange digit and of the '<' opening /Contents.
func buildSignatureDictBytes(reservedBytes int, subFilter Name, m time.Time) (dictBytes []byte, relByteRangeStart, relContentsOpenAngle int) {
	var buf bytes.Buffer
	buf.WriteString("<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /")
	writeNameEscaped(&buf, subFilter)
	buf.WriteString(" /ByteRange [")
	relByteRangeStart = buf.Len()
	placeholderField := strings.Repeat("0", byteRangeFieldWidth)
	for i := 0; i < 4; i++ {
		if i > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(placeholderField)
	}
	buf.WriteString("] /Contents ")
	relContentsOpenAngle = buf.Len()
	buf.WriteByte('<')
	buf.WriteString(strings.Repeat("0", reservedBytes*2))
	buf.WriteByte('>')
	buf.WriteString(" /M ")
	buf.WriteString(formatPDFDate(m))
	buf.WriteString(" >>")
	return buf.Bytes(), relByteRangeStart, relContentsOpenAngle
}

// byteRangeTotalWidth is the total byte length of the four
// placeholder-width numbers plus their three single-space separators —
// the fixed span writeByteRangeValues must exactly fill, since every
// offset after it in the file depends on that length never changing
// (F3 §4.2 step 3).
const byteRangeTotalWidth = 4*byteRangeFieldWidth + 3

// writeByteRangeValues overwrites the four /ByteRange fields at
// absStart with br's real values, written compactly — single spaces
// between numbers — with all the padding needed to preserve
// byteRangeTotalWidth placed after the last number (Task 1 fix — see
// docs/decisions.md).
//
// F3 §4.4's original instruction ("some validators dislike leading
// zeros in the final values, so use spaces, not zeros") was read as
// "right-align each field independently within its own fixed width" —
// the array rendered as "[         0     603053     668591        615]".
// Adobe Acrobat rejected that with "Unexpected byte range values
// defining scope of signed data": Adobe does not parse /ByteRange
// through its general object parser but scans it from raw bytes before
// the document is resolved, and that scanner is strict about the shape
// "[0 603053 668591 615                        ]" — real numbers
// packed together with single-space separators, all padding trailing
// after the last one — which is what every reference implementation
// (the document's own MUP-produced signature, iText, PDFBox, pyHanko)
// writes. Padding each field to its own width independently, as before,
// put a run of up to nine consecutive spaces ahead of every number but
// the first; writing the numbers compactly and moving all the slack to
// the end avoids that entirely.
func writeByteRangeValues(out []byte, absStart int64, br [4]int64) {
	compact := fmt.Sprintf("%d %d %d %d", br[0], br[1], br[2], br[3])
	if len(compact) > byteRangeTotalWidth {
		// Unreachable in practice: byteRangeFieldWidth (10 digits) was
		// sized for files up to 9.9 GB, far above MaxInputSize, so a
		// real offset never needs more room than the placeholder
		// reserved. Guarded rather than silently truncated, per SPEC
		// §0's instruction not to silently soften a constraint.
		panic(fmt.Sprintf("pdf: /ByteRange value %q exceeds reserved width %d", compact, byteRangeTotalWidth))
	}
	field := make([]byte, byteRangeTotalWidth)
	copy(field, compact)
	for i := len(compact); i < len(field); i++ {
		field[i] = ' '
	}
	copy(out[absStart:absStart+int64(byteRangeTotalWidth)], field)
}

// formatPDFDate formats t as a PDF date string, e.g.
// "(D:20260901101422+01'00')" (PDF 32000-1 §7.9.4), for the signature
// dictionary's /M entry.
func formatPDFDate(t time.Time) string {
	_, offset := t.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	hh := offset / 3600
	mm := (offset % 3600) / 60
	return fmt.Sprintf("(D:%04d%02d%02d%02d%02d%02d%s%02d'%02d')",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), sign, hh, mm)
}

// collectFieldNames walks fields — an AcroForm /Fields array — and every
// /Kids subtree beneath it, returning every /T value found at any depth
// (Task 1). Field names can be hierarchical (a non-terminal field's /T
// is only a partial name, qualified by its /Parent chain), so a scan of
// the top-level array alone would miss a /T value that only appears on
// a nested field — collecting from the full tree is what makes the
// resulting name actually unused anywhere in the document.
func collectFieldNames(doc *Document, fields Array) map[string]bool {
	names := make(map[string]bool)
	visited := make(map[int]bool)
	var walk func(Array)
	walk = func(arr Array) {
		for _, item := range arr {
			if ref, ok := item.(Reference); ok {
				if visited[ref.Num] {
					continue // guards against a malformed circular /Kids chain
				}
				visited[ref.Num] = true
			}
			dict, ok := doc.ResolveDict(item)
			if !ok {
				continue
			}
			if t, ok := dict.Get(Name("T")).(String); ok {
				names[string(t)] = true
			}
			if kids, ok := doc.Resolve(dict.Get(Name("Kids"))).(Array); ok {
				walk(kids)
			}
		}
	}
	walk(fields)
	return names
}

// uniqueFieldName returns a signature field name absent from fields'
// entire tree (Task 1). Adobe, NexU and the eUprava applet all emit
// "SignatureN", so a document already signed in Serbia is likely to
// already contain those names — "Liro-Signature-N" is chosen specifically
// to avoid that collision rather than risk repeating it.
func uniqueFieldName(doc *Document, fields Array) string {
	existing := collectFieldNames(doc, fields)
	for i := 1; ; i++ {
		name := fmt.Sprintf("Liro-Signature-%d", i)
		if !existing[name] {
			return name
		}
	}
}

// copyDict returns a shallow copy of d, so a modified object can be
// appended as a full copy without ever mutating the parsed original
// (F3 §3.3).
func copyDict(d Dict) Dict {
	out := make(Dict, len(d)+1)
	for k, v := range d {
		out[k] = v
	}
	return out
}
