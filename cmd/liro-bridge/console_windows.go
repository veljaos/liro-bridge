//go:build windows

package main

import (
	"log"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
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
	// Which standard handles name the console, read before it goes. A
	// caller that redirected one handed over a pipe or a file, which
	// FreeConsole does not touch and nothing here may replace — that is
	// how `--version` reaches a script.
	onConsole := consoleStandardHandles()
	if r, _, _ := procFreeConsole.Call(); r == 0 {
		return
	}
	pointStandardHandlesAtNUL(onConsole)
}

// stdHandles are the three standard handles, in os.Stdin, os.Stdout,
// os.Stderr order.
var stdHandles = [3]uint32{windows.STD_INPUT_HANDLE, windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE}

// consoleStandardHandles reports, for each standard handle, whether it
// is a console handle — GetConsoleMode answers for a console's input or
// screen buffer and refuses a file, a pipe and NUL alike.
func consoleStandardHandles() [3]bool {
	var out [3]bool
	for i, std := range stdHandles {
		h, err := windows.GetStdHandle(std)
		if err != nil || h == 0 || h == windows.InvalidHandle {
			continue
		}
		var mode uint32
		out[i] = windows.GetConsoleMode(h, &mode) == nil
	}
	return out
}

// retiredStdFiles keeps the *os.File values the os package made at
// start, for the standard handles replaced below, alive for the life of the process. Each closes its handle when
// it is collected, and after FreeConsole the number each holds may be
// another file's — bridge.log's, crash.log's, the audit chain's — so a
// collected one would close that file under whoever has it.
var retiredStdFiles []*os.File

// pointStandardHandlesAtNUL gives the three standard handles somewhere
// harmless to point once the console they named is gone.
//
// FreeConsole closes the console the loader made, and the standard
// handles keep its numbers. A number that has been closed is given to
// the next handle opened, so the first file this process opened after
// detaching — bridge.log, in D-454's watch — became "standard error"
// as far as anything asking GetStdHandle was concerned: the runtime's
// fatal errors (runtime.write1 asks on every write), os.Stderr, a DLL's
// C runtime. D-454 watched the crash text land in bridge.log that way,
// which was luck: the next file might have been the audit chain or a
// document being signed (C21 row 11, D44, [[D-457]]).
//
// So: each standard handle that named the console — only those; a
// redirected one is the caller's — is replaced by a handle to NUL, and
// the os package's file for it and the log package's writer re-pointed
// at the new one. What was written to a console nobody could see now goes
// nowhere, which is where it went before, without the chance of going
// somewhere else. Nothing is said on failure, for detachAllocatedConsole's
// reason: there is nowhere yet to say it.
func pointStandardHandlesAtNUL(onConsole [3]bool) {
	nul, err := windows.UTF16PtrFromString("NUL")
	if err != nil {
		return
	}
	files := [3]**os.File{&os.Stdin, &os.Stdout, &os.Stderr}
	access := [3]uint32{windows.GENERIC_READ, windows.GENERIC_WRITE, windows.GENERIC_WRITE}
	for i, std := range stdHandles {
		if !onConsole[i] {
			continue
		}
		h, err := windows.CreateFile(nul, access[i], windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		if err := windows.SetStdHandle(std, h); err != nil {
			_ = windows.CloseHandle(h)
			continue
		}
		retiredStdFiles = append(retiredStdFiles, *files[i])
		*files[i] = os.NewFile(uintptr(h), "NUL")
	}
	log.SetOutput(os.Stderr)
}
