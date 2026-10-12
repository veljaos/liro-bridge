//go:build windows

package ui

// C21's measurements on a real Windows machine: CI's runner, which has
// WebView2 and no card ([[D-457]]). Each test turns one row of the
// memory-ownership audit from a reading into something that can fail.
// The predictions they were written against are in
// docs/s33-predictions.md, committed with them, before any of them ran.

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/fstest"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// onUIThread runs fn on the window's own thread and waits for it, with a
// ceiling that turns a thread that never runs it into a failure rather
// than a hang (D-201). Everything a test reads from w.dropTargets goes
// through here: the slice belongs to that thread.
func onUIThread(t *testing.T, w *window, fn func()) {
	t.Helper()
	done := make(chan struct{})
	w.invoke(func() { fn(); close(done) })
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the UI thread never ran the closure")
	}
}

// pinnedNow reports which of the given handler addresses are still
// pinned — still held by somebody outside Go, by this package's own
// bookkeeping.
func pinnedNow(addrs []uintptr) []uintptr {
	handlerPinsMu.Lock()
	defer handlerPinsMu.Unlock()
	var out []uintptr
	for _, a := range addrs {
		if _, ok := handlerPins[a]; ok {
			out = append(out, a)
		}
	}
	return out
}

// TestAClosedWindowGivesBackEveryDropTargetItRegistered is C21's row on
// drop targets revoked after ICoreWebView2Controller::Close.
//
// RegisterDragDrop takes a reference to the target and RevokeDragDrop
// gives it back. The teardown closed the controller first and revoked
// afterwards, and Close is what takes the browser's own windows away —
// four of the five windows a target was registered on. A revoke on a
// window that no longer exists fails, and OLE's reference is then never
// given back, so the target — and the *window its callback captures —
// stays pinned for the life of the process.
//
// What is counted is this package's own pins of exactly the targets
// this window registered, read before the close, so the window's other
// handlers (the navigation guards) cannot be mistaken for them.
func TestAClosedWindowGivesBackEveryDropTargetItRegistered(t *testing.T) {
	w, err := NewWindow(Options{
		Title:          "liro-bridge drop target lifetime test",
		Width:          300,
		Height:         200,
		OnFilesDropped: func([]string) {},
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	win := w.(*window)

	// Two ticks of the watch, so that a window the browser adds after
	// the snapshot is registered too and counted with the rest.
	time.Sleep(2500 * time.Millisecond)
	var targets []uintptr
	onUIThread(t, win, func() {
		for _, dt := range win.dropTargets {
			targets = append(targets, dt.addr())
		}
	})
	if len(targets) < 2 {
		_ = w.Close()
		t.Fatalf("only %d drop targets were registered; the frame and the browser's windows should each have one, so this test proves nothing", len(targets))
	}
	if got := pinnedNow(targets); len(got) != len(targets) {
		_ = w.Close()
		t.Fatalf("%d of %d registered targets were pinned before the close; the count is not measuring what this test thinks", len(got), len(targets))
	}

	_ = w.Close()

	// RevokeDragDrop releases synchronously; the ceiling is only there
	// for a release that comes from somewhere else, later.
	deadline := time.Now().Add(10 * time.Second)
	for {
		left := pinnedNow(targets)
		if len(left) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of the %d drop targets this window registered are still pinned after it closed: "+
				"OLE kept its reference, and every main window leaves its targets and itself behind",
				len(left), len(targets))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// recordingHandler keeps every record at Warn or above and passes
// everything on to the handler it replaced.
type recordingHandler struct {
	next slog.Handler
	mu   *sync.Mutex
	recs *[]string
}

func (h recordingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return l >= slog.LevelWarn || h.next.Enabled(ctx, l)
}

func (h recordingHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		var b strings.Builder
		b.WriteString(r.Message)
		r.Attrs(func(a slog.Attr) bool {
			b.WriteString(" " + a.String())
			return true
		})
		h.mu.Lock()
		*h.recs = append(*h.recs, b.String())
		h.mu.Unlock()
	}
	if h.next.Enabled(ctx, r.Level) {
		return h.next.Handle(ctx, r)
	}
	return nil
}

func (h recordingHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return recordingHandler{next: h.next.WithAttrs(as), mu: h.mu, recs: h.recs}
}

func (h recordingHandler) WithGroup(g string) slog.Handler {
	return recordingHandler{next: h.next.WithGroup(g), mu: h.mu, recs: h.recs}
}

// recordWarnings installs a recording handler as slog's default for the
// rest of the test and returns what it has recorded so far.
func recordWarnings(t *testing.T) func() []string {
	t.Helper()
	prev := slog.Default()
	var mu sync.Mutex
	var recs []string
	// Not prev.Handler(): while slog's own default is in place, that
	// handler writes through log.Default(), and SetDefault points
	// log.Default() back at the new handler — so every record went round
	// the loop into log.Logger's mutex a second time and the test hung
	// for ten minutes on CI's first run of it (session 33).
	slog.SetDefault(slog.New(recordingHandler{next: slog.NewTextHandler(os.Stderr, nil), mu: &mu, recs: &recs}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), recs...)
	}
}

// TestNavigatingRegistersTheNewTreesDropTargetsOnTheWindowsOwnThread is
// C21's row on Navigate's drop-target registration.
//
// RegisterDragDrop binds a target to the calling thread's OLE apartment,
// and the code says so (registerDropTarget's comment). Navigate waited
// for the navigation on the UI thread and then revoked and registered the
// new tree's targets on whatever goroutine had called it — a thread with
// no OLE on it, writing w.dropTargets while the UI thread's watch reads
// and appends to it. RegisterDragDrop on a thread without OleInitialize
// is documented to fail, and the failure is logged, which is what this
// reads: the warnings recorded during Navigate itself, before the watch's
// next tick could paper over them.
func TestNavigatingRegistersTheNewTreesDropTargetsOnTheWindowsOwnThread(t *testing.T) {
	assets := fstest.MapFS{
		"pages/one.html": &fstest.MapFile{Data: []byte(`<!doctype html><title>one</title><p>one</p>`)},
		"pages/two.html": &fstest.MapFile{Data: []byte(`<!doctype html><title>two</title><p>two</p>`)},
	}
	w, err := NewWindow(Options{
		Title:          "liro-bridge navigate drop target test",
		Width:          320,
		Height:         240,
		Assets:         assets,
		VirtualHost:    "navdrop.liro.invalid",
		StartPage:      "/pages/one.html",
		OnFilesDropped: func([]string) {},
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	win := w.(*window)

	warnings := recordWarnings(t)
	if err := w.Navigate("/pages/two.html"); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	var registered int
	var tree []uintptr
	var uiThreadID uintptr
	var offThread []uintptr
	onUIThread(t, win, func() {
		registered = len(win.dropTargets)
		tree = append([]uintptr{win.hwnd}, descendantWindows(win.hwnd)...)
		uiThreadID = currentThreadID()
		for _, target := range win.dropTargets {
			if target.thread != uiThreadID {
				offThread = append(offThread, target.thread)
			}
		}
	})

	// The measurement this test was missing (worklist 0.1, D-473): it
	// passed before the fix too, because a registration off the window's
	// thread succeeded on the runner. What the fix changed is the thread
	// each target is bound to, and that is read here.
	if len(offThread) > 0 {
		t.Errorf("%d of %d drop targets were registered on another thread (%v), not the window's own (%d): OLE delivers their drops to that thread's apartment",
			len(offThread), registered, offThread, uiThreadID)
	}

	var failures []string
	for _, rec := range warnings() {
		if strings.Contains(rec, "will not accept dropped files") {
			failures = append(failures, rec)
		}
	}
	t.Logf("after Navigate: %d targets registered, %d windows in the tree, %d registration warnings", registered, len(tree), len(failures))
	if len(failures) > 0 {
		t.Errorf("registering the new page's drop targets failed during Navigate (%d warnings), first: %s", len(failures), failures[0])
	}
	if registered == 0 {
		t.Errorf("no drop target is registered right after Navigate; until the watch's next tick a drop on this window goes nowhere")
	}
	var missing int
	for _, h := range tree {
		if !hasDropTarget(h) {
			missing++
		}
	}
	if missing > 0 {
		t.Errorf("%d of %d windows in the tree have no drop target right after Navigate", missing, len(tree))
	}
}

// failingFS is an asset tree that cannot be read at all, which is the
// one way to make setUpWebView2 fail after the native window and its
// icons already exist without touching anything outside this test.
type failingFS struct{}

func (failingFS) Open(string) (fs.File, error) { return nil, errors.New("this tree cannot be read") }

// TestAWindowThatFailsToOpenGivesBackItsIcons is C21's first error-path
// row: create's failure path destroyed the window and returned, and the
// two icons setWindowIcons had loaded for it — six GDI objects, measured
// in FTEST Group 3 — were never given back. The success path frees them
// in wndProc's WM_CLOSE; this path never reaches WM_CLOSE.
func TestAWindowThatFailsToOpenGivesBackItsIcons(t *testing.T) {
	open := func() error {
		_, err := NewWindow(Options{
			Title:       "liro-bridge failed window test",
			Width:       200,
			Height:      120,
			Assets:      failingFS{},
			VirtualHost: "failing.liro.invalid",
			StartPage:   "/x.html",
		})
		return err
	}
	// The first failure pays for the environment and the extracted icon,
	// which every later window shares.
	if err := open(); err == nil {
		t.Fatal("a window over an unreadable asset tree opened; this test proves nothing")
	}
	runtime.GC()

	const n = 5
	before := gdiObjects()
	for i := 0; i < n; i++ {
		if err := open(); err == nil {
			t.Fatal("a window over an unreadable asset tree opened; this test proves nothing")
		}
	}
	after := gdiObjects()
	t.Logf("%d failed windows: %d GDI objects before, %d after", n, before, after)
	const slack = 8
	if after-before > slack {
		t.Errorf("%d windows that failed to open left %d GDI objects behind (%.1f each): their title-bar icons were never destroyed",
			n, after-before, float64(after-before)/n)
	}
}

var (
	procFindWindowW      = user32DLL.NewProc("FindWindowW")
	procIsWindowForTest  = user32DLL.NewProc("IsWindow")
	ownerTestClassName   = windows.StringToUTF16Ptr("STATIC")
	ownerTestWindowTitle = windows.StringToUTF16Ptr("liro-bridge PIN owner test")
)

// TestAPINDialogWhoseOwnerIsDestroyedReturns is C21's row on the PIN
// dialog's own message loop.
//
// The dialog runs on a thread of its own and its loop ends only when
// OK, Cancel or the title bar's close sets closed. Its owner — the sign
// window, on the UI thread — can be destroyed while it is up, and
// Windows destroys a destroyed window's owned windows with it. The
// dialog's procedure did not handle WM_DESTROY, so nothing set closed,
// GetMessage waited for a message nothing would send, and the caller of
// CollectPIN — the PKCS#11 login — waited for ever.
//
// Whether Windows destroys an owned window that belongs to another
// thread at all is the least certain part, and is read here too: the
// dialog's window is looked for after the owner goes.
func TestAPINDialogWhoseOwnerIsDestroyedReturns(t *testing.T) {
	// The owner, on a locked thread of its own that pumps its messages,
	// as the UI thread does for the sign window.
	type ownerThread struct {
		hwnd    uintptr
		destroy chan struct{}
		gone    chan struct{}
	}
	o := ownerThread{destroy: make(chan struct{}), gone: make(chan struct{})}
	ready := make(chan uintptr, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		h, _, _ := procCreateWindowExW.Call(0,
			uintptr(unsafe.Pointer(ownerTestClassName)), uintptr(unsafe.Pointer(ownerTestWindowTitle)),
			wsPopup|wsCaption|wsVisible, 100, 100, 300, 200, 0, 0, 0, 0)
		ready <- h
		if h == 0 {
			return
		}
		var m winMsg
		for {
			select {
			case <-o.destroy:
				_, _, _ = procDestroyWindow.Call(h)
				close(o.gone)
				return
			default:
			}
			// Peek rather than GetMessage, so the destroy request is seen
			// without a message to wake the loop.
			r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
			if r != 0 {
				translateMessage(&m)
				dispatchWinMessage(&m)
				continue
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	o.hwnd = <-ready
	if o.hwnd == 0 {
		t.Fatal("the owner window could not be created")
	}

	type result struct {
		ok  bool
		err error
	}
	returned := make(chan result, 1)
	go func() {
		dst := make([]byte, 16)
		_, ok, err := CollectPIN(o.hwnd, PINPrompt{
			Title: "liro-bridge PIN owner test", Heading: "Liro Bridge",
			Label: "PIN", OK: "OK", Cancel: "Cancel",
		}, 15, dst)
		returned <- result{ok, err}
	}()

	// Wait for the dialog by its class, so the owner is destroyed while
	// it is really up and not before it was made.
	var dialog uintptr
	deadline := time.Now().Add(30 * time.Second)
	for dialog == 0 {
		dialog, _, _ = procFindWindowW.Call(uintptr(unsafe.Pointer(pinClassName)), 0)
		if dialog == 0 {
			if time.Now().After(deadline) {
				t.Fatal("the PIN dialog never appeared")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	close(o.destroy)
	<-o.gone

	select {
	case r := <-returned:
		if r.err != nil {
			t.Fatalf("CollectPIN: %v", r.err)
		}
		if r.ok {
			t.Fatal("CollectPIN reported a PIN entered when its owner was destroyed and nothing was typed")
		}
	case <-time.After(15 * time.Second):
		alive, _, _ := procIsWindowForTest.Call(dialog)
		// Let the stuck dialog go, so it does not sit on the runner's
		// desktop for the rest of the suite.
		// Bounded too: a dialog destroyed with its owner takes no message.
		postMessage(dialog, wmClose, 0, 0)
		select {
		case <-returned:
		case <-time.After(5 * time.Second):
		}
		t.Fatalf("CollectPIN did not return within 15 s of its owner being destroyed (the dialog's window %s); "+
			"the PKCS#11 login that called it would wait for ever",
			map[bool]string{true: "still exists", false: "was destroyed with it"}[alive != 0])
	}
}

var procPeekMessageW = user32DLL.NewProc("PeekMessageW")

const pmRemove = 0x0001

// A stand-in ICoreWebView2 whose every method fails, so that the add_*
// calls fail where a real one would — the one way to reach their error
// paths without a browser that refuses (worklist 0.3, D-473). Package
// variables, so that nothing the callback or comCall reads can move or
// be collected.
var (
	failingCW2Vtbl [64]uintptr
	failingCW2     struct{ vtbl uintptr }
)

func failingCoreWebView2() uintptr {
	eFail := syscall.NewCallback(func(this, handler, token uintptr) uintptr { return 0x80004005 })
	for i := range failingCW2Vtbl {
		failingCW2Vtbl[i] = eFail
	}
	failingCW2.vtbl = uintptr(unsafe.Pointer(&failingCW2Vtbl[0]))
	return uintptr(unsafe.Pointer(&failingCW2))
}

// pinnedHandlers is every handler this package holds pinned right now.
func pinnedHandlers() map[uintptr]bool {
	handlerPinsMu.Lock()
	defer handlerPinsMu.Unlock()
	out := make(map[uintptr]bool, len(handlerPins))
	for a := range handlerPins {
		out[a] = true
	}
	return out
}

// TestAFailedSubscriptionGivesItsHandlerBack is C21's row on add_*
// failing: the handler was made and pinned for the call, and when the
// call failed nothing would ever release it — pinned for the life of
// the process. Untested until the owner asked why (D-471): it could be,
// with a browser that refuses, and this is one.
func TestAFailedSubscriptionGivesItsHandlerBack(t *testing.T) {
	cw2 := failingCoreWebView2()
	before := pinnedHandlers()

	if _, err := coreWebView2AddNavigationStarting(cw2, "fail.liro.invalid"); err == nil {
		t.Fatal("add_NavigationStarting on the failing stand-in succeeded; this test measures nothing")
	}
	if _, err := coreWebView2AddFrameNavigationStarting(cw2, "fail.liro.invalid"); err == nil {
		t.Fatal("add_FrameNavigationStarting on the failing stand-in succeeded")
	}
	if _, err := coreWebView2AddNewWindowRequested(cw2); err == nil {
		t.Fatal("add_NewWindowRequested on the failing stand-in succeeded")
	}

	var kept []uintptr
	for a := range pinnedHandlers() {
		if !before[a] {
			kept = append(kept, a)
		}
	}
	if len(kept) > 0 {
		t.Errorf("%d handler(s) whose add_* failed are still pinned: nothing will ever release them", len(kept))
	}
}

var (
	procQuitTestFindWindowExW        = user32DLL.NewProc("FindWindowExW")
	procQuitTestGetWindowTextLengthW = user32DLL.NewProc("GetWindowTextLengthW")
	quitTestEditClass                = windows.StringToUTF16Ptr("Edit")
	quitTestTyped                    = windows.StringToUTF16Ptr("1234")
)

// quitTestWMQuit is WM_QUIT, which this package never posts.
const quitTestWMQuit = 0x0012

// TestAQuitReachingThePINLoopIsACancel is C21's WM_QUIT row (D-471: it
// could be tested and was not). A WM_QUIT is the one way out of the
// dialog's GetMessage loop that sets nothing; before the fix the loop
// ended, CollectPIN returned, and the dialog stayed up on a thread that
// no longer read its messages, with what had been typed still in its
// field. Now it is a cancel: the window is destroyed and the field
// cleared first.
//
// It does not post the WM_QUIT on again for an outer loop, the usual
// rule for a nested one: the login's thread has none, and a WM_QUIT
// left in its queue would end the next PIN dialog on that thread the
// moment it opened (D-473).
func TestAQuitReachingThePINLoopIsACancel(t *testing.T) {
	type result struct {
		n   int
		ok  bool
		err error
	}
	returned := make(chan result, 1)
	go func() {
		dst := make([]byte, 16)
		n, ok, err := CollectPIN(0, PINPrompt{
			Title: "liro-bridge PIN quit test", Heading: "Liro Bridge",
			Label: "PIN", OK: "OK", Cancel: "Cancel",
		}, 15, dst)
		returned <- result{n, ok, err}
	}()

	var dialog uintptr
	deadline := time.Now().Add(30 * time.Second)
	for dialog == 0 {
		dialog, _, _ = procFindWindowW.Call(uintptr(unsafe.Pointer(pinClassName)), 0)
		if dialog == 0 {
			if time.Now().After(deadline) {
				t.Fatal("the PIN dialog never appeared")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	edit, _, _ := procQuitTestFindWindowExW.Call(dialog, 0, uintptr(unsafe.Pointer(quitTestEditClass)), 0)
	if edit == 0 {
		t.Fatal("the PIN dialog has no edit control to type into")
	}
	_, _, _ = procSendMessageW.Call(edit, wmSetText, 0, uintptr(unsafe.Pointer(quitTestTyped)))
	if n, _, _ := procQuitTestGetWindowTextLengthW.Call(edit); n != 4 {
		t.Fatalf("typing into the field left %d characters, want 4; this test measures nothing", n)
	}
	tid, _, _ := procGetWindowThreadPID.Call(dialog, 0)
	if r, _, err := procPostThreadMessageW.Call(tid, quitTestWMQuit, 0, 0); r == 0 {
		t.Fatalf("PostThreadMessage(WM_QUIT): %v", err)
	}

	select {
	case r := <-returned:
		if r.err != nil || r.ok || r.n != 0 {
			t.Fatalf("CollectPIN returned (%d, %v, %v) after a WM_QUIT; want a cancel", r.n, r.ok, r.err)
		}
	case <-time.After(15 * time.Second):
		postMessage(dialog, wmClose, 0, 0)
		t.Fatal("CollectPIN did not return within 15 s of a WM_QUIT reaching its loop")
	}
	if alive, _, _ := procIsWindowForTest.Call(dialog); alive != 0 {
		left, _, _ := procQuitTestGetWindowTextLengthW.Call(edit)
		// The thread that owns it no longer pumps; a posted close would
		// sit in a queue nobody reads. It goes with the test process.
		t.Errorf("after a WM_QUIT the dialog is still up, on a thread that no longer reads its messages, with %d typed characters in its field", left)
	}
}

// TestAPINDialogThatFailsToBuildGivesItsFontsBack is C21's row on the
// PIN dialog's fonts (D-471 said it needed a seam; it does not). build
// makes both fonts first and then the controls; a heading with a NUL in
// it cannot become a control's text, so build fails after the fonts
// exist — the same path a refused CreateWindowExW takes. The success
// path deletes the fonts after the loop, which this path never reached:
// two GDI objects per failed dialog, for the life of the process.
func TestAPINDialogThatFailsToBuildGivesItsFontsBack(t *testing.T) {
	bad := PINPrompt{
		Title: "liro-bridge PIN font test", Heading: "Liro\x00Bridge",
		Label: "PIN", OK: "OK", Cancel: "Cancel",
	}
	dst := make([]byte, 16)
	// Once first, so that whatever the class and the DPI awareness make
	// the first time is not counted as the dialog's.
	if _, _, err := CollectPIN(0, bad, 15, dst); err == nil {
		t.Fatal("a heading with a NUL in it built a dialog; this test measures nothing")
	}
	before := gdiObjects()
	const runs = 10
	for i := 0; i < runs; i++ {
		if _, _, err := CollectPIN(0, bad, 15, dst); err == nil {
			t.Fatal("a heading with a NUL in it built a dialog")
		}
	}
	after := gdiObjects()
	t.Logf("GDI objects: %d before, %d after %d failed dialogs", before, after, runs)
	if after-before >= runs {
		t.Errorf("%d failed PIN dialogs kept %d GDI objects (%.1f each): their fonts were never deleted",
			runs, after-before, float64(after-before)/runs)
	}
}
