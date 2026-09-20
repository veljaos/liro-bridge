//go:build linux

package ui

// The Window contract, on a real GTK4 window hosting a real WebKitGTK
// web process. What F12 §3's "every window this program has" is checked
// against, as far as it can be without hardware and without synthetic
// input (D-094).

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const testPage = `<!doctype html><title>t</title><body><p id="p">start</p>
<script>
  window.__received = null;
  // An object, not a string to parse — see postjson.go.
  window.__liroReceive = function (payload) { window.__received = payload; };
  window.__send = function (type) {
    window.webkit.messageHandlers.liro.postMessage({type: type});
  };
</script>`

const secondPage = `<!doctype html><title>t2</title><body><p id="p">second</p>
<script>window.__liroReceive = function (payload) { window.__received = payload; };</script>`

func testAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":  {Data: []byte(testPage)},
		"second.html": {Data: []byte(secondPage)},
	}
}

// newTestWindow opens a window on the test assets, skipping when this
// machine cannot start a web process at all (D-324) or has no display.
func newTestWindow(t *testing.T, opts Options) Window {
	t.Helper()
	requireWebKitCanStart(t)

	if opts.Assets == nil {
		opts.Assets = testAssets()
	}
	if opts.VirtualHost == "" {
		opts.VirtualHost = "liro.invalid"
	}
	if opts.StartPage == "" {
		opts.StartPage = "/index.html"
	}
	if opts.Width == 0 {
		opts.Width, opts.Height = 400, 300
	}

	w, err := NewWindow(opts)
	if err != nil {
		if errors.Is(err, ErrNoDisplay) {
			t.Skip("no windowing system: ", err)
		}
		t.Fatalf("NewWindow: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// NewWindow blocks until the start page's scripts have run, so the
// caller's first PostJSON lands in a page that is ready for it. If that
// were not so, this would be flaky rather than failing.
func TestNewWindowBlocksUntilTheStartPageIsReady(t *testing.T) {
	w := newTestWindow(t, Options{})

	got, err := w.Eval("document.getElementById('p').textContent")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != `"start"` {
		t.Errorf("page content = %s, want \"start\"", got)
	}
	if w.Handle() == 0 {
		t.Error("Handle() is 0 after a successful NewWindow")
	}
}

func TestPostJSONArrivesAsAnObjectNotAString(t *testing.T) {
	w := newTestWindow(t, Options{})

	type payload struct {
		Kind  string `json:"kind"`
		Count int    `json:"count"`
		Text  string `json:"text"`
	}
	// The text carries quotes and a backslash on purpose: PostJSON must
	// not be building JavaScript by concatenation (F5 §2.4).
	want := payload{Kind: "certificates", Count: 2, Text: `a "quoted" \ backslash`}

	if err := w.PostJSON(want); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	got, err := w.Eval("window.__received.kind")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != `"certificates"` {
		t.Errorf("kind = %s, want \"certificates\"", got)
	}
	if got, err = w.Eval("window.__received.count"); err != nil || got != "2" {
		t.Errorf("count = %s (err %v), want 2", got, err)
	}
	if got, err = w.Eval("window.__received.text"); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if !strings.Contains(got, `quoted`) || !strings.Contains(got, `\\`) {
		t.Errorf("text came back as %s — quotes or backslash did not survive", got)
	}
}

// D-083's surface, in the direction the page drives.
func TestAMessageFromThePageReachesOnMessage(t *testing.T) {
	got := make(chan Message, 4)
	w := newTestWindow(t, Options{
		OnMessage: func(m Message) { got <- m },
	})

	if _, err := w.Eval("window.__send('approve')"); err != nil {
		t.Fatalf("Eval: %v", err)
	}

	select {
	case m := <-got:
		if m.Type != MessageTypeApprove {
			t.Errorf("message type = %q, want approve", m.Type)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no message reached OnMessage")
	}
}

// Anything that is not one of the three is dropped before OnMessage,
// not passed through for the caller to filter.
func TestAnUnknownMessageIsDroppedBeforeOnMessage(t *testing.T) {
	got := make(chan Message, 4)
	w := newTestWindow(t, Options{
		OnMessage: func(m Message) { got <- m },
	})

	// "; void 0" is not decoration: postMessage returns a host object
	// and WebKit refuses to marshal one back, so the script must end in
	// something JSON can encode. See Eval's doc comment.
	if _, err := w.Eval(
		`window.webkit.messageHandlers.liro.postMessage({type: "deleteEverything"}); void 0`,
	); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	// Then a real one, so we are waiting for something rather than for
	// a duration: if the bad one were passed through it would arrive
	// first, because delivery is ordered.
	if _, err := w.Eval("window.__send('cancel')"); err != nil {
		t.Fatalf("Eval: %v", err)
	}

	select {
	case m := <-got:
		if m.Type != MessageTypeCancel {
			t.Errorf("first delivered message was %q; the unknown one was not dropped", m.Type)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no message reached OnMessage")
	}
}

// Navigate keeps the window and changes only the content, and blocks
// until the new page's scripts have run.
func TestNavigateSwapsThePageAndWaitsForIt(t *testing.T) {
	w := newTestWindow(t, Options{})
	before := w.Handle()

	if err := w.Navigate("/second.html"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	got, err := w.Eval("document.getElementById('p').textContent")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != `"second"` {
		t.Errorf("after Navigate the page is %s, want \"second\"", got)
	}
	if w.Handle() != before {
		t.Error("Navigate replaced the native window; it is supposed to keep it")
	}
}

// D-259: a window does not become any document but one of its own
// pages. Checked by asking the page to go somewhere and observing that
// it did not.
func TestTheWindowRefusesToLeaveItsOwnPages(t *testing.T) {
	w := newTestWindow(t, Options{})

	if _, err := w.Eval(`window.location.href = "https://example.invalid/"`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	// Give the navigation every chance to happen before concluding it
	// did not.
	time.Sleep(2 * time.Second)

	got, err := w.Eval("document.getElementById('p').textContent")
	if err != nil {
		t.Fatalf("Eval after the refused navigation: %v", err)
	}
	if got != `"start"` {
		t.Errorf("the window left its own page: content is now %s", got)
	}
}

func TestCloseIsIdempotentAndLaterCallsSaySo(t *testing.T) {
	w := newTestWindow(t, Options{})

	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("second Close: %v, want nil (idempotent)", err)
	}
	if _, err := w.Eval("1"); !errors.Is(err, ErrWindowClosed) {
		t.Errorf("Eval after Close returned %v, want ErrWindowClosed", err)
	}
	if err := w.Navigate("/second.html"); !errors.Is(err, ErrWindowClosed) {
		t.Errorf("Navigate after Close returned %v, want ErrWindowClosed", err)
	}
}

// Options.OnFilesDropped fails loudly rather than silently doing
// nothing on this platform.
func TestDropIsRefusedRatherThanIgnored(t *testing.T) {
	requireWebKitCanStart(t)

	_, err := NewWindow(Options{
		Assets:         testAssets(),
		VirtualHost:    "liro.invalid",
		StartPage:      "/index.html",
		Width:          200,
		Height:         200,
		OnFilesDropped: func([]string) {},
	})
	if !errors.Is(err, ErrDropNotImplemented) {
		t.Errorf("NewWindow with OnFilesDropped returned %v, want ErrDropNotImplemented", err)
	}
}

// The real bridge.js, not a page written from the same description the
// host was written from.
//
// This test exists because of what it caught. The Linux PostJSON was
// built from window.go's doc comment, which said the page received a
// JSON string and parsed it; the implementation on the other platform
// inlines an object literal, and bridge.js returns early on anything
// that is not an object. Every payload would have been dropped in
// silence. The test that was supposed to cover PostJSON did not, because
// its page was written from the same wrong sentence — so the only
// version of this test worth having is one that serves the file this
// program actually ships.
func TestPostJSONReachesTheRealBridgeJS(t *testing.T) {
	requireWebKitCanStart(t)

	bridge, err := Assets.ReadFile("assets/bridge.js")
	if err != nil {
		t.Fatalf("reading the shipped bridge.js: %v", err)
	}

	page := `<!doctype html><title>real</title><body>
<script src="/bridge.js"></script>
<script>
  window.__liroSeen = null;
  window.__liroOnMessage = function (payload) { window.__liroSeen = payload; };
</script>`

	w := newTestWindow(t, Options{
		Assets: fstest.MapFS{
			"index.html": {Data: []byte(page)},
			"bridge.js":  {Data: bridge},
		},
	})

	// bridge.js only forwards to __liroOnMessage, and only for an
	// object — so if PostJSON sends the wrong shape this stays null.
	if err := w.PostJSON(map[string]any{
		"kind":    "certificates",
		"strings": map[string]string{"hello": "zdravo"},
	}); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	got, err := w.Eval("window.__liroSeen && window.__liroSeen.kind")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != `"certificates"` {
		t.Fatalf("bridge.js did not receive the payload: __liroSeen.kind = %s", got)
	}

	// bridge.js also keeps the localised strings it was sent, which is
	// the half of the payload every window depends on.
	if got, err = w.Eval(`window.liroT("hello")`); err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != `"zdravo"` {
		t.Errorf("liroT(\"hello\") = %s, want \"zdravo\" — the strings did not land", got)
	}
}

// And the other direction, through the same shipped file: a click that
// calls liroSend must reach OnMessage on this platform too. bridge.js
// resolves the native host at load — WebView2's or WebKitGTK's — and
// this is the only place that resolution is exercised against a real
// one.
func TestLiroSendFromTheRealBridgeJSReachesOnMessage(t *testing.T) {
	requireWebKitCanStart(t)

	bridge, err := Assets.ReadFile("assets/bridge.js")
	if err != nil {
		t.Fatalf("reading the shipped bridge.js: %v", err)
	}

	got := make(chan Message, 4)
	w := newTestWindow(t, Options{
		Assets: fstest.MapFS{
			"index.html": {Data: []byte(
				`<!doctype html><title>real</title><body><script src="/bridge.js"></script>`)},
			"bridge.js": {Data: bridge},
		},
		OnMessage: func(m Message) { got <- m },
	})

	// liroAct is what a real button calls: it records what the click
	// meant and then sends "approve" through liroSend.
	if _, err := w.Eval(`window.liroAct("sign", {}); void 0`); err != nil {
		t.Fatalf("Eval: %v", err)
	}

	select {
	case m := <-got:
		if m.Type != MessageTypeApprove {
			t.Errorf("message type = %q, want approve", m.Type)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("liroSend did not reach OnMessage through the shipped bridge.js")
	}

	// The action the click meant is read back through Eval, which is
	// the channel __liroAction exists for.
	action, err := w.Eval(`window.__liroAction()`)
	if err != nil {
		t.Fatalf("Eval __liroAction: %v", err)
	}
	if !strings.Contains(action, "sign") {
		t.Errorf("__liroAction() = %s, want it to name the action", action)
	}
}

// A page opened outside this program has no native host, and a click
// must fail visibly rather than throw on every press or do nothing.
func TestTheRealBridgeJSFailsLoudlyWithNoNativeHost(t *testing.T) {
	requireWebKitCanStart(t)

	bridge, err := Assets.ReadFile("assets/bridge.js")
	if err != nil {
		t.Fatalf("reading the shipped bridge.js: %v", err)
	}

	// The handler is registered per window by the host, so to have a
	// page with no host we hide it from the page before bridge.js runs.
	page := `<!doctype html><title>hostless</title><body>
<script>window.webkit = undefined; window.chrome = undefined;</script>
<script src="/bridge.js"></script>
<script>
  window.__err = null;
  try { window.liroSend("approve"); } catch (e) { window.__err = String(e.message || e); }
</script>`

	w := newTestWindow(t, Options{
		Assets: fstest.MapFS{
			"index.html": {Data: []byte(page)},
			"bridge.js":  {Data: bridge},
		},
	})

	got, err := w.Eval("window.__err")
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if !strings.Contains(got, "no native message host") {
		t.Errorf("window.__err = %s, want it to name the missing host", got)
	}
}
