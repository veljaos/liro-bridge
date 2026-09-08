package jobs

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/signing"
)

// Outcome is what one successful signature produced.
type Outcome struct {
	OutputPath    string
	AchievedLevel string

	// StampAdjusted is set when a remembered stamp position did not fit
	// this document as it stood and had to be brought inside the page's
	// margin, or when the page it was saved on is past this document's
	// last one (F6b §3).
	//
	// It is not a failure: a saved position is meant to be reused
	// across documents of different lengths and shapes, and adjusting
	// is what "reused" means. But the person has to be told, because a
	// stamp somewhere other than where they put it is a surprise if
	// nothing says so.
	StampAdjusted bool
}

// ErrSkipDocument is what a SignFunc returns for a document the caller
// has decided not to sign at all — not one that failed.
//
// It exists for J-3's answer: a batch can contain documents whose names
// already end in the configured output suffix, and the person is asked
// once whether to skip them. A skipped document is not a failure (it did
// not fail, and naming it in the report's failure list would be a lie),
// and it is not "still waiting" either, so it takes the state the runner
// already has for a document the batch did not sign: StateSkipped.
//
// A skip costs no signature and so contributes no timing sample: its
// near-zero duration would otherwise drag the measured first-signature
// and median times, which are what the ETA and the per-signature PIN
// detection are built from (F2 §5.5/§5.6, SPEC §12.9).
var ErrSkipDocument = errors.New("jobs: this document is deliberately not signed")

// SignFunc signs one document and writes it out. Injected, so the
// queue's own decisions — order, skip-and-continue, when to abort, what
// Stop means, how the ETA is computed — are testable without a card, a
// PDF engine or a window. Everything F6 §7's "bad timing" list asks
// about is a SignFunc that fails in a particular way at a particular
// document.
//
// index is the item's position in the queue; item is a copy of it as it
// stood when the runner reached it.
type SignFunc func(ctx context.Context, index int, item Item) (Outcome, error)

// Progress is one moment of a run, pushed to the interface whenever it
// changes. It carries no strings: every word on screen is chosen by the
// interface from the user's own catalogue (SPEC §7 — codes, never
// prose).
type Progress struct {
	// Phase is what the interface should be showing.
	Phase Phase

	// Current is how many documents have been attempted, Total how many
	// there are.
	Current int
	Total   int

	// ETA is the estimated remaining time for the whole batch, and
	// ETAKnown says whether it means anything yet. Computed from the
	// measured first signature and the measured median of the rest
	// (signing.EstimatedTotal, F2 §5.6) — never from a constant, so it
	// stays honest when a card or a key size changes (SPEC §12.9).
	ETA      time.Duration
	ETAKnown bool

	// PerSignaturePIN is true once the batch's measured timings say the
	// card is asking for a PIN per signature rather than per batch (F2
	// §5.5). The interface says so; a person who is about to be asked
	// for a PIN a hundred times should be told before the second one.
	PerSignaturePIN bool

	// Succeeded, Failed and Skipped count documents so far.
	Succeeded int
	Failed    int
	Skipped   int
}

// Phase is the coarse state of a run.
type Phase string

const (
	// PhasePreparingCard covers the first signature, which is card
	// initialisation and was measured at about 4.9 seconds (SPEC
	// §12.9). It is indeterminate on purpose: a progress bar sitting
	// still at 0% for five seconds reads as a freeze (F6 §3).
	PhasePreparingCard Phase = "preparingCard"
	// PhaseSigning is every signature after the first is under way.
	PhaseSigning Phase = "signing"
	// PhaseFinished means the run is over, however it ended.
	PhaseFinished Phase = "finished"
)

// Report is what happened, once a run is over (F6 §5).
type Report struct {
	Succeeded int
	Failed    int
	Skipped   int

	// Failures lists every document that was attempted and did not
	// work, by display name and code. The interface turns each code
	// into a sentence; no code ever reaches the screen (SPEC §7).
	Failures []Failure

	// OutputDir is where the signatures were written, when they all
	// went to one place — which is the normal case, and the one the
	// "open the output folder" button needs. Empty when the batch wrote
	// to more than one folder.
	OutputDir string

	// AchievedLevel is the weakest level any successful document
	// reached, as pades.Level's own string. The weakest, not the best:
	// reporting a batch as B-LT when one document in it fell back to
	// B-B would be claiming a level that was not reached (SPEC §18.11).
	AchievedLevel string

	// Stopped is true when the user pressed Stop and the run ended
	// early with documents still waiting.
	Stopped bool

	// Aborted is true when the run ended early because continuing was
	// pointless — the card was removed, or the PIN is now blocked
	// (F6 §3). AbortCode says which.
	Aborted   bool
	AbortCode errs.Code

	// StampAdjusted counts the documents whose stamp had to be moved to
	// fit — a page that was shorter than the one the position was
	// chosen on, or a smaller page box. Zero for a batch that all took
	// the position as it stood, which is the ordinary case.
	StampAdjusted int

	// Timing is the measured shape of this batch, for the log and for
	// anyone asking why it took what it took.
	Timing signing.TimingReport
}

// Failure is one document that did not sign.
type Failure struct {
	// Name is the sanitised display name (F5 §5.3) — never a path.
	Name string
	Code errs.Code
}

// Hooks receive a run's progress. Every one may be nil.
type Hooks struct {
	// OnItem fires whenever one item's state changes, with its index
	// and the item as it now stands.
	OnItem func(index int, item Item)
	// OnProgress fires whenever the overall picture changes.
	OnProgress func(Progress)
}

// Runner executes a queue. It exists as a type, rather than Run being a
// method on Queue, because a run has one thing a queue does not: a stop
// switch somebody else is holding.
type Runner struct {
	// stopped is set by Stop from another goroutine — the interface
	// thread, when the user presses the button — and read by the run
	// loop between documents.
	stopped atomic.Bool
}

// Stop asks the run to finish the document it is signing and then end
// (F6 §3). It never interrupts a signature in flight: a cancelled
// signature is how a half-written file gets left on disk, and F6 §3 and
// SPEC §18.10 both forbid that. On a batch of a hundred at the measured
// ~0.41s per document, the wait a person sees is one document long.
//
// Safe to call from any goroutine, at any time, including before the
// run starts and after it ends.
func (r *Runner) Stop() { r.stopped.Store(true) }

// Stopped reports whether Stop has been called.
func (r *Runner) Stopped() bool { return r.stopped.Load() }

// Run signs every document in q, in order, through one session, and
// returns what happened.
//
// The rules it implements, each from a specific requirement:
//
//   - One document at a time, in queue order. The card is a serial
//     device (F2 §4.2, D-027).
//   - A document that fails is skipped and the run continues (SPEC
//     §12.10) — the alternative is asking for the PIN again for
//     everything after it.
//   - Except when continuing is pointless: CARD_NOT_PRESENT and
//     PIN_LOCKED end the run (F6 §3, F2 §5.3), and every document not
//     reached is marked skipped rather than left looking as though it
//     is still waiting.
//   - Stop ends the run after the document in flight, never during it.
//   - ctx cancellation is checked in the same place, and means the same
//     thing, so a closing window cannot tear down a signature midway.
//
// q's items are updated in place as the run proceeds, so the caller's
// snapshot after Run reflects what happened to each one.
func (r *Runner) Run(ctx context.Context, q *Queue, sign SignFunc, hooks Hooks) Report {
	return r.RunItems(ctx, q.items, sign, hooks)
}

// RunItems is Run over a plain slice rather than a Queue.
//
// It exists because a batch that arrived over the protocol has no
// queue: its documents are digests a caller computed, or PDF bytes it
// sent, and neither is a file on disk that a Queue could be built from
// (a Queue's whole surface — Add, folder expansion, duplicate paths,
// output paths — is about files). Everything a run actually decides,
// though, is identical for both: one document at a time in order,
// skip-and-continue, the two codes that end a batch, Stop between
// documents and never during one, and an ETA from measurement. That is
// this function, and both front doors use it rather than each having
// their own.
//
// items is updated in place, so the caller's own slice reflects what
// happened to each document.
func (r *Runner) RunItems(ctx context.Context, items []Item, sign SignFunc, hooks Hooks) Report {
	report := Report{}
	var durations []time.Duration
	outputDirs := map[string]bool{}

	emitProgress := func(phase Phase, current int) {
		if hooks.OnProgress == nil {
			return
		}
		p := Progress{
			Phase:     phase,
			Current:   current,
			Total:     len(items),
			Succeeded: report.Succeeded,
			Failed:    report.Failed,
			Skipped:   report.Skipped,
		}
		p.ETA, p.ETAKnown = etaFor(durations, len(items)-current)
		p.PerSignaturePIN = signing.DetectPINPolicy(durations) == signing.PINPolicyPerSignature
		hooks.OnProgress(p)
	}

	emitProgress(PhasePreparingCard, 0)

	for i := range items {
		if r.stopped.Load() || ctx.Err() != nil {
			report.Stopped = true
			break
		}

		items[i].State = StateSigning
		if hooks.OnItem != nil {
			hooks.OnItem(i, items[i])
		}

		start := time.Now()
		outcome, err := sign(ctx, i, items[i])
		elapsed := time.Since(start)

		if errors.Is(err, ErrSkipDocument) {
			// Deliberately not signed. No timing sample, no failure
			// entry, and a state that says what happened.
			items[i].State = StateSkipped
			report.Skipped++
			if hooks.OnItem != nil {
				hooks.OnItem(i, items[i])
			}
			emitProgress(PhaseSigning, i+1)
			continue
		}
		durations = append(durations, elapsed)

		if err != nil {
			code := signing.CodeOf(err)
			items[i].State = StateFailed
			items[i].FailureCode = code
			report.Failed++
			report.Failures = append(report.Failures, Failure{Name: items[i].DisplayName, Code: code})
			if hooks.OnItem != nil {
				hooks.OnItem(i, items[i])
			}
			if signing.AbortsBatch(code) {
				report.Aborted = true
				report.AbortCode = code
				break
			}
		} else {
			items[i].State = StateDone
			items[i].OutputPath = outcome.OutputPath
			items[i].AchievedLevel = outcome.AchievedLevel
			report.Succeeded++
			report.AchievedLevel = weakestLevel(report.AchievedLevel, outcome.AchievedLevel)
			if outcome.StampAdjusted {
				report.StampAdjusted++
			}
			if dir := parentDir(outcome.OutputPath); dir != "" {
				outputDirs[dir] = true
			}
			if hooks.OnItem != nil {
				hooks.OnItem(i, items[i])
			}
		}

		phase := PhaseSigning
		if len(durations) == 0 {
			phase = PhasePreparingCard
		}
		emitProgress(phase, i+1)
	}

	// Anything the run never reached is skipped, not still waiting. A
	// row left saying "waiting" after the run has ended is a row that
	// looks like the program forgot about it.
	for i := range items {
		if items[i].State == StateWaiting {
			items[i].State = StateSkipped
			report.Skipped++
			if hooks.OnItem != nil {
				hooks.OnItem(i, items[i])
			}
		}
	}

	if len(outputDirs) == 1 {
		for dir := range outputDirs {
			report.OutputDir = dir
		}
	}
	report.Timing = signing.BuildTimingReport(durations)
	emitProgress(PhaseFinished, len(items))
	return report
}

// etaFor is the estimate a run reports at one moment of itself: the
// measured first signature, plus what is still to come at the measured
// median of the signatures after the first (F2 §5.6, SPEC §12.9).
//
// durations holds one entry per signature attempt made so far, in the
// order they were made; remaining is how many documents the run has not
// reached. Nothing has been measured yet means no estimate at all —
// the indeterminate "Preparing card…" state — rather than a guess.
//
// It is a function rather than four lines inside RunItems so that the
// property SPEC §12.9 actually asks for — that this number comes from
// what was measured and never from a constant — can be asserted by
// handing it durations. Producing durations by sleeping and then
// comparing two runs asserts something else: that the machine ran the
// short sleep faster than the long one, which on a loaded two-core
// runner is not true and is not a property of this code (D-201, D-112).
func etaFor(durations []time.Duration, remaining int) (time.Duration, bool) {
	if len(durations) == 0 {
		return 0, false
	}
	var median time.Duration
	if len(durations) > 1 {
		median = signing.MedianOf(durations[1:])
	}
	return signing.EstimatedTotal(durations[0], remaining, median)
}

// parentDir is filepath.Dir, but empty for an empty path rather than
// ".", so an unset OutputPath cannot make a batch look as though it
// wrote to the working directory.
func parentDir(path string) string {
	if path == "" {
		return ""
	}
	return filepath.Dir(path)
}

// levelRank orders the three PAdES levels so weakestLevel can pick the
// lowest one a batch actually reached. The strings are pades.Level's
// own; this package does not import internal/pades for three constants
// it only ever compares.
func levelRank(level string) int {
	switch level {
	case "B-B":
		return 1
	case "B-T":
		return 2
	case "B-LT":
		return 3
	default:
		return 0
	}
}

// weakestLevel returns whichever of a and b is the lower level,
// ignoring an unset one. SPEC §18.11: a batch's reported level is the
// weakest any document in it reached, never the best.
func weakestLevel(a, b string) string {
	if levelRank(a) == 0 {
		return b
	}
	if levelRank(b) == 0 {
		return a
	}
	if levelRank(b) < levelRank(a) {
		return b
	}
	return a
}
