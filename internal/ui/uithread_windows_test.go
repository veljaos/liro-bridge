//go:build windows

package ui

// J-6's property, stated as something observable: one WebView2
// environment per process, on one thread, whatever else happens.
//
// The measurement that motivated the change is in docs/decisions.md;
// this is what stops the shape drifting back. A test that opened one
// window could not see any of it — a per-window cost is invisible to a
// fixture there is only one of (D-172) — so every one of these opens at
// least two.

import (
	"testing"
	"time"
)

// twoWindows opens two real windows and hands them back with a cleanup
// that closes both.
func twoWindows(t *testing.T) (*window, *window) {
	t.Helper()
	open := func(title string) *window {
		t.Helper()
		w, err := NewWindow(Options{Title: title, Width: 300, Height: 200})
		if err != nil {
			t.Fatalf("NewWindow(%s): %v", title, err)
		}
		t.Cleanup(func() { _ = w.Close() })
		win, ok := w.(*window)
		if !ok {
			t.Fatalf("NewWindow returned a %T", w)
		}
		return win
	}
	return open("liro-bridge ui thread test 1"), open("liro-bridge ui thread test 2")
}

// The environment is the process's, not the window's: two windows share
// one, and it is the one the UI thread holds.
func TestEveryWindowSharesOneWebView2Environment(t *testing.T) {
	a, b := twoWindows(t)

	tt, err := ensureUIThread()
	if err != nil {
		t.Fatalf("ensureUIThread: %v", err)
	}
	if tt.env == 0 {
		t.Fatal("two windows are open and the UI thread holds no environment")
	}
	// And no window holds one of its own to release: the field is gone,
	// which is what makes "one per process" true by construction rather
	// than by agreement. What is left to check is that both windows
	// really came from the one the thread holds — which the thread
	// identity below proves, since an environment may only be used from
	// the apartment that created it.
	if a.threadID == 0 || a.threadID != tt.threadID {
		t.Fatalf("window a was created on thread %d, the UI thread is %d", a.threadID, tt.threadID)
	}
	if b.threadID != a.threadID {
		t.Fatalf("two windows on two threads: %d and %d", a.threadID, b.threadID)
	}
}

// Closing a window does not take the environment with it. This is the
// half that would break silently: releasing it in closeWebView would
// leave every window still open holding a dead environment, and the
// next window would be created from one.
func TestClosingAWindowLeavesTheEnvironmentForTheNext(t *testing.T) {
	tt, err := ensureUIThread()
	if err != nil {
		t.Fatalf("ensureUIThread: %v", err)
	}

	first, err := NewWindow(Options{Title: "liro-bridge env test 1", Width: 300, Height: 200})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	env := tt.env
	if env == 0 {
		t.Fatal("a window is open and the UI thread holds no environment")
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if tt.env != env {
		t.Fatalf("closing a window changed the environment from %#x to %#x", env, tt.env)
	}

	second, err := NewWindow(Options{Title: "liro-bridge env test 2", Width: 300, Height: 200})
	if err != nil {
		t.Fatalf("NewWindow after a close: %v", err)
	}
	defer func() { _ = second.Close() }()
	if tt.env != env {
		t.Fatalf("the second window built a second environment (%#x, was %#x)", tt.env, env)
	}
}

// A window opened while another is open is created on the same thread
// and works — which is the case the shared thread could have broken,
// since creating the second one runs inside a message loop the first
// one's window is also being served by.
func TestASecondWindowOpensWhileTheFirstIsUp(t *testing.T) {
	a, b := twoWindows(t)

	for _, w := range []*window{a, b} {
		if w.Handle() == 0 {
			t.Fatal("a window has no HWND")
		}
		// Both are alive and answering on the shared thread: a posted
		// closure that never ran would be a thread serving one window
		// and not the other.
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = w.Resize(320, 220)
		}()
		select {
		case <-done:
		case <-time.After(newWindowTimeout):
			t.Fatal("a window on the shared thread stopped answering")
		}
	}
}

// Closing one window does not end the thread the others live on. Before
// this change every window's WM_DESTROY posted WM_QUIT to end its own
// loop; on a shared thread that would end everyone's.
func TestClosingOneWindowLeavesTheOthersWorking(t *testing.T) {
	a, b := twoWindows(t)

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- b.Resize(340, 240) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the surviving window answers with %v", err)
		}
	case <-time.After(newWindowTimeout):
		t.Fatal("closing one window stopped the thread the other lives on")
	}

	// And a window opened afterwards still works, so the loop is still
	// running rather than merely not yet noticed to have stopped.
	third, err := NewWindow(Options{Title: "liro-bridge ui thread test 3", Width: 300, Height: 200})
	if err != nil {
		t.Fatalf("NewWindow after a close: %v", err)
	}
	_ = third.Close()
}
