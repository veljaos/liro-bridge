//go:build linux

package ui

// End to end, on a real WebKitGTK web process: the hand-written
// evaluate_javascript bridge D-330 exists for. This is the test that
// says the remedy works, as opposed to compiles.

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// requireWebKitCanStart skips unless WebKitGTK could actually bring up a
// web process on this machine.
//
// **This is a precondition and not a workaround.** D-324 measured that
// WebKitGTK 6.0 always sandboxes its web process, that the sandbox is
// bubblewrap, and that on Ubuntu 23.10 and later AppArmor refuses
// bwrap the user namespace it needs unless a profile names the binary.
// No profile on this machine names a Go binary, so a Go test that
// creates a WebView dies the way the program would:
//
//	bwrap: setting up uid map: Permission denied
//	Failed to fully launch dbus-proxy: Child process exited with code 1
//	SIGTRAP: trace trap
//
// That kills the process, so it cannot be recovered from after the
// fact — the check has to come first. It runs the same operation bwrap
// runs, rather than inspecting AppArmor's settings and reasoning about
// what they imply, which is D-201's rule applied to a precondition.
//
// The one thing it deliberately does **not** do is set
// WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS itself. A test suite that
// quietly turns off the sandbox to stay green would hide the very
// failure D-324 exists to record, on every machine, forever. Setting
// it is a person's decision and is respected when already made.
func requireWebKitCanStart(t *testing.T) {
	t.Helper()

	if v, ok := os.LookupEnv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS"); ok && v != "0" {
		// Somebody turned the sandbox off on purpose. Nothing measured
		// under this says anything about §3.2 (D-327 ran its probe the
		// same way and said so).
		return
	}

	out, err := exec.Command("bwrap", "--dev-bind", "/", "/", "true").CombinedOutput()
	if err != nil {
		t.Skipf("WebKitGTK cannot start here: bubblewrap has no user namespace (D-324): %v: %s\n"+
			"Install the AppArmor profile §8 ships, or set "+
			"WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 deliberately.",
			err, strings.TrimSpace(string(out)))
	}
}

// loadedWebView brings up one off-screen WebKitGTK view with content in
// it, and skips the test rather than failing it when the machine has no
// windowing system — a headless CI runner is not a defect, and
// ErrNoDisplay exists precisely so that it can be told apart from one.
func loadedWebView(t *testing.T, html string) *webkit.WebView {
	t.Helper()

	requireWebKitCanStart(t)

	if err := theUIThread.start(); err != nil {
		if errors.Is(err, ErrNoDisplay) {
			t.Skip("no windowing system on this machine: ", err)
		}
		t.Fatalf("starting the UI thread: %v", err)
	}

	var view *webkit.WebView
	loaded := make(chan struct{})
	var once sync.Once

	if err := theUIThread.do(func() {
		view = webkit.NewWebView()

		// A window is created and never presented. The view needs a
		// toplevel to belong to before WebKit will spin up a web
		// process for it; it does not need to be on screen, and a test
		// that put a window in front of whoever is running it would be
		// its own kind of wrong.
		win := gtk.NewWindow()
		win.SetChild(view)

		view.ConnectLoadChanged(func(e webkit.LoadEvent) {
			if e == webkit.LoadFinished {
				once.Do(func() { close(loaded) })
			}
		})
		view.LoadHtml(html, "liro://test/")
	}); err != nil {
		t.Fatalf("creating the view: %v", err)
	}

	select {
	case <-loaded:
	case <-time.After(30 * time.Second):
		t.Fatal("the page never finished loading")
	}
	return view
}

func TestEvaluateJavascriptReturnsTheEnginesOwnJSON(t *testing.T) {
	view := loadedWebView(t, "<!doctype html><title>t</title><body>ok</body>")

	for _, c := range []struct {
		name, script, want string
	}{
		{"number", "1 + 1", "2"},
		{"string", "document.body.textContent.trim()", `"ok"`},
		{"boolean", "typeof window === 'object'", "true"},
		// The case Window.Eval's contract is written around: a value
		// that is not encodable comes back as the JSON "null", which is
		// what WebView2 reports for the same thing.
		{"undefined", "void 0", "null"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := evalInWebView(view, c.script)
			if err != nil {
				t.Fatalf("evaluating %q: %v", c.script, err)
			}
			if got != c.want {
				t.Errorf("%q = %s, want %s", c.script, got, c.want)
			}
		})
	}
}

// An object round-trips as an object rather than as a description of
// one — this is what PostJSON's opposite direction rests on.
func TestEvaluateJavascriptEncodesAnObject(t *testing.T) {
	view := loadedWebView(t, "<!doctype html><body>")

	got, err := evalInWebView(view, "({a: 1, b: [2, 3]})")
	if err != nil {
		t.Fatalf("evaluating: %v", err)
	}
	for _, want := range []string{`"a":1`, `"b":[2,3]`} {
		if !strings.Contains(got, want) {
			t.Errorf("got %s, want it to contain %s", got, want)
		}
	}
}

// A script that throws is an error and not a value. Getting this wrong
// in the other direction — reporting the exception as the JSON string
// "null" — would make a broken page look like an empty one.
func TestEvaluateJavascriptReportsAThrownException(t *testing.T) {
	view := loadedWebView(t, "<!doctype html><body>")

	got, err := evalInWebView(view, `throw new Error("boom")`)
	if err == nil {
		t.Fatalf("expected an error, got %s", got)
	}
	if !errors.Is(err, ErrEvalFailed) {
		t.Errorf("error %v does not wrap ErrEvalFailed", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %v does not carry the page's own message", err)
	}
}

// The registry must not leak an entry per call: a long-lived window
// evaluating on every step of a flow would otherwise grow without
// bound. Checked by arithmetic on the map rather than by timing.
func TestEvaluateJavascriptLeavesNothingInTheRegistry(t *testing.T) {
	view := loadedWebView(t, "<!doctype html><body>")

	for i := 0; i < 20; i++ {
		if _, err := evalInWebView(view, "1"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}

	evalRegistry.Lock()
	n := len(evalRegistry.m)
	evalRegistry.Unlock()
	if n != 0 {
		t.Errorf("registry holds %d entries after 20 completed calls, want 0", n)
	}
}
