//go:build windows

package worker

import (
	"context"
	"io"
	"testing"
	"time"
)

// TestAKilledChildHoldingARealModuleIsStillReaped is the one measurement the
// per-request deadline should be chosen from.
//
// # The concern, stated precisely
//
// Worker.reap does this:
//
//	select {
//	case err = <-waited:
//	case <-ctx.Done():
//	    w.killLocked()
//	    err = <-waited      // <- no bound of any kind
//	}
//
// The second receive is outside the select on purpose: having killed the child,
// reap waits for it to actually go. That is correct as long as a killed child
// goes.
//
// Measured on this machine, a process that has loaded netsetpkcs11_x64.dll —
// either build — and exits **without calling C_Finalize** can reach os.Exit,
// report HasExited=true, and still sit in the process table indefinitely with
// one thread parked in Wait/UserRequest, surviving TerminateProcess. Three such
// processes accumulated during one afternoon of running scripts/p11probe, whose
// no-token path exits without finalising. They cleared only on a reboot.
//
// TerminateProcess is exactly what killLocked does, and a killed child never
// gets to call C_Finalize. So if such a child is unreapable, that unbounded
// receive waits for ever, and it does so while holding w.mu — which means the
// whole Worker, and every caller of it, stops.
//
// That is a worse failure than any slow module: a listing that takes 890 ms is
// a listing, and an agent that never returns is a bug report that says "it
// froze". Which is why the deadline should be chosen against this rather than
// against the timings table.
//
// # What it does
//
// It spawns a real worker over a real module, then closes it with a context
// that has already expired. That drives exactly the path above: roundTrip sees
// the dead context and kills, Close closes stdin, reap sees the dead context,
// kills again, and then waits with no bound.
//
// No card is needed and nothing is spent: the child loads the module, answers
// or does not, and is killed. It is the kill that is under test, not the read.
//
// # What it asserts
//
// That Close returns. Not how fast — the duration is reported and nothing
// depends on it (D-201). A Close that has not returned within the bound is the
// finding, and the test says so in those words rather than timing out.
func TestAKilledChildHoldingARealModuleIsStillReaped(t *testing.T) {
	path, card, _ := realModuleConditions(t)
	t.Logf("module %s, card %s", path, card)

	// Three rounds, because one is an anecdote. If a child is unreapable only
	// sometimes, once is the number of times that tells you nothing — D-294's
	// point about a crash that cannot be scheduled applies to a hang as well.
	const rounds = 3

	for i := 0; i < rounds; i++ {
		w := New(path, io.Discard)

		// Get the child actually running and holding the module open. A Worker
		// that never spawned has nothing to kill and the test would pass for
		// the emptiest possible reason.
		startCtx, startCancel := bounded(t)
		_, err := w.Enumerate(startCtx)
		startCancel()
		if err != nil {
			t.Fatalf("round %d: the child would not answer, so there is nothing "+
				"to kill: %v\nchild stderr:\n%s", i, err, w.ChildStderr())
		}

		// An already-expired context: every step of Close takes the kill path.
		dead, cancelDead := context.WithCancel(context.Background())
		cancelDead()

		done := make(chan time.Duration, 1)
		go func() {
			begin := time.Now()
			_ = w.Close(dead)
			done <- time.Since(begin)
		}()

		select {
		case took := <-done:
			t.Logf("round %d: Close after a kill returned in %v", i, took)
		case <-time.After(realModuleBound):
			// Deliberately not t.Fatalf inside the goroutine: the goroutine is
			// the thing that is stuck, and it is not coming back to report.
			t.Fatalf("round %d: Close did not return within %v after killing a child "+
				"that had %s open.\n\n"+
				"reap's second receive from `waited` is outside its select and has no "+
				"bound, so a child that cannot be reaped stops this Worker for ever — "+
				"while holding w.mu, so it stops every caller too. This is the case "+
				"the per-request deadline has to be chosen against.",
				i, realModuleBound, path)
		}
	}
}
