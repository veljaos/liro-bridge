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
	// StateTSAChoice is SPEC §12.8's required choice, asked of the
	// person signing rather than decided for them (Task 1, F5
	// second-real-run review): sign without a timestamp at B-B,
	// configure a timestamp authority, or cancel. It is reached either
	// before the batch begins (no TSA is configured at all) or during it
	// (a TSA is configured but did not answer after F3 §6.3's three
	// attempts) — the same two situations `--on-tsa-failure` already
	// covers on the command line, where the flag is visible and this
	// choice is not.
	StateTSAChoice State = "tsaChoice"
	// StateOutputExists is the choice offered when the file the signed
	// document would be written to already exists (Task 4, F5
	// fourth-real-run review): overwrite it, save under a different
	// name, or cancel. SPEC §12.11 forbids silently replacing a file,
	// and the previous build honoured that by refusing — but reported
	// the refusal as "an unexpected error occurred", which reads as a
	// defect rather than as the protection it is.
	StateOutputExists State = "outputExists"
	// StateAlreadySigned is the choice offered when the batch contains
	// documents whose own names already end in the configured output
	// suffix — that is, documents this program has very probably
	// produced already (J-3). Signing one produces a second signature on
	// top of the first, and its output picks up a second suffix:
	// ugovor-signed-signed.pdf.
	//
	// It is a choice rather than a rule because both answers are
	// legitimate: counter-signing a "ugovor-signed.pdf" that arrived
	// from somebody else is an ordinary thing to want, and guessing
	// which of the two was meant is how a helpful rule becomes a wrong
	// one. The screen says how many there are and offers to skip them;
	// the answer applies to the whole batch, like the output-file
	// choice above.
	StateAlreadySigned State = "alreadySigned"
)

// TSAReason says why the timestamp step cannot complete — the two
// situations StateTSAChoice is reached from. The page renders a
// different sentence for each; both offer the same three actions.
type TSAReason string

const (
	// TSAReasonNotConfigured: no timestamp authority is configured.
	// This project ships no default one (D-067, and Task 1a's own
	// decision), so it is the out-of-the-box state.
	TSAReasonNotConfigured TSAReason = "notConfigured"
	// TSAReasonUnreachable: one is configured, and did not answer after
	// the retry policy in F3 §6.3.
	TSAReasonUnreachable TSAReason = "unreachable"
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

	// AchievedLevel is the PAdES level the batch actually reached, as
	// pades.Level's own string ("B-B", "B-T", "B-LT"). Shown on the done
	// screen, always, and marked as a downgrade when it is B-B: SPEC
	// §12.8 allows saving without a timestamp only when it is "visibly
	// marked", and §18.11 forbids ever claiming a level that was not
	// reached.
	AchievedLevel string

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
