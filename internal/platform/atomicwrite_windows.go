package platform

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isSharingViolation reports whether err — a failed rename onto path —
// is Windows saying the destination is held by another program, as
// opposed to any other reason a write could not happen.
//
// Three of the four statuses are unambiguous:
//
//	ERROR_SHARING_VIOLATION (32) — the other program opened the file
//	    without FILE_SHARE_DELETE, which is what a C runtime's
//	    fopen("rb") and most Windows programs merely displaying a file
//	    do.
//	ERROR_LOCK_VIOLATION (33) — a byte-range lock on the destination.
//	ERROR_USER_MAPPED_FILE (1224) — the destination is memory-mapped,
//	    which is how some readers open a PDF.
//
// ERROR_ACCESS_DENIED (5) is the fourth, and it is ambiguous. It is what
// J-8 measured for a destination open in another program — a reader that
// had only opened the old signed file made os.Rename fail with "Access
// is denied" — and it is also, measured here directly, what a rename
// onto a read-only file returns. The two need opposite answers: one
// needs a file closed, the other needs an attribute cleared. So this
// asks which it is rather than guessing, and treats a read-only
// destination as what it is: a write that could not happen, not a file
// somebody is holding.
//
// The folder is not in question by this point: WriteFileAtomic only
// consults this after creating, writing and closing a temporary file in
// that same folder.
func isSharingViolation(path string, err error) bool {
	var errno windows.Errno
	if !errors.As(err, &errno) {
		return false
	}
	switch errno {
	case windows.ERROR_SHARING_VIOLATION,
		windows.ERROR_LOCK_VIOLATION,
		windows.ERROR_USER_MAPPED_FILE:
		return true
	case windows.ERROR_ACCESS_DENIED:
		return !isReadOnly(path)
	}
	return false
}

// isReadOnly reports whether path carries FILE_ATTRIBUTE_READONLY. An
// attribute that cannot be read at all is reported as not read-only:
// the caller has already established the file exists, so a failure here
// says nothing useful, and the fallback keeps the more specific,
// actionable message rather than losing it to a second unknown.
func isReadOnly(path string) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return false
	}
	return attrs&windows.FILE_ATTRIBUTE_READONLY != 0
}
