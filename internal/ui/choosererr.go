package ui

import "errors"

// The two ways a chooser can fail that a person is told about (D-410).
//
// Only the Linux choosers return them; the Windows dialogs are the system's
// own, called in process, and never do. They are declared here, with no
// build tag, so that the windows that open a chooser can say something when
// one fails without a platform file of their own, and so that on Windows
// that code is present and never reached.
var (
	// ErrChooserFailed is a chooser that could not be shown: the desktop's
	// portal refused, did not answer, or is not there. The error wrapping
	// it says which, for the log.
	ErrChooserFailed = errors.New("ui: the chooser could not be shown")

	// ErrChooserExpired is a chooser closed by its ceiling (chooser.Ceiling)
	// after nobody answered it.
	ErrChooserExpired = errors.New("ui: the chooser was closed after being open too long")
)
