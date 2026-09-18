package pkcs11

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestTheRealModuleKillsItsChildAndThisProcessSurvives is F12 §2's exit
// property taken against the module that actually does it, rather than against
// a stand-in.
//
// It opts in through LIRO_PKCS11_MODULE like every other real-module test in
// this package (D-038's pattern), and it is read-only: C_Initialize, C_GetInfo,
// C_Finalize, in a child, once per iteration. Nothing logs in, so nothing here
// can spend a PIN attempt.
//
//	LIRO_PKCS11_MODULE="C:\Program Files\MUP RS\Celik\netsetpkcs11_x64.dll" \
//	LIRO_PKCS11_PROBE_CARD=in LIRO_PKCS11_PROBE_ITERATIONS=300 \
//	go test -count=1 -run TestTheRealModule -v ./internal/keysource/pkcs11/
//
// # The card state is a required input, not a detail
//
// D-272 measured the crash "with the card in the reader throughout", and its §3
// is titled *the card-out control could not have seen it*: a run with an empty
// reader is the control that already misled this project once. A clean run with
// no card is not evidence about the crash, and the only thing that stops it
// being read as evidence later is the condition sitting beside the number.
//
// So LIRO_PKCS11_PROBE_CARD must be "in" or "out", it is logged with the
// result, and it has no default. A default is how the condition goes missing.
//
// # Why it needs a real module and cannot be faked
//
// D-272 measured NetSeT 1.1.0.0 — the build MUP's own middleware installs —
// dying inside its own C_Initialize, in two different ways, and established by
// direct measurement that no Go process survives either: recover() catches
// neither, and a vectored handler does not help. D-094 forbids synthetic input
// for exactly this reason. A fabricated crash would prove that this code
// handles a fabricated crash.
//
// # What it asserts, and what it only reports
//
// It asserts the thing the phase asks for: **this process is still running at
// the end**, and every death arrived as an ordinary error. That is a property
// of this program and it holds whether the crash rate is one in ten or zero in
// three hundred.
//
// It reports the rate rather than asserting it. D-272's own table has a clean
// run of 150 and another of 120 with the card in, so the crash is bursty rather
// than steady, and a test that failed when the module behaved itself would be a
// test about the module.
func TestTheRealModuleKillsItsChildAndThisProcessSurvives(t *testing.T) {
	path := os.Getenv("LIRO_PKCS11_MODULE")
	if path == "" {
		t.Skip("LIRO_PKCS11_MODULE is not set; skipping the real-module tests")
	}

	card := os.Getenv("LIRO_PKCS11_PROBE_CARD")
	if card != "in" && card != "out" {
		t.Fatalf(`LIRO_PKCS11_PROBE_CARD=%q; set it to "in" or "out".`+"\n\n"+
			"D-272 measured this crash with the card in the reader throughout, and "+
			"recorded that the card-out control could not have seen it. A run with "+
			"an empty reader is not a measurement of the crash, so the reader's "+
			"state is logged with the result and has no default.", card)
	}

	iterations := 100
	if n := os.Getenv("LIRO_PKCS11_PROBE_ITERATIONS"); n != "" {
		parsed, err := strconv.Atoi(n)
		if err != nil || parsed < 1 {
			t.Fatalf("LIRO_PKCS11_PROBE_ITERATIONS=%q is not a positive number", n)
		}
		iterations = parsed
	}

	var answered, died, silent, other int
	var firstDeath error
	start := time.Now()

	for i := 0; i < iterations; i++ {
		res, err := probeOutOfProcess(context.Background(), path)
		switch {
		case err == nil:
			answered++
			if res.Manufacturer == "" && res.LibraryDescription == "" {
				t.Fatalf("iteration %d: the module was accepted while saying nothing about itself", i)
			}
		case errors.Is(err, errWorkerDied):
			died++
			if firstDeath == nil {
				firstDeath = err
			}
		case errors.Is(err, errWorkerSilent):
			silent++
		case errors.Is(err, ErrChildRecursion):
			t.Fatalf("iteration %d: this process is marked as a probe child, so nothing was "+
				"spawned and the measurement is of the guard rather than the module", i)
		default:
			other++
			t.Logf("iteration %d: %v", i, err)
		}
	}

	elapsed := time.Since(start)
	t.Logf("%d probes of %s, card %s, in %s (%.0fms each): %d answered, %d killed "+
		"the child, %d timed out, %d other", iterations, path, card,
		elapsed.Round(time.Millisecond),
		float64(elapsed.Milliseconds())/float64(iterations), answered, died, silent, other)
	if firstDeath != nil {
		t.Logf("the first death read: %v", firstDeath)
	}

	// The assertion. Reaching this line at all is most of it: D-275's whole
	// finding is that in-process this loop ends at the first death, and no
	// amount of recover() changes that.
	if answered+died+silent+other != iterations {
		t.Fatalf("%d probes accounted for %d outcomes", iterations, answered+died+silent+other)
	}
	if answered == 0 {
		t.Errorf("not one of %d probes got an answer from %s.\n\n"+
			"Every outcome being a failure means the module is not being loaded "+
			"at all rather than crashing sometimes, and then this measurement is "+
			"about something else — a wrong path, or a child that never ran the "+
			"probe.", iterations, path)
	}
	if died == 0 && card == "out" {
		t.Logf("no death in %d probes, but the reader was empty. This is the control "+
			"D-272 §3 found could not see the crash; it says nothing about the rate "+
			"and does not demonstrate F12 §2's exit property. Re-run with the card in.",
			iterations)
	}
	if died > 0 {
		t.Logf("D-272's crash reproduced %d times in %d and this process survived all of "+
			"them, which is what F12 §2 asks for and what D-275 recorded as impossible "+
			"in-process", died, iterations)
	}
}
