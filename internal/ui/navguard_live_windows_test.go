//go:build windows

package ui

// What the guard does to a real window, measured rather than reasoned
// about: a page of this program's own, and a real WebView2 underneath
// it, told to go somewhere else.
//
// Every click here is driven through the page's own DOM (D-094's Eval
// carve-out); nothing touches the real cursor.

import (
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const guardHost = "guard.liro.invalid"

// guardedWindow opens a real window on a page of its own.
func guardedWindow(t *testing.T) *window {
	t.Helper()
	assets := fstest.MapFS{
		"pages/probe.html": &fstest.MapFile{Data: []byte(
			`<!doctype html><title>guard probe</title><body><p id="here">ours</p></body>`)},
	}
	w, err := NewWindow(Options{
		Title:       "liro-bridge navigation guard test",
		Width:       320,
		Height:      240,
		Assets:      assets,
		VirtualHost: guardHost,
		StartPage:   "/pages/probe.html",
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	win, ok := w.(*window)
	if !ok {
		t.Fatalf("NewWindow returned a %T", w)
	}
	return win
}

// refusals reads the guards' counters on the thread that writes them.
// Reading them from the test's own goroutine would be a data race, and
// this suite runs under -race on the Windows runner (D-257 met the same
// thing and answered it the same way).
func refusals(w *window) (nav, frame, newWin int) {
	done := make(chan struct{})
	w.invoke(func() {
		if w.navGuard != nil {
			nav = w.navGuard.refused
		}
		if w.frameGuard != nil {
			frame = w.frameGuard.refused
		}
		if w.windowGuard != nil {
			newWin = w.windowGuard.refused
		}
		close(done)
	})
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
	return
}

func location(t *testing.T, w *window) string {
	t.Helper()
	got, err := w.Eval("location.href")
	if err != nil {
		t.Fatalf("Eval(location.href): %v", err)
	}
	return strings.Trim(got, `"`)
}

// The vector the report names as "a link in rendered content": the page
// asks to become a different document, and does not.
func TestAWindowRefusesToBecomeSomebodyElsesDocument(t *testing.T) {
	w := guardedWindow(t)

	before := location(t, w)
	if !strings.HasPrefix(before, "https://"+guardHost+"/") {
		t.Fatalf("the window did not start on its own page: %s", before)
	}
	navBefore, _, _ := refusals(w)

	if _, err := w.Eval(`location.href = "https://example.invalid/somewhere"`); err != nil {
		t.Fatalf("Eval(navigate away): %v", err)
	}

	// Give the navigation every chance to happen before concluding it
	// did not: wait until the guard says it refused one.
	deadline := time.Now().Add(20 * time.Second)
	navAfter := navBefore
	for time.Now().Before(deadline) && navAfter == navBefore {
		time.Sleep(100 * time.Millisecond)
		navAfter, _, _ = refusals(w)
	}

	if navAfter == navBefore {
		t.Errorf("the guard refused nothing; the window is now at %s", location(t, w))
	}
	if after := location(t, w); after != before {
		t.Errorf("the window became a different document:\n  was %s\n  now %s", before, after)
	}
	if got, err := w.Eval(`document.getElementById("here") !== null`); err != nil || got != "true" {
		t.Errorf("the page is no longer ours: Eval = %q, %v", got, err)
	}
}

// A redirect is a second NavigationStarting with IsRedirected set, and
// is refused by the same rule: what matters is where it ends up, not
// how it got there. Driven through a page of ours that redirects to a
// foreign host the only way an asset can — a meta refresh.
func TestAWindowRefusesARedirectOutOfItsOwnPages(t *testing.T) {
	assets := fstest.MapFS{
		"pages/probe.html": &fstest.MapFile{Data: []byte(
			`<!doctype html><title>redirect probe</title>` +
				`<meta http-equiv="refresh" content="0;url=https://example.invalid/gone">` +
				`<body><p id="here">ours</p></body>`)},
	}
	w, err := NewWindow(Options{
		Title:       "liro-bridge redirect guard test",
		Width:       320,
		Height:      240,
		Assets:      assets,
		VirtualHost: guardHost,
		StartPage:   "/pages/probe.html",
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	win := w.(*window)

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if nav, _, _ := refusals(win); nav > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if nav, _, _ := refusals(win); nav == 0 {
		t.Errorf("the guard refused nothing")
	}
	if got := location(t, win); !strings.HasPrefix(got, "https://"+guardHost+"/") {
		t.Errorf("the window followed the refresh: now at %s", got)
	}
}

// window.open, and a link with target="_blank": WebView2's answer to
// either is a second browser window this program neither created nor
// controls.
func TestAWindowRefusesToOpenASecondBrowserWindow(t *testing.T) {
	w := guardedWindow(t)
	_, _, before := refusals(w)

	if _, err := w.Eval(`window.open("https://example.invalid/", "_blank")`); err != nil {
		t.Fatalf("Eval(window.open): %v", err)
	}

	deadline := time.Now().Add(20 * time.Second)
	after := before
	for time.Now().Before(deadline) && after == before {
		time.Sleep(100 * time.Millisecond)
		_, _, after = refusals(w)
	}
	if after == before {
		t.Error("window.open was not refused")
	}
}

// The other half of the report: a window that takes no dropped files
// has nothing under it that accepts a drop at all.
//
// Read the way a drop is delivered — the OleDropTargetInterface window
// property, over the whole hosted tree — rather than from this
// package's own bookkeeping, because the window that carries Chromium's
// own target belongs to the msedgewebview2 process and this package has
// no record of it.
func TestNothingUnderAWindowThatTakesNoDropsAcceptsADrop(t *testing.T) {
	w := guardedWindow(t)

	var accepting []string
	for _, h := range append([]uintptr{w.hwnd}, descendantWindows(w.hwnd)...) {
		if hasDropTarget(h) {
			accepting = append(accepting, windowClass(h))
		}
	}
	if len(accepting) != 0 {
		t.Errorf("a window with no drop handler still accepts drops on %v —\n"+
			"a PDF dropped there navigates the window to it", accepting)
	}
}

// The mechanism behind that, read back from WebView2 itself rather than
// inferred from the absence of a window property: every window in this
// program has the browser's own external-drop handling switched off,
// whether or not it takes dropped documents.
//
// Whether a window carries an IDropTarget cannot say whose it is from
// outside (D-123 registers this program's own on windows the browser
// owns), so it cannot on its own distinguish "the browser will not
// handle a drop" from "nobody handles a drop". get_AllowExternalDrop
// answers exactly that, and only from in here.
func TestTheBrowsersOwnDropHandlingIsOffOnEveryWindow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drops func([]string)
	}{
		{"a window that takes no dropped documents", nil},
		{"the kind of window that does", func([]string) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := NewWindow(Options{
				Title:          "liro-bridge external drop test",
				Width:          300,
				Height:         200,
				OnFilesDropped: tc.drops,
			})
			if err != nil {
				t.Fatalf("NewWindow: %v", err)
			}
			t.Cleanup(func() { _ = w.Close() })
			win := w.(*window)

			var allowed uintptr = 1
			var readErr error
			done := make(chan struct{})
			win.invoke(func() {
				defer close(done)
				c4, err := queryInterface(win.controller, iidCoreWebView2Controller4)
				if err != nil {
					readErr = err
					return
				}
				defer comRelease(c4)
				var pin runtime.Pinner
				defer pin.Unpin()
				// ICoreWebView2Controller4::get_AllowExternalDrop, IDL
				// slot 36 — the getter immediately before slot 37's
				// setter, which this package already calls.
				_, readErr = comCall(c4, 36, pinPtr(&pin, &allowed))
			})
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("the UI thread never answered")
			}
			if readErr != nil {
				t.Fatalf("get_AllowExternalDrop: %v", readErr)
			}
			if allowed != 0 {
				t.Error("the browser is still allowed to handle files dropped on this window")
			}
		})
	}
}
