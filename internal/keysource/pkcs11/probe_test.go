package pkcs11

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheGuardIsWhatRefuses is the test for the fork bomb, and it is written as
// a pair so that the refusal can be attributed to the guard rather than to the
// path being a bad one.
//
// The treatment and the control differ in exactly one thing: whether this
// process is marked as a probe child. Both ask about the same file, which is
// not a module either way. If only the treatment were run it would pass with
// the guard deleted, because a file full of text produces an error regardless;
// the control is what makes the error mean something.
//
// # The defect this stands over
//
// probeOutOfProcess spawns os.Executable(). In the agent that is the agent. In
// any test binary it is the test binary, which runs its whole suite — including
// the tests that call Modules, which spawn again, and so on. That is not a risk
// that was reasoned about: `go test ./internal/keysource/pkcs11/...` took this
// machine from 254 processes to 827 in a few seconds and had to be killed by
// name. See ChildMarker.
func TestTheGuardIsWhatRefuses(t *testing.T) {
	dir := t.TempDir()
	notAModule := filepath.Join(dir, "definitely-not-a-module.dll")
	if err := os.WriteFile(notAModule, []byte("MZ but not really"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Control: not marked. A child is really spawned, really fails to load the
	// file, and really answers — which is also the assertion that this binary
	// dispatches the subcommand at all (TestMain). A test binary that ran its
	// suite instead would answer with test output, and the error would be "the
	// probe answered something that is not a result".
	if _, err := probeOutOfProcess(context.Background(), notAModule); err == nil {
		t.Error("a text file was accepted as a PKCS#11 module")
	} else if errors.Is(err, ErrChildRecursion) {
		t.Fatalf("the control refused before spawning: %v\n\n"+
			"It should have spawned a child, which means %s was already set in "+
			"this process's environment. Then the treatment below proves nothing.",
			err, ChildMarker)
	} else if errors.Is(err, errWorkerSilent) || errors.Is(err, errWorkerDied) {
		t.Fatalf("the control's child neither answered nor loaded: %v\n\n"+
			"Loading a text file should be an ordinary refusal from the child, "+
			"not the child dying or hanging.", err)
	}

	// Treatment: marked, exactly as ChildEnv marks the children this
	// package spawns. Nothing else about the call changes.
	t.Setenv(ChildMarker, "1")

	_, err := probeOutOfProcess(context.Background(), notAModule)
	if !errors.Is(err, ErrChildRecursion) {
		t.Fatalf("a process marked as a probe child spawned one anyway; got %v\n\n"+
			"This is the guard that bounds recursion at a single generation. "+
			"Without it, any binary that spawns without dispatching "+
			"%q — every test binary that has no TestMain, and every future one — "+
			"re-runs whatever it does run, which spawns again.",
			err, ProbeSubcommand)
	}
}

// TestAChildIsMarked is the other half of the guard, and the half the pair
// above cannot see.
//
// TestTheGuardIsWhatRefuses sets the marker by hand, so it would pass unchanged
// if probeOutOfProcess stopped setting it on what it spawns — and that is
// exactly the fork bomb, because then no child is ever marked and no child ever
// refuses. The deletion is invisible from outside a child too: one that answers
// correctly answers identically whether or not it was marked.
//
// So the environment the child is given is a value this reads directly, which
// is why ChildEnv exists as a function at all.
func TestAChildIsMarked(t *testing.T) {
	const ambient = "LIRO_BRIDGE_PROBE_ENV_CANARY"
	t.Setenv(ambient, "present")

	env := ChildEnv()

	marked := false
	inherited := false
	for _, kv := range env {
		switch kv {
		case ChildMarker + "=1":
			marked = true
		case ambient + "=present":
			inherited = true
		}
	}

	if !marked {
		t.Errorf("a probe child is spawned without %s set.\n\n"+
			"That marker is the only thing that stops a child which does not "+
			"dispatch the subcommand from spawning children of its own. Measured "+
			"without it: 254 processes to 827.", ChildMarker)
	}
	if !inherited {
		t.Errorf("the child's environment does not carry this process's own.\n\n" +
			"A vendor module reads the environment during DllMain, so a child " +
			"probing with a replaced environment is probing under conditions the " +
			"agent never runs in, and any difference in what a module does would " +
			"have been measured in the wrong process.")
	}
}

// TestTheProbeSubcommandIsNotAPlausibleFileName guards a small thing with a
// large consequence: the subcommand travels as a positional argument, and the
// agent decides on args[0] alone. A name that could be mistaken for a path, or
// for one of the real subcommands, would make that decision ambiguous.
func TestTheProbeSubcommandIsNotAPlausibleFileName(t *testing.T) {
	if ProbeSubcommand == "" || strings.ContainsAny(ProbeSubcommand, `\/.:`) {
		t.Errorf("ProbeSubcommand is %q, which could be read as a path", ProbeSubcommand)
	}
	for _, taken := range []string{"certs", "sign", "open", "tray", "uninstall-notice"} {
		if ProbeSubcommand == taken {
			t.Errorf("ProbeSubcommand is %q, which is already a subcommand", taken)
		}
	}
}

// TestAChildThatExitsNonZeroBecomesAFailure is F12 §2's exit property, taken
// deterministically rather than by waiting for a module to crash.
//
// D-272 measured NetSeT 1.1.0.0 killing its host process about once in a
// hundred C_Initialize calls, in two ways, and D-275 established there is no
// in-process remedy. The whole point of the child is that such a death arrives
// in the parent as a value. What the parent actually sees is a non-zero exit
// status — it cannot tell a fail-fast from an access violation from a refusal,
// and does not need to.
//
// So the property under test is exactly that translation, and it can be taken
// without any module at all: a child asked to probe the empty path refuses and
// exits 2, which is a non-zero exit from a child, which is what a crash is.
// Waiting for a one-in-a-hundred crash to observe the same branch would measure
// the module rather than this program.
func TestAChildThatExitsNonZeroBecomesAFailure(t *testing.T) {
	_, err := probeOutOfProcess(context.Background(), "")
	if !errors.Is(err, errWorkerDied) {
		t.Fatalf("a child that exited non-zero produced %v, want %v\n\n"+
			"Every way a child can end must arrive here as an ordinary error, "+
			"because Modules puts it in a list and carries on. A crash that "+
			"reached the caller as anything else would be D-275 again.", err, errWorkerDied)
	}
	if !strings.Contains(err.Error(), "exit 0x") {
		t.Errorf("the failure reads %q and does not carry the exit status.\n\n"+
			"Which way the child died is the only thing distinguishing a "+
			"fail-fast from an access violation, and it is the number a person "+
			"debugging a vendor module has to go on.", err)
	}
}
