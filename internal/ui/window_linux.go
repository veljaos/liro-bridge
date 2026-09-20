//go:build linux

package ui

// The GTK4 + WebKitGTK 6.0 host for every window this program has
// (F12 §3). The Windows counterpart is window_windows.go; the contract
// both satisfy is window.go's Window interface, and where the two
// platforms differ the difference is stated here rather than smoothed
// over.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"time"

	javascriptcore "github.com/diamondburned/gotk4-webkitgtk/pkg/javascriptcore/v6"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// pageLoadTimeout bounds NewWindow and Navigate. Both are documented as
// blocking until the page's scripts have run; neither may block for
// ever, because the page they are waiting for is one of ours and a page
// of ours that never finishes loading is a defect rather than a wait.
const pageLoadTimeout = 30 * time.Second

// ErrDropNotImplemented is returned by NewWindow when Options.
// OnFilesDropped is set. F6's drop target is GTK4's GtkDropTarget and
// is not wired yet.
//
// It is an error and not a silent no-op deliberately: a window that
// became a drop target on Windows and quietly did not on Linux would
// present a caller with a feature that works on one machine and does
// nothing on another, which is the shape of bug that takes a week to
// find. Failing where the window is created names it immediately.
var ErrDropNotImplemented = errors.New("ui: dropped files are not implemented on Linux yet (F12 §3, F6 §1)")

// liveWindows maps a WebKitWebView's GObject address to the window that
// owns it.
//
// It exists because the liro:// scheme handler is registered once per
// process, on the default web context, while what a request may reach
// belongs to one window — two windows can map the same VirtualHost onto
// different content. WebKitURISchemeRequest.WebView() says which view
// asked, and this says which window that is.
var liveWindows = struct {
	sync.Mutex
	m map[uintptr]*linuxWindow
}{m: make(map[uintptr]*linuxWindow)}

// registerAssetSchemeOnce registers liro:// on the default web context.
// Once per process: registering a scheme twice is a WebKit warning and
// the second handler silently never runs.
var registerAssetSchemeOnce sync.Once

type linuxWindow struct {
	win  *gtk.Window
	view *webkit.WebView

	// viewAddr is the key this window is registered under in
	// liveWindows, kept so Close can remove it without asking the
	// binding for the address of an object it is destroying.
	viewAddr uintptr

	// hosts is what this window's pages may reach, by hostname.
	hosts map[string]fs.FS

	// events carries callbacks to the one goroutine that delivers
	// them. Per window, in order, and never on the UI thread — the
	// same guarantee window_windows.go's dispatchEvents makes, for the
	// same reason: a caller's handler that blocks must delay later
	// callbacks and nothing else.
	events chan func()

	closeOnce sync.Once
	closed    chan struct{}
}

// NewWindow creates one window hosting one WebKitGTK view.
func NewWindow(opts Options) (Window, error) {
	if opts.OnFilesDropped != nil {
		return nil, ErrDropNotImplemented
	}
	if opts.Assets == nil || opts.VirtualHost == "" {
		return nil, errors.New("ui: Options.Assets and Options.VirtualHost are both required")
	}
	if err := theUIThread.start(); err != nil {
		return nil, err
	}

	w := &linuxWindow{
		hosts:  map[string]fs.FS{opts.VirtualHost: opts.Assets},
		events: make(chan func(), 64),
		closed: make(chan struct{}),
	}
	if opts.ScratchHost != "" && opts.ScratchDir != "" {
		w.hosts[opts.ScratchHost] = os.DirFS(opts.ScratchDir)
	}

	go w.dispatchEvents()

	loaded := make(chan error, 1)

	if err := theUIThread.do(func() {
		registerAssetSchemeOnce.Do(registerAssetScheme)

		w.view = webkit.NewWebView()
		w.viewAddr = coreglib.BaseObject(w.view).Native()

		liveWindows.Lock()
		liveWindows.m[w.viewAddr] = w
		liveWindows.Unlock()

		w.connectMessages(opts.OnMessage)
		w.connectNavigationPolicy()
		w.connectLoad(loaded)

		w.win = gtk.NewWindow()
		w.win.SetTitle(opts.Title)
		w.win.SetDefaultSize(opts.Width, opts.Height)
		// F5 §2.3: every window this program has is fixed-size.
		w.win.SetResizable(false)
		w.win.SetChild(w.view)

		if opts.AlwaysOnTop {
			// **Not honoured, and this is a decision rather than a
			// gap.** F12 §4: there is no always-on-top on Wayland —
			// no protocol lets a client raise itself, the compositor
			// decides, and no amount of GTK will change that.
			//
			// It is not an error either, unlike OnFilesDropped above,
			// because §4 has already designed the replacement: a new
			// window per request rather than a hidden one shown again,
			// a desktop notification alongside, and nothing in the
			// consent argument resting on the window being in front.
			// A caller setting this flag is asking for something the
			// platform does not have and §4 supplies differently, so
			// the flag is recorded and ignored rather than refused.
			//
			// The X11 fallback that would restore it is refused on
			// security grounds (§4.1): under X11 any local client can
			// send synthetic input to any window, so xdotool could
			// click Approve. Native Wayland is the safer target and
			// the harder path is the more secure one.
			slog.Debug("ui: Options.AlwaysOnTop has no effect on Wayland (F12 §4)", "title", opts.Title)
		}

		if owner := windowByHandle(opts.Owner); owner != nil {
			// GTK's own word for Options.Owner. It gets the two
			// behaviours the Owner doc comment says matter and that a
			// bare "new window" does not: kept above its owner by the
			// compositor, and centred on it.
			w.win.SetTransientFor(owner.win)
		}

		w.win.ConnectCloseRequest(func() bool {
			// The user closed it. F5 §2.3: that is a Cancel, and the
			// caller is told through the same ordered channel as
			// everything else.
			w.emit(opts.OnClosed)
			w.markClosed()
			return false // false: let GTK destroy the window.
		})

		w.view.LoadURI(assetScheme + "://" + opts.VirtualHost + "/" + trimLeadingSlash(opts.StartPage))
		w.win.Present()
	}); err != nil {
		return nil, err
	}

	select {
	case err := <-loaded:
		if err != nil {
			_ = w.Close()
			return nil, err
		}
	case <-time.After(pageLoadTimeout):
		_ = w.Close()
		return nil, fmt.Errorf("ui: %q did not finish loading within %s", opts.StartPage, pageLoadTimeout)
	}

	return w, nil
}

func trimLeadingSlash(p string) string {
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	return p
}

// windowByHandle finds a live window by the value its Handle() returned.
// Zero, and anything this process does not own, is nil — a caller that
// names a window that has gone is given an unowned window rather than
// an error, which is what closing the owner first already means.
func windowByHandle(h uintptr) *linuxWindow {
	if h == 0 {
		return nil
	}
	liveWindows.Lock()
	defer liveWindows.Unlock()
	for _, w := range liveWindows.m {
		if w.win != nil && coreglib.BaseObject(w.win).Native() == h {
			return w
		}
	}
	return nil
}

// registerAssetScheme wires liro:// on the default context. On the UI
// thread, once.
func registerAssetScheme() {
	ctx := webkit.WebContextGetDefault()

	// A custom scheme is "opaque" to WebKit unless it is told
	// otherwise, and an opaque origin is not a secure context — which
	// silently disables a list of web APIs that grows with every
	// release. These pages are served from the binary over a scheme
	// that cannot leave the process, so they are as local and as
	// trusted as content gets.
	sm := ctx.SecurityManager()
	sm.RegisterURISchemeAsLocal(assetScheme)
	sm.RegisterURISchemeAsSecure(assetScheme)

	ctx.RegisterURIScheme(assetScheme, func(request *webkit.URISchemeRequest) {
		uri := request.URI()

		var hosts map[string]fs.FS
		if v := request.WebView(); v != nil {
			liveWindows.Lock()
			if w := liveWindows.m[coreglib.BaseObject(v).Native()]; w != nil {
				hosts = w.hosts
			}
			liveWindows.Unlock()
		}
		if hosts == nil {
			// A request from a view this process does not know about.
			// Refused rather than served from some other window's
			// content.
			request.FinishError(fmt.Errorf("ui: no window owns this request: %s", uri))
			return
		}

		data, contentType, err := resolveAsset(uri, hosts)
		if err != nil {
			slog.Warn("ui: refused an asset request", "uri", uri, "err", err)
			request.FinishError(err)
			return
		}

		// A GMemoryInputStream over a GBytes: WebKit takes a reference
		// and reads it on its own schedule, so the bytes must not be
		// Go memory it could outlive.
		stream := gio.NewMemoryInputStreamFromBytes(glib.NewBytes(data))
		request.Finish(stream, int64(len(data)), contentType)
	})
}

func (w *linuxWindow) connectMessages(onMessage func(Message)) {
	ucm := w.view.UserContentManager()

	// D-083's three-message surface. The handler name is what the page
	// posts to: window.webkit.messageHandlers.liro.postMessage(...).
	if !ucm.RegisterScriptMessageHandler("liro", "") {
		slog.Warn("ui: the page->Go message handler could not be registered")
	}

	ucm.ConnectScriptMessageReceived(func(value *javascriptcore.Value) {
		// The page is only ever allowed to send one of three things,
		// and validation happens in Go before anything is acted on
		// (F5 §2.4).
		//
		// ToJson is the deliberate analogue of WebView2's
		// WebMessageAsJson: it serialises whatever the page passed.
		// assets/bridge.js passes the *object* and its comment records
		// why — a pre-stringified argument arrives as a string message
		// and is therefore JSON-encoded one level deeper than
		// ParseMessage expects, which once dropped every
		// approve/cancel/selectCertificate click silently.
		//
		// **So this does not unwrap a string, on purpose.** Being
		// lenient here would make that same page bug work on Linux and
		// fail on Windows, which is worse than failing on both: the
		// platforms would disagree about a message surface, and the
		// bug would be invisible on whichever one someone tested.
		raw := value.ToJson(0)
		w.emit(func() { dispatchMessage(onMessage, []byte(raw)) })
	})
}

// connectNavigationPolicy refuses to let a window become any document
// but one of its own pages (D-259).
func (w *linuxWindow) connectNavigationPolicy() {
	w.view.ConnectDecidePolicy(func(decision webkit.PolicyDecisioner, kind webkit.PolicyDecisionType) bool {
		if kind != webkit.PolicyDecisionTypeNavigationAction &&
			kind != webkit.PolicyDecisionTypeNewWindowAction {
			return false
		}
		nav, ok := decision.(*webkit.NavigationPolicyDecision)
		if !ok {
			return false
		}
		uri := nav.NavigationAction().Request().URI()

		if _, _, err := resolveAsset(uri, w.hosts); err != nil {
			slog.Warn("ui: refused a navigation", "uri", uri, "err", err)
			webkit.BasePolicyDecision(decision).Ignore()
			return true
		}
		return false
	})
}

func (w *linuxWindow) connectLoad(first chan<- error) {
	var once sync.Once
	w.view.ConnectLoadChanged(func(e webkit.LoadEvent) {
		if e == webkit.LoadFinished {
			once.Do(func() { first <- nil })
		}
	})
	w.view.ConnectLoadFailed(func(_ webkit.LoadEvent, uri string, err error) bool {
		once.Do(func() { first <- fmt.Errorf("ui: loading %s: %w", uri, err) })
		return false
	})
}

// dispatchEvents delivers every caller callback for this window, one at
// a time, in order, off the UI thread.
func (w *linuxWindow) dispatchEvents() {
	for f := range w.events {
		f()
	}
}

func (w *linuxWindow) emit(f func()) {
	if f == nil {
		return
	}
	select {
	case w.events <- f:
	case <-w.closed:
	}
}

func (w *linuxWindow) markClosed() {
	w.closeOnce.Do(func() {
		close(w.closed)
		liveWindows.Lock()
		delete(liveWindows.m, w.viewAddr)
		liveWindows.Unlock()
		close(w.events)
	})
}

func (w *linuxWindow) PostJSON(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("ui: encoding a message for the page: %w", err)
	}
	// JSON.parse, never string concatenation with caller data in it
	// (F5 §2.4). The payload is itself JSON-encoded a second time so
	// that it arrives as one string literal.
	quoted, err := json.Marshal(string(payload))
	if err != nil {
		return fmt.Errorf("ui: encoding a message for the page: %w", err)
	}
	_, err = w.Eval("__liroReceive(" + string(quoted) + ")")
	return err
}

// Eval runs script in the page and returns the engine's JSON encoding
// of its result.
//
// **The script must end in something JSON can encode.** WebKit refuses
// to marshal a host object back across evaluate_javascript and reports
// `Unsupported result type`; measured, on this machine:
//
//	void 0                               -> "null"
//	undefined                            -> "null"
//	document.body                        -> Unsupported result type
//	window.webkit.messageHandlers.liro   -> Unsupported result type
//	…postMessage({…})                    -> Unsupported result type
//
// So a script whose last expression is a DOM node or a WebKit API
// object — including a bare call to postMessage, whose return value is
// one — needs `; void 0` after it.
//
// **This is reported as an error rather than as the JSON "null" the
// Windows side would produce**, and the divergence is deliberate.
// window.go's contract describes WebView2's behaviour, where an
// unencodable value and a thrown exception both come back as "null".
// Answering that way here would mean a script that is simply wrong —
// a typo'd property, an exception — is indistinguishable from one that
// returned nothing. This package has already paid for a silent drop
// once: assets/bridge.js carries the comment about every
// approve/cancel/selectCertificate click being discarded without a
// word. An error that names the problem is worth the divergence, and
// the divergence is recorded rather than smoothed over.
func (w *linuxWindow) Eval(script string) (string, error) {
	select {
	case <-w.closed:
		return "", ErrWindowClosed
	default:
	}
	return evalInWebView(w.view, script)
}

func (w *linuxWindow) Navigate(page string) error {
	select {
	case <-w.closed:
		return ErrWindowClosed
	default:
	}

	done := make(chan error, 1)
	var once sync.Once
	var handle coreglib.SignalHandle

	if err := theUIThread.do(func() {
		handle = w.view.ConnectLoadChanged(func(e webkit.LoadEvent) {
			if e == webkit.LoadFinished {
				once.Do(func() { done <- nil })
			}
		})
		w.view.LoadURI(assetScheme + "://" + w.startHost() + "/" + trimLeadingSlash(page))
	}); err != nil {
		return err
	}

	defer func() {
		_ = theUIThread.do(func() { coreglib.BaseObject(w.view).HandlerDisconnect(handle) })
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(pageLoadTimeout):
		return fmt.Errorf("ui: %q did not finish loading within %s", page, pageLoadTimeout)
	case <-w.closed:
		return ErrWindowClosed
	}
}

// startHost is the window's own virtual host — the one Navigate's
// argument is a path within. Only that content can be navigated to, so
// nothing a caller passes can send the window off the machine.
func (w *linuxWindow) startHost() string {
	for h := range w.hosts {
		return h
	}
	return ""
}

func (w *linuxWindow) Resize(width, height int) error {
	select {
	case <-w.closed:
		return ErrWindowClosed
	default:
	}
	return theUIThread.do(func() {
		// GTK keeps the window where it is; there is no re-centring to
		// suppress, which is what Resize's contract asks for.
		w.win.SetDefaultSize(width, height)
	})
}

func (w *linuxWindow) Close() error {
	select {
	case <-w.closed:
		return nil // idempotent
	default:
	}
	err := theUIThread.do(func() {
		if w.win != nil {
			w.win.Destroy()
		}
	})
	w.markClosed()
	return err
}

// Handle returns this window's GtkWindow address.
//
// **It is not an OS window handle and nothing on this platform consumes
// it.** Handle exists for one caller: parenting the Windows CNG PIN
// prompt through NCRYPT_WINDOW_HANDLE_PROPERTY, and there is no CNG on
// Linux (SPEC §11.11) — the PIN comes from PKCS#11 and F12 §5's dialog.
// Wayland has no such thing as a window id a client may hand to
// another process even in principle.
//
// It is non-zero, as the contract requires, and it is the value
// Options.Owner is matched against, so "the window this one was opened
// from" keeps working. That is the whole of what it is for here.
func (w *linuxWindow) Handle() uintptr {
	if w.win == nil {
		return 0
	}
	return coreglib.BaseObject(w.win).Native()
}

// The rest of this package's platform surface, for Linux.
//
// Each is a deliberate not-yet rather than an oversight, and says which
// phase owns it, because "returns ErrUnsupportedPlatform" on a platform
// the program now genuinely runs on is otherwise indistinguishable from
// something nobody noticed.

// detectRuntime reports whether a window could actually be shown.
//
// On Windows this asks whether the WebView2 Evergreen Runtime is
// installed. Linux has no such thing to install — WebKitGTK is a
// declared package dependency (SPEC §1.1) and is linked, so if this
// binary is running it is present. The question that *is* worth asking
// here is the one D-324 found: whether GTK can reach a display at all.
// A machine where it cannot is exactly the case DetectRuntime exists
// for, and ShowWarning is how it gets said.
func detectRuntime() (bool, string, error) {
	if err := theUIThread.start(); err != nil {
		return false, "", err
	}
	return true, "", nil
}

// showNativeMessage has no Linux implementation yet: F12 §5 decides
// which native dialog this program uses, and the message box and the
// PIN dialog are the same decision. Until then a caller that cannot
// show a window has nowhere to say so, which is recorded rather than
// papered over with a GtkMessageDialog nobody chose.
func showNativeMessage(title, body string, _ bool) {
	slog.Warn("ui: no native message box on Linux yet (F12 §5)", "title", title, "body", body)
}

// pickFolder and pickFiles are F12's and are not this section's. The
// GTK4 answer is GtkFileDialog, which is asynchronous like everything
// else in this binding and therefore lands with the same shape the
// evaluate_javascript bridge has (D-330).
func pickFolder(uintptr, string, string) (string, bool, error) {
	return "", false, ErrUnsupportedPlatform
}

func pickFiles(uintptr, string, string, string) ([]string, bool, error) {
	return nil, false, ErrUnsupportedPlatform
}

// iconFilePath has no Linux implementation: there is no Explorer to
// register a menu icon with, and §8's desktop entry names the icon by
// XDG theme name rather than by path.
func iconFilePath() (string, error) { return "", ErrUnsupportedPlatform }
