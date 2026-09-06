package placement

// Saved is a remembered stamp placement: which page, and where on it.
//
// It is one position, not a named list of them. F6b §3 leaves that
// choice open and asks for the reasoning to be recorded: the need it
// describes is a person who signs the same shaped document a thousand
// times and wants the stamp under the same printed initials every time.
// One remembered position answers that completely. A list of named
// positions answers a different need — several document shapes, each
// with its own place — which nobody has asked for, and which brings a
// naming step, a picker, and a wrong-preset failure mode into a window
// whose whole value is that it asks one thing. It is also the easy
// direction to grow in later: a list of these is a list of these.
type Saved struct {
	// Page is the 1-based page the position was chosen on. Zero means
	// nothing is remembered.
	Page int

	// X and Y are the content-space coordinates of the stamp's
	// lower-left corner on that page, in points.
	X, Y float64
}

// IsSet reports whether anything is remembered.
func (s Saved) IsSet() bool { return s.Page > 0 }

// ResolvePage decides which page of a document a saved page number
// means, and says when it had to change.
//
// F6b §3: "a position saved on page 12 means nothing in a four-page
// document — in that case fall back to the same relative place on the
// last page and say so." The last page is where a signature goes when
// it does not go on the first, so it is the only sensible fallback; the
// position itself is kept as it stands and clamped by Fit, which is
// what "the same relative place" amounts to once the page it was
// measured on no longer exists.
func ResolvePage(want, pageCount int) (page int, fellBack bool) {
	if pageCount < 1 {
		return 1, false
	}
	if want < 1 {
		return 1, want != 0 && want != 1
	}
	if want > pageCount {
		return pageCount, true
	}
	return want, false
}

// Fit puts a saved position onto one document's page, clamped inside
// that page's margin, and says whether it had to move.
//
// A one-page contract and a fifty-page report do not have the same last
// page, and an A4 position on an A5 page is off the edge. Both are
// ordinary — a saved position is meant to be reused across documents —
// so neither is a refusal; both are an adjustment the person is told
// about afterwards.
func Fit(s Saved, box [4]float64, rotate int, stampW, stampH float64) (x, y float64, moved bool) {
	return Clamp(box, rotate, s.X, s.Y, stampW, stampH)
}

// Adjustment records what had to change for one document, so a batch
// can report it by name afterwards.
type Adjustment struct {
	// Name is the document's file name, as the report shows it.
	Name string

	// PageFellBack is set when the saved page was past this document's
	// last one.
	PageFellBack bool

	// Page is the page the stamp actually went on.
	Page int

	// Moved is set when the position had to be brought inside this
	// page's margin.
	Moved bool
}

// Adjusted reports whether anything about this document's placement
// differs from what was saved.
func (a Adjustment) Adjusted() bool { return a.PageFellBack || a.Moved }
