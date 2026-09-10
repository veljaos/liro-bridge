//go:build windows

package main

import (
	"io"
	"os"
	"path/filepath"
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

var procFindWindowExT = user32Test.NewProc("FindWindowExW")

// liroWindowsTitled is every top-level window of this process's own
// window class carrying title. There may already be some: the suite
// shares long-lived windows between tests (sharedwindow_windows_test.go)
// and one of them is titled main.title too, so "a window appeared" has
// to mean a window that was not there before.
func liroWindowsTitled(t *testing.T, title string) map[uintptr]bool {
	t.Helper()
	class, err := windows.UTF16PtrFromString("LiroBridgeWindow")
	if err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(title)
	if err != nil {
		t.Fatal(err)
	}
	found := map[uintptr]bool{}
	var after uintptr
	for {
		h, _, _ := procFindWindowExT.Call(0, after, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)))
		if h == 0 {
			return found
		}
		found[h] = true
		after = h
	}
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
