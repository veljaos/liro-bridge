//go:build darwin && cgo

package ui

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// F13 §4's window, measured on CI's macOS runner: a real WKWebView loads
// a real page from the embedded assets through the liro:// scheme, a
// payload reaches the page, a click on the page reaches Go as a message,
// a navigation off the assets is refused, and the window is photographed
// for a person to look at (LIRO_SCREENSHOT_DIR, uploaded by the job).
//
// What it cannot show is the thing that matters most: a person clicking
// Approve. It drives the page's own bridge (liroSend) the way the Linux
// and Windows tests do, which D-094 allows for proving the path and
// forbids for proving consent.
func TestAMacWindowLoadsAPageAndCarriesMessagesBothWays(t *testing.T) {
	assets, err := fs.Sub(Assets, "assets")
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan Message, 4)
	w, err := NewWindow(Options{
		Title:       "Liro Bridge (CI)",
		Width:       420,
		Height:      406,
		Assets:      assets,
		VirtualHost: "liro.invalid",
		StartPage:   "/pages/origin.html",
		OnMessage:   func(m Message) { got <- m },
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer func() { _ = w.Close() }()

	if err := w.PostJSON(map[string]any{
		"type": "init",
		"strings": map[string]string{
			"origin.server_label":  "Server",
			"origin.explain":       "A page on this server asked to send you documents to sign.",
			"origin.every_request": "Every request still opens the signing screen.",
			"origin.allow":         "Allow",
			"origin.deny":          "Do not allow",
		},
		"model": map[string]any{"origin": "https://erp.example.com"},
	}); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	raw, err := w.Eval("document.getElementById('origin').textContent")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	var text string
	if err := json.Unmarshal([]byte(raw), &text); err != nil || text != "https://erp.example.com" {
		t.Fatalf("the payload did not land: Eval gave %s (%v)", raw, err)
	}

	screenshot(t, w)

	if _, err := w.Eval("window.liroSend('cancel'), true"); err != nil {
		t.Fatalf("Eval(liroSend): %v", err)
	}
	select {
	case m := <-got:
		if m.Type != MessageTypeCancel {
			t.Fatalf("the page sent %q, want cancel", m.Type)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no message reached Go from the page within 10 s")
	}

	// Off the assets: refused, and the page stays.
	if _, err := w.Eval("location.href = 'https://example.invalid/'; true"); err != nil {
		t.Fatalf("Eval(location): %v", err)
	}
	time.Sleep(time.Second)
	if raw, _ := w.Eval("location.protocol"); raw != `"liro:"` {
		t.Fatalf("after a navigation off the assets the page is at %s, want liro:", raw)
	}

	if err := w.Resize(420, 300); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if err := w.Navigate("/pages/pairing.html"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
}

// screenshot photographs the window with screencapture when
// LIRO_SCREENSHOT_DIR names a directory: the one way a person can see a
// window this machine drew without a Mac of their own.
func screenshot(t *testing.T, w Window) {
	dir := os.Getenv("LIRO_SCREENSHOT_DIR")
	if dir == "" {
		return
	}
	dw, ok := w.(*darwinWindow)
	if !ok {
		return
	}
	var num int
	if err := theUIThread.do(func() { num = nativeWindowNumber(dw.id) }); err != nil {
		t.Logf("screenshot: %v", err)
		return
	}
	time.Sleep(500 * time.Millisecond) // let the page paint
	out := filepath.Join(dir, "origin-window.png")
	if b, err := exec.Command("screencapture", "-x", "-o", "-l"+strconv.Itoa(num), out).CombinedOutput(); err != nil {
		t.Logf("screencapture: %v %s", err, b)
		return
	}
	t.Logf("photographed the window to %s", out)
}
