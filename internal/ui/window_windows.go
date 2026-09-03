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
import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"sync/atomic"
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

	webMessageHandler *webMessageReceivedHandler // see coreWebView2AddWebMessageReceived's doc comment: must outlive the subscription

	widthPts, heightPts int

	workCh   chan func()
	closedCh chan struct{}

	onClosed      func()
	closingFromGo atomic.Bool
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
		workCh:   make(chan func(), 8),
		closedCh: make(chan struct{}),
		onClosed: opts.OnClosed,
	}
	ready := make(chan error, 1)
	go w.run(opts, ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return w, nil
}

func (w *window) run(opts Options, ready chan<- error) {
	runtime.LockOSThread()
	// Deliberately never unlocked: this goroutine and the OS thread it
	// is pinned to live exactly as long as the window. Ending the
	// goroutine (after WM_QUIT, at the bottom of this function) ends the
	// thread too, which is correct — nothing else may use this STA
	// apartment once the window that owns it is gone.

	ensureDPIAware()
	if err := coInitialize(); err != nil {
		ready <- err
		return
	}
	registerWindowClass()

	dpi := uint32(96)
	hwnd, err := w.createNativeWindow(opts, dpi)
	if err != nil {
		coUninitialize()
		ready <- err
		return
	}
	w.hwnd = hwnd
	setWindowUserData(hwnd, unsafe.Pointer(w))

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
		w.closeWebView()
		// DestroyWindow dispatches WM_DESTROY synchronously on this same
		// thread, which is what actually calls coUninitialize (wndProc's
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

	if opts.OnMessage != nil {
		h, err := coreWebView2AddWebMessageReceived(cw2, opts.OnMessage)
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
		if err := coreWebView2Navigate(cw2, "https://"+opts.VirtualHost+opts.StartPage); err != nil {
			return fmt.Errorf("ui: Navigate: %w", err)
		}
		pumpUntil(func() bool { return navDone.done })
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

	mon := cursorMonitorRect()
	x := mon.Left + ((mon.Right - mon.Left - winW) / 2)
	y := mon.Top + ((mon.Bottom - mon.Top - winH) / 2)

	titlePtr, err := windows.UTF16PtrFromString(opts.Title)
	if err != nil {
		return 0, err
	}
	hwnd, _, callErr := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(windowClassName)),
		uintptr(unsafe.Pointer(titlePtr)),
		style,
		uintptr(x), uintptr(y), uintptr(winW), uintptr(winH),
		0, 0, moduleHandle(), 0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("ui: CreateWindowExW: %v", callErr)
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
			fn()
		default:
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
		// F5 §2.3: closing the window is equivalent to Cancel. A
		// Go-initiated Close (window.go's Close method) sets
		// closingFromGo first, so OnClosed only fires for a user-driven
		// close (title bar, Alt+F4) — see window.go's doc comment on
		// Options.OnClosed.
		if !w.closingFromGo.Load() && w.onClosed != nil {
			w.onClosed()
		}
		// ICoreWebView2Controller::Close must run before DestroyWindow,
		// not after (in a WM_DESTROY handler): DestroyWindow tears down
		// this window's children — including the WebView2 control's own
		// child HWND — before this window's own WM_DESTROY is
		// dispatched, so calling Close() afterward operates on an
		// already-torn-down control and crashes. Closing explicitly
		// here, first, is the order Microsoft's own samples use.
		w.closeWebView()
		_, _, _ = procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		coUninitialize()
		close(w.closedCh)
		postQuitMessage(0)
		return 0

	default:
		return defWindowProc(hwnd, msg, wparam, lparam)
	}
}

// closeWebView closes the WebView2 controller and releases every
// interface pointer this window holds. Must run before DestroyWindow —
// see the comment on wndProc's wmClose case.
func (w *window) closeWebView() {
	if w.controller != 0 {
		controllerClose(w.controller)
	}
	comRelease(w.cw2v3)
	comRelease(w.cw2)
	comRelease(w.controller)
	comRelease(w.env)
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

// Close implements Window.Close (window.go).
func (w *window) Close() error {
	w.closingFromGo.Store(true)
	postMessage(w.hwnd, wmClose, 0, 0)
	return nil
}
