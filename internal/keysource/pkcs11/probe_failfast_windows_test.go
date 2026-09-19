package pkcs11

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAChildThatFailFastsIsReapedAsAFailureAndNotATimeout is the half of F12
// §2's exit property that can be taken on demand.
//
// # What it demonstrates, and what it does not
//
// It demonstrates **the parent's handling of a dead child**, against a real
// death: a child process that genuinely ended, whose exit status the parent
// reads from the operating system through exactly the path a vendor module's
// crash takes. Nothing in the parent is simulated.
//
// It does **not** demonstrate the module's behaviour, and it does not close F12
// §2's exit item, which asks for the module that does it. D-294 records why
// that item stays open. A synthetic child closes the half this program
// controls; that is worth having and it is not the same thing.
//
// # The specific defect it is looking for
//
// A fail-fast is not a tidy exit. WerFault launches on one — measured in this
// project while establishing that error reporting cannot be disabled — and this
// machine has LocalDumps in effect. If the reporter holds the dying child open
// for longer than ProbeTimeout, the parent stops waiting and reports
// errWorkerSilent, "did not answer in time". That is the wrong reason for the
// right outcome: same Failure to a person, but anybody debugging a vendor
// module is told the module hung when it actually died, and a listing that
// should cost a tenth of a second costs ten seconds per crashing module.
//
// Nothing else in this package can see that. TestAChildThatExitsNonZeroBecomes-
// AFailure uses a clean `return 2`, which exits immediately and through the
// ordinary path; the whole question here is what the *abnormal* path costs.
func TestAChildThatFailFastsIsReapedAsAFailureAndNotATimeout(t *testing.T) {
	dumpsBefore := crashDumpNames(t)

	start := time.Now()
	_, err := probeOutOfProcess(context.Background(), crashSentinelPath, nil)
	elapsed := time.Since(start)

	t.Logf("a fail-fasting child was reaped in %s (ProbeTimeout is %s); the parent got: %v",
		elapsed.Round(time.Millisecond), ProbeTimeout, err)

	if errors.Is(err, errWorkerSilent) {
		t.Fatalf("a child that died was reported as one that did not answer, after %s.\n\n"+
			"This is the defect the test exists for: the crash is being reported "+
			"as a hang because something — WerFault, a dump being written — held "+
			"the dying process open past ProbeTimeout (%s). A person debugging a "+
			"vendor module would be told it hung when it died, and every crashing "+
			"module would cost a listing that much time.", elapsed, ProbeTimeout)
	}
	if !errors.Is(err, errWorkerDied) {
		t.Fatalf("a fail-fasting child produced %v, want %v.\n\n"+
			"Every way a child can end must arrive as an ordinary error carrying "+
			"the exit status, because Modules puts it in a list and carries on.", err, errWorkerDied)
	}

	// Not an assertion about how fast it must be — that would be timing the
	// machine, which D-201 rules out — but about the property that matters:
	// the reaping finished inside the bound, with room, rather than near it.
	if elapsed > ProbeTimeout/2 {
		t.Errorf("the child was reaped in %s, more than half of ProbeTimeout (%s).\n\n"+
			"It is still reported correctly, but the margin is thin enough that a "+
			"slower machine, a bigger dump or a busier WerFault would push it over "+
			"and turn every crash into a timeout.", elapsed, ProbeTimeout)
	}

	// The machine must not be quietly different afterwards. CrashDumps holds
	// ten by default and this one was full, so a dump written here evicts
	// somebody's, and nothing in a test run is worth that.
	if after := crashDumpNames(t); len(after) > 0 && !sameStringSet(dumpsBefore, after) {
		t.Errorf("the crash dump directory changed: %d entries before, %d after.\n\n"+
			"A test that deliberately crashes a child must not cost this machine a "+
			"dump it was keeping. If this fires, LocalDumps has started capturing "+
			"fail-fasts and the sentinel needs to exclude itself.", len(dumpsBefore), len(after))
	}
}

// TestWhetherTheCppThrowTerminationIsReachableFromAGoChild records which of
// D-272's two terminations this instrument can and cannot produce.
//
// D-272 measured NetSeT 1.1.0.0 dying two ways from the same call: 0xC0000417,
// the CRT's invalid-parameter handler reaching __fastfail, and 0xE06D7363, a
// Microsoft C++ exception raised by throw with nothing catching it. They are
// not interchangeable — a C++ throw travels through the exception dispatcher,
// where a fail-fast does not — and D-272's own conclusion was that a remedy
// sized for the first would not survive the second.
//
// So it matters whether a Go child can produce both, and the honest answer goes
// in the record rather than one standing in for the other. This test asserts
// only that the parent survives whatever happens and reports it; what the
// termination actually was is logged, because that is the thing being found out.
func TestWhetherTheCppThrowTerminationIsReachableFromAGoChild(t *testing.T) {
	start := time.Now()
	_, err := probeOutOfProcess(context.Background(), crashSentinelThrow, nil)
	elapsed := time.Since(start)

	t.Logf("a child raising 0xE06D7363 was reaped in %s; the parent got: %v",
		elapsed.Round(time.Millisecond), err)

	if err == nil {
		t.Fatal("a child that raised an unhandled exception answered as a module")
	}
	if errors.Is(err, errWorkerSilent) {
		t.Errorf("the child neither answered nor died within %s: %v", ProbeTimeout, err)
	}
}

// crashDumpNames lists what is in the per-user crash dump directory, so that a
// test which deliberately kills a process can prove it did not cost this
// machine one of the dumps it was keeping.
func crashDumpNames(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "CrashDumps")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // no directory is a fine answer; there is nothing to lose
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}
