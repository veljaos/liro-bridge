//go:build windows

package pinscreen

import (
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// collect is the seam the window is behind.
//
// A package variable rather than a parameter, because Entry's signature is
// fixed by pkcs11.PINEntry and the thing behind it is a native modal dialog
// with its own message loop. It is the same seam cmd/liro-bridge already uses
// for auditStore and interactiveGather (D-236), and it exists so that
// everything in this file except the call itself is testable: that the screen
// is asked exactly once, that a cancellation comes back as a cancellation, and
// that the caller's buffer is handed through rather than copied.
var collect = ui.CollectPIN

// Entry returns the PIN entry: a pkcs11.PINEntry that draws the real dialog.
//
// owner is the HWND of the window the signature is being approved in —
// ui.Window's Handle(). It is disabled while the dialog is up and re-enabled
// before it is destroyed, so a person cannot answer two windows at once and
// activation returns to the right one (D-129). Zero is correct for a caller
// that has no window of its own, which is what scripts/p11worker is; the
// dialog then centres on the monitor under the cursor.
//
// # This function does four things and must never do a fifth
//
//   - builds the prompt (Text, in the neutral file)
//   - shows the dialog, once
//   - hands the caller's buffer straight through
//   - turns "the person cancelled" into pkcs11.ErrPINCancelled
//
// It does not loop, and there is nothing here for a loop to do. SPEC §6.5.1
// clause 5: "Nothing retries a PIN automatically, ever, for any reason. One
// wrong PIN is one attempt. Three block the card, and for a national identity
// card unblocking means a visit to a police station."
//
// It does not read dst, copy it, or take its length for anything but the one
// argument ui.CollectPIN needs. The buffer belongs to the function that will
// pass it to C_Login and overwrite it, and the whole reason pkcs11.PINEntry
// fills a caller's buffer instead of returning a PIN is that there is then
// exactly one copy and that function owns it.
//
// And it does not log. There is nothing here worth a log line that is not
// already in the request, and the one thing that would be worth one is the one
// thing clause 3 forbids recording.
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
			// ui.ErrPINTooLong arrives here, and it is returned rather than
			// folded into a cancellation. Those two were one answer out of
			// CollectPIN until this function existed — see ui.ErrPINTooLong,
			// which this caller is the reason for.
			return 0, err
		}
		if !ok {
			// A cancellation is not a failure and must not be reported as one
			// (D-145) — and it must not be reported as a PIN of length zero
			// either, which is what returning (0, nil) would be. D-268
			// measured what an empty PIN costs: the module handed it to the
			// card, the card counted it as wrong, and one of three attempts
			// was gone.
			return 0, pkcs11.ErrPINCancelled
		}
		return n, nil
	}
}
