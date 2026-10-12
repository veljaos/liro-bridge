//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// staleStderrHelperEnv makes a copy of this test binary the child of
// the test below; its value is the directory the child writes into.
const staleStderrHelperEnv = "LIRO_STALE_STDERR_HELPER"

// staleStderrMarker is what the child writes to standard error after it
// has given its console back. It must reach no file.
const staleStderrMarker = "LIRO-STDERR-AFTER-FREECONSOLE"

// TestStaleStderrHelper is not a test. It is this binary started the way
// every launcher starts an installed agent — no console to inherit and no
// standard handles handed down, so the loader makes a console for it and
// points the three standard handles at it — doing what main does first:
// give that console back. Then it opens a file, as SetupLogging opens
// bridge.log, and writes to standard error through the runtime's own
// path, the one a fatal error takes (runtime.write1 asks GetStdHandle on
// every write).
func TestStaleStderrHelper(t *testing.T) {
	dir := os.Getenv(staleStderrHelperEnv)
	if dir == "" {
		t.Skip("not the helper; this runs only as a child of the test below")
	}
	errBefore, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	var attached [2]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&attached[0])), 2)

	detachAllocatedConsole()
	// Read before anything is opened, so a reused number cannot stand in
	// for the answer (worklist 0.2, D-473).
	errDetached, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	stderrKind := handleKind(errDetached)
	osStderrFollows := os.Stderr.Fd() == uintptr(errDetached)

	f, err := os.Create(filepath.Join(dir, "opened-after.log"))
	if err != nil {
		t.Fatalf("creating the file: %v", err)
	}
	errAfter, _ := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	// Read before Close: a closed os.File's Fd is -1, which is what the
	// first run of this printed (session 33), measuring nothing.
	fileHandle := f.Fd()
	println(staleStderrMarker + " runtime")
	fmt.Fprintln(os.Stderr, staleStderrMarker+" os.Stderr")
	_ = f.Close()

	diag := fmt.Sprintf("consoleProcesses=%d stderrBefore=%#x file=%#x stderrAfter=%#x stderrAfterDetach=%s osStderrFollows=%v ",
		n, uintptr(errBefore), fileHandle, uintptr(errAfter), stderrKind, osStderrFollows)
	_ = os.WriteFile(filepath.Join(dir, "diag.txt"), []byte(diag), 0o644)
}

// TestNothingWrittenToStderrAfterTheConsoleIsGivenBackLandsInAFile is
// C21 row 11 and D44's mechanism ([[D-454]], [[D-457]]).
//
// D-454 watched the runtime's crash text land in bridge.log, which no
// code of ours routes stderr to. The reading: FreeConsole closes the
// console the standard handles named, leaves their numbers in the
// process's parameters, and the next file opened can be given one of
// those numbers — after which anything written to "standard error", the
// runtime's fatal errors and os.Stderr alike, goes into that file. The
// first file the agent opens is bridge.log; it could as well have been
// the audit chain or a document being signed.
//
// The child is created with CreateProcess directly, not os/exec: exec
// hands a child NUL for every standard handle it was not given, and
// that is not a launcher's situation — Explorer and the Run key hand
// down none, which is what makes the loader point them at a console of
// the child's own.
func TestNothingWrittenToStderrAfterTheConsoleIsGivenBackLandsInAFile(t *testing.T) {
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmdline := windows.ComposeCommandLine([]string{exe, "-test.run=^TestStaleStderrHelper$", "-test.v"})
	cmdlinePtr, err := windows.UTF16PtrFromString(cmdline)
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), staleStderrHelperEnv+"="+dir)
	envBlock := environmentBlock(env)

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	const createUnicodeEnvironment = 0x00000400
	if err := windows.CreateProcess(nil, cmdlinePtr, nil, nil, false,
		createNewConsole|createUnicodeEnvironment, &envBlock[0], nil, &si, &pi); err != nil {
		t.Fatalf("CreateProcess: %v", err)
	}
	defer func() {
		_ = windows.CloseHandle(pi.Thread)
		_ = windows.CloseHandle(pi.Process)
	}()
	ev, err := windows.WaitForSingleObject(pi.Process, uint32((60 * time.Second).Milliseconds()))
	if err != nil || ev != windows.WAIT_OBJECT_0 {
		_ = windows.TerminateProcess(pi.Process, 1)
		t.Fatalf("the helper did not finish within 60 s (wait %d, %v)", ev, err)
	}
	var code uint32
	_ = windows.GetExitCodeProcess(pi.Process, &code)

	diag, err := os.ReadFile(filepath.Join(dir, "diag.txt"))
	if err != nil {
		t.Fatalf("the helper wrote no diagnosis (exit %d), so it never reached the measurement: %v", code, err)
	}
	t.Logf("helper (exit %d): %s", code, diag)
	if !strings.Contains(string(diag), "consoleProcesses=1 ") {
		t.Fatalf("the helper was not alone on its console, so it did not give one back and this proves nothing: %s", diag)
	}
	opened, err := os.ReadFile(filepath.Join(dir, "opened-after.log"))
	if err != nil {
		t.Fatalf("reading the file the helper opened: %v", err)
	}
	// What the fix does, read directly: this test passed on the code
	// before it, because no file happened to be given the console's old
	// number on the runner (D-457, P3f). So it is not enough that the
	// marker missed the file; standard error must have been re-pointed
	// at NUL — a character device that is not a console — and os.Stderr
	// with it. On the old code the handle still named the closed
	// console, which GetFileType refuses (worklist 0.2, D-473).
	if !strings.Contains(string(diag), "stderrAfterDetach=nul ") {
		t.Errorf("after the console was given back, standard error is not NUL: %s", diag)
	}
	if !strings.Contains(string(diag), "osStderrFollows=true ") {
		t.Errorf("os.Stderr does not write where standard error now points: %s", diag)
	}
	if strings.Contains(string(opened), staleStderrMarker) {
		t.Errorf("standard error, written after the console was given back, landed in a file the process opened afterwards:\n%s\n"+
			"after FreeConsole the standard handles named a number the next file was given", opened)
	}
}

// handleKind says what a handle is: "invalid" when the system refuses it
// (a closed number), "console" for a console's own buffer, "nul" for a
// character device that is not a console, or the file type otherwise.
func handleKind(h windows.Handle) string {
	if h == 0 || h == windows.InvalidHandle {
		return "none"
	}
	ft, err := windows.GetFileType(h)
	if err != nil {
		return "invalid"
	}
	switch ft {
	case windows.FILE_TYPE_CHAR:
		var mode uint32
		if windows.GetConsoleMode(h, &mode) == nil {
			return "console"
		}
		return "nul"
	case windows.FILE_TYPE_DISK:
		return "disk"
	case windows.FILE_TYPE_PIPE:
		return "pipe"
	default:
		return fmt.Sprintf("type%d", ft)
	}
}

// environmentBlock is a UTF-16 environment block: each NAME=value
// terminated, the whole block terminated once more.
func environmentBlock(env []string) []uint16 {
	var b []uint16
	for _, kv := range env {
		u, err := syscall.UTF16FromString(kv)
		if err != nil {
			continue
		}
		b = append(b, u...)
	}
	return append(b, 0)
}
