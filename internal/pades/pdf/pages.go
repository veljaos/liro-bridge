package pdf

import (
	"fmt"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// a4MediaBox is the fallback page box (F4 §2.1: "Fall back to A4 only
// when it is genuinely absent everywhere"), in points — the standard
// ISO 216 A4 size rounded to whole points, the same rounding PDF
// producers conventionally use.
var a4MediaBox = [4]float64{0, 0, 595, 842}

// maxPageTreeDepth bounds the /Pages tree walk against a cyclic or
// pathologically deep tree in a malformed document, matching
// findFirstPage's own existing limit.
const maxPageTreeDepth = 50

// collectPages walks the page tree from ref (the catalog's /Pages node)
// in reading order and returns every leaf /Page object's number.
func collectPages(doc *Document, ref Reference, depth int) ([]int, error) {
	if depth > maxPageTreeDepth {
		return nil, fmt.Errorf("pdf: page tree deeper than %d levels", maxPageTreeDepth)
	}
	dict, ok := doc.ResolveDict(ref)
	if !ok {
		return nil, errs.New(errs.CodePDFInvalid, fmt.Errorf("pdf: page tree node %d does not resolve to a dictionary", ref.Num))
	}
	if dict.GetName(Name("Type")) == "Page" {
		return []int{ref.Num}, nil
	}
	kids, _ := doc.Resolve(dict.Get(Name("Kids"))).(Array)
	var out []int
	for _, k := range kids {
		kref, ok := k.(Reference)
		if !ok {
			continue
		}
		pages, err := collectPages(doc, kref, depth+1)
		if err != nil {
			continue // a malformed sibling does not sink the whole tree
		}
		out = append(out, pages...)
	}
	if len(out) == 0 {
		return nil, errs.New(errs.CodePDFInvalid, fmt.Errorf("pdf: no /Page found under node %d", ref.Num))
	}
	return out, nil
}

// FindPage resolves the catalog's /Pages tree and returns the object
// number of page n (1-based reading order; n == -1 means the last page —
// F4 §2/§7's --stamp-page). Works on any page, not only the first
// (F4 §2.1).
func FindPage(doc *Document, catalog Dict, n int) (int, error) {
	pagesRef, ok := catalog.Get(Name("Pages")).(Reference)
	if !ok {
		return 0, fmt.Errorf("pdf: catalog /Pages is not an indirect reference")
	}
	pages, err := collectPages(doc, pagesRef, 0)
	if err != nil {
		return 0, err
	}
	if n == -1 {
		return pages[len(pages)-1], nil
	}
	if n < 1 || n > len(pages) {
		return 0, fmt.Errorf("pdf: page %d requested, document has %d page(s)", n, len(pages))
	}
	return pages[n-1], nil
}

// ResolveMediaBox returns pageDict's /MediaBox, following the /Parent
// chain when it is not set directly on the page (F4 §2.1: "MediaBox is
// very commonly inherited from the Pages node rather than set per
// page"), falling back to A4 only when no ancestor sets one either.
func ResolveMediaBox(doc *Document, pageDict Dict) [4]float64 {
	d := pageDict
	for depth := 0; depth < maxPageTreeDepth; depth++ {
		if box, ok := mediaBoxOf(d); ok {
			return box
		}
		parentRef, ok := d.Get(Name("Parent")).(Reference)
		if !ok {
			break
		}
		next, ok := doc.ResolveDict(parentRef)
		if !ok {
			break
		}
		d = next
	}
	return a4MediaBox
}

func mediaBoxOf(d Dict) ([4]float64, bool) {
	arr, ok := d.Get(Name("MediaBox")).(Array)
	if !ok || len(arr) != 4 {
		return [4]float64{}, false
	}
	var box [4]float64
	for i, v := range arr {
		n, ok := asFloat64(v)
		if !ok {
			return [4]float64{}, false
		}
		box[i] = n
	}
	// PDF permits a /MediaBox with corners in either order; normalise so
	// box[0] < box[2] and box[1] < box[3], which every caller (this
	// package's own placeholder geometry included) can then rely on.
	if box[0] > box[2] {
		box[0], box[2] = box[2], box[0]
	}
	if box[1] > box[3] {
		box[1], box[3] = box[3], box[1]
	}
	return box, true
}

func asFloat64(o Object) (float64, bool) {
	switch v := o.(type) {
	case int64:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

// ResolveRotate returns pageDict's /Rotate, following the /Parent chain
// the same way ResolveMediaBox does (F4 §2.1), normalised to
// 0/90/180/270. Absent anywhere, it defaults to 0.
func ResolveRotate(doc *Document, pageDict Dict) int {
	d := pageDict
	for depth := 0; depth < maxPageTreeDepth; depth++ {
		if v, ok := asFloat64(d.Get(Name("Rotate"))); ok {
			r := int(v) % 360
			if r < 0 {
				r += 360
			}
			return r
		}
		parentRef, ok := d.Get(Name("Parent")).(Reference)
		if !ok {
			break
		}
		next, ok := doc.ResolveDict(parentRef)
		if !ok {
			break
		}
		d = next
	}
	return 0
}
