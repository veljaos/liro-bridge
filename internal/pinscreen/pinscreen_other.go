//go:build !windows && !linux

package pinscreen

import (
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// Entry returns a PIN entry that refuses, because there is no dialog to draw
// yet on this platform.
//
// It returns ErrNoDialogOnThisPlatform rather than pkcs11.ErrNoPINEntry, and
// the difference is the whole of what this file says: ErrNoPINEntry means
// nobody wired one up, which is a caller's omission and fixable by wiring one
// up. This is a platform that has no PIN dialog at all, which is F12 §5's work
// and not a mistake at the call site.
//
// The sentinel was ui.ErrUnsupportedPlatform until [[D-335]], and this file no
// longer imports internal/ui at all. Its declaration carries both reasons; the
// one that belongs here is that a refusal is not a window, so the package that
// draws windows did not have to be named to write one.
//
// Text, in the neutral file, builds the same sentences here as it does on
// Windows — deliberately, since F12 §5's GTK dialog will want them and a
// second copy of SPEC §6.5.1 clause 6's wording is the thing this package
// exists to prevent. What is missing on this platform is the window, not the
// words.
func Entry(cat *i18n.Catalogue, owner uintptr) pkcs11.PINEntry {
	// Both parameters are unused here and are kept so that the signature is
	// one signature. A caller written for the agent must compile for every
	// platform this project builds for (F0 §10's cross-compilation), and a
	// function whose shape changed per platform would make that a second
	// wiring rather than a second implementation.
	_, _ = cat, owner
	return func(dst []byte, req pkcs11.PINRequest) (int, error) {
		_, _ = dst, req
		return 0, ErrNoDialogOnThisPlatform
	}
}
