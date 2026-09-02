package windowscng

import (
	"fmt"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// Windows/CNG status codes this package treats specially (F2 §2.4). Kept
// in a plain, OS-independent file — unlike the DLL calls themselves —
// so the mapping table has a real, always-running unit test rather than
// depending on the Windows-only build to prove it, which CI's own test
// step (running on ubuntu-latest) never exercises.
const (
	scardWCancelledByUser uint32 = 0x8010006E
	scardWWrongCHV        uint32 = 0x8010006B
	scardWCHVBlocked      uint32 = 0x8010006C
	scardWRemovedCard     uint32 = 0x80100069
	nteBadKeyset          uint32 = 0x80090016
	nteNoKey              uint32 = 0x8009000D
)

// mapStatus converts a raw Windows/CNG status code into the errs.Code
// table from F2 §2.4. op names the failing call; it is carried only for
// local logging via the wrapped cause, never serialised — errs.Error's
// only cross-boundary fields are Code and Details (SPEC §7).
func mapStatus(op string, status uint32) *errs.Error {
	switch status {
	case scardWCancelledByUser:
		return errs.New(errs.CodeConsentDenied, fmt.Errorf("%s: cancelled by user (status 0x%08X)", op, status))
	case scardWWrongCHV:
		return errs.New(errs.CodePINIncorrect, fmt.Errorf("%s: wrong PIN (status 0x%08X)", op, status))
	case scardWCHVBlocked:
		return errs.New(errs.CodePINLocked, fmt.Errorf("%s: PIN blocked (status 0x%08X)", op, status))
	case scardWRemovedCard:
		return errs.New(errs.CodeCardNotPresent, fmt.Errorf("%s: card removed (status 0x%08X)", op, status))
	case nteBadKeyset:
		return errs.New(errs.CodeCertNotFound, fmt.Errorf("%s: bad keyset (status 0x%08X)", op, status))
	case nteNoKey:
		return errs.New(errs.CodeCertNotUsable, fmt.Errorf("%s: no key (status 0x%08X)", op, status))
	default:
		// Never retried automatically (F2 §2.4) and never turned into a
		// human-readable message across a boundary (SPEC §7) — the hex
		// status is structured Details, not prose.
		return errs.WithDetails(errs.CodeSignFailed, fmt.Errorf("%s: unmapped status 0x%08X", op, status),
			map[string]any{"status": fmt.Sprintf("0x%08X", status)})
	}
}
