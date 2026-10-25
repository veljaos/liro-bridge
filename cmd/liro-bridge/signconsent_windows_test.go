//go:build windows

package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This is the consent defect's own regression test.
//
// `liro-bridge sign` used to sign without asking anybody: the consent
// window appeared only behind `--interactive`. That is a bypass
// reachable by any process on the machine, with no pairing, no origin
// binding and no human — against SPEC §18.2 ("No signature without
// human approval. No flag, no configuration, no header bypasses the
// consent screen") and SPEC §4.3 ("All four entry points go through the
// same consent screen, the same session and the same audit log").
//
// It carries no build tag on purpose. The phase asked for a test that
// `sign` opens a window *without* the softtoken tag; `sign` opens one in
// every build, so asserting it in every build is both stronger and the
// only way this runs on the Windows CI job, which builds the suite with
// the tag. A check that only runs where it was written is not evidence
// about where it will run (D-221).
//
// Run against the code as it stood, it fails twice over: no window
// appears at all, and `sign` reports a missing --thumbprint instead.

var (
	procFindWindowExT    = user32Test.NewProc("FindWindowExW")
	procEnumWindowsT     = user32Test.NewProc("EnumWindows")
	procGetWindowTextWT  = user32Test.NewProc("InternalGetWindowText")
	procGetWindowPIDT    = user32Test.NewProc("GetWindowThreadProcessId")
	enumTopLevelCallback = syscall.NewCallback(enumTopLevel)
	enumTopLevelMu       sync.Mutex
	enumTopLevelOut      []uintptr
)

// enumTopLevel collects every top-level window EnumWindows lists. One
// callback, made once: syscall.NewCallback's slots are never given back
// ([[D-453]]).
func enumTopLevel(hwnd, _ uintptr) uintptr {
	enumTopLevelOut = append(enumTopLevelOut, hwnd)
	return 1
}

// topLevelWindows is EnumWindows' list: every top-level window on the
// desktop at one moment, taken by Windows before the first callback.
func topLevelWindows() []uintptr {
	enumTopLevelMu.Lock()
	defer enumTopLevelMu.Unlock()
	enumTopLevelOut = nil
	_, _, _ = procEnumWindowsT.Call(enumTopLevelCallback, 0)
	out := enumTopLevelOut
	enumTopLevelOut = nil
	return out
}

// windowTextT reads a class name or a title. The title through
// InternalGetWindowText, which reads it where Windows keeps it rather
// than sending WM_GETTEXT to the UI thread — a busy UI thread must not
// stall a test that is only looking.
func windowTextT(hwnd uintptr, proc *windows.LazyProc) string {
	buf := make([]uint16, 256)
	n, _, _ := proc.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// liroWindowsTitled is every top-level window of this process's own
// window class carrying title. There may already be some: the suite
// shares long-lived windows between tests (sharedwindow_windows_test.go)
// and one of them is titled main.title too, so "a window appeared" has
// to mean a window that was not there before.
//
// **A snapshot, not a walk** ([[D-458]], D39). This used to call
// FindWindowExW with the last window found as the cursor, and
// FindWindowExW given a cursor that has since been destroyed finds
// nothing — so a walk that met a window being torn down (the previous
// test's, closed a moment earlier) stopped there, the shared main window
// below it was left out of "before", and a test then took the shared
// window for the new one and closed it: run 37833946737, where the sign
// window was never closed and every later test that borrowed the shared
// window found it closed. EnumWindows lists the windows as they are at
// one moment and has no cursor to lose.
func liroWindowsTitled(t *testing.T, title string) map[uintptr]bool {
	t.Helper()
	pid := uint32(os.Getpid())
	found := map[uintptr]bool{}
	for _, h := range topLevelWindows() {
		var owner uint32
		_, _, _ = procGetWindowPIDT.Call(h, uintptr(unsafe.Pointer(&owner)))
		if owner != pid {
			continue
		}
		if windowTextT(h, procGetClassNameWT) != "LiroBridgeWindow" {
			continue
		}
		if windowTextT(h, procGetWindowTextWT) != title {
			continue
		}
		found[h] = true
	}
	return found
}

// waitForNewWindowTitled waits for a window of this process's own class
// carrying title that was not in before, and that is actually on screen.
//
// Visible, not merely created: ui.NewWindow makes the frame first and
// shows it only once its WebView2 control and page are ready, a couple
// of seconds later. A WM_CLOSE posted into that gap is dispatched into a
// half-built window and NewWindow then waits for a navigation that will
// never complete — measured, as a hang rather than a failure. Waiting
// for an observable state rather than for a moment is D-201's rule.
func waitForNewWindowTitled(t *testing.T, title string, before map[uintptr]bool, within time.Duration) uintptr {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		for h := range liroWindowsTitled(t, title) {
			if before[h] {
				continue
			}
			if visible, _, _ := procIsWindowVisibleT.Call(h); visible != 0 {
				return h
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no new visible window titled %q appeared within %v", title, within)
	return 0
}

// waitForNewWindowTitledOrExit is waitForNewWindowTitled for a command
// running on its own goroutine: it gives up the moment that command
// returns, and says so, rather than waiting out a window that cannot
// arrive any more.
//
// A command that has already exited is a different failure from a
// command that is still running and has drawn nothing, and they are the
// two this test can produce. Reporting them as one — "no window
// appeared" — is what made a Windows CI failure a symptom with no cause,
// which is the whole subject of D-236.
func waitForNewWindowTitledOrExit(t *testing.T, title string, before map[uintptr]bool, done <-chan int, within time.Duration) uintptr {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		for h := range liroWindowsTitled(t, title) {
			if before[h] {
				continue
			}
			if visible, _, _ := procIsWindowVisibleT.Call(h); visible != 0 {
				return h
			}
		}
		select {
		case code := <-done:
			t.Fatalf("sign returned %d without ever showing a window; nobody was asked and nothing said so", code)
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatalf("no new visible window titled %q appeared within %v, and sign is still running", title, within)
	return 0
}

func TestTheSignCommandOpensAWindowAndSignsNothingUntilItIsAnswered(t *testing.T) {
	tempConfigHome(t)

	dir := t.TempDir()
	in := filepath.Join(dir, "ugovor.pdf")
	blank, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, blank, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "ugovor-signed.pdf")

	// The title is the same string in all three catalogues, so this
	// does not depend on which locale the temporary config home
	// defaults to.
	const title = "Liro Bridge"
	before := liroWindowsTitled(t, title)

	// run(), not runSignCommand(): the dispatch is half of what broke,
	// so the test drives the command exactly as main does.
	done := make(chan int, 1)
	go func() { done <- run([]string{"sign", "--in", in}, io.Discard) }()

	// The exit code is watched alongside the window, because the two
	// ways this can fail need different answers and the wait alone
	// cannot tell them apart. On windows-latest it failed the second
	// way: `sign` was already over — 1.011s, exit 1, nothing on stdout
	// — and the test spent the remaining eighty-nine seconds waiting
	// for a window belonging to a process that had finished. "No window
	// appeared" was true and said nothing about why (D-236).
	hwnd := waitForNewWindowTitledOrExit(t, title, before, done, 90*time.Second)

	// Nothing may have been signed by the time a person is first asked.
	if _, err := os.Stat(out); err == nil {
		t.Errorf("sign wrote %s before anybody approved anything", out)
	}

	// A window message, not synthetic input: SetCursorPos/mouse_event
	// are global on Windows and land on whatever happens to be in the
	// foreground (D-094).
	const wmClose = 0x0010
	_, _, _ = procPostMessageT.Call(hwnd, wmClose, 0, 0)

	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("sign did not return after its window was closed")
	}

	if _, err := os.Stat(out); err == nil {
		t.Errorf("sign wrote %s although its window was closed rather than approved", out)
	}
}

func TestSignRejectsInteractiveRatherThanIgnoringIt(t *testing.T) {
	if _, _, err := parseSignArgs([]string{"--interactive", "--in", "x.pdf"}); err == nil {
		t.Fatal("sign still accepts --interactive, which advertises a distinction that no longer exists")
	}
}

// TestAFindWindowWalkLosesItsPlaceWhenItsCursorIsDestroyed is the
// mechanism D39's reading rests on, measured where it happened — a
// Windows runner — rather than taken from memory ([[D-458]]).
//
// Two top-level windows of one class and title; the walk's cursor, the
// first, destroyed; FindWindowExW asked for the next one after it. If it
// finds the second, the reading is wrong and D39 is still open.
func TestAFindWindowWalkLosesItsPlaceWhenItsCursorIsDestroyed(t *testing.T) {
	class, _ := windows.UTF16PtrFromString("STATIC")
	title, _ := windows.UTF16PtrFromString("liro-bridge D39 walk " + time.Now().Format("150405.000000"))
	const wsPopup = 0x80000000

	type result struct {
		first, second, nextAfterDead, fromStart uintptr
		lastErr                                 error
	}
	got := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		create := windows.NewLazySystemDLL("user32.dll").NewProc("CreateWindowExW")
		destroy := windows.NewLazySystemDLL("user32.dll").NewProc("DestroyWindow")
		var r result
		r.first, _, _ = create.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 10, 10, 0, 0, 0, 0)
		r.second, _, _ = create.Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 10, 10, 0, 0, 0, 0)
		if r.first != 0 {
			_, _, _ = destroy.Call(r.first)
		}
		r.nextAfterDead, _, r.lastErr = procFindWindowExT.Call(0, r.first, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
		r.fromStart, _, _ = procFindWindowExT.Call(0, 0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
		if r.second != 0 {
			_, _, _ = destroy.Call(r.second)
		}
		got <- r
	}()
	r := <-got
	if r.first == 0 || r.second == 0 {
		t.Fatalf("the two windows could not be made (%#x, %#x)", r.first, r.second)
	}
	t.Logf("second %#x; from the start %#x; after the destroyed cursor %#x (%v)", r.second, r.fromStart, r.nextAfterDead, r.lastErr)
	if r.fromStart != r.second {
		t.Fatalf("a walk from the start did not find the window that exists (%#x, want %#x); this measures nothing", r.fromStart, r.second)
	}
	if r.nextAfterDead != 0 {
		t.Fatalf("FindWindowExW continued past a destroyed cursor and found %#x; D-458's reading of D39 is wrong", r.nextAfterDead)
	}
}
