//go:build windows

package ui

// NewWindow ties together win32_windows.go (the native frame),
// loader_windows.go (finding and calling into WebView2Loader.dll) and
// webview2_windows.go (the COM calls once an environment exists) into
// one window: a native HWND with a WebView2 control filling its
// client area.
//
// Everything after window creation runs on a single OS thread with its
// own STA COM apartment (F5 §2.1) — CoInitializeEx(COINIT_APARTMENTTHREADED)
// is called once, on a goroutine pinned there with runtime.LockOSThread,
// and every COM call this package makes for that window's lifetime,
// Go->page or page->Go, is marshaled onto that same thread. This is not
// an optimisation; calling a single-threaded-apartment COM object from
// any other thread is undefined behaviour, so PostJSON and Close must
// never touch a COM pointer directly — they hand a closure to the
// owning thread via wmRunFunc instead (see invoke below).
//
// Nothing a *caller* supplies ever runs on that thread. OnMessage,
// OnFilesDropped and OnClosed are queued by the thread that produces
// them and delivered, in order, by one goroutine per window
// (dispatchEvents below). This is not tidiness. Before it, every
// callback ran inside wndProc, so a handler that blocked — and every
// handler in this project is a send on a bounded channel, which blocks
// as soon as the code draining it is busy with something else — stopped
// the window's message loop dead. The window then had no way back:
// it did not repaint, did not answer the title bar, and Close's posted
// WM_CLOSE was never dispatched, so the window stayed on screen with
// Windows calling it "not responding". Measured directly: a settings
// window whose caller was waiting on another window froze on the ninth
// click, with its own goroutine parked in "chan send, locked to thread"
// inside webMessageReceivedInvoke.
//
// Teardown obeys the same rule, and D-101 records what happened when it
// only nearly did. One thread owns the WebView2 controller — the thread
// that created it — and it is the only thread that may release it.
// Everything else asks: Close posts WM_CLOSE and waits on closedCh for
// that thread to say it is done, and touches no COM pointer of its own.
//
// Owning the release is necessary but not sufficient, because the
// owning thread can be asked to do it twice, and can be asked while it
// is already doing it: ICoreWebView2Controller::Close runs a nested
// message loop, which dispatches whatever else is in this window's
// queue — including a second WM_CLOSE — back into wndProc, on this same
// thread, from inside the first Close. Making the release idempotent by
// zeroing pointers afterwards does not help against that, because the
// second entry reads the pointers before the first entry has finished
// with them and zeroed them. The teardown is therefore guarded by a
// flag set on entry rather than on exit (tearingDown, below), which is
// what actually makes "exactly once" true.
import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// coreWebView2HostResourceAccessKindDeny is
// COREWEBVIEW2_HOST_RESOURCE_ACCESS_KIND_DENY (0): other origins get no
// access to the virtual host's resources. The agent's own pages never
// need to be reachable from anywhere else.
const coreWebView2HostResourceAccessKindDeny = 0

type window struct {
	hwnd       uintptr
	env        uintptr
	controller uintptr
	cw2        uintptr // ICoreWebView2 (base)
	cw2v3      uintptr // ICoreWebView2_3, QueryInterface'd once at setup

	// Both handlers must outlive their subscriptions — see
	// coreWebView2AddWebMessageReceived's doc comment. They are this
	// package's own COM references to those objects, and closeWebView
	// releases them after ICoreWebView2Controller::Close has released
	// WebView2's.
	webMessageHandler *webMessageReceivedHandler
	navHandler        *navigationCompletedHandler

	widthPts, heightPts int

	// threadID is the OS thread that created this window's WebView2
	// controller and is the only one allowed to release it (see this
	// file's package doc comment). Written once, before NewWindow's
	// ready channel is signalled, and only read afterwards.
	threadID uintptr

	workCh    chan func()
	closedCh  chan struct{}
	closeOnce sync.Once

	// owner is the window this one was opened from, disabled for as
	// long as this window is up and re-enabled before it is destroyed
	// (Options.Owner). Zero when this window stands on its own.
	owner uintptr

	onMessage      func(Message)
	onClosed       func()
	onFilesDropped func([]string)
	closingFromGo  atomic.Bool

	// events is the queue between the window's own thread, which
	// produces callbacks, and dispatchEvents, which delivers them. It
	// is a slice under a mutex rather than a channel because it must
	// never make the producer wait: a channel of any fixed size
	// eventually blocks, and blocking the producer here is the defect
	// this whole arrangement exists to remove.
	eventMu     sync.Mutex
	eventQueue  []windowEvent
	eventSignal chan struct{}

	// dropTargets are this window's registered IDropTargets — one per
	// window in the hosted tree, because a drop lands on exactly one of
	// them (droptarget_windows.go). Empty when the window asked for no
	// drops, or when registration failed.
	dropTargets []*dropTarget

	// apartmentIsOLE says which call put this thread into its
	// apartment (com_windows.go's initApartment), so the teardown
	// undoes the matching one. OleInitialize and CoInitializeEx keep
	// separate counts; calling CoUninitialize against an OleInitialize
	// leaves OLE half-shut-down on a thread that is about to end.
	apartmentIsOLE bool

	// tearingDown is set by the owning thread, on entry to the teardown
	// and before it releases anything, so that a second WM_CLOSE — from
	// the title bar, from Close, or dispatched by the nested message
	// loop inside ICoreWebView2Controller::Close itself — is a no-op
	// rather than a second release of the same pointers. Only ever
	// touched by the owning thread, which is why it is a plain bool: a
	// mutex here would make two threads take turns at a teardown only
	// one of them is allowed to perform at all.
	tearingDown bool

	// webViewReleased says the COM pointers below have already been
	// released. Separate from tearingDown because closeWebView is also
	// reached from setUpWebView2's failure path, which is not a WM_CLOSE
	// at all; both flags are set on entry, never on exit.
	webViewReleased bool
}

// NewWindow implements ui.NewWindow (window.go) on Windows: it spawns
// the dedicated OS thread described in this file's package doc comment,
// waits for the window and its WebView2 control to be fully ready (or
// for setup to fail), and returns only then — by the time it returns
// successfully, PostJSON is safe to call immediately.
func NewWindow(opts Options) (Window, error) {
	if opts.Width <= 0 || opts.Height <= 0 {
		return nil, fmt.Errorf("ui: Options.Width and Height must be positive")
	}

	w := &window{
		widthPts: opts.Width, heightPts: opts.Height,
		workCh:         make(chan func(), 8),
		closedCh:       make(chan struct{}),
		eventSignal:    make(chan struct{}, 1),
		owner:          opts.Owner,
		onMessage:      opts.OnMessage,
		onClosed:       opts.OnClosed,
		onFilesDropped: opts.OnFilesDropped,
	}
	go w.dispatchEvents()
	ready := make(chan error, 1)
	go w.run(opts, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return w, nil
}

// windowEvent is one queued caller callback. Exactly one field is set.
type windowEvent struct {
	message *Message
	dropped []string
	closed  bool
}

// queueEvent hands an event to dispatchEvents and returns immediately.
// Called from the window's own message-loop thread, which is why it may
// not wait for anything.
func (w *window) queueEvent(ev windowEvent) {
	w.eventMu.Lock()
	w.eventQueue = append(w.eventQueue, ev)
	w.eventMu.Unlock()
	select {
	case w.eventSignal <- struct{}{}:
	default:
	}
}

// dispatchEvents delivers queued callbacks, in order, on its own
// goroutine. It drains once more after the window has closed so that a
// message the page sent, or the OnClosed a user-driven close queued,
// is never lost to the teardown that followed it.
func (w *window) dispatchEvents() {
	for {
		select {
		case <-w.eventSignal:
			w.drainEvents()
		case <-w.closedCh:
			w.drainEvents()
			return
		}
	}
}

func (w *window) drainEvents() {
	for {
		w.eventMu.Lock()
		if len(w.eventQueue) == 0 {
			w.eventMu.Unlock()
			return
		}
		ev := w.eventQueue[0]
		w.eventQueue = w.eventQueue[1:]
		w.eventMu.Unlock()

		switch {
		case ev.message != nil:
			if w.onMessage != nil {
				w.onMessage(*ev.message)
			}
		case ev.dropped != nil:
			if w.onFilesDropped != nil {
				w.onFilesDropped(ev.dropped)
			}
		case ev.closed:
			if w.onClosed != nil {
				w.onClosed()
			}
		}
	}
}

func (w *window) run(opts Options, ready chan<- error) {
	runtime.LockOSThread()
	w.threadID = currentThreadID()
	// Deliberately never unlocked: this goroutine and the OS thread it
	// is pinned to live exactly as long as the window. Ending the
	// goroutine (after WM_QUIT, at the bottom of this function) ends the
	// thread too, which is correct — nothing else may use this STA
	// apartment once the window that owns it is gone.

	ensureDPIAware()
	ole, err := initApartment()
	if err != nil {
		ready <- err
		return
	}
	w.apartmentIsOLE = ole
	registerWindowClass()

	dpi := uint32(96)
	hwnd, err := w.createNativeWindow(opts, dpi)
	if err != nil {
		shutdownApartment(w.apartmentIsOLE)
		ready <- err
		return
	}
	w.hwnd = hwnd
	setWindowUserData(hwnd, unsafe.Pointer(w))
	// Task 4 (F5 second-real-run review): the real Liro mark in the
	// title bar and in Alt+Tab, for every window this package creates —
	// the tray icon already came from icon.ico, the windows did not.
	setWindowIcons(hwnd)

	// The window's actual monitor — and so its actual DPI — is only
	// known once CreateWindowExW has placed it; correct the size
	// immediately if the 96 assumed above was wrong, rather than waiting
	// for a WM_DPICHANGED that will not arrive for a window created
	// directly on an already-scaled monitor.
	if real := getDpiForWindow(hwnd); real != dpi {
		dpi = real
		w.resizeToClientPoints(opts.Width, opts.Height, dpi)
	}

	if err := w.setUpWebView2(opts); err != nil {
		if w.owner != 0 {
			enableWindow(w.owner, true)
		}
		w.closeWebView()
		// DestroyWindow dispatches WM_DESTROY synchronously on this same
		// thread, which is what actually calls shutdownApartment (wndProc's
		// wmDestroy case, below) — no separate call needed here.
		_, _, _ = procDestroyWindow.Call(hwnd)
		ready <- err
		return
	}

	showAndFocusWindow(hwnd, opts.AlwaysOnTop)
	ready <- nil

	pumpUntil(func() bool { return false })
}

func (w *window) setUpWebView2(opts Options) error {
	userDataDir := filepath.Join(platform.ConfigDir(runtime.GOOS, platform.OSEnv), "webview2-profile")

	env, err := createEnvironment(userDataDir)
	if err != nil {
		return fmt.Errorf("ui: creating WebView2 environment: %w", err)
	}
	w.env = env

	controller, err := environmentCreateController(env, w.hwnd)
	if err != nil {
		return fmt.Errorf("ui: creating WebView2 controller: %w", err)
	}
	w.controller = controller

	cw2, err := controllerGetCoreWebView2(controller)
	if err != nil {
		return fmt.Errorf("ui: getting CoreWebView2: %w", err)
	}
	w.cw2 = cw2

	cw2v3, err := queryInterface(cw2, iidCoreWebView2_3)
	if err != nil {
		return fmt.Errorf("ui: querying ICoreWebView2_3: %w", err)
	}
	w.cw2v3 = cw2v3

	if opts.Assets != nil && opts.VirtualHost != "" {
		assetsDir, err := materializeAssets(opts.Assets)
		if err != nil {
			return fmt.Errorf("ui: materialising assets: %w", err)
		}
		if err := coreWebView2SetVirtualHost(cw2v3, opts.VirtualHost, assetsDir, coreWebView2HostResourceAccessKindDeny); err != nil {
			return fmt.Errorf("ui: SetVirtualHostNameToFolderMapping: %w", err)
		}
	}

	// F6 §1, first half: switch WebView2's own external-drop handling
	// off, before the page is ever navigated, so Chromium never
	// registers a drop target of its own to displace. The page could
	// only ever see a File object anyway, never a path (D-114).
	//
	// The second half — registering this side's drop targets — waits
	// until the bottom of this function, once the page has loaded and
	// the browser's window tree has stopped changing shape.
	if opts.OnFilesDropped != nil {
		controller4, err := queryInterface(controller, iidCoreWebView2Controller4)
		if err != nil {
			return fmt.Errorf("ui: querying ICoreWebView2Controller4 for AllowExternalDrop: %w", err)
		}
		defer comRelease(controller4)
		if err := controllerSetAllowExternalDrop(controller4, false); err != nil {
			return fmt.Errorf("ui: put_AllowExternalDrop(FALSE): %w", err)
		}
	}

	if opts.OnMessage != nil {
		// The handler WebView2 calls queues; it never calls the caller's
		// OnMessage itself. See this file's package doc comment.
		h, err := coreWebView2AddWebMessageReceived(cw2, func(m Message) {
			w.queueEvent(windowEvent{message: &m})
		})
		if err != nil {
			return fmt.Errorf("ui: add_WebMessageReceived: %w", err)
		}
		w.webMessageHandler = h
	}

	dpi := getDpiForWindow(w.hwnd)
	clientW, clientH := scaleForDPI(opts.Width, dpi), scaleForDPI(opts.Height, dpi)
	if err := controllerSetBounds(controller, 0, 0, clientW, clientH); err != nil {
		return fmt.Errorf("ui: setting WebView2 bounds: %w", err)
	}
	if err := controllerSetVisible(controller, true); err != nil {
		return fmt.Errorf("ui: showing WebView2 control: %w", err)
	}

	if opts.StartPage != "" && opts.VirtualHost != "" {
		// Subscribed before Navigate, and waited on synchronously here, so
		// that NewWindow never returns until the page's own <script> tags
		// (bridge.js, then the page script) have actually run — otherwise
		// the caller's first PostJSON (carrying every localised string)
		// races the page load and is silently dropped by bridge.js's
		// "window.__liroReceive &&" guard. See navigationCompletedHandler's
		// doc comment (webview2_windows.go).
		navDone, err := coreWebView2AddNavigationCompleted(cw2)
		if err != nil {
			return fmt.Errorf("ui: add_NavigationCompleted: %w", err)
		}
		// Kept on the window, not just for the length of this function:
		// the subscription lives as long as the window does, and WebView2
		// holds this object's bare address for all of it (D-101).
		w.navHandler = navDone
		if err := coreWebView2Navigate(cw2, "https://"+opts.VirtualHost+opts.StartPage); err != nil {
			return fmt.Errorf("ui: Navigate: %w", err)
		}
		pumpUntil(func() bool { return navDone.done })
	}

	// F6 §1, second half. Registered here, after the page has finished
	// loading, because the browser's window tree is what receives a
	// drop and it is not complete until then — the compositor's own
	// window appears somewhere between controller creation and the
	// first frame. Each page in this project navigates exactly once, so
	// the tree measured here is the tree that lives for the window's
	// whole life.
	if opts.OnFilesDropped != nil {
		w.dropTargets = registerDropTargets(w.hwnd, func(paths []string) {
			w.queueEvent(windowEvent{dropped: paths})
		})
		if len(w.dropTargets) == 0 {
			// Not fatal: a window that cannot take drops is still a
			// usable window, with Browse and the Explorer menu. Loud,
			// though — silence here is exactly what produced a window
			// that looked like a drop target and refused every drop.
			slog.Error("ui: this window will not accept dropped files")
		}
		// The WM_DROPFILES fallback, for a drop that lands on the frame
		// itself rather than on the browser's windows. One call.
		setDragAcceptFiles(w.hwnd, true)
	}
	return nil
}

func (w *window) createNativeWindow(opts Options, dpi uint32) (uintptr, error) {
	style := uintptr(wsPopup | wsCaption | wsSysMenu)
	exStyle := uintptr(0)
	if opts.AlwaysOnTop {
		exStyle |= wsExTopMost
	}

	clientW, clientH := scaleForDPI(opts.Width, dpi), scaleForDPI(opts.Height, dpi)
	r := rect{Left: 0, Top: 0, Right: clientW, Bottom: clientH}
	_, _, _ = procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), style, 0, exStyle)
	winW, winH := r.Right-r.Left, r.Bottom-r.Top

	// An owned window is centred on its owner, not on the monitor under
	// the cursor: the cursor can be anywhere by the time a second window
	// opens, and a window that appears somewhere else does not read as
	// belonging to the one that opened it.
	box := cursorMonitorRect()
	if opts.Owner != 0 {
		if ob, ok := windowRect(opts.Owner); ok {
			box = ob
		}
	}
	x := box.Left + ((box.Right - box.Left - winW) / 2)
	y := box.Top + ((box.Bottom - box.Top - winH) / 2)

	titlePtr, err := windows.UTF16PtrFromString(opts.Title)
	if err != nil {
		return 0, err
	}
	// opts.Owner goes in CreateWindowExW's hWndParent slot. For a
	// WS_POPUP window that makes it the *owner*, not the parent:
	// Windows then keeps this window above that one in z-order
	// unconditionally — which is the half that was missing, since the
	// window this project opened from an always-on-top Settings window
	// was created underneath it and could not be seen at all.
	hwnd, _, callErr := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(windowClassName)),
		uintptr(unsafe.Pointer(titlePtr)),
		style,
		uintptr(x), uintptr(y), uintptr(winW), uintptr(winH),
		opts.Owner, 0, moduleHandle(), 0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("ui: CreateWindowExW: %v", callErr)
	}
	// The owner is disabled for as long as this window is up. The
	// caller that opened it is blocked waiting for its answer, so the
	// owner is not listening to clicks anyway; disabling it is what
	// makes that visible instead of making the program look broken.
	if opts.Owner != 0 {
		enableWindow(opts.Owner, false)
	}
	return hwnd, nil
}

// resizeToClientPoints repositions/resizes the native window so its
// client area is Options.Width x Options.Height points at dpi,
// recentred on the monitor under the cursor — used only for the
// one-shot correction right after creation (run, above); WM_DPICHANGED
// uses the OS-suggested rect instead (wndProc, below), not this.
func (w *window) resizeToClientPoints(widthPts, heightPts int, dpi uint32) {
	style := uintptr(wsPopup | wsCaption | wsSysMenu)
	clientW, clientH := scaleForDPI(widthPts, dpi), scaleForDPI(heightPts, dpi)
	r := rect{Left: 0, Top: 0, Right: clientW, Bottom: clientH}
	_, _, _ = procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), style, 0, 0)
	winW, winH := r.Right-r.Left, r.Bottom-r.Top

	mon := cursorMonitorRect()
	x := mon.Left + ((mon.Right - mon.Left - winW) / 2)
	y := mon.Top + ((mon.Bottom - mon.Top - winH) / 2)
	setWindowPos(w.hwnd, 0, x, y, winW, winH, 0)
}

// wndProc is the single, process-wide window procedure registered for
// every window this package creates (win32_windows.go's
// registerWindowClass). It looks up the owning *window from
// GWLP_USERDATA, set immediately after CreateWindowExW in run above.
func wndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	p := getWindowUserData(hwnd)
	if p == nil {
		return defWindowProc(hwnd, msg, wparam, lparam)
	}
	w := (*window)(p)

	switch msg {
	case wmRunFunc:
		// Drains exactly one pending closure per message — PostJSON
		// posts exactly one wmRunFunc per invoke (window.go's PostJSON),
		// so there is never more than one waiting when this fires.
		select {
		case fn := <-w.workCh:
			// Not once teardown has begun. The nested message loop that
			// ICoreWebView2Controller::Close runs dispatches whatever is
			// queued, and a closure queued by Eval or PostJSON would call
			// into COM pointers that are in the middle of being released.
			// The caller is not left hanging: it also selects on closedCh,
			// which wmDestroy closes moments later.
			if !w.tearingDown {
				fn()
			}
		default:
		}
		return 0

	case wmDropFiles:
		// wparam is the HDROP. droppedFiles reads every path out of it
		// and releases it; the callback runs on this window's own
		// thread, so a slow handler would block the message loop —
		// callers hand the paths straight to a channel for that reason.
		//
		// This arriving at all is worth a log line: it is the fallback
		// path, taken only when the drop landed on the frame rather
		// than on the browser's own child windows, and knowing which of
		// the two delivered a drop is the difference between reading a
		// bug report and guessing at one.
		if w.onFilesDropped != nil {
			paths := droppedFiles(wparam)
			slog.Info("ui: WM_DROPFILES received on the native frame", "paths", len(paths))
			if len(paths) > 0 {
				w.queueEvent(windowEvent{dropped: paths})
			}
		} else {
			// Nothing asked for these; release the HDROP anyway rather
			// than leak the shell's memory for the drop.
			droppedFiles(wparam)
		}
		return 0

	case wmDPIChanged:
		newDPI := uint32(wparam & 0xFFFF)
		suggested := (*rect)(unsafe.Pointer(lparam))
		setWindowPos(hwnd, 0, suggested.Left, suggested.Top, suggested.Right-suggested.Left, suggested.Bottom-suggested.Top, 0)
		if w.controller != 0 {
			cw, ch := scaleForDPI(w.widthPts, newDPI), scaleForDPI(w.heightPts, newDPI)
			if err := controllerSetBounds(w.controller, 0, 0, cw, ch); err != nil {
				slog.Warn("ui: resizing WebView2 bounds after WM_DPICHANGED failed", "error", err)
			}
		}
		return 0

	case wmClose:
		// Exactly one WM_CLOSE per window does any work. Every later one
		// — a second close request, or one the nested message loop inside
		// ICoreWebView2Controller::Close dispatches back into here while
		// the first is still running — returns immediately. See this
		// file's package doc comment for why the flag is set here, on
		// entry, rather than inferred from pointers zeroed on the way
		// out.
		if w.tearingDown {
			return 0
		}
		w.tearingDown = true

		// F5 §2.3: closing the window is equivalent to Cancel. A
		// Go-initiated Close (window.go's Close method) sets
		// closingFromGo first, so OnClosed only fires for a user-driven
		// close (title bar, Alt+F4) — see window.go's doc comment on
		// Options.OnClosed.
		if !w.closingFromGo.Load() && w.onClosed != nil {
			w.queueEvent(windowEvent{closed: true})
		}
		// ICoreWebView2Controller::Close must run before DestroyWindow,
		// not after (in a WM_DESTROY handler): DestroyWindow tears down
		// this window's children — including the WebView2 control's own
		// child HWND — before this window's own WM_DESTROY is
		// dispatched, so calling Close() afterward operates on an
		// already-torn-down control and crashes. Closing explicitly
		// here, first, is the order Microsoft's own samples use.
		// The owner is re-enabled before this window is destroyed, not
		// after: destroying a window whose owner is still disabled hands
		// activation to whatever else is on the desktop, and the person
		// is left looking at somebody else's window.
		if w.owner != 0 {
			enableWindow(w.owner, true)
		}
		w.closeWebView()
		// Give the drop registrations back before the apartment holding
		// them goes away. Harmless when nothing was registered.
		if w.onFilesDropped != nil {
			revokeDropTargets(w.dropTargets)
			w.dropTargets = nil
			setDragAcceptFiles(hwnd, false)
		}
		_, _, _ = procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		shutdownApartment(w.apartmentIsOLE)
		// Once: DestroyWindow is reached from the WM_CLOSE path and from
		// setUpWebView2's failure path, and a closed channel closed twice
		// panics — which would turn a teardown ordering bug into a dead
		// process rather than a harmless repeat.
		w.closeOnce.Do(func() { close(w.closedCh) })
		postQuitMessage(0)
		return 0

	default:
		return defWindowProc(hwnd, msg, wparam, lparam)
	}
}

// closeWebView closes the WebView2 controller and releases every
// interface pointer and handler object this window holds. Must run
// before DestroyWindow — see the comment on wndProc's wmClose case —
// and must run on the thread that created the controller, which is the
// only thread allowed to touch it at all (this file's package doc
// comment).
//
// Runs at most once per window, guarded by tearingDown on entry. The
// previous guard — zeroing each pointer after releasing it — was set
// too late to help: ICoreWebView2Controller::Close pumps this window's
// message queue, so a second WM_CLOSE could be dispatched into wndProc
// and reach this function again while the first call was still inside
// controllerClose, with every pointer still non-zero. That is the
// access violation D-101 records: the controller was read, released,
// and read again before the first release had finished.
func (w *window) closeWebView() {
	if w.webViewReleased {
		return
	}
	w.webViewReleased = true

	if w.controller != 0 {
		controllerClose(w.controller)
	}
	comRelease(w.cw2v3)
	comRelease(w.cw2)
	comRelease(w.controller)
	comRelease(w.env)
	w.cw2v3, w.cw2, w.controller, w.env = 0, 0, 0, 0

	// This package's own references to the two event handlers, dropped
	// after ICoreWebView2Controller::Close has dropped WebView2's. Each
	// unpins its handler object once the last reference goes, which is
	// the point at which nothing outside Go holds its address any more
	// (com_windows.go's pinning commentary).
	if w.navHandler != nil {
		w.navHandler.release()
		w.navHandler = nil
	}
	if w.webMessageHandler != nil {
		w.webMessageHandler.release()
		w.webMessageHandler = nil
	}
}

// invoke marshals fn onto the window's owning OS thread (see this
// file's package doc comment) by posting wmRunFunc, which wndProc
// handles by draining workCh. Silently drops fn if the window has
// already closed — there is no thread left to run it on.
func (w *window) invoke(fn func()) {
	select {
	case w.workCh <- fn:
		postMessage(w.hwnd, wmRunFunc, 0, 0)
	case <-w.closedCh:
	}
}

// PostJSON implements Window.PostJSON (window.go).
func (w *window) PostJSON(v any) error {
	payload, err := marshalMessagePayload(v)
	if err != nil {
		return err
	}
	_, err = w.Eval(postJSONScript(payload))
	return err
}

// Eval implements Window.Eval (window.go): runs script and returns its
// JSON-encoded result via ExecuteScript's own completion value — see
// executeScriptCompletedHandler's doc comment (webview2_windows.go) for
// why this is the channel Window.Eval uses, rather than a fourth
// page->Go message type.
func (w *window) Eval(script string) (string, error) {
	type evalResult struct {
		s   string
		err error
	}
	result := make(chan evalResult, 1)
	w.invoke(func() {
		s, err := coreWebView2ExecuteScript(w.cw2, script)
		result <- evalResult{s, err}
	})

	select {
	case r := <-result:
		return r.s, r.err
	case <-w.closedCh:
		return "", errors.New("ui: window is closed")
	}
}

// closeTeardownTimeout bounds how long Close waits for the window's own
// OS thread to finish tearing down the WebView2 controller and
// releasing every COM reference it holds (Task 8) before giving up and
// returning anyway — a bug in that teardown must not hang the whole
// agent forever, only fail to fully suppress the shutdown noise it
// exists to avoid.
const closeTeardownTimeout = 5 * time.Second

// Close implements Window.Close (window.go).
//
// Waits for the window's dedicated OS thread to finish processing
// WM_CLOSE — closeWebView (releasing the WebView2 controller and every
// COM interface pointer this window holds) and DestroyWindow — before
// returning, rather than posting WM_CLOSE and returning immediately
// (Task 8, F5 first-real-run review). Every caller in this codebase
// already calls Close in a deferred, blocking position right before
// its own function returns (runSettingsWindow, runCertificatesWindow,
// runAuditLogWindow, runSignInteractive), so this was always the
// point at which the caller intended to be done with the window; it
// just was not actually done *waiting* for it. Returning before our
// own COM references were released gave the WebView2 runtime's
// browser-process teardown as little time as possible to run before
// the whole agent process could reach ExitProcess — which is what let
// its own "Failed to unregister class Chrome_WidgetWin_0" console line
// (emitted by that browser process, inheriting this process's
// stderr — not something this codebase logs) fire during or after a
// visible shutdown instead of quietly, before the user ever notices
// the window is gone.
func (w *window) Close() error {
	w.closingFromGo.Store(true)

	// Called from the owning thread — from an OnMessage or OnClosed
	// callback, which run there — there is no other thread to wait for
	// and nothing to wait on: this goroutine IS the message loop, so a
	// posted WM_CLOSE would never be dispatched and the wait below would
	// block until the timeout with the window still open. Dispatch it
	// synchronously instead. SendMessage to a window owned by the
	// calling thread calls wndProc directly, so the teardown still runs
	// on the one thread allowed to run it.
	if currentThreadID() == w.threadID {
		sendMessage(w.hwnd, wmClose, 0, 0)
		return nil
	}

	postMessage(w.hwnd, wmClose, 0, 0)
	select {
	case <-w.closedCh:
	case <-time.After(closeTeardownTimeout):
	}
	return nil
}

// Handle implements Window.Handle (window.go). w.hwnd is set once, in
// run, before NewWindow's ready channel is signalled — safe to read
// without synchronisation for the window's entire remaining lifetime.
func (w *window) Handle() uintptr {
	return w.hwnd
}
