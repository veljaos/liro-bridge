package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// The backstop this file is about cannot be triggered on demand. That is not a
// gap in the tests — it is the finding: six attempts to produce an unreapable
// child through reap's path produced none (D-306). So what is testable is the
// other direction, and it is the direction that matters day to day.
//
// A backstop that fires when it should not is worse than no backstop: it would
// turn every ordinary close into an abandoned child, leak a goroutine and a
// process each time, and log a line saying something is wrong when nothing is.
// These tests say it does not.
//
// The mutation that makes them mean something is reapBackstop set to 1ns, which
// makes every reap abandon. Both tests below fail under it, in different
// places. Without that mutation they are two tests that watch nothing happen.

// TestAnOrdinaryCloseDoesNotReachTheBackstop is the common path: a healthy
// child, asked to shut down, with a context that is in no hurry.
func TestAnOrdinaryCloseDoesNotReachTheBackstop(t *testing.T) {
	var said childStderr // safe to read; a bytes.Buffer here is D-303's race
	w := New(cannedPath(cannedAnswers{Label: "healthy"}), &said)

	if _, err := w.Enumerate(context.Background()); err != nil {
		t.Fatalf("Enumerate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	began := time.Now()
	err := w.Close(ctx)
	took := time.Since(began)

	if err != nil {
		t.Errorf("Close: %v, want nil", err)
	}
	if errors.Is(err, ErrWorkerAbandoned) {
		t.Error("an ordinary close abandoned its child")
	}
	if got := said.String(); strings.Contains(got, "abandoned") {
		t.Errorf("an ordinary close logged an abandonment:\n%s", got)
	}
	// Reported, not asserted on (D-201, D-304): what matters is that the
	// backstop was not reached, which the error and the log already say.
	t.Logf("Close took %v against a %v backstop", took, reapBackstop)
}

// TestKillingAWorkerDoesNotReachTheBackstopEither covers endLocked, which is
// the path the backstop was actually added for: it kills first and only then
// calls reap, so reap's *first* wait is already a wait for a killed child.
//
// A rogue child is used because it is the one thing that reliably drives
// endLocked in this package: it asks for a PIN with nothing pending, which the
// parent refuses and kills over (D-302).
func TestKillingAWorkerDoesNotReachTheBackstopEither(t *testing.T) {
	var said childStderr
	w := New(cannedPath(askingCard(cannedAnswers{AskUnbidden: true})), &said)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = w.Close(ctx)
	})

	began := time.Now()
	_, _, err := w.List(context.Background())
	took := time.Since(began)

	if !errors.Is(err, ErrUnexpectedPINRequest) {
		t.Fatalf("List against a rogue worker: %v, want ErrUnexpectedPINRequest", err)
	}
	if errors.Is(err, ErrWorkerAbandoned) {
		t.Error("killing a rogue child reached the backstop")
	}
	if got := said.String(); strings.Contains(got, "abandoned") {
		t.Errorf("killing a rogue child logged an abandonment:\n%s", got)
	}
	t.Logf("refusal and kill took %v against a %v backstop", took, reapBackstop)
}

// TestTheBackstopIsTheNumberThatWasRuledOn is a decision record rather than a
// behavioural check, and it is here because the number is the decision.
//
// Ten seconds was chosen against a measured failure — 890 ms worst legitimate
// call, 2–5 ms to reap a killed child, no reproduction at all of the case it
// guards — and not derived from any of them (D-306). A later change to it is a
// change to that ruling, and this makes it one somebody has to look at.
//
// It also states what the number is *not*: the per-request deadline D-297 left
// open. Nothing here bounds a request.
func TestTheBackstopIsTheNumberThatWasRuledOn(t *testing.T) {
	if reapBackstop != 10*time.Second {
		t.Errorf("reapBackstop is %v; it was ruled at 10s.\n\n"+
			"It is a liveness backstop on reap's wait for a child it has killed, "+
			"chosen at about eleven times the worst measured legitimate call so that "+
			"reaching it means something is wrong rather than slow. It is not the "+
			"per-request deadline, which D-297 leaves open and which this does not "+
			"answer.", reapBackstop)
	}

	// The bound must be comfortably clear of the worst call this project has
	// measured, or it stops being a backstop and starts being a deadline on
	// legitimate work. 890 ms is NetSeT 1.1.3.3 reading a two-certificate card
	// (D-305), and it is not a ceiling.
	const worstMeasuredLegitimateCall = 890 * time.Millisecond
	if reapBackstop < 4*worstMeasuredLegitimateCall {
		t.Errorf("reapBackstop (%v) is within four times the worst legitimate call "+
			"this project has measured (%v). A bound that close kills working "+
			"modules on ordinary work.", reapBackstop, worstMeasuredLegitimateCall)
	}
}
