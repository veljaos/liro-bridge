//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	consoleKernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleProcessList = consoleKernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = consoleKernel32.NewProc("FreeConsole")
)

// detachAllocatedConsole gives back a console that Windows created for
// this process, and leaves alone one it inherited from a shell.
//
// Why this exists. This binary is linked for the console subsystem
// (IMAGE_SUBSYSTEM_WINDOWS_CUI), which is what lets `certs` print a
// listing and `--version` report a tag. The loader's rule for such a
// binary is that a process created with no console to inherit gets one
// allocated for it, before main runs. Every way a person reaches this
// program after installing it is exactly that: the installer's own
// final step, the HKCU\...\Run value, the Start menu shortcut and the
// Explorer verb are all CreateProcess from a parent with no console. So
// all four produced an empty terminal window beside the agent, with
// nothing in it and nothing to say — measured on a real install, and
// the first thing a stranger saw.
//
// Why the rule is "am I alone" rather than "which command is this".
// The console is allocated before main runs and before any argument has
// been looked at, so the command word cannot be what decides. What does
// decide, exactly, is whether anybody else is attached to it:
// GetConsoleProcessList reports every process on this console, and it
// reports 1 when and only when the loader made this console for this
// process. A console inherited from cmd.exe or PowerShell always has at
// least the shell on it as well.
//
// Why freeing loses nothing. The only output that dies with the console
// is output written to the console itself, and a console this process
// is alone on is one that closes when this process exits — nobody could
// have read it. A shell that redirected stdout to a pipe or a file
// handed over a handle that is not a console at all, and FreeConsole
// does not touch it, so capture and exit codes are unaffected.
//
// Two alternatives were measured and rejected; see the decision entry.
// Linking with -H=windowsgui removes the console at the cost of output
// capture and the exit code — PowerShell returns nothing and an empty
// $LASTEXITCODE for a GUI-subsystem binary, which CI's own --version
// check and D-236's "a script sees only the exit code" both depend on.
// Hiding the console window cannot work at all: with Windows Terminal
// as the default terminal, GetConsoleWindow returns a zero-sized
// PseudoConsoleWindow inside this process, not the window a person can
// see.
//
// It must run before anything writes anywhere, so main calls it first.
// It says nothing on failure because there is nothing to say and
// nowhere yet to say it: logging is not set up this early, and a
// process that has no console has already got what this function wants.
func detachAllocatedConsole() {
	// Two entries is enough to tell "one" from "more than one".
	// GetConsoleProcessList returns the total when the buffer is too
	// small, without filling it, so a crowded console still answers the
	// only question being asked. Zero means no console at all, which is
	// already the state this function exists to reach.
	var attached [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&attached[0])),
		uintptr(len(attached)),
	)
	if n != 1 {
		return
	}
	_, _, _ = procFreeConsole.Call()
}
