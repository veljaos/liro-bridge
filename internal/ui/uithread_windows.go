//go:build windows

package ui

// One UI thread, one COM apartment, one WebView2 environment, for the
// whole process (J-6).
//
// Every WebView2 window used to bring its own: a goroutine locked to a
// fresh OS thread, its own STA apartment, its own message loop, and —
// the expensive part — its own call to
// CreateCoreWebView2EnvironmentWithOptions. An environment is what
// starts a browser process group, so a window cost one; and the handles
// that process group opened in this process were never given back when
// the window closed, because releasing ICoreWebView2Environment does
// not close them (D-170). Three things were recorded against that shape
// and all three are this one:
//
//   - C-3, roughly one kernel handle leaked per window, two thirds of
//     them handles to msedgewebview2.exe processes that had already
//     exited.
//   - C-5, about one window creation in a few hundred failing outright.
//   - J-6 itself, a window costing what it costs because it builds an
//     environment before it can build anything else.
//
// Microsoft's own samples create one environment and use it for every
// WebView2 in the application, and that is what this is. The
// consequence that made it a lifetime change rather than a small one is
// the reason it was deferred three times: an environment belongs to the
// apartment that created it, and CreateCoreWebView2Controller must be
// called on that thread — so sharing the environment means sharing the
// thread, and this package's model was a thread per window.
//
// So the thread is shared. Everything else about the model is
// unchanged, and deliberately so:
//
//   - Exactly one thread touches a controller, and it is the thread
//     that created it (D-101's ownership rule). That is now the same
//     thread for every window, which satisfies the rule more simply
//     than before rather than less.
//   - PostJSON, Eval, Navigate, Resize and Close still marshal onto it
//     by posting to their own window (window_windows.go's invoke), so
//     wndProc's per-window teardown guard still decides whether a
//     queued closure may run.
//   - Nothing a caller supplied ever runs on it: OnMessage,
//     OnFilesDropped and OnClosed are still delivered by one goroutine
//     per window (dispatchEvents).
//
// What is genuinely new is coupling: one wedged window creation now
// stops every other window from being created, where before it stopped
// only its own. That is accepted on measurement rather than on faith —
// the hang D-099 recorded was fixed by D-101 and did not reappear in
// 600 consecutive creations before this change or after it — and it is
// the price of the environment being shared at all.

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// hwndMessage is HWND_MESSAGE, the parent that makes a window
// message-only: it is never shown, never enumerated, and exists only to
// receive messages.
const hwndMessage = ^uintptr(2) // (HWND)(-3), sign-extended

// uiThreadClassName is the class of the work window below. It is its
// own class rather than the window class every real window shares,
// because it needs its own procedure and because a window with no
// *window behind it is exactly what that procedure must not see.
var uiThreadClassName = windows.StringToUTF16Ptr("LiroBridgeUIThread")

var registerUIThreadClassOnce sync.Once

// uiThread is the one OS thread every WebView2 window in this process
// lives on.
type uiThread struct {
	// threadID is this thread's own id, for the "am I already on it"
	// question do asks.
	threadID uintptr

	// hwnd is a message-only window whose only job is to receive
	// wmRunFunc and drain queue.
	//
	// A message-only window rather than PostThreadMessage, and the
	// difference matters: a thread message has no window to be
	// dispatched to, so any nested modal loop that retrieves it —
	// ICoreWebView2Controller::Close runs one — discards it, and the
	// closure it was announcing would never run. A window message is
	// routed to this window's procedure by whichever loop retrieves it,
	// nested or not.
	hwnd uintptr

	// env is the process's one ICoreWebView2Environment, created on
	// this thread at the first window and kept for the process's life.
	// Only ever touched on this thread.
	env uintptr

	mu    sync.Mutex
	queue []func()
}

var (
	uiThreadOnce sync.Once
	theUIThread  *uiThread
	uiThreadErr  error
)

// ensureUIThread starts the UI thread if it is not already running and
// returns it. Safe to call from any goroutine.
func ensureUIThread() (*uiThread, error) {
	uiThreadOnce.Do(func() {
		t := &uiThread{}
		ready := make(chan error, 1)
		go t.run(ready)
		if err := <-ready; err != nil {
			uiThreadErr = err
			return
		}
		theUIThread = t
	})
	return theUIThread, uiThreadErr
}

// run is the thread itself: it takes an apartment, registers the window
// classes, creates the work window, and then pumps messages for as long
// as the process lives.
//
// It never returns and its apartment is never shut down. That is the
// point rather than an omission: the environment, the browser process
// group behind it and every window created from it belong to this
// apartment, and an apartment that outlives them all is what makes a
// second window cost a navigation instead of a browser process. The
// operating system reclaims all of it at process exit.
func (t *uiThread) run(ready chan<- error) {
	runtime.LockOSThread()
	t.threadID = currentThreadID()

	ensureDPIAware()
	if err := initApartment(); err != nil {
		ready <- err
		return
	}
	registerWindowClass()
	registerUIThreadClass()

	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(uiThreadClassName)),
		0,
		0,
		0, 0, 0, 0,
		hwndMessage, 0, moduleHandle(), 0,
	)
	if hwnd == 0 {
		ready <- fmt.Errorf("ui: creating the UI thread's work window: %v", callErr)
		return
	}
	t.hwnd = hwnd

	ready <- nil
	pumpUntil(func() bool { return false })
}

// do runs fn on the UI thread and returns once it has finished.
//
// Called from the UI thread itself it simply runs fn: there is no other
// thread to hand it to, and posting would deadlock against the wait
// below.
func (t *uiThread) do(fn func()) {
	if currentThreadID() == t.threadID {
		fn()
		return
	}
	done := make(chan struct{})
	t.mu.Lock()
	t.queue = append(t.queue, func() {
		defer close(done)
		fn()
	})
	t.mu.Unlock()
	postMessage(t.hwnd, wmRunFunc, 0, 0)
	<-done
}

// drain runs every queued closure, in order. It runs on the UI thread,
// from the work window's procedure, and takes each closure off the
// queue before running it — so a closure that pumps the message loop
// (window creation does) cannot see itself again.
func (t *uiThread) drain() {
	for {
		t.mu.Lock()
		if len(t.queue) == 0 {
			t.mu.Unlock()
			return
		}
		fn := t.queue[0]
		t.queue = t.queue[1:]
		t.mu.Unlock()
		fn()
	}
}

// environment is the process's one WebView2 environment, created on
// first use. Must be called on the UI thread.
//
// A failure is not remembered. Creating an environment can fail for
// reasons that pass — the runtime updating itself underneath the
// process is the documented one — and caching the first failure would
// turn a moment into a process that can never open a window again.
func (t *uiThread) environment() (uintptr, error) {
	if t.env != 0 {
		return t.env, nil
	}
	dir := filepath.Join(platform.ConfigDir(runtime.GOOS, platform.OSEnv), "webview2-profile")
	env, err := createEnvironment(dir)
	if err != nil {
		return 0, err
	}
	t.env = env
	return env, nil
}

func registerUIThreadClass() {
	registerUIThreadClassOnce.Do(func() {
		var wc wndClassExW
		wc.Size = uint32(unsafe.Sizeof(wc))
		wc.WndProc = uiThreadWndProcCallback
		wc.Instance = moduleHandle()
		wc.ClassName = uiThreadClassName
		r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if r == 0 {
			panic(fmt.Sprintf("ui: RegisterClassExW for the UI thread's work window failed: %v", err))
		}
	})
}

// uiThreadWndProcCallback is created once, at package initialisation,
// for the reason every other syscall.NewCallback in this package is
// (com_windows.go): a trampoline is a scarce, never-released process
// resource, and one per call would be a leak.
var uiThreadWndProcCallback = syscall.NewCallback(uiThreadWndProc)

func uiThreadWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	if msg == wmRunFunc {
		if t := theUIThread; t != nil {
			t.drain()
		}
		return 0
	}
	return defWindowProc(hwnd, msg, wparam, lparam)
}
