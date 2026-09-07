//go:build windows

package main

import (
	"errors"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// These two tests deliberately do not point LOCALAPPDATA at a temporary
// directory, unlike most of this package's window tests.
//
// ShowPairing creates its own WebView2 window, and a WebView2 process
// may only ever open one user data folder — which is derived from
// LOCALAPPDATA. A window created here under a different one than the
// shared windows were created under does come up, and then closes on
// its own a moment later: measured, as this test failing with "ui:
// window is closed" on the first Eval when run alongside the rest of
// the package, and passing when run alone.
//
// Not redirecting it is safe because nothing here writes anything: the
// only thing ShowPairing reads the configuration for is the interface
// language, and every assertion below is about a value that is the same
// in all three (a name, a code, a refusal reaching Go).

// The adapter internal/api is actually handed, driven the way the
// pairing flow drives it: open a window for a prompt, watch for the
// person's refusal, tell it the pairing succeeded, close it.
//
// It opens its own window rather than borrowing the shared one because
// what is under test is ShowPairing itself — the window it creates, the
// payload it posts and the refusal it reports — not the page's
// rendering, which the tests beside this one already cover.
func TestShowPairingOpensARealWindowAndItsRefusalReachesGo(t *testing.T) {
	win, err := newPairingUI(config.Default()).ShowPairing(testPrompt())
	if err != nil {
		t.Fatalf("ShowPairing: %v", err)
	}
	defer win.Close()

	select {
	case <-win.Denied():
		t.Fatal("the window reported a refusal nobody made")
	default:
	}

	// A successful pairing: the window says so rather than vanishing.
	win.Confirmed()

	pw, ok := win.(*pairingWindow)
	if !ok {
		t.Fatalf("ShowPairing returned %T, want *pairingWindow", win)
	}
	if display := evalString(t, pw.win,
		"getComputedStyle(document.getElementById('state-connected')).display"); display == "none" {
		t.Fatal("Confirmed did not put the connected screen on screen")
	}

	// And dismissing the window reaches Go. Clicking through the page's
	// own DOM is D-094's carve-out — nothing here touches the real
	// cursor — and this channel is what the pairing flow's watcher waits
	// on.
	//
	// The Eval that delivers the click can itself report the window
	// closed, and that is the click having worked rather than having
	// failed: the handler this button reaches closes the window, and
	// ExecuteScript's completion never arrives from a WebView2 that is
	// being torn down. Measured — it passes alone and reports "ui:
	// window is closed" alongside the rest of the package, which is a
	// race between two correct outcomes, not a defect. What the click
	// was for is asserted below, on the channel.
	if _, err := pw.win.Eval("document.getElementById('close-btn').click()"); err != nil &&
		!errors.Is(err, ui.ErrWindowClosed) {
		t.Fatalf("Eval(click close-btn): %v", err)
	}
	select {
	case <-win.Denied():
	case <-time.After(5 * time.Second):
		t.Fatal("dismissing the window never reached Go")
	}
}

// A pairing prompt is a prompt for one application, and the window it
// opens carries that application's own name and code — not the ones of
// whatever asked before it.
func TestShowPairingPostsThePromptItWasGiven(t *testing.T) {
	prompt := testPrompt()
	prompt.Name = "A Different ERP"
	prompt.Code = "900001"

	win, err := newPairingUI(config.Default()).ShowPairing(prompt)
	if err != nil {
		t.Fatalf("ShowPairing: %v", err)
	}
	defer win.Close()

	pw := win.(*pairingWindow)
	if got := evalString(t, pw.win, "document.getElementById('app-name').textContent"); got != prompt.Name {
		t.Fatalf("the window shows %q, want %q", got, prompt.Name)
	}
	if got := evalString(t, pw.win, "document.getElementById('pairing-code').textContent"); got != prompt.Code {
		t.Fatalf("the window shows the code %q, want %q", got, prompt.Code)
	}
}
