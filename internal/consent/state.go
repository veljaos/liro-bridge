package consent

import (
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/signing"
)

// State is one of the five states the consent window's progress area
// can be in (F5 §5.4).
type State string

const (
	// StateWaiting shows the summary and the Approve/Cancel buttons.
	StateWaiting State = "waiting"
	// StatePreparingCard covers the first signature's card
	// initialisation (measured ~4.9s, SPEC §12.9): indeterminate, no
	// numbers, no percentage.
	StatePreparingCard State = "preparingCard"
	// StateSigning shows "Signing N of Total" with a determinate bar.
	StateSigning State = "signing"
	StateDone    State = "done"
	StateFailed  State = "failed"
)

// Progress is what the page renders for the current moment of a batch
// in progress (F5 §5.4). A single Progress value, pushed to the page
// via Window.PostJSON each time it changes, drives the whole state
// machine — the page has no timers or state of its own beyond what it
// is told.
type Progress struct {
	State State

	// Current and Total apply to StateSigning ("Signing 7 of 100").
	Current int
	Total   int

	// ETA is populated only once known (F5 §5.4: computed from the
	// measured first signature, SPEC §12.9 — never a constant).
	ETA      time.Duration
	ETAKnown bool

	// PerSignaturePIN is true once the batch's PIN policy is detected as
	// per-signature (F2 §5.5) — the page changes its message to warn
	// the PIN will be requested repeatedly (F5 §5.4).
	PerSignaturePIN bool

	// Succeeded and Failed apply to StateDone.
	Succeeded  int
	Failed     int
	OutputPath string

	// FailureCode applies to StateFailed — never rendered directly; the
	// page maps it to an action via the table in F5 §5.5 (ActionForError).
	FailureCode errs.Code
}

// ProgressForTiming builds the StateSigning/StatePreparingCard Progress
// for one point in a batch's execution. first/median mirror the exact
// measurements internal/signing.EstimatedTotal takes (F2 §5.6): first
// <= 0 means no signature has completed yet, which is
// StatePreparingCard's whole trigger — an indeterminate state instead
// of a progress bar sitting at 0% for the ~4.9s card-initialisation
// window (F5 §5.4/SPEC §12.9).
func ProgressForTiming(current, total int, first, median time.Duration, pinPolicy signing.PINPolicy) Progress {
	if first <= 0 {
		return Progress{State: StatePreparingCard, Current: current, Total: total}
	}
	eta, ok := signing.EstimatedTotal(first, total-current, median)
	return Progress{
		State:           StateSigning,
		Current:         current,
		Total:           total,
		ETA:             eta,
		ETAKnown:        ok,
		PerSignaturePIN: pinPolicy == signing.PINPolicyPerSignature,
	}
}
