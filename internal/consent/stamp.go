package consent

// The visible-stamp decision offered on the consent window (Task 1, F5
// fourth-real-run review). SPEC §13.4 keeps stamp generation entirely
// separate from the invisible signature path, and the command line
// keeps --stamp as the only thing that reaches it; this type is the
// window's equivalent of that flag, plus the corner it draws in.
//
// It lives in this package, not in cmd/, because it is part of what the
// consent window renders and reads back, and because the four corners
// must be validated in exactly one place: the page offers them, the
// configuration stores one, and a value arriving from either has to
// mean the same thing.

// StampPositionBottomRight is SPEC §13.1's own default corner.
const (
	StampPositionBottomRight = "bottom-right"
	StampPositionBottomLeft  = "bottom-left"
	StampPositionTopRight    = "top-right"
	StampPositionTopLeft     = "top-left"
)

// StampPositions lists the four corners in the order the window offers
// them: the default first, then the remaining three. SPEC §13.1 defers
// a visual placement picker to a later phase, so these four are the
// whole vocabulary — explicit coordinates stay a command-line
// capability (--stamp-xy).
func StampPositions() []string {
	return []string{
		StampPositionBottomRight,
		StampPositionBottomLeft,
		StampPositionTopRight,
		StampPositionTopLeft,
	}
}

// ValidStampPosition reports whether position is one of the four
// corners. Anything else — a stale configuration file, a page that has
// been tampered with — is not silently mapped onto a corner; the
// caller substitutes the default and says so.
func ValidStampPosition(position string) bool {
	for _, p := range StampPositions() {
		if p == position {
			return true
		}
	}
	return false
}

// StampChoice is what the consent window shows and reads back: whether
// to draw a visible stamp, and where. Both values are persisted in the
// configuration so the next signature starts from the same answer
// (Task 1).
type StampChoice struct {
	Visible  bool
	Position string
}

// DefaultStampChoice is the out-of-the-box answer: a visible stamp, in
// the bottom-right corner. On by default because a signature the
// signer cannot see reads as one that was never applied — the finding
// this exists to fix. See docs/decisions.md (D-103).
func DefaultStampChoice() StampChoice {
	return StampChoice{Visible: true, Position: StampPositionBottomRight}
}

// Normalised returns c with an unrecognised position replaced by the
// default corner, so nothing downstream has to re-check it.
func (c StampChoice) Normalised() StampChoice {
	if !ValidStampPosition(c.Position) {
		c.Position = StampPositionBottomRight
	}
	return c
}
