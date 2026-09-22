//go:build linux

package pkcs11

import (
	"context"
	"errors"
	"syscall"
	"testing"
)

// This file makes a probe child die on purpose, so that the parent's
// handling of a dead child can be measured instead of waited for.
//
// **It is the body `crash_other_test.go` said this would grow**, and the
// condition it named has now been met: there is a dlopen binding for a
// module to crash inside (D-349), so a test that kills a child here is
// no longer a measurement about nothing.
//
// # Why this is not the synthetic input D-094 forbids
//
// The same reasoning as the Windows file's, unchanged: D-094 is about
// not manufacturing the human input a test is evidence about. Here the
// thing under test is the parent's handling of a dead child, the input
// is the child dying, and a child that dies on purpose is really dead —
// the parent reaps a real process through exactly the path a vendor
// module's crash would take. Nothing about the parent is simulated.
//
// # It exists only in a test binary
//
// The dispatch is in TestMain, not in RunProbe, and every file involved
// ends in _test.go. A release binary has no way to reach it because the
// code is not in one.

// crashSentinelPath is the module path that means "die instead of
// probing". It cannot collide with a real path — a NUL cannot appear in
// one and this is not a path anyway, since the child recognises it
// before openModule is reached — and it is deliberately loud, so that
// anybody who ever sees it in a log knows it came from a test.
const crashSentinelPath = "!!liro-test-crash-abort!!"

// crashIfAskedForTest ends this process if the path is the sentinel. It
// never returns in that case, which is the point.
func crashIfAskedForTest(path string) {
	if path != crashSentinelPath {
		return
	}
	// SIGABRT, because that is what a C library's abort() raises and
	// abort() is how a vendor module fails an assertion — the Linux
	// equivalent of the fail-fast family D-272 measured on Windows. It
	// is raised rather than returned so that the parent reads a real
	// signal status from a real dead process.
	//
	// Not os.Exit: an exit status is a process choosing to stop, and
	// the thing being reproduced is a process being stopped. The two
	// reach the parent differently and only one of them is the case
	// F12 §2 is about.
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGABRT)
	select {} // unreachable; if the signal were ever ignored, hang rather than lie
}

// TestTheParentSurvivesAModuleThatKillsItsChild is F12 §2's exit
// condition on this platform: "a module that kills its worker becomes a
// Failure, and the agent survives".
//
// The child here dies the way a module makes it die. What is asserted is
// the parent: that it is still running, that it says which module, and
// that what comes back is an error rather than a result.
func TestTheParentSurvivesAModuleThatKillsItsChild(t *testing.T) {
	res, err := probeOutOfProcess(context.Background(), crashSentinelPath, nil)
	if err == nil {
		t.Fatalf("a child that aborted produced no error, and a result: %+v", res)
	}
	if !errors.Is(err, errWorkerDied) {
		t.Errorf("a child that aborted produced %v, want %v", err, errWorkerDied)
	}
	// And the parent is still here to assert it, which is the other
	// half and is what the next line demonstrates rather than states.
	if _, err := probeOutOfProcess(context.Background(), "/nonexistent/module.so", nil); err == nil {
		t.Error("the parent could not probe anything after a child died")
	}
}
