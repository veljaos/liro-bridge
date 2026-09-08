package jobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// queueOf builds a queue of n real files, so every test below runs
// against the same shape of input the window produces.
func queueOf(t *testing.T, n int) (*Queue, string) {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for i := 0; i < n; i++ {
		paths = append(paths, writeFile(t, dir, fmt.Sprintf("doc%03d.pdf", i), 1))
	}
	q := &Queue{}
	if added, notices := q.Add(paths); added != n {
		t.Fatalf("added = %d, want %d (notices %+v)", added, n, notices)
	}
	return q, dir
}

// alwaysSucceeds is the SignFunc for a run where nothing goes wrong.
func alwaysSucceeds(dir string) SignFunc {
	return func(_ context.Context, _ int, item Item) (Outcome, error) {
		return Outcome{
			OutputPath:    OutputPathFor(item.Path, dir, "-signed"),
			AchievedLevel: "B-LT",
		}, nil
	}
}

func TestRunSignsEveryDocumentInOrder(t *testing.T) {
	q, dir := queueOf(t, 5)
	var order []string
	var r Runner
	report := r.Run(context.Background(), q, func(_ context.Context, _ int, item Item) (Outcome, error) {
		order = append(order, item.DisplayName)
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-LT"}, nil
	}, Hooks{})

	if report.Succeeded != 5 || report.Failed != 0 || report.Skipped != 0 {
		t.Fatalf("report = %+v", report)
	}
	for i, name := range order {
		if want := fmt.Sprintf("doc%03d.pdf", i); name != want {
			t.Fatalf("signed %q at position %d, want %q", name, i, want)
		}
	}
	for _, item := range q.Items() {
		if item.State != StateDone {
			t.Fatalf("%s left in state %q", item.DisplayName, item.State)
		}
	}
	if report.AchievedLevel != "B-LT" {
		t.Fatalf("AchievedLevel = %q", report.AchievedLevel)
	}
	if report.OutputDir != filepath.Clean(dir) {
		t.Fatalf("OutputDir = %q, want %q", report.OutputDir, dir)
	}
}

// TestOneCorruptFileAmongAHundredGoodOnes is F6 §7's headline case:
// skip it, name it, carry on (SPEC §12.10).
func TestOneCorruptFileAmongAHundredGoodOnes(t *testing.T) {
	q, dir := queueOf(t, 100)
	var r Runner
	report := r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		if index == 46 {
			return Outcome{}, errs.New(errs.CodePDFInvalid, errors.New("not a PDF"))
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-LT"}, nil
	}, Hooks{})

	if report.Succeeded != 99 || report.Failed != 1 || report.Skipped != 0 {
		t.Fatalf("report = %+v, want 99 succeeded and 1 failed", report)
	}
	if len(report.Failures) != 1 || report.Failures[0].Code != errs.CodePDFInvalid {
		t.Fatalf("Failures = %+v", report.Failures)
	}
	if report.Failures[0].Name != "doc046.pdf" {
		t.Fatalf("failure named %q, want doc046.pdf — F6 §5 lists failures by name", report.Failures[0].Name)
	}
	if q.Items()[46].State != StateFailed {
		t.Fatalf("item 46 state = %q", q.Items()[46].State)
	}
	if q.Items()[47].State != StateDone {
		t.Fatal("the run did not continue past the failure")
	}
}

// TestCardRemovedAtDocumentFiftyAbortsTheRest is F6 §3's abort rule and
// F6 §7's "card removed at document fifty". Continuing is pointless, so
// the run ends — and every document it never reached says so, rather
// than sitting at "waiting" forever.
func TestCardRemovedAtDocumentFiftyAbortsTheRest(t *testing.T) {
	q, dir := queueOf(t, 100)
	var r Runner
	report := r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		if index == 50 {
			return Outcome{}, errs.New(errs.CodeCardNotPresent, errors.New("card removed"))
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-LT"}, nil
	}, Hooks{})

	if !report.Aborted || report.AbortCode != errs.CodeCardNotPresent {
		t.Fatalf("report = %+v, want an abort on CARD_NOT_PRESENT", report)
	}
	if report.Succeeded != 50 || report.Failed != 1 || report.Skipped != 49 {
		t.Fatalf("report = %+v, want 50/1/49", report)
	}
	for i, item := range q.Items() {
		var want State
		switch {
		case i < 50:
			want = StateDone
		case i == 50:
			want = StateFailed
		default:
			want = StateSkipped
		}
		if item.State != want {
			t.Fatalf("item %d state = %q, want %q", i, item.State, want)
		}
	}
}

func TestPINLockedAbortsTheRest(t *testing.T) {
	q, dir := queueOf(t, 10)
	var r Runner
	report := r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		if index == 3 {
			return Outcome{}, errs.New(errs.CodePINLocked, errors.New("blocked"))
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-T"}, nil
	}, Hooks{})
	if !report.Aborted || report.AbortCode != errs.CodePINLocked {
		t.Fatalf("report = %+v", report)
	}
}

// TestOtherFailuresNeverAbort pins the other half of the same rule:
// only those two codes end a run. Everything else is a skip.
func TestOtherFailuresNeverAbort(t *testing.T) {
	for _, code := range []errs.Code{
		errs.CodePDFInvalid, errs.CodePDFEncrypted, errs.CodeSignFailed,
		errs.CodeOutputExists, errs.CodeOutputWriteFailed, errs.CodeTSAUnavailable,
		errs.CodePINIncorrect, errs.CodeStampGlyphMissing, errs.CodeInternal,
	} {
		t.Run(string(code), func(t *testing.T) {
			q, dir := queueOf(t, 4)
			var r Runner
			report := r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
				if index == 1 {
					return Outcome{}, errs.New(code, errors.New("failed"))
				}
				return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-T"}, nil
			}, Hooks{})
			if report.Aborted {
				t.Fatalf("%s aborted the batch; only CARD_NOT_PRESENT and PIN_LOCKED may", code)
			}
			if report.Succeeded != 3 || report.Failed != 1 {
				t.Fatalf("report = %+v", report)
			}
		})
	}
}

// TestStopFinishesTheDocumentInFlight is F6 §3's Stop button. The
// document being signed when Stop arrives is completed; nothing after
// it is started. Nothing is interrupted mid-signature, because that is
// how a half-written file is left behind.
func TestStopFinishesTheDocumentInFlight(t *testing.T) {
	q, dir := queueOf(t, 10)
	var r Runner
	finished := map[int]bool{}
	report := r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		if index == 3 {
			// Pressing Stop from another goroutine, while this document
			// is being signed, is exactly what the button does.
			r.Stop()
		}
		finished[index] = true
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-LT"}, nil
	}, Hooks{})

	if !report.Stopped {
		t.Fatal("report does not say the run was stopped")
	}
	if !finished[3] {
		t.Fatal("the document in flight when Stop was pressed did not finish")
	}
	if finished[4] {
		t.Fatal("a document was started after Stop")
	}
	if report.Succeeded != 4 || report.Skipped != 6 {
		t.Fatalf("report = %+v, want 4 signed and 6 skipped", report)
	}
	if q.Items()[4].State != StateSkipped {
		t.Fatalf("item 4 state = %q, want skipped", q.Items()[4].State)
	}
}

// TestClosingTheWindowMidBatchEndsCleanly is F6 §7's "window closed
// mid-batch": the context the window owns is cancelled, and the run
// ends between documents with every state accounted for — never
// half-way through writing one.
func TestClosingTheWindowMidBatchEndsCleanly(t *testing.T) {
	q, dir := queueOf(t, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var r Runner
	started := 0
	report := r.Run(ctx, q, func(_ context.Context, index int, item Item) (Outcome, error) {
		started++
		if index == 2 {
			cancel()
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-LT"}, nil
	}, Hooks{})

	if started != 3 {
		t.Fatalf("started %d documents, want 3 — the third finishes, the fourth never begins", started)
	}
	if report.Succeeded != 3 || report.Skipped != 7 {
		t.Fatalf("report = %+v", report)
	}
	if report.Succeeded+report.Failed+report.Skipped != q.Len() {
		t.Fatal("the report does not account for every document")
	}
}

// TestETAIsMeasuredNeverConstant is SPEC §12.9 and F6 §3: no number in
// the progress may come from a constant. The estimate is handed the
// measurements a run would have taken and checked against the
// arithmetic F2 §5.6 states — first + remaining × median — so what is
// asserted is that the number is a function of the measurement, on any
// machine.
//
// This used to sign one batch through a deliberately slow SignFunc and
// another through a fast one and require the slow batch's estimate to
// be the larger. That premise is not about this code: on a loaded
// two-core runner the "slow" batch's first sleep came back in 31.7 ms
// and the "fast" one's in 41.6 ms, and the test failed for being
// right about a machine it was never asserting anything about (D-201,
// D-112).
func TestETAIsMeasuredNeverConstant(t *testing.T) {
	// Two cards, measured rather than assumed: the MUP card SPEC §12.9
	// records, and a Pošta card whose first signature is 12.7s. A
	// hard-coded 4900 would be right for one and wrong for the other,
	// which is the whole reason the estimate is computed.
	mup := []time.Duration{4900 * time.Millisecond, 410 * time.Millisecond, 413 * time.Millisecond}
	posta := []time.Duration{12700 * time.Millisecond, 900 * time.Millisecond, 906 * time.Millisecond}

	cases := []struct {
		name      string
		durations []time.Duration
		remaining int
		want      time.Duration
		wantKnown bool
	}{{
		name:      "nothing measured yet is no estimate at all",
		durations: nil,
		remaining: 100,
		wantKnown: false,
	}, {
		// One signature measured and no median yet: F2 §5.6's formula
		// with a median of zero, which is what "nothing to base the
		// rest on" arithmetically is.
		name:      "one signature measured",
		durations: mup[:1],
		remaining: 99,
		want:      4900 * time.Millisecond,
		wantKnown: true,
	}, {
		name:      "MUP card, 97 to go",
		durations: mup,
		remaining: 97,
		want:      4900*time.Millisecond + 96*411500*time.Microsecond,
		wantKnown: true,
	}, {
		// The same batch shape on a slower card. Nothing about the
		// numbers below is written down anywhere in the product: they
		// are these durations, arithmetic.
		name:      "Pošta card, 97 to go",
		durations: posta,
		remaining: 97,
		want:      12700*time.Millisecond + 96*903*time.Millisecond,
		wantKnown: true,
	}, {
		name:      "the last document",
		durations: mup,
		remaining: 1,
		want:      4900 * time.Millisecond,
		wantKnown: true,
	}}

	for _, c := range cases {
		got, known := etaFor(c.durations, c.remaining)
		if known != c.wantKnown {
			t.Errorf("%s: ETAKnown = %v, want %v", c.name, known, c.wantKnown)
			continue
		}
		if known && got != c.want {
			t.Errorf("%s: ETA = %v, want %v", c.name, got, c.want)
		}
	}

	// And the property the arithmetic exists for, stated as itself: a
	// card measured slower produces a larger estimate for the same
	// batch. No constant can do that.
	slow, _ := etaFor(posta, 97)
	fast, _ := etaFor(mup, 97)
	if slow <= fast {
		t.Fatalf("a card measured slower gave the same or a smaller estimate: %v against %v", slow, fast)
	}
	// Both directions, so the estimate is not merely monotonic in one
	// measurement that happens to be bigger.
	half := []time.Duration{mup[0] / 2, mup[1] / 2, mup[2] / 2}
	if faster, _ := etaFor(half, 97); faster >= fast {
		t.Fatalf("halving every measurement did not shrink the estimate: %v against %v", faster, fast)
	}
}

// TestTheRunnerEstimatesFromWhatItMeasured is the other half of SPEC
// §12.9 at the runner: the number that reaches the interface is this
// run's own measurement, not a value from anywhere else.
//
// It asserts an identity rather than a comparison, so it holds however
// long the machine took: the first estimate a run reports is the first
// signature it measured, and Report.Timing.FirstSignature is that same
// measurement as the run recorded it. A constant — any constant —
// fails, because a measured duration is never exactly one.
func TestTheRunnerEstimatesFromWhatItMeasured(t *testing.T) {
	q, dir := queueOf(t, 3)
	succeed := alwaysSucceeds(dir)

	// The SignFunc returns once the clock has visibly moved, because a
	// signature this run cannot measure at all is one it will not
	// estimate from: time.Since across an instant call is exactly 0 on
	// Windows, and a first signature of 0 is F2 §5.6's "no measurement
	// yet". This waits for an observation — the clock having advanced —
	// and not for a period: a loaded machine reaches it later and never
	// sooner, so nothing about it can be defeated by load. It costs one
	// clock tick, measured at ~0.5 ms on this machine.
	sign := func(ctx context.Context, index int, item Item) (Outcome, error) {
		start := time.Now()
		for time.Since(start) == 0 {
			runtime.Gosched()
		}
		return succeed(ctx, index, item)
	}

	var r Runner
	var firstKnown Progress
	found := false
	report := r.Run(context.Background(), q, sign, Hooks{
		OnProgress: func(p Progress) {
			if !found && p.ETAKnown {
				firstKnown, found = p, true
			}
		},
	})

	if !found {
		t.Fatal("no progress update ever carried a known ETA")
	}
	if report.Timing.FirstSignature <= 0 {
		t.Fatal("the run reported no measured first signature")
	}
	if firstKnown.ETA != report.Timing.FirstSignature {
		t.Fatalf("the first estimate was %v; the run measured its first signature at %v — the estimate did not come from the measurement",
			firstKnown.ETA, report.Timing.FirstSignature)
	}
}

// TestFirstProgressIsPreparingCard is F6 §3's "Preparing card…": the
// first update, before any signature has completed, is indeterminate
// and carries no ETA.
func TestFirstProgressIsPreparingCard(t *testing.T) {
	q, dir := queueOf(t, 3)
	var r Runner
	var first Progress
	seen := false
	r.Run(context.Background(), q, alwaysSucceeds(dir), Hooks{
		OnProgress: func(p Progress) {
			if !seen {
				first, seen = p, true
			}
		},
	})
	if !seen {
		t.Fatal("no progress was reported at all")
	}
	if first.Phase != PhasePreparingCard {
		t.Fatalf("first phase = %q, want preparingCard", first.Phase)
	}
	if first.ETAKnown {
		t.Fatal("the first update claimed to know an ETA before any signature had happened")
	}
	if first.Total != 3 {
		t.Fatalf("Total = %d, want 3", first.Total)
	}
}

// TestPerSignaturePINIsReportedWhenMeasured is F6 §3's "if the PIN
// policy is detected as per-signature, say so". The threshold is F2's
// own measured 2000ms (D-028), reached through signing.DetectPINPolicy
// rather than reimplemented here.
func TestPerSignaturePINIsReportedWhenMeasured(t *testing.T) {
	q, dir := queueOf(t, 4)
	var r Runner
	var last Progress
	r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		// Signatures 2 and 3 take longer than a human can dismiss a PIN
		// dialog in, which is what the detection is looking for.
		if index > 0 {
			time.Sleep(2100 * time.Millisecond)
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-T"}, nil
	}, Hooks{OnProgress: func(p Progress) { last = p }})

	if !last.PerSignaturePIN {
		t.Fatalf("PerSignaturePIN = false after a batch whose later signatures each took over two seconds (timing %+v)", last)
	}
}

func TestPerSignaturePINIsNotClaimedForAFastBatch(t *testing.T) {
	q, dir := queueOf(t, 4)
	var r Runner
	var last Progress
	r.Run(context.Background(), q, alwaysSucceeds(dir), Hooks{OnProgress: func(p Progress) { last = p }})
	if last.PerSignaturePIN {
		t.Fatal("a batch of instant signatures was reported as asking for a PIN each time")
	}
}

// TestReportedLevelIsTheWeakestReached is SPEC §18.11: a batch where
// one document fell back to B-B is a B-B batch, not a B-LT one.
func TestReportedLevelIsTheWeakestReached(t *testing.T) {
	q, dir := queueOf(t, 5)
	var r Runner
	report := r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		level := "B-LT"
		if index == 2 {
			level = "B-B"
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: level}, nil
	}, Hooks{})
	if report.AchievedLevel != "B-B" {
		t.Fatalf("AchievedLevel = %q, want B-B — the weakest any document reached", report.AchievedLevel)
	}
}

// TestOutputDirIsEmptyWhenTheBatchWroteToSeveralPlaces keeps the "open
// the output folder" button honest: there is no one folder to open.
func TestOutputDirIsEmptyWhenTheBatchWroteToSeveralPlaces(t *testing.T) {
	q, _ := queueOf(t, 2)
	dirA, dirB := t.TempDir(), t.TempDir()
	var r Runner
	report := r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		dir := dirA
		if index == 1 {
			dir = dirB
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-T"}, nil
	}, Hooks{})
	if report.OutputDir != "" {
		t.Fatalf("OutputDir = %q, want empty when a batch wrote to more than one folder", report.OutputDir)
	}
}

// TestHooksSeeEveryStateTransition is what the window's per-file rows
// are built from (F6 §3: "per-file state: waiting, signing, done,
// failed, skipped").
func TestHooksSeeEveryStateTransition(t *testing.T) {
	q, dir := queueOf(t, 3)
	var r Runner
	states := map[int][]State{}
	r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		if index == 1 {
			return Outcome{}, errs.New(errs.CodePDFInvalid, errors.New("bad"))
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-T"}, nil
	}, Hooks{OnItem: func(i int, item Item) { states[i] = append(states[i], item.State) }})

	want := map[int][]State{
		0: {StateSigning, StateDone},
		1: {StateSigning, StateFailed},
		2: {StateSigning, StateDone},
	}
	for i, w := range want {
		got := states[i]
		if len(got) != len(w) {
			t.Fatalf("item %d saw %v, want %v", i, got, w)
		}
		for j := range w {
			if got[j] != w[j] {
				t.Fatalf("item %d saw %v, want %v", i, got, w)
			}
		}
	}
}

// TestRunOnAnEmptyQueueIsHarmless covers pressing Sign with nothing in
// the list — which the interface prevents, and which must still not
// panic if it ever gets through.
func TestRunOnAnEmptyQueueIsHarmless(t *testing.T) {
	q := &Queue{}
	var r Runner
	report := r.Run(context.Background(), q, func(context.Context, int, Item) (Outcome, error) {
		t.Fatal("signed something in an empty queue")
		return Outcome{}, nil
	}, Hooks{})
	if report.Succeeded != 0 || report.Failed != 0 || report.Skipped != 0 {
		t.Fatalf("report = %+v", report)
	}
}

// TestDiskFillsAtDocumentSeventy is F6 §7's disk case. The write
// failure is the signing step's to report; the run's job is to keep
// going and account for every document, which is what this checks.
func TestDiskFillsAtDocumentSeventy(t *testing.T) {
	q, dir := queueOf(t, 100)
	var r Runner
	report := r.Run(context.Background(), q, func(_ context.Context, index int, item Item) (Outcome, error) {
		if index >= 70 {
			return Outcome{}, errs.WithDetails(errs.CodeOutputWriteFailed,
				os.ErrPermission, map[string]any{"path": item.Path})
		}
		return Outcome{OutputPath: OutputPathFor(item.Path, dir, "-signed"), AchievedLevel: "B-T"}, nil
	}, Hooks{})

	if report.Succeeded != 70 || report.Failed != 30 {
		t.Fatalf("report = %+v, want 70 signed and 30 failed", report)
	}
	if report.Aborted {
		t.Fatal("a disk problem aborted the batch; only a missing card or blocked PIN may")
	}
	if len(report.Failures) != 30 {
		t.Fatalf("Failures listed %d, want 30", len(report.Failures))
	}
}

// TestRunSkipsADocumentTheCallerDeclines covers J-3's mechanism: a
// document the person has said not to sign is skipped, not failed, and
// the rest of the batch runs.
func TestRunSkipsADocumentTheCallerDeclines(t *testing.T) {
	dir := t.TempDir()
	q := &Queue{}
	if added, _ := q.Add([]string{
		writeFile(t, dir, "a.pdf", 1),
		writeFile(t, dir, "b-signed.pdf", 1),
		writeFile(t, dir, "c.pdf", 1),
	}); added != 3 {
		t.Fatalf("added = %d, want 3", added)
	}

	var signed []string
	r := &Runner{}
	report := r.Run(context.Background(), q, func(_ context.Context, _ int, item Item) (Outcome, error) {
		if strings.Contains(item.Path, "b-signed") {
			return Outcome{}, ErrSkipDocument
		}
		signed = append(signed, filepath.Base(item.Path))
		return Outcome{OutputPath: item.Path + ".out", AchievedLevel: "B-T"}, nil
	}, Hooks{})

	if report.Succeeded != 2 || report.Skipped != 1 || report.Failed != 0 {
		t.Fatalf("succeeded=%d skipped=%d failed=%d, want 2/1/0", report.Succeeded, report.Skipped, report.Failed)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("a skipped document was reported as a failure: %+v", report.Failures)
	}
	if len(signed) != 2 {
		t.Fatalf("signed %v, want the two documents that were not declined", signed)
	}
	states := q.Items()
	if states[1].State != StateSkipped {
		t.Errorf("the declined document's state = %q, want %q", states[1].State, StateSkipped)
	}
	if states[1].FailureCode != "" {
		t.Errorf("the declined document carries a failure code %q", states[1].FailureCode)
	}
}

// TestASkippedDocumentContributesNoTiming keeps the ETA honest: a skip
// takes no time at all, and letting it into the measured durations
// would drag the first-signature and median figures the ETA and the
// per-signature PIN detection are built from (SPEC §12.9).
func TestASkippedDocumentContributesNoTiming(t *testing.T) {
	dir := t.TempDir()
	q := &Queue{}
	if added, _ := q.Add([]string{
		writeFile(t, dir, "skipped.pdf", 1),
		writeFile(t, dir, "real.pdf", 1),
	}); added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}

	r := &Runner{}
	report := r.Run(context.Background(), q, func(_ context.Context, index int, _ Item) (Outcome, error) {
		if index == 0 {
			return Outcome{}, ErrSkipDocument
		}
		time.Sleep(20 * time.Millisecond)
		return Outcome{OutputPath: "out", AchievedLevel: "B-T"}, nil
	}, Hooks{})

	if report.Timing.FirstSignature < 15*time.Millisecond {
		t.Fatalf("FirstSignature = %s — the skipped document was counted as the first signature", report.Timing.FirstSignature)
	}
}
