//go:build windows

package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// This file is the one place F12 §2 meets real hardware.
//
// # Why it had to be written before the phase could be called done
//
// Everything else in this package is measured against a canned child that never
// calls into internal/keysource/pkcs11 at all. That is the right way to test a
// protocol, and it means the three entries §2 rests on each assert something
// about a configuration nothing had run:
//
//   - D-297 says the worker exists so C_Initialize is paid once rather than per
//     call, against a module D-272 measured dying inside it about once in a
//     hundred calls. A module *held open across many requests* had never been
//     opened.
//   - D-298 says runtime.LockOSThread is the braces and CKF_OS_LOCKING_OK is
//     the belt, because two of four measured modules never read the arguments
//     structure at all. Run locks the thread before the open; that pairing had
//     never run.
//   - D-299 moved the read operations onto an already-open module so that one
//     rule has two callers. The one-shot caller has been run against a card.
//     The held caller had not.
//
// A phase that ends there ends on an argument. This ends it on a measurement.
//
// # What it asserts, and what it only reports
//
// It asserts **agreement**: for the same module and the same card, what the
// worker answers through a held module and what the in-process Source answers
// through a one-shot open are the same bytes, in the same order. That is
// D-299's refactor checked where it can actually be wrong, and it needs no
// product code to observe it.
//
// It *reports* timings and asserts nothing about them. D-201 is the rule —
// observe the property rather than time the machine — and it is not being bent
// here, because no assertion depends on a duration. The numbers exist because
// the owner asked for what a real Enumerate costs before anybody picks a
// per-request deadline (D-297 left that open on purpose), and a number with its
// conditions printed beside it is the only kind worth having.
//
// # It is read-only and spends nothing
//
// C_Initialize, C_GetSlotList, C_GetTokenInfo, a read-only public session,
// C_FindObjects, C_GetAttributeValue, C_Finalize. No C_Login, no private
// object, no PIN. It can be run at will and costs no attempt. The login is a
// separate thing, deliberately, and is authorised separately.
//
//	set LIRO_PKCS11_MODULE=C:\Program Files\TrustEdgeID\netsetpkcs11_x64.dll
//	set LIRO_PKCS11_WORKER_CARD=in
//	go test -count=1 -run TestARealModule -v ./internal/keysource/pkcs11/worker/
//
// # The card's state is a required input, not a detail
//
// D-272 §3 is titled *the card-out control could not have seen it*: a run with
// an empty reader already misled this project once. So LIRO_PKCS11_WORKER_CARD
// must be "in" or "out", it is printed with every number, and it has no
// default. A default is how the condition goes missing.

// realModuleRounds is how many times the read operations are repeated.
//
// More than one, because one round cannot distinguish "C_Initialize was paid
// once" from "C_Initialize was paid every time" — that difference is the whole
// of D-297, and it is visible only as a first call that costs more than the
// rest. Twenty rather than a hundred: this is not the crash-rate measurement,
// which probe_real_test.go already takes, and takes differently.
const realModuleRounds = 20

// round is one pass of the three read operations.
//
// chainForRan is not a nicety. Without it a ChainFor that was never called and
// a ChainFor that returned faster than the clock can see both print as "0s",
// and the owner caught exactly that: SafeSign reported chainFor 0s across all
// twenty rounds, which was "never called" (no certificate, nothing to ask
// about) wearing the costume of a measurement. D-304.
type round struct {
	enumerate, list, chainFor time.Duration
	chainForRan               bool
}

// clockFloor is the smallest interval time.Since can report on this machine,
// measured rather than assumed.
//
// D-201 recorded it at 512 us on this machine and it is 506.5 us today: a
// time.Since across an instant call reads exactly 0, and 199997 of 200000
// back-to-back pairs read exactly 0. So "0s" in this file's output means
// "below the floor", never "instant", and a number within a few multiples of
// the floor is one tick wide.
//
// It is measured at run time rather than hard-coded because the machine that
// runs this is not necessarily the one D-201 measured, and a floor quoted from
// somebody else's machine is the thing this whole entry is about.
func clockFloor() time.Duration {
	smallest := time.Hour
	for i := 0; i < 50000; i++ {
		a := time.Now()
		if d := time.Since(a); d > 0 && d < smallest {
			smallest = d
		}
	}
	if smallest == time.Hour {
		return 0
	}
	return smallest
}

// showDuration prints a duration against the clock's floor, so that three
// different things stop looking alike: a real measurement, an interval too
// short for this clock, and a call that never happened.
func showDuration(d time.Duration, ran bool, floor time.Duration) string {
	if !ran {
		return "not called"
	}
	if floor > 0 && d < floor {
		return "<" + floor.String() + " (below this machine's clock)"
	}
	return d.String()
}

func realModuleConditions(t *testing.T) (path, card string, rounds int) {
	t.Helper()

	// The wall-clock time this run started, printed so that a replay is
	// obvious. `go test` caches a successful result and replays its output
	// verbatim when nothing it tracks has changed, and a cached run cannot
	// detect that it is cached -- it does not execute at all. The owner hit
	// this: three invocations of a card test printed identical numbers to the
	// last decimal, which was one run shown three times.
	//
	// `(cached)` on the package line is the real tell and -count=1 is the real
	// fix -- it is in this file's own example command and in the home list.
	// This is the belt: two runs reporting the same start time are the same
	// run, whoever misses the package line (D-304).
	t.Logf("run started %s", time.Now().Format(time.RFC3339Nano))

	path = os.Getenv("LIRO_PKCS11_MODULE")
	if path == "" {
		t.Skip("LIRO_PKCS11_MODULE is not set; skipping the real-module tests")
	}

	card = os.Getenv("LIRO_PKCS11_WORKER_CARD")
	if card != "in" && card != "out" {
		t.Fatalf(`LIRO_PKCS11_WORKER_CARD=%q; set it to "in" or "out".`+"\n\n"+
			"Every number this test prints is meaningless without it, and D-272 §3 "+
			"records what it cost to learn that: a run with an empty reader is not a "+
			"measurement of a card. It has no default on purpose.", card)
	}

	rounds = realModuleRounds
	if n := os.Getenv("LIRO_PKCS11_WORKER_ROUNDS"); n != "" {
		parsed, err := strconv.Atoi(n)
		if err != nil || parsed < 1 {
			t.Fatalf("LIRO_PKCS11_WORKER_ROUNDS=%q is not a positive number", n)
		}
		rounds = parsed
	}
	return path, card, rounds
}

// realModuleBound is how long this test will wait for any one call before
// calling it stuck.
//
// It is a **test** picking a number, which a test may do and the product may
// not — D-297 leaves the per-request deadline open deliberately and it is the
// owner's to choose. Nothing here asserts on a duration; this bound exists so
// that a call which never returns is *reported* rather than wedging the run.
//
// That is not hypothetical and it is why this was added after the file was
// written. Measured on this machine, with no card in the reader: a process that
// has loaded netsetpkcs11_x64.dll (TrustEdgeID 1.1.3.3) and WinSCard.dll can
// reach os.Exit and still not go away — `HasExited` reports true while the
// process object persists with one thread in Wait/UserRequest, indefinitely,
// surviving TerminateProcess. Anything waiting on such a child waits for ever,
// and Worker.reap's wait after a kill is deliberately unbounded.
//
// Thirty seconds rather than five: a real Enumerate on a card has never been
// timed through a worker, which is the point of this test, so the bound has to
// be far past anything plausible rather than near it.
const realModuleBound = 30 * time.Second

// bounded returns a context this test is willing to wait on, and a reason to
// print when it runs out.
func bounded(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), realModuleBound)
}

// stuck turns a context deadline into the finding it is, rather than a bare
// "context deadline exceeded" the reader has to interpret.
func stuck(t *testing.T, w *Worker, what string, err error) {
	t.Helper()
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%s did not return within %v.\n\n"+
			"That is the per-request deadline question (D-297) arriving as a fact: "+
			"this call had no bound but the one this test invented. Measured on this "+
			"machine with no card, a process holding netsetpkcs11_x64.dll can exit "+
			"and still not be reaped, so whatever waits on it waits for ever.\n\n"+
			"child stderr:\n%s", what, realModuleBound, w.ChildStderr())
	}
}

// closingReal bounds this file's teardown for the same reason login_test.go's
// does: Worker.Close takes the caller's context as its bound, and a test may
// pick a number where the product may not.
func closingReal(t *testing.T, w *Worker) func() {
	t.Helper()
	return func() {
		ctx, cancel := bounded(t)
		defer cancel()
		if err := w.Close(ctx); err != nil {
			t.Logf("Close: %v\nchild stderr:\n%s", err, w.ChildStderr())
		}
	}
}

// TestARealModuleHeldOpenAnswersTheSameAsAOneShotOpen is the whole of it.
//
// The module is first read the way everything before F12 §2 read it — open,
// enumerate, close, in this process. Then one worker process holds the same
// module open and is asked the same things, repeatedly. The answers must be
// identical.
//
// If they are not, D-299's refactor is wrong on real hardware in a way no
// amount of canned-child testing could have shown, and the failure it guards
// against is the quiet one: a listing that differs depending on which path
// produced it.
func TestARealModuleHeldOpenAnswersTheSameAsAOneShotOpen(t *testing.T) {
	path, card, rounds := realModuleConditions(t)

	t.Logf("module %s", path)
	t.Logf("card   %s", card)
	t.Logf("rounds %d", rounds)

	// --- the established path, first, so the comparison has a baseline -------

	source := pkcs11.NewSource(path)

	directCtx, directCancel := bounded(t)
	start := time.Now()
	direct, err := source.Enumerate(directCtx)
	oneShot := time.Since(start)
	directCancel()
	if err != nil {
		t.Fatalf("Enumerate in this process: %v", err)
	}

	// --- the held module, many times over ------------------------------------

	w := New(path, io.Discard)
	t.Cleanup(closingReal(t, w))

	rs := make([]round, 0, rounds)
	var held []CertificatePayload

	for i := 0; i < rounds; i++ {
		var r round

		ctx, cancel := bounded(t)
		begin := time.Now()
		certs, err := w.Enumerate(ctx)
		r.enumerate = time.Since(begin)
		cancel()
		if err != nil {
			stuck(t, w, "Enumerate through the worker", err)
			t.Fatalf("round %d: Enumerate through the worker: %v\nchild stderr:\n%s",
				i, err, w.ChildStderr())
		}
		if i == 0 {
			held = certs
		} else if !samePayloads(certs, held) {
			t.Fatalf("round %d: the held module answered differently from round 0.\n\n"+
				"A module held open across many reads must not drift between them; "+
				"that is what holding it open is for (D-297).", i)
		}

		listCtx, listCancel := bounded(t)
		begin = time.Now()
		_, err = w.List(listCtx)
		listCancel()
		if err != nil {
			stuck(t, w, "List through the worker", err)
			t.Fatalf("round %d: List through the worker: %v\nchild stderr:\n%s",
				i, err, w.ChildStderr())
		}
		r.list = time.Since(begin)

		// ChainFor needs a thumbprint, and the thumbprint comes from the
		// in-process read because the protocol deliberately does not carry one
		// (CertificatePayload is DER and a label; see its own comment). With an
		// empty reader there is nothing to ask about and this stays zero, which
		// is printed rather than silently skipped.
		if len(direct) > 0 {
			chainCtx, chainCancel := bounded(t)
			begin = time.Now()
			_, err = w.ChainFor(chainCtx, direct[0].Thumbprint)
			chainCancel()
			if err != nil {
				stuck(t, w, "ChainFor through the worker", err)
				t.Fatalf("round %d: ChainFor through the worker: %v\nchild stderr:\n%s",
					i, err, w.ChildStderr())
			}
			r.chainFor = time.Since(begin)
			r.chainForRan = true
		}

		rs = append(rs, r)
	}

	// --- the assertion -------------------------------------------------------

	if len(direct) != len(held) {
		t.Fatalf("a one-shot open saw %d certificates and the held module saw %d, on "+
			"the same module and the same card.\n\n"+
			"D-299 moved the read body onto an already-open module so that one rule "+
			"has two callers. The two callers disagree.", len(direct), len(held))
	}
	for i := range direct {
		if !bytes.Equal(direct[i].DER, held[i].DER) {
			t.Errorf("certificate %d differs between a one-shot open and the held module", i)
		}
		if direct[i].Label != held[i].Label {
			t.Errorf("certificate %d has label %q through a one-shot open and %q through "+
				"the held module", i, direct[i].Label, held[i].Label)
		}
	}

	// --- what it reports -----------------------------------------------------

	t.Logf("")
	t.Logf("certificates: %d", len(direct))
	for i, c := range direct {
		// ProtectedPIN is clause 1's branch and nothing has ever seen it set:
		// D-268 and D-273 both found no module offering a protected
		// authentication path. If it is true for some token, a login on it
		// collects no PIN at all and the PIN seam's asking half is never
		// reached — which is why it is printed here, before any login is
		// planned against this card.
		t.Logf("  [%d] %d bytes  label=%q  token=%q serial=%q slot=%d protectedPIN=%v",
			i, len(c.DER), c.Label, c.TokenLabel, c.TokenSerial, c.SlotID, c.ProtectedPIN)
	}

	floor := clockFloor()
	t.Logf("")
	t.Logf("timings — card %s — %s", card, path)
	t.Logf("  this machine's clock floor, measured now: %v", floor)
	t.Logf("  (D-201 measured 512µs here; anything below it cannot be timed at all)")
	t.Logf("  one-shot open+enumerate+close, in this process:  %s", showDuration(oneShot, true, floor))
	t.Logf("  worker round 0 (child spawn + C_Initialize + read):")
	t.Logf("    enumerate %s", showDuration(rs[0].enumerate, true, floor))
	t.Logf("    list      %s", showDuration(rs[0].list, true, floor))
	t.Logf("    chainFor  %s", showDuration(rs[0].chainFor, rs[0].chainForRan, floor))
	if len(rs) > 1 {
		t.Logf("  worker rounds 1..%d, module already open:", len(rs)-1)
		t.Logf("    enumerate  %s", spread(rs[1:], func(r round) time.Duration { return r.enumerate }, func(round) bool { return true }, floor))
		t.Logf("    list       %s", spread(rs[1:], func(r round) time.Duration { return r.list }, func(round) bool { return true }, floor))
		t.Logf("    chainFor   %s", spread(rs[1:], func(r round) time.Duration { return r.chainFor }, func(r round) bool { return r.chainForRan }, floor))
	}
	t.Logf("")
	t.Logf("D-297's claim is the gap between round 0 and the rounds after it, and")
	t.Logf("between those and the one-shot. Nothing above asserts on any of these")
	t.Logf("numbers (D-201); they are the input to the per-request deadline that")
	t.Logf("D-297 left open on purpose.")
}

// TestARealModuleSurvivesAWorkerBeingClosedAndAnotherOpened is the other half
// of holding one open: C_Finalize happens, and the next worker can still load
// the same module in a fresh process.
//
// It is worth its own test because D-272's crash is inside C_Initialize, so a
// second and third open is where a module that does not clean up after itself
// would show it — and the supervisor's whole design is that a replacement
// worker is ordinary rather than exceptional.
//
// It reports deaths rather than asserting their absence, for the reason
// probe_real_test.go gives: D-272's own table has clean runs of 150 and of 120,
// so a test that failed when the module behaved itself would be a test about
// the module. What it asserts is that this process is still running at the end
// and that every failure arrived as an ordinary error.
func TestARealModuleSurvivesAWorkerBeingClosedAndAnotherOpened(t *testing.T) {
	path, card, _ := realModuleConditions(t)
	t.Logf("module %s, card %s", path, card)

	const workers = 3
	for i := 0; i < workers; i++ {
		w := New(path, io.Discard)

		ctx, cancel := bounded(t)
		certs, err := w.Enumerate(ctx)
		cancel()
		if err != nil {
			t.Errorf("worker %d: Enumerate: %v\nchild stderr:\n%s", i, err, w.ChildStderr())
		} else {
			t.Logf("worker %d saw %d certificates", i, len(certs))
		}

		closeCtx, closeCancel := bounded(t)
		err = w.Close(closeCtx)
		closeCancel()
		if err != nil {
			t.Errorf("worker %d: Close: %v\nchild stderr:\n%s", i, err, w.ChildStderr())
		}
	}
}

// samePayloads compares two answers by their bytes and their order.
//
// By order as well as by content, deliberately. One module returning one card's
// certificates in a different order on two consecutive reads would be a fact
// worth knowing — dedupe's own comment records that the two NetSeT builds
// disagree with each other about order, which is a different thing and is
// already handled.
func samePayloads(a, b []CertificatePayload) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i].DER, b[i].DER) || a[i].Label != b[i].Label {
			return false
		}
	}
	return true
}

// spread prints the shape of a set of durations rather than one number, because
// D-272 measured this module's behaviour as bursty rather than steady and a
// mean on its own would hide exactly that.
func spread(rs []round, pick func(round) time.Duration, ran func(round) bool, floor time.Duration) string {
	ds := make([]time.Duration, 0, len(rs))
	total := time.Duration(0)
	for _, r := range rs {
		if !ran(r) {
			continue
		}
		d := pick(r)
		ds = append(ds, d)
		total += d
	}
	if len(ds) == 0 {
		return "not called in any round"
	}
	for i := 1; i < len(ds); i++ {
		for j := i; j > 0 && ds[j] < ds[j-1]; j-- {
			ds[j], ds[j-1] = ds[j-1], ds[j]
		}
	}
	mean := total / time.Duration(len(ds))
	out := "min " + ds[0].String() +
		"  median " + ds[len(ds)/2].String() +
		"  max " + ds[len(ds)-1].String() +
		"  mean " + mean.String()
	// A spread whose largest value is under the floor is not a spread; it is
	// the clock. Say so rather than letting four numbers imply four
	// measurements.
	if floor > 0 && ds[len(ds)-1] < floor {
		out += "  -- every value is below this machine's " + floor.String() + " clock floor"
	}
	return out
}
