//go:build linux

package pinscreen

import (
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// collect is the seam the window is behind, and it is a package
// variable for the same reason the Windows one is: Entry's signature is
// fixed by pkcs11.PINEntry, and everything in this file except the call
// itself is then testable without a display.
var collect = ui.CollectPIN

// Entry returns the PIN entry: a pkcs11.PINEntry that draws the real
// GTK dialog (SPEC §10, F12 §5).
//
// **The body is the Windows one and that is deliberate.** What differs
// between the platforms is which window opens, and that difference is
// inside ui.CollectPIN. Everything here — asking once, handing the
// caller's buffer straight through, turning a cancellation into
// pkcs11.ErrPINCancelled — is SPEC §6.5.1, which is the same on both,
// and a second copy of it would be a second thing to keep in step.
//
// owner is the window the signature is being approved in. On Windows it
// is disabled while the dialog is up; on this platform a client cannot
// parent itself onto an arbitrary toplevel under Wayland (D-337), so
// ui.CollectPIN ignores it and makes the dialog modal instead. The
// parameter stays so that one caller compiles for both.
//
// # This function does four things and must never do a fifth
//
//   - builds the prompt (Text, in the neutral file)
//   - shows the dialog, once
//   - hands the caller's buffer straight through
//   - turns "the person cancelled" into pkcs11.ErrPINCancelled
//
// It does not loop, and there is nothing here for a loop to do. SPEC
// §6.5.1 clause 5: nothing retries a PIN automatically, ever, for any
// reason. One wrong PIN is one attempt; three block the card.
//
// It does not read dst, copy it, or take its length for anything but
// the one argument ui.CollectPIN needs. And it does not log: the one
// thing here that would be worth a log line is the one thing clause 3
// forbids recording.
func Entry(cat *i18n.Catalogue, owner uintptr) pkcs11.PINEntry {
	return func(dst []byte, req pkcs11.PINRequest) (int, error) {
		p := Text(cat, req, len(dst))
		n, ok, err := collect(owner, ui.PINPrompt{
			Title:   p.Title,
			Heading: p.Heading,
			Subject: p.Subject,
			Label:   p.Label,
			Hint:    p.Hint,
			OK:      p.OK,
			Cancel:  p.Cancel,
		}, len(dst), dst)
		if err != nil {
			return 0, err
		}
		if !ok {
			// A cancellation is not a failure and must not be reported
			// as one (D-145) — and not as a PIN of length zero either,
			// which is what (0, nil) would be. D-268 measured what an
			// empty PIN costs: the module handed it to the card, the
			// card counted it wrong, and one of three attempts was
			// gone.
			return 0, pkcs11.ErrPINCancelled
		}
		return n, nil
	}
}
