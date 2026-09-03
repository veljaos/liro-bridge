//go:build windows

package main

// TestConsentWindowRendersAndRoundTrips is the consent-window half of
// the F5 first-real-run findings: it proves, against a real WebView2
// window, both that the certificate list actually renders (Task 1's
// class of bug: text posted via PostJSON reaching the DOM) and that a
// click in the real page actually reaches Go as a Message (the
// direction Task 2's hang — internal/keysource/windowscng's presence
// probe blocking on OS UI it should never trigger — hid completely: a
// window that never appears also never proves its own message channel
// works). internal/consent's own tests (viewmodel_test.go etc.) prove
// the Go-side data is right; this proves it survives the round trip
// through the actual browser engine, which is the boundary the F5
// report's untested claims turned out to be wrong about.
import (
	"encoding/json"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

func TestConsentWindowRendersAndRoundTrips(t *testing.T) {
	c := i18n.Load("sr-Latn")

	usable := classify.Info{
		Thumbprint:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Subject:       classify.Subject{DisplayName: "Test Testić"},
		IssuerCN:      "Test CA",
		Qualification: classify.QualificationQualified,
		Purpose:       classify.PurposeSigning,
		Usable:        true,
	}
	digests := [][]byte{{1, 2, 3}}
	vm := consent.BuildViewModel(consent.ApplicationLocal, digests, []string{"document.pdf"}, []classify.Info{usable})

	messages := make(chan ui.Message, 8)
	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("consent.window_title"),
		Width:       480,
		Height:      420,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/consent.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(buildConsentInit(c, vm)); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	// Task 1's class of check, on the consent page: the row's rendered
	// text actually reached the DOM, not just the Go-side ViewModel.
	gotName := evalString(t, win, "document.querySelector('.liro-cert-row div')?.textContent")
	if gotName == "" {
		t.Fatal("certificate row rendered no text at all — the init payload never reached the page")
	}
	if gotName != usable.Subject.DisplayName {
		t.Errorf("certificate row rendered %q, want %q", gotName, usable.Subject.DisplayName)
	}

	// Click the usable row exactly as a user would, and confirm Go
	// actually receives selectCertificate with the right thumbprint —
	// the page->Go direction of the same bridge Task 1's fix repairs
	// Go->page for.
	if _, err := win.Eval("document.querySelector('.liro-cert-row:not(.liro-cert-row-disabled)').click()"); err != nil {
		t.Fatalf("Eval(click cert row): %v", err)
	}
	msg := recvMessage(t, messages, 5*time.Second)
	if msg.Type != ui.MessageTypeSelectCertificate || msg.Thumbprint != usable.Thumbprint {
		t.Fatalf("after clicking the certificate row, got %+v, want selectCertificate/%s", msg, usable.Thumbprint)
	}

	if disabled := evalString(t, win, "String(document.getElementById('approve-btn').disabled)"); disabled != "false" {
		t.Fatalf("approve-btn.disabled = %s after selecting a usable certificate, want false", disabled)
	}

	if _, err := win.Eval("document.getElementById('approve-btn').click()"); err != nil {
		t.Fatalf("Eval(click approve-btn): %v", err)
	}
	msg = recvMessage(t, messages, 5*time.Second)
	if msg.Type != ui.MessageTypeApprove {
		t.Fatalf("after clicking Approve, got %+v, want approve", msg)
	}
}

// evalString runs script via Window.Eval and decodes its JSON-encoded
// result into a Go string ("" for null/undefined).
func evalString(t *testing.T, win ui.Window, script string) string {
	t.Helper()
	raw, err := win.Eval(script)
	if err != nil {
		t.Fatalf("Eval(%s): %v", script, err)
	}
	var s *string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("Eval(%s): decoding result %q: %v", script, raw, err)
	}
	if s == nil {
		return ""
	}
	return *s
}

// recvMessage waits up to timeout for a message on ch, failing the test
// if none arrives — a real WebView2 round trip is asynchronous, so this
// replaces a synchronous assertion, not a synchronous one.
func recvMessage(t *testing.T, ch chan ui.Message, timeout time.Duration) ui.Message {
	t.Helper()
	select {
	case m := <-ch:
		return m
	case <-time.After(timeout):
		t.Fatalf("no message received within %s", timeout)
		return ui.Message{}
	}
}
