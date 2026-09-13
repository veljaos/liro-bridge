//go:build windows

package ui

// Every window in this program refuses to become any document but one
// of its own pages.
//
// A WebView2 is a browser, and a browser navigates. Left alone it will
// navigate on a link, on a redirect, on window.open, and — the way this
// was found — on a file dropped onto it, because that is what dropping
// a file on a browser means. Nothing about those paths asks this
// program's permission, and the window that renders the consent screen
// (SPEC §6.5) is the same kind of window as the one that renders
// Settings.
//
// So the rule here is not a list of things to refuse. It is a statement
// of the only thing allowed: the document is a page served from this
// window's own virtual host, and every other navigation is cancelled
// before it starts. A rule shaped that way cannot be got round by a
// vector nobody thought of, which is the whole reason it is shaped that
// way rather than as a check on the drop path alone.
//
// Slot indices are read from WebView2.idl in the Microsoft.Web.WebView2
// NuGet package (1.0.4191.47), never from memory or from prose
// documentation — D-080's discipline. The counts were cross-checked
// against the four slots this package already uses and knows work
// (Navigate 5, add_NavigationCompleted 15, ExecuteScript 29,
// add_WebMessageReceived 34) and against the two IIDs already in
// com_windows.go.

import (
	"log/slog"
	"runtime"
	"strings"
	"unsafe"
)

// allowedNavigation answers whether this window may become the document
// at uri. It is pure, and it is the whole policy: a page of this
// program's own, served from this window's own virtual host.
//
// about:blank is allowed because it is where a WebView2 starts before
// anything has been navigated to, and because an empty document with no
// origin can display nothing.
func allowedNavigation(uri, virtualHost string) bool {
	if strings.EqualFold(uri, "about:blank") {
		return true
	}
	if virtualHost == "" {
		return false
	}
	want := "https://" + virtualHost
	if len(uri) < len(want) || !strings.EqualFold(uri[:len(want)], want) {
		return false
	}
	// The prefix alone is not enough: "https://liro.invalid.example.com"
	// starts with "https://liro.invalid". What follows the host must
	// end it — a path, a query, a fragment, or nothing at all. A port
	// (':') is deliberately not on that list; this program never uses
	// one, so a URI that carries one was not built here.
	rest := uri[len(want):]
	return rest == "" || rest[0] == '/' || rest[0] == '?' || rest[0] == '#'
}

// navigationTarget reduces a URI to the most that may be written to a
// log file about it. SPEC §18.3 keeps file names out of logs, and the
// URI of a dropped document is a file name; so a hierarchical URL gives
// up its scheme and host and nothing else, and everything else (file:,
// data:, blob:) gives up only its scheme.
//
// This is not caution for its own sake. The one navigation this guard
// exists to refuse carries the name of a document somebody is about to
// sign.
func navigationTarget(uri string) (scheme, host string) {
	i := strings.IndexByte(uri, ':')
	if i <= 0 || i > 16 {
		return "malformed", ""
	}
	scheme = strings.ToLower(uri[:i])
	rest := uri[i+1:]
	if !strings.HasPrefix(rest, "//") {
		return scheme, ""
	}
	host = rest[2:]
	if j := strings.IndexAny(host, "/?#"); j >= 0 {
		host = host[:j]
	}
	// Strip userinfo, which can carry anything at all.
	if at := strings.LastIndexByte(host, '@'); at >= 0 {
		host = host[at+1:]
	}
	return scheme, strings.ToLower(host)
}

// --- ICoreWebView2NavigationStartingEventHandler ---
// (IDL uuid 9adbe429-f36d-432b-9ddc-f8881fbd76e3;
// HRESULT Invoke(ICoreWebView2* sender, ICoreWebView2NavigationStartingEventArgs* args);)
//
// The same handler type serves add_NavigationStarting and
// add_FrameNavigationStarting: both deliver the same args interface and
// the rule is the same for a frame as for the top-level document — this
// program's pages contain no frames, so a frame navigating anywhere is
// already something it did not ask for.
type navigationStartingHandler struct {
	comBase
	virtualHost string
	what        string // "navigation" or "frame navigation", for the log line

	// refused counts cancellations, for the tests: a guard that never
	// fires and a guard that is not there look identical from outside.
	refused int
}

func newNavigationStartingHandler(virtualHost, what string) *navigationStartingHandler {
	h := &navigationStartingHandler{virtualHost: virtualHost, what: what}
	h.vtbl = uintptr(unsafe.Pointer(navigationStartingVtbl))
	h.refs = 1
	pinHandler(h)
	return h
}

// navigationStartingInvoke reads get_Uri (args slot 3) and, when the
// answer is not one of this window's own pages, sets put_Cancel (args
// slot 8).
//
// A failure to read the URI cancels. This runs in front of the window
// that renders the consent screen, and the safe answer to "I could not
// tell what this is" is not to show it.
func navigationStartingInvoke(this, _sender, args uintptr) uintptr {
	h := (*navigationStartingHandler)(unsafe.Pointer(this))
	var pin runtime.Pinner
	defer pin.Unpin()

	var uriPtr uintptr
	if _, err := comCall(args, 3, pinPtr(&pin, &uriPtr)); err != nil {
		slog.Error("ui: refusing a "+h.what+" whose target could not be read", "error", err)
		h.cancel(args)
		return sOK
	}
	uri := stringFromLPWSTR(uriPtr)
	if allowedNavigation(uri, h.virtualHost) {
		return sOK
	}

	scheme, host := navigationTarget(uri)
	var userInitiated, redirected uintptr
	_, _ = comCall(args, 4, pinPtr(&pin, &userInitiated)) // get_IsUserInitiated
	_, _ = comCall(args, 5, pinPtr(&pin, &redirected))    // get_IsRedirected
	slog.Warn("ui: refused a "+h.what+" this program did not ask for",
		"scheme", scheme, "host", host,
		"userInitiated", userInitiated != 0, "redirected", redirected != 0)
	h.cancel(args)
	return sOK
}

func (h *navigationStartingHandler) cancel(args uintptr) {
	if _, err := comCall(args, 8, 1); err != nil { // put_Cancel(TRUE)
		slog.Error("ui: put_Cancel failed; the "+h.what+" was not stopped", "error", err)
		return
	}
	h.refused++
}

// --- ICoreWebView2NewWindowRequestedEventHandler ---
// (IDL uuid d4c185fe-c81c-4989-97af-2d3fa7ab5651;
// HRESULT Invoke(ICoreWebView2* sender, ICoreWebView2NewWindowRequestedEventArgs* args);)
//
// window.open, and a link with target="_blank". Left unhandled,
// WebView2 opens a second, bare browser window this program neither
// created nor controls — outside everything window_windows.go arranges
// about ownership, icons, size and teardown. Handled and given no
// window, nothing opens.
type newWindowRequestedHandler struct {
	comBase
	refused int
}

func newNewWindowRequestedHandler() *newWindowRequestedHandler {
	h := &newWindowRequestedHandler{}
	h.vtbl = uintptr(unsafe.Pointer(newWindowRequestedVtbl))
	h.refs = 1
	pinHandler(h)
	return h
}

// newWindowRequestedInvoke sets put_Handled (args slot 6) and leaves
// put_NewWindow unset, which is how WebView2 is told the host dealt
// with the request and no window is to be created.
func newWindowRequestedInvoke(this, _sender, args uintptr) uintptr {
	h := (*newWindowRequestedHandler)(unsafe.Pointer(this))
	var pin runtime.Pinner
	defer pin.Unpin()

	var uriPtr uintptr
	scheme, host := "unknown", ""
	if _, err := comCall(args, 3, pinPtr(&pin, &uriPtr)); err == nil { // get_Uri
		scheme, host = navigationTarget(stringFromLPWSTR(uriPtr))
	}
	if _, err := comCall(args, 6, 1); err != nil { // put_Handled(TRUE)
		slog.Error("ui: put_Handled failed; a new browser window may open", "error", err)
		return sOK
	}
	h.refused++
	slog.Warn("ui: refused to open a second browser window", "scheme", scheme, "host", host)
	return sOK
}

// coreWebView2AddNavigationStarting calls
// ICoreWebView2::add_NavigationStarting (IDL slot 7: 3=get_Settings,
// 4=get_Source, 5=Navigate, 6=NavigateToString, 7=add_NavigationStarting).
func coreWebView2AddNavigationStarting(cw2 uintptr, virtualHost string) (*navigationStartingHandler, error) {
	h := newNavigationStartingHandler(virtualHost, "navigation")
	var pin runtime.Pinner
	defer pin.Unpin()
	var token int64
	if _, err := comCall(cw2, 7, uintptr(unsafe.Pointer(h)), pinPtr(&pin, &token)); err != nil {
		return nil, err
	}
	return h, nil
}

// coreWebView2AddFrameNavigationStarting calls
// ICoreWebView2::add_FrameNavigationStarting (IDL slot 17: 15/16 are
// add/remove_NavigationCompleted, 17=add_FrameNavigationStarting).
func coreWebView2AddFrameNavigationStarting(cw2 uintptr, virtualHost string) (*navigationStartingHandler, error) {
	h := newNavigationStartingHandler(virtualHost, "frame navigation")
	var pin runtime.Pinner
	defer pin.Unpin()
	var token int64
	if _, err := comCall(cw2, 17, uintptr(unsafe.Pointer(h)), pinPtr(&pin, &token)); err != nil {
		return nil, err
	}
	return h, nil
}

// coreWebView2AddNewWindowRequested calls
// ICoreWebView2::add_NewWindowRequested (IDL slot 44: 43=Stop,
// 44=add_NewWindowRequested).
func coreWebView2AddNewWindowRequested(cw2 uintptr) (*newWindowRequestedHandler, error) {
	h := newNewWindowRequestedHandler()
	var pin runtime.Pinner
	defer pin.Unpin()
	var token int64
	if _, err := comCall(cw2, 44, uintptr(unsafe.Pointer(h)), pinPtr(&pin, &token)); err != nil {
		return nil, err
	}
	return h, nil
}
