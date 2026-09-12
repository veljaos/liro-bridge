//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// createNewConsole puts a process in the state every launcher of an
// installed agent puts it in: a console of its own, with nobody else on
// it. The installer's final step, the HKCU\...\Run value, the Start
// menu shortcut and the Explorer verb all reach that state by having no
// console to hand down, after which the loader allocates one. This flag
// reaches the same state deliberately, from a test whose own process
// has a console it needs to keep.
const createNewConsole = 0x00000010

// The helper's two answers. Exit codes rather than output, because the
// helper gives up its own console to ask the question and has nowhere
// left to print.
const (
	exitTargetHasConsole   = 30
	exitTargetHasNoConsole = 31
)

// TestConsoleObserverHelper is not a test. It is this binary acting as
// a probe for the tests below.
//
// Whether another process has a console can only be asked by a process
// that has none of its own — that is AttachConsole's own rule — so the
// question cannot be asked from a test binary running in somebody's
// terminal. A second copy of this binary asks it instead, giving up its
// console first and answering through its exit code.
func TestConsoleObserverHelper(t *testing.T) {
	target := os.Getenv("LIRO_CONSOLE_OBSERVE_PID")
	if target == "" {
		t.Skip("not the helper; this runs only as a child of the tests below")
	}
	pid, err := strconv.Atoi(target)
	if err != nil {
		t.Fatalf("LIRO_CONSOLE_OBSERVE_PID=%q: %v", target, err)
	}
	freed, _, freeErr := procFreeConsole.Call()
	attach := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	attached, _, attachErr := attach.Call(uintptr(uint32(pid)))
	if d := os.Getenv("LIRO_CONSOLE_OBSERVE_DIAG"); d != "" {
		_ = os.WriteFile(d, []byte(fmt.Sprintf("freed=%d freeErr=%v attached=%d attachErr=%v", freed, freeErr, attached, attachErr)), 0o644)
	}
	if attached != 0 {
		os.Exit(exitTargetHasConsole)
	}
	os.Exit(exitTargetHasNoConsole)
}

// observeConsoleOf reports whether a process is attached to a console,
// by asking a copy of this test binary that has given up its own.
func observeConsoleOf(t *testing.T, pid int) bool {
	t.Helper()
	helper := exec.Command(os.Args[0], "-test.run=^TestConsoleObserverHelper$")
	diag := filepath.Join(t.TempDir(), "diag.txt")
	helper.Env = append(os.Environ(),
		"LIRO_CONSOLE_OBSERVE_PID="+strconv.Itoa(pid),
		"LIRO_CONSOLE_OBSERVE_DIAG="+diag)
	err := helper.Run()
	if b, e := os.ReadFile(diag); e == nil {
		t.Logf("console observer: %s", b)
	}
	switch code := helper.ProcessState.ExitCode(); code {
	case exitTargetHasConsole:
		return true
	case exitTargetHasNoConsole:
		return false
	default:
		t.Fatalf("the console observer answered neither way: exit %d (%v)", code, err)
		return false
	}
}

// startAgent starts the built agent as a tray process and returns its
// PID. A long-lived command is needed because the question is about a
// process that is still running, and `tray` is the only one — so the
// Explorer verb and the autostart value it registers about itself
// (startup_windows.go) are put back afterwards, and everything else it
// writes goes into a config home of its own.
func startAgent(t *testing.T, exe string, ownConsole bool) int {
	t.Helper()
	cmd := exec.Command(exe, "tray")
	if ownConsole {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the agent: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		// By its own handle, which is its exact PID, and never by image
		// name.
		_ = cmd.Process.Kill()
		<-done
	})

	// Wait for the agent to say it is up, rather than for a length of
	// time (D-201). Its discovery file is that statement: the tray
	// writes bridge.json once it is listening, and a console's terminal
	// handoff is long finished by then — asking before it is what made
	// an earlier version of this test report "no console" for a binary
	// that plainly had one. The ceiling only turns an agent that never
	// starts into a failure instead of a hang.
	discovery := filepath.Join(os.Getenv("LOCALAPPDATA"), "Liro", "bridge.json")
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(discovery); err == nil {
			return cmd.Process.Pid
		}
		select {
		case <-done:
			t.Fatalf("the agent exited before it was listening; nothing about a console can be read from that")
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("the agent never wrote %s, so it never finished starting", discovery)
		}
	}
}

// TestAConsoleTheAgentIsAloneOnIsGivenBack is the regression test for
// the first thing a stranger saw after installing: an empty terminal
// window beside the agent, saying nothing.
//
// It starts the built agent with a console of its own — a launcher's
// situation — and requires that it no longer has one. Against the
// binary this phase began from it keeps it, and that console is the
// window.
//
// What this asserts is the rule rather than the pixels, on purpose.
// Whether a window is drawn for the length of a frame depends on when
// the terminal handoff completes, and racing that is how a check passes
// for the wrong reason: an earlier version of this test polled for the
// window and went green against the unfixed binary, because it looked
// before the window had been created. Whether a launcher's own path
// shows a window at all was measured separately, and the number is in
// the decision entry, where a number belongs.
func TestAConsoleTheAgentIsAloneOnIsGivenBack(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and starts the agent")
	}
	keepThisMachinesAutostartAndMenu(t)
	tempConfigHome(t)

	exe := build(t, t.TempDir(), "alone", nil)
	pid := startAgent(t, exe, true)

	if observeConsoleOf(t, pid) {
		t.Error("the agent kept the console it was alone on; every launcher an installed agent is reached by puts it in exactly this state, so a person who has just installed it sees an empty terminal beside the agent")
	}
}

// TestAConsoleSharedWithACallerIsKept is the other half, and the one
// that keeps `sign` and `certs` printing for a person at a prompt.
//
// An agent started the ordinary way inherits this test binary's own
// console, so it is not alone on it and must keep it. Without this the
// rule could be "always give the console back", which would take the
// output of every command run from a shell with it and still pass the
// test above.
func TestAConsoleSharedWithACallerIsKept(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and starts the agent")
	}
	if !thisProcessHasAConsole() {
		t.Skip("this test process has no console to share, so there is nothing here to keep")
	}
	keepThisMachinesAutostartAndMenu(t)
	tempConfigHome(t)

	exe := build(t, t.TempDir(), "shared", nil)
	pid := startAgent(t, exe, false)

	if !observeConsoleOf(t, pid) {
		t.Error("the agent gave back a console it shared with its caller; a console with somebody else on it is the caller's, and taking it silences certs and sign at a prompt")
	}
}

// thisProcessHasAConsole says whether the question above can be asked
// here at all. A test binary whose output a runner has redirected may
// have no console, and then there is no shared console to inherit.
func thisProcessHasAConsole() bool {
	var attached [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&attached[0])),
		uintptr(len(attached)),
	)
	return n > 0
}

// TestOutputSurvivesForACallerThatRedirectedIt is the third thing the
// rule must not break: a caller that redirected stdout handed over a
// handle that is not a console at all, and nothing here may touch it.
func TestOutputSurvivesForACallerThatRedirectedIt(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	exe := build(t, t.TempDir(), "redirected", nil)

	out, err := exec.Command(exe, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("--version: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), "liro-bridge") {
		t.Errorf("--version printed %q; every script that reads this output, CI's own tag check included, gets whatever this is", out)
	}
}

// TestTheAgentStillLinksForTheConsoleSubsystem pins the half of the
// arrangement that is easiest to undo by accident.
//
// Linking with -H=windowsgui is the obvious way to remove a console
// window, and it was measured to cost output capture and the exit code
// outright: PowerShell returns nothing at all and an empty
// $LASTEXITCODE for a GUI-subsystem binary. CI's own --version check
// depends on the first and D-236's "a script sees only the exit code"
// on the second, and neither would fail loudly — both would quietly
// start reading nothing. This is what says so at the moment somebody
// tries it.
func TestTheAgentStillLinksForTheConsoleSubsystem(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	exe := build(t, t.TempDir(), "subsystem", nil)
	if got := peSubsystem(t, exe); got != imageSubsystemWindowsCUI {
		t.Errorf("the agent links for subsystem %d, want %d (console); a GUI-subsystem binary loses output capture and the exit code, which CI and every script depend on", got, imageSubsystemWindowsCUI)
	}
}

const imageSubsystemWindowsCUI = 3

// peSubsystem reads the Subsystem field out of the PE optional header:
// e_lfanew at 0x3C, then the 24-byte COFF header, then the field at
// offset 68 of the optional header, which is where it sits in both
// PE32 and PE32+.
func peSubsystem(t *testing.T, path string) uint16 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if len(b) < 0x40 {
		t.Fatalf("%s is too short to be a PE file", path)
	}
	peOff := int(uint32(b[0x3C]) | uint32(b[0x3D])<<8 | uint32(b[0x3E])<<16 | uint32(b[0x3F])<<24)
	off := peOff + 24 + 68
	if off+2 > len(b) {
		t.Fatalf("%s: the optional header does not reach its Subsystem field", path)
	}
	return uint16(b[off]) | uint16(b[off+1])<<8
}
