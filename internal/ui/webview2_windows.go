//go:build windows

package ui

// The handler objects this package implements (direction 2 from
// com_windows.go's doc comment), and the thin Go wrappers around the
// WebView2 interfaces this package calls into (direction 1). Every slot
// index is annotated with the WebView2.idl interface and method it
// corresponds to, in declaration order starting after IUnknown's three
// slots (0=QueryInterface, 1=AddRef, 2=Release) — see D-080.
import (
	"encoding/json"
	"fmt"
	"log/slog"
	"unsafe"

	"golang.org/x/sys/windows"
)

// --- ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler ---
// (IDL: interface ...EnvironmentCompletedHandler : IUnknown { HRESULT
// Invoke(HRESULT errorCode, ICoreWebView2Environment* result); })

type environmentCompletedHandler struct {
	comBase
	hr   uintptr
	env  uintptr
	done bool
}

func newEnvironmentCompletedHandler() *environmentCompletedHandler {
	h := &environmentCompletedHandler{}
	h.vtbl = uintptr(unsafe.Pointer(environmentCompletedVtbl))
	h.refs = 1
	return h
}

// environmentCompletedInvoke is called by the WebView2 runtime,
// synchronously, from inside this thread's own message loop (COM
// apartment marshaling delivers it via a posted message — see
// window_windows.go's package doc comment). It never runs concurrently
// with the loop that reads h.done, so plain field writes are safe with
// no atomics.
// environmentCompletedInvoke must AddRef env itself before returning:
// the pointer WebView2Loader hands to a *CompletedHandler is a borrowed
// reference, valid only for the duration of this call, exactly like any
// other COM [out] parameter with no [retval] — [retval] out-parameters
// (ordinary property getters) transfer a reference already accounted
// for by convention, but this is not one. Without an explicit AddRef
// here, the pointer this package keeps and uses later (once pumpUntil,
// win32_windows.go, returns control to createEnvironment) would be
// dangling.
func environmentCompletedInvoke(this, hr, env uintptr) uintptr {
	h := (*environmentCompletedHandler)(unsafe.Pointer(this))
	if int32(hr) >= 0 && env != 0 {
		_, _ = comCall(env, 1) // AddRef
	}
	h.hr, h.env, h.done = hr, env, true
	return sOK
}

// --- ICoreWebView2CreateCoreWebView2ControllerCompletedHandler ---
// (IDL: HRESULT Invoke(HRESULT errorCode, ICoreWebView2Controller* result);)

type controllerCompletedHandler struct {
	comBase
	hr         uintptr
	controller uintptr
	done       bool
}

func newControllerCompletedHandler() *controllerCompletedHandler {
	h := &controllerCompletedHandler{}
	h.vtbl = uintptr(unsafe.Pointer(controllerCompletedVtbl))
	h.refs = 1
	return h
}

// controllerCompletedInvoke: see environmentCompletedInvoke's comment —
// the same borrowed-reference rule applies to the controller pointer.
func controllerCompletedInvoke(this, hr, controller uintptr) uintptr {
	h := (*controllerCompletedHandler)(unsafe.Pointer(this))
	if int32(hr) >= 0 && controller != 0 {
		_, _ = comCall(controller, 1) // AddRef
	}
	h.hr, h.controller, h.done = hr, controller, true
	return sOK
}

// --- ICoreWebView2NavigationCompletedEventHandler ---
// (IDL: HRESULT Invoke(ICoreWebView2* sender, ICoreWebView2NavigationCompletedEventArgs* args);)
//
// Subscribed once per window, before Navigate is called, so that
// setUpWebView2 (window_windows.go) can pump the message loop until the
// very first navigation has actually finished before returning from
// NewWindow. Without this, ExecuteScript (PostJSON's init call, the one
// carrying every localised string — F5 §2.4) races Navigate: Navigate
// only starts the load, so a script run immediately afterward can
// execute against the still-loading (or, worse, prior about:blank)
// document, before the page's own <script> tags have defined
// window.__liroReceive — "window.__liroReceive &&" then silently no-ops
// and every data-i18n label is left blank while the native window title
// and any text hard-coded in the HTML render normally. Verified directly
// against the running binary, not inferred: see docs/decisions.md.
type navigationCompletedHandler struct {
	comBase
	done bool
}

func newNavigationCompletedHandler() *navigationCompletedHandler {
	h := &navigationCompletedHandler{}
	h.vtbl = uintptr(unsafe.Pointer(navigationCompletedVtbl))
	h.refs = 1
	return h
}

// navigationCompletedInvoke deliberately ignores args (IsSuccess /
// WebErrorStatus): a failed navigation still unblocks setUpWebView2 —
// there is nothing more useful to do here than let the page load
// whatever it managed to load and let the rest of the window behave
// exactly as it would for any other broken page.
func navigationCompletedInvoke(this, _sender, _args uintptr) uintptr {
	h := (*navigationCompletedHandler)(unsafe.Pointer(this))
	h.done = true
	return sOK
}

// --- ICoreWebView2WebMessageReceivedEventHandler ---
// (IDL: HRESULT Invoke(ICoreWebView2* sender, ICoreWebView2WebMessageReceivedEventArgs* args);)

type webMessageReceivedHandler struct {
	comBase
	onMessage func(Message)
}

func newWebMessageReceivedHandler(onMessage func(Message)) *webMessageReceivedHandler {
	h := &webMessageReceivedHandler{onMessage: onMessage}
	h.vtbl = uintptr(unsafe.Pointer(webMessageReceivedVtbl))
	h.refs = 1
	return h
}

// webMessageReceivedInvoke reads WebMessageAsJson (IDL:
// ICoreWebView2WebMessageReceivedEventArgs, [propget] WebMessageAsJson,
// slot 4 = 3 (get_Source) + 1) and hands the raw bytes to
// dispatchMessage (messages.go), which validates before anything is
// acted on (F5 §2.4).
func webMessageReceivedInvoke(this, _sender, args uintptr) uintptr {
	h := (*webMessageReceivedHandler)(unsafe.Pointer(this))
	var jsonPtr uintptr
	if _, err := comCall(args, 4, uintptr(unsafe.Pointer(&jsonPtr))); err != nil {
		slog.Warn("ui: WebMessageAsJson failed", "error", err)
		return sOK
	}
	raw := stringFromLPWSTR(jsonPtr)
	dispatchMessage(h.onMessage, []byte(raw))
	return sOK
}

// --- ICoreWebView2ExecuteScriptCompletedHandler ---
// (IDL: HRESULT Invoke(HRESULT errorCode, LPCWSTR result);)
//
// A fresh instance is created per ExecuteScript call (window_windows.go's
// coreWebView2Eval), mirroring environmentCompletedHandler/
// controllerCompletedHandler above — the shared vtable singleton
// (com_windows.go) is what stays fixed, not the object. This exists
// because Window.Eval (used by the settings window to read back its
// form state through ExecuteScript's own return-value channel, rather
// than widening the page->Go message surface past the three types F5
// §2.4 specifies — see D-08x) needs each call's own result, not a
// shared, overwritten-by-the-next-call field.
type executeScriptCompletedHandler struct {
	comBase
	hr     uintptr
	result string
	done   bool
}

func newExecuteScriptCompletedHandler() *executeScriptCompletedHandler {
	h := &executeScriptCompletedHandler{}
	h.vtbl = uintptr(unsafe.Pointer(executeScriptCompletedVtbl))
	h.refs = 1
	return h
}

// executeScriptCompletedInvoke's result parameter is `[in] LPCWSTR`,
// not `[out, retval]` (contrast WebMessageAsJson above) — WebView2.idl
// documents ownership only for retval out-parameters ("the caller must
// free with CoTaskMemFree"); an [in] parameter is the callee's own
// memory for the duration of the call and must not be freed here, so
// its content is copied out (windows.UTF16PtrToString) rather than
// retaining the pointer.
func executeScriptCompletedInvoke(this, hr, result uintptr) uintptr {
	h := (*executeScriptCompletedHandler)(unsafe.Pointer(this))
	if int32(hr) < 0 {
		slog.Warn("ui: ExecuteScript reported failure", "hresult", fmt.Sprintf("0x%08X", uint32(hr)))
	} else if result != 0 {
		h.result = windows.UTF16PtrToString((*uint16)(unsafe.Pointer(result)))
	}
	h.hr, h.done = hr, true
	return sOK
}

// --- Thin wrappers over the interfaces this package calls into ---

// environmentCreateController calls
// ICoreWebView2Environment::CreateCoreWebView2Controller (IDL slot 3)
// and pumps the message loop until the completion handler fires.
func environmentCreateController(env uintptr, hwnd uintptr) (uintptr, error) {
	h := newControllerCompletedHandler()
	if _, err := comCall(env, 3, hwnd, uintptr(unsafe.Pointer(h))); err != nil {
		return 0, fmt.Errorf("CreateCoreWebView2Controller: %w", err)
	}
	pumpUntil(func() bool { return h.done })
	if int32(h.hr) < 0 {
		return 0, fmt.Errorf("CreateCoreWebView2Controller completed with HRESULT 0x%08X", uint32(h.hr))
	}
	return h.controller, nil
}

// controllerSetBounds calls ICoreWebView2Controller::put_Bounds (IDL
// slot 6: 3=get_IsVisible? no — slots: 3 propget IsVisible, 4 propput
// IsVisible, 5 propget Bounds, 6 propput Bounds).
func controllerSetBounds(controller uintptr, x, y, w, h int32) error {
	r := rect{Left: x, Top: y, Right: x + w, Bottom: y + h}
	_, err := comCall(controller, 6, uintptr(unsafe.Pointer(&r)))
	return err
}

// controllerSetVisible calls ICoreWebView2Controller::put_IsVisible
// (IDL slot 4).
func controllerSetVisible(controller uintptr, visible bool) error {
	v := uintptr(0)
	if visible {
		v = 1
	}
	_, err := comCall(controller, 4, v)
	return err
}

// controllerClose calls ICoreWebView2Controller::Close (IDL slot 24:
// counted from put_IsVisible=4 through the 21 methods listed in
// WebView2.idl before Close).
func controllerClose(controller uintptr) { _, _ = comCall(controller, 24) }

// controllerGetCoreWebView2 calls
// ICoreWebView2Controller::get_CoreWebView2 (IDL slot 25, immediately
// after Close).
func controllerGetCoreWebView2(controller uintptr) (uintptr, error) {
	var out uintptr
	if _, err := comCall(controller, 25, uintptr(unsafe.Pointer(&out))); err != nil {
		return 0, err
	}
	return out, nil
}

// coreWebView2SetVirtualHost calls
// ICoreWebView2_3::SetVirtualHostNameToFolderMapping (IDL slot 71: base
// ICoreWebView2's 58 methods [3..60], plus ICoreWebView2_2's own 7
// [61..67], plus TrySuspend/Resume/get_IsSuspended [68..70], then this
// method). cw2 must already be the ICoreWebView2_3-queried pointer
// (window_windows.go does this once at setup), not the base
// ICoreWebView2 pointer — slot 71 does not exist on the base vtable.
func coreWebView2SetVirtualHost(cw2v3 uintptr, hostName, folderPath string, accessKindDeny uintptr) error {
	h, hBuf, err := utf16Ptr(hostName)
	if err != nil {
		return err
	}
	f, fBuf, err := utf16Ptr(folderPath)
	if err != nil {
		return err
	}
	_, err = comCall(cw2v3, 71, uintptr(unsafe.Pointer(h)), uintptr(unsafe.Pointer(f)), accessKindDeny)
	_ = hBuf
	_ = fBuf
	return err
}

// coreWebView2Navigate calls ICoreWebView2::Navigate (IDL slot 5:
// 3=get_Settings, 4=get_Source, 5=Navigate).
func coreWebView2Navigate(cw2 uintptr, uri string) error {
	u, uBuf, err := utf16Ptr(uri)
	if err != nil {
		return err
	}
	_, err = comCall(cw2, 5, uintptr(unsafe.Pointer(u)))
	_ = uBuf
	return err
}

// coreWebView2AddNavigationCompleted calls
// ICoreWebView2::add_NavigationCompleted (IDL slot 15: 5=Navigate,
// 6=NavigateToString, 7/8=add/remove_NavigationStarting,
// 9/10=add/remove_ContentLoading, 11/12=add/remove_SourceChanged,
// 13/14=add/remove_HistoryChanged, 15=add_NavigationCompleted). Must be
// called before coreWebView2Navigate so the subscription is active
// before the navigation it needs to observe starts.
func coreWebView2AddNavigationCompleted(cw2 uintptr) (*navigationCompletedHandler, error) {
	h := newNavigationCompletedHandler()
	var token uintptr
	if _, err := comCall(cw2, 15, uintptr(unsafe.Pointer(h)), uintptr(unsafe.Pointer(&token))); err != nil {
		return nil, err
	}
	return h, nil
}

// coreWebView2AddWebMessageReceived calls
// ICoreWebView2::add_WebMessageReceived (IDL slot 34 — counted directly
// against the method list in the package doc comment of
// webview2_windows.go's source; see also com_windows.go's package doc).
// The returned *webMessageReceivedHandler must be kept alive by the
// caller (window_windows.go stores it on the Window) for as long as the
// subscription lives: WebView2 holds a raw pointer to it, not a Go
// reference, so nothing stops the garbage collector from reclaiming an
// otherwise-unreferenced handler object out from under the runtime —
// which then corrupts whatever WebView2 finds at that address the next
// time it touches the handler (observed directly: ICoreWebView2Controller::Close
// releases every registered event handler as part of closing, and an
// uncollected-but-unreferenced handler crashed there with SEH
// 0xc0000005 before this field was added — see D-080).
func coreWebView2AddWebMessageReceived(cw2 uintptr, onMessage func(Message)) (*webMessageReceivedHandler, error) {
	h := newWebMessageReceivedHandler(onMessage)
	var token uintptr // EventRegistrationToken is a single __int64; 8 bytes fits one uintptr on amd64
	_, err := comCall(cw2, 34, uintptr(unsafe.Pointer(h)), uintptr(unsafe.Pointer(&token)))
	if err != nil {
		return nil, err
	}
	return h, nil
}

// coreWebView2ExecuteScript calls ICoreWebView2::ExecuteScript (IDL
// slot 29: 27=AddScriptToExecuteOnDocumentCreated,
// 28=RemoveScriptToExecuteOnDocumentCreated, 29=ExecuteScript) and pumps
// the message loop (like environmentCreateController above) until the
// completion handler fires, returning the script's JSON-encoded result
// — ExecuteScript's own result value, not a page->Go message (see
// executeScriptCompletedHandler's doc comment for why this channel,
// not a new Message type, backs Window.Eval).
func coreWebView2ExecuteScript(cw2 uintptr, script string) (string, error) {
	s, sBuf, err := utf16Ptr(script)
	if err != nil {
		return "", err
	}
	h := newExecuteScriptCompletedHandler()
	_, err = comCall(cw2, 29, uintptr(unsafe.Pointer(s)), uintptr(unsafe.Pointer(h)))
	_ = sBuf
	if err != nil {
		return "", err
	}
	pumpUntil(func() bool { return h.done })
	if int32(h.hr) < 0 {
		return "", fmt.Errorf("ExecuteScript completed with HRESULT 0x%08X", uint32(h.hr))
	}
	return h.result, nil
}

// postJSONScript builds the tiny script PostJSON runs: JSON.parse the
// payload (never string-concatenated into the script) and hand it to
// the page's own __liroReceive function. json.Marshal already produces
// valid JS-string-literal-safe output for embedding inside a
// single-quoted... no: to avoid any string-escaping subtlety at the JS
// layer, the payload itself is passed as a *second* JSON-encoded
// argument to Function, decoded with JSON.parse on the page side — this
// is the "never build JavaScript by string concatenation with user
// data in it" rule from F5 §2.4 applied literally: the only thing
// concatenated into the script text is the JSON encoding of the
// payload, which json.Marshal guarantees is a single, self-contained
// JSON value with no unescaped quote or script-terminating sequence
// (Go's encoding/json escapes '<', '>', '&', U+2028 and U+2029 by
// default specifically because JSON is often embedded in HTML/JS like
// this).
func postJSONScript(payload []byte) string {
	return "window.__liroReceive && window.__liroReceive(" + string(payload) + ");"
}

// marshalMessagePayload is a small seam so window_windows.go's PostJSON
// does not need to import encoding/json itself.
func marshalMessagePayload(v any) ([]byte, error) { return json.Marshal(v) }
