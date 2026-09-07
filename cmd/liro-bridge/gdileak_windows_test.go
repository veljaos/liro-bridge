//go:build windows

package main

import (
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/ui"
)

// Every window this agent opens loaded two icons with LoadImageW and
// never gave them back: 6 GDI objects and 2 USER objects per window,
// measured exactly, monotonic, against a process quota of 10,000.
// Found by FTEST Group 3 by opening each of the seven windows a hundred
// times and reading the counters — never by any test, because every
// test in this package borrows one shared window per page and closes it
// once, at the end (sharedwindow_windows_test.go). One window that
// leaks is indistinguishable from one that does not.
//
// internal/ui's TestAnIconLoadedForAWindowIsGivenBack proves the
// primitive frees them. This proves a real window calls it.
//
// Ten windows rather than a hundred: the property is per-window and
// linear, so ten measures it, and ten is about four seconds — which is
// a price worth paying on every run for a defect whose whole symptom is
// that nothing ever noticed it.
func TestOpeningAndClosingWindowsDoesNotLeakGDIObjects(t *testing.T) {
	const windows = 10

	open := func() {
		messages := make(chan ui.Message, 8)
		win, err := ui.NewWindow(ui.Options{
			Title:       "gdi leak check",
			Width:       460,
			Height:      520,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/auditlog.html",
			OnMessage:   func(m ui.Message) { messages <- m },
			OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
		})
		if err != nil {
			t.Fatalf("NewWindow: %v", err)
		}
		if err := win.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	// One window first, so the one-time costs of the window layer — the
	// extracted .ico, the WebView2 environment, a thread or two — are
	// paid before the baseline is read.
	open()
	time.Sleep(time.Second)

	gdiBefore, userBefore := processGDIObjects(), processUserObjects()

	for i := 0; i < windows; i++ {
		open()
	}
	time.Sleep(time.Second)

	gdiAfter, userAfter := processGDIObjects(), processUserObjects()
	gdiGrowth := int(gdiAfter) - int(gdiBefore)
	userGrowth := int(userAfter) - int(userBefore)
	t.Logf("%d windows: GDI %d -> %d (%+d, %.2f per window), USER %d -> %d (%+d, %.2f per window)",
		windows, gdiBefore, gdiAfter, gdiGrowth, float64(gdiGrowth)/windows,
		userBefore, userAfter, userGrowth, float64(userGrowth)/windows)

	// The measured leak was 6.00 GDI per window. Two per window is well
	// under that and well above the noise of a process that also has a
	// browser engine starting and stopping inside it.
	if gdiGrowth > 2*windows {
		t.Errorf("GDI objects grew by %d across %d open/close cycles (%.2f per window): "+
			"a window is not giving back what it allocated",
			gdiGrowth, windows, float64(gdiGrowth)/windows)
	}
	if userGrowth > 2*windows {
		t.Errorf("USER objects grew by %d across %d open/close cycles (%.2f per window): "+
			"a window is not giving back what it allocated",
			userGrowth, windows, float64(userGrowth)/windows)
	}
}
