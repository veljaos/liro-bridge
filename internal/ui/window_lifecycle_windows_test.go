//go:build windows

package ui

// The two concurrency defects D-101 fixed, each with the test that
// would have caught it.
//
// Both open real WebView2 windows, which is why they live here rather
// than in cmd/liro-bridge with the rendering tests: what they exercise
// is this package's own COM lifetime and teardown, not any page the
// agent ships.
import (
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
	"unsafe"
)

// iterations reads an override from the environment so a run can be
// made longer than the committed default without editing the test —
// the three-hundred-window measurement in D-101 was taken this way.
func iterations(t *testing.T, def int) int {
	t.Helper()
	if s := os.Getenv("LIRO_UI_ITERATIONS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			t.Fatalf("LIRO_UI_ITERATIONS=%q: want a positive integer", s)
		}
		return n
	}
	return def
}

// newWindowTimeout is how long one ui.NewWindow may take before this
// package calls it hung. Window creation takes well under a second on
// any machine that can run WebView2 at all; the defect D-101 fixed did
// not make it slow, it made it never return, so anything on this scale
// separates the two cleanly.
const newWindowTimeout = 20 * time.Second

// TestNewWindowAlwaysCompletes is the regression test for the first
// half of D-101: ui.NewWindow parking forever in
// environmentCreateController, waiting for a WebView2 completion
// handler that could never fire because the handler object was
// stack-allocated and the goroutine's stack had moved out from under
// it. Measured at roughly one call in twenty-five before the fix.
//
// The windows here deliberately have no page: an Options with nothing
// but a size still builds a WebView2 environment and a controller,
// which is the whole surface this defect lived on, and skipping the
// navigation makes one window cost tens of milliseconds rather than
// hundreds. A rate as low as one in twenty-five needs volume to mean
// anything, which is what LIRO_UI_ITERATIONS is for; the committed
// default is the largest number that leaves this package quick.
func TestNewWindowAlwaysCompletes(t *testing.T) {
	n := iterations(t, 40)
	for i := range n {
		type created struct {
			w   Window
			err error
		}
		done := make(chan created, 1)
		go func() {
			w, err := NewWindow(Options{Title: "liro-bridge lifecycle test", Width: 300, Height: 200})
			done <- created{w, err}
		}()
		select {
		case c := <-done:
			if c.err != nil {
				t.Fatalf("iteration %d: NewWindow: %v", i, c.err)
			}
			if c.w.Handle() == 0 {
				t.Fatalf("iteration %d: NewWindow returned a window with no HWND", i)
			}
			_ = c.w.Close()
		case <-time.After(newWindowTimeout):
			// Deliberately fatal rather than counted: the goroutine and
			// the OS thread behind it stay stuck for the rest of this
			// process's life, so there is nothing useful to carry on
			// measuring after the first one.
			t.Fatalf("iteration %d of %d: NewWindow did not return within %v — a WebView2 completion never fired (D-101)", i, n, newWindowTimeout)
		}
	}
}

// lifecycleAssets is the smallest page that makes a window take every
// path the agent's real windows take: it is navigated to (so the window
// subscribes to NavigationCompleted and waits for it) and it defines
// __liroReceive (so PostJSON has something to call). Nothing about the
// content matters — what matters is that the window ends up holding
// both of the event handlers whose lifetime D-101 was about.
var lifecycleAssets = fstest.MapFS{
	"test.html": &fstest.MapFile{Data: []byte(
		"<!doctype html><meta charset=\"utf-8\"><title>t</title>" +
			"<script>window.__liroReceive=function(v){window.__last=v;};</script>",
	)},
}

// TestCloseRacesUserClose is the regression test for the second half of
// D-101: the access violation inside ICoreWebView2Controller::Close
// when a window is torn down from two directions at once.
//
// Each iteration puts a Go-initiated Close — which posts WM_CLOSE from
// another goroutine and waits for the window's own thread to confirm —
// against a user-initiated one, the title bar's close button, which is
// a WM_CLOSE this package did not send. Both land in the same window
// procedure on the same thread, and the second can be dispatched into
// it by the nested message loop ICoreWebView2Controller::Close runs
// while the first is still inside it.
//
// The window is built the way the agent's real ones are — a page, a
// navigation to it, a message handler — because the pointers that crash
// are the two event handler objects the controller releases as it
// closes, and a window with no page registers neither.
//
// There is nothing to assert about the crash itself: an access
// violation inside the WebView2 runtime takes the whole test binary
// with it, so surviving the loop is the assertion. What is asserted is
// everything around it — that Close still returns rather than timing
// out, and that a window closed twice reports itself closed once.
func TestCloseRacesUserClose(t *testing.T) {
	n := iterations(t, 100)
	for i := range n {
		var closedCount atomic.Int32
		w, err := NewWindow(Options{
			Title:       "liro-bridge close race test",
			Width:       300,
			Height:      200,
			Assets:      lifecycleAssets,
			VirtualHost: "liro-lifecycle.test",
			StartPage:   "/test.html",
			OnMessage:   func(Message) {},
			OnClosed:    func() { closedCount.Add(1) },
		})
		if err != nil {
			t.Fatalf("iteration %d: NewWindow: %v", i, err)
		}

		// Collect before closing. The handler objects WebView2 holds the
		// bare addresses of are Go values; if this package has stopped
		// referencing one, this is what actually reclaims it, and the
		// controller's Close — which releases every handler registered
		// against it — is what then touches the reclaimed memory. Without
		// this the same bug is a matter of whether a collection happened
		// to run, which is how it stayed a once-in-a-blue-moon crash in
		// the first place.
		runtime.GC()

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		// The user closing the window: a WM_CLOSE from outside any of
		// this package's own bookkeeping.
		go func() {
			defer wg.Done()
			<-start
			postMessage(w.Handle(), wmClose, 0, 0)
		}()

		// The agent closing the window: window.go's Close, called from a
		// goroutine that is not the window's own thread.
		var closeTook time.Duration
		go func() {
			defer wg.Done()
			<-start
			began := time.Now()
			_ = w.Close()
			closeTook = time.Since(began)
		}()

		close(start)
		wg.Wait()

		if closeTook >= closeTeardownTimeout {
			t.Fatalf("iteration %d: Close took %v, hitting its %v teardown timeout — the window thread never confirmed", i, closeTook, closeTeardownTimeout)
		}
		if got := closedCount.Load(); got > 1 {
			t.Fatalf("iteration %d: OnClosed fired %d times for one window; a close must be reported at most once", i, got)
		}
		select {
		case <-w.(*window).closedCh:
		default:
			t.Fatalf("iteration %d: window did not report itself closed", i)
		}
	}
}

// TestHandlerObjectsAreUnpinnedWhenReleased checks the bookkeeping the
// two tests above depend on but cannot see: a handler object stays
// pinned — neither movable nor collectable — for exactly as long as its
// COM reference count says something outside Go may still call into it,
// and not one reference longer. Getting the second half wrong would
// turn D-101's fix into a leak that grows with every ExecuteScript.
func TestHandlerObjectsAreUnpinnedWhenReleased(t *testing.T) {
	before := handlerPinCount()

	h := newExecuteScriptCompletedHandler()
	if got := handlerPinCount(); got != before+1 {
		t.Fatalf("after creating a handler: pinned %d, want %d", got, before+1)
	}

	// WebView2 taking, then dropping, its own reference.
	addRefThunk(h.addr())
	releaseThunk(h.addr())
	if got := handlerPinCount(); got != before+1 {
		t.Fatalf("while this package still holds a reference: pinned %d, want %d", got, before+1)
	}

	h.release()
	if got := handlerPinCount(); got != before {
		t.Fatalf("after the last reference went: pinned %d, want %d", got, before)
	}
}

// TestBlockingCallbackDoesNotFreezeTheWindow is the regression test for
// the third defect of this shape, and the one the owner met: a caller's
// OnMessage that blocks used to run inside wndProc, on the window's own
// message-loop thread, so a handler waiting on a full channel stopped
// the window dead.
//
// What that cost was not subtle. Every window in this project delivers
// page messages by sending on a channel of eight, drained by a loop
// that is itself sometimes busy — waiting for another window, for a
// folder chooser, for a card. When it was, the ninth click parked the
// window's own goroutine in "chan send, locked to thread" and the
// window stopped painting, stopped answering the title bar, and could
// not be closed: Close's posted WM_CLOSE had nothing left to dispatch
// it. Measured directly on a settings window before the fix, and it
// took nine clicks.
//
// So the property under test is not "the callback is called" but "the
// window survives a callback that never returns": it still answers
// WM_NULL, Eval still completes, and Close still returns rather than
// timing out. Order is asserted too, because moving callbacks off the
// window thread would be a poor trade if it let them arrive shuffled.
func TestBlockingCallbackDoesNotFreezeTheWindow(t *testing.T) {
	const messages = 20

	release := make(chan struct{})
	var delivered []string
	var mu sync.Mutex
	first := true

	w, err := NewWindow(Options{
		Title:       "liro-bridge blocking callback test",
		Width:       300,
		Height:      200,
		Assets:      lifecycleAssets,
		VirtualHost: "liro-lifecycle.test",
		StartPage:   "/test.html",
		OnMessage: func(m Message) {
			// The first callback blocks until the test lets go — the
			// stand-in for a caller busy with another window. Every
			// later one is queued behind it.
			mu.Lock()
			isFirst := first
			first = false
			mu.Unlock()
			if isFirst {
				<-release
			}
			mu.Lock()
			delivered = append(delivered, string(m.Type)+":"+m.Thumbprint)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	for i := range messages {
		script := "window.chrome.webview.postMessage({type:'selectCertificate',thumbprint:'" + strconv.Itoa(i) + "'})"
		if _, err := w.Eval(script); err != nil {
			t.Fatalf("message %d: Eval failed, so the window had already stopped answering: %v", i, err)
		}
	}

	// The window is still its own master: a synchronous call into it
	// returns, which is exactly what a frozen message loop cannot do.
	if !windowIsResponding(w.Handle()) {
		t.Fatal("the window stopped responding while a callback was blocked")
	}
	if _, err := w.Eval("1+1"); err != nil {
		t.Fatalf("Eval failed while a callback was blocked: %v", err)
	}

	began := time.Now()
	_ = w.Close()
	if took := time.Since(began); took >= closeTeardownTimeout {
		t.Fatalf("Close took %v, hitting its %v timeout: the window thread never confirmed", took, closeTeardownTimeout)
	}

	close(release)
	// Everything the page sent still arrives, in the order it was sent.
	deadline := time.Now().Add(10 * time.Second)
	for {
		mu.Lock()
		n := len(delivered)
		mu.Unlock()
		if n >= messages || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(delivered) != messages {
		t.Fatalf("delivered %d of %d messages", len(delivered), messages)
	}
	for i, got := range delivered {
		if want := "selectCertificate:" + strconv.Itoa(i); got != want {
			t.Fatalf("message %d was delivered as %q, want %q — callbacks must arrive in order", i, got, want)
		}
	}
}

var procSendMessageTimeoutW = user32DLL.NewProc("SendMessageTimeoutW")

// windowIsResponding asks the window a question its own message loop
// must answer. SendMessageTimeoutW with SMTO_ABORTIFHUNG returns zero
// for a window whose thread is not pumping, which is precisely the
// state this file's tests exist to rule out.
func windowIsResponding(hwnd uintptr) bool {
	var result uintptr
	const smtoAbortIfHung, smtoBlock = 0x0002, 0x0001
	r, _, _ := procSendMessageTimeoutW.Call(hwnd, 0 /* WM_NULL */, 0, 0,
		smtoAbortIfHung|smtoBlock, 1000, uintptr(unsafe.Pointer(&result)))
	return r != 0
}
