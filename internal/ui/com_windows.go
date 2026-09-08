//go:build windows

package ui

// Generic COM plumbing shared by every hand-written interface in this
// package (F5 §2.1: hand-written COM interop, consistent with how F1
// and F2 handled winscard.dll/ncrypt.dll — pure logic separated from
// DLL glue, though here there is no "pure logic" half at all: COM
// vtable dispatch cannot be exercised without the real WebView2 runtime,
// so unlike windowscng there is no ...conn interface behind this file to
// unit-test against). Every interface layout and IID used anywhere in
// this package was read directly from the WebView2 SDK's own
// WebView2.idl (shipped inside the Microsoft.Web.WebView2 NuGet
// package; the redistributed loader and its licence live in
// internal/ui/assets/webview2), not reconstructed from memory — see
// D-080.
//
// Two directions of COM call happen here:
//
//  1. Calling INTO an interface the WebView2 runtime implements
//     (Environment, Controller, CoreWebView2): comCall reads the
//     vtable pointer from the object's first machine word (the
//     universal C++/COM ABI: an interface pointer IS the address of a
//     struct whose first field is a pointer to an array of function
//     pointers) and invokes the method at a fixed slot index. The slot
//     indices used throughout this package are recorded next to each
//     call site as "IDL line N, slot K" so they can be checked against
//     WebView2.idl directly.
//  2. Implementing an interface the runtime calls INTO (the
//     environment/controller-created completion handlers,
//     WebMessageReceived, ExecuteScript's completion handler): each is
//     a Go struct whose first field is comBase (vtbl pointer + a
//     refcount), with a package-level, singleton vtable per kind whose
//     function pointers are created once via syscall.NewCallback and
//     never released — see the comment on vtable singletons below for
//     why that is safe here.
import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32DLL = windows.NewLazySystemDLL("ole32.dll")

	procCoInitializeEx = ole32DLL.NewProc("CoInitializeEx")
	procCoTaskMemFree  = ole32DLL.NewProc("CoTaskMemFree")

	// OleInitialize, not CoInitializeEx, is what a window that accepts
	// dropped files needs on its own thread — see initApartment below.
	// There is no OleUninitialize beside it: the one apartment this
	// package takes is never given back (initApartment).
	procOleInitializeCOM = ole32DLL.NewProc("OleInitialize")
)

const (
	coinitApartmentThreaded = 0x2

	sOK          = uintptr(0)
	eNoInterface = uintptr(0x80004002)
)

// mustGUID parses a canonical "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
// GUID string. Every call site passes a compile-time constant copied
// directly from a WebView2.idl [uuid(...)] attribute, so a parse
// failure here can only be a transcription bug in this file — it is
// caught immediately by TestKnownGUIDsParse rather than surfacing as a
// mysterious E_NOINTERFACE at runtime.
func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic("ui: invalid GUID literal " + s + ": " + err.Error())
	}
	return g
}

// IIDs, read directly from WebView2.idl's [uuid(...)] attributes
// (D-080). Only the two actually dereferenced anywhere in this package
// are kept: every completion/event handler this package implements is
// only ever handed to the one WebView2 method that already knows its
// concrete type statically (queryInterfaceThunk's own doc comment
// explains why no handler needs its own IID recognised), and every
// interface this package calls into is reached structurally (via
// CreateCoreWebView2Controller's result, Controller::CoreWebView2, and
// one explicit QueryInterface for ICoreWebView2_3) rather than by
// looking up an IID from this table.
var (
	iidIUnknown       = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidCoreWebView2_3 = mustGUID("a0d6df20-3b92-416d-aa0c-437a9c727857")

	// iidCoreWebView2Controller4 is the interface carrying
	// AllowExternalDrop, which the main window turns off so that files
	// dropped from Explorer reach the native HWND as WM_DROPFILES
	// instead of being swallowed by the page (F6 §1, D-114). Read from
	// WebView2.idl, like every other identifier in this package
	// (D-080), not from documentation or memory.
	iidCoreWebView2Controller4 = mustGUID("97d418d5-a426-4e49-a151-e1a10f327d9e")
)

// initApartment puts the calling thread into a single-threaded
// apartment with the OLE subsystem running.
//
// It is called once per process, by the UI thread
// (uithread_windows.go), and is never undone: that thread outlives
// every window on it, and the environment, the controllers and the
// browser process group behind them all belong to this apartment.
// There is no OleUninitialize here for the same reason there is no
// WM_QUIT — the operating system reclaims the apartment at process
// exit, and anything that shut it down earlier would be shutting it
// down under a window still using it.
//
// OleInitialize is used in preference to CoInitializeEx because
// RegisterDragDrop — which droptarget_windows.go calls for a window
// that accepts dropped files — is OLE, not plain COM: it fails with
// E_OUTOFMEMORY on a thread where only CoInitializeEx has run.
//
// Measured on this machine rather than assumed: the WebView2 runtime
// already calls OleInitialize on this thread as part of its own setup,
// so registration happens to succeed either way today (with
// CoInitializeEx only, Chromium's own drop target still appeared on
// Chrome_WidgetWin_1). Depending on that is depending on the order and
// the internals of somebody else's initialisation for a guarantee this
// package needs for its own call, so the call is made explicitly here.
//
// OleInitialize itself calls CoInitializeEx(NULL,
// COINIT_APARTMENTTHREADED), so every WebView2 COM call this package
// makes is in exactly the apartment it was before.
func initApartment() error {
	r0, _, _ := procOleInitializeCOM.Call(0)
	// S_FALSE (1) means OLE was already initialised on this thread;
	// that is not an error.
	if int32(r0) >= 0 {
		return nil
	}
	oleHR := uint32(r0)

	// RPC_E_CHANGED_MODE is the one refusal worth falling back from: the
	// thread is already in an apartment of a different kind, which is
	// not a state this package's own threads can reach but is not worth
	// refusing to open a window over. Drops will not work on such a
	// thread; registerDropTarget says so in the log rather than
	// silently producing a window that looks like a drop target.
	r1, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	if int32(r1) < 0 {
		return fmt.Errorf("OleInitialize: HRESULT 0x%08X; CoInitializeEx: HRESULT 0x%08X", oleHR, uint32(r1))
	}
	return nil
}

func coTaskMemFree(p uintptr) {
	if p != 0 {
		_, _, _ = procCoTaskMemFree.Call(p)
	}
}

// --- Handing Go memory to foreign code: pinning ---
//
// Everything in this package that gives WebView2 the bare address of Go
// memory has to answer one question: is that address still the address
// of that memory when the callee uses it? Two separate mechanisms can
// invalidate it, and D-101 records both being hit for real:
//
//  1. The garbage collector may free memory nothing points at any more.
//     A uintptr is a number, not a pointer — the collector cannot see
//     it, so an object whose only remaining reference is one WebView2
//     holds is, as far as Go is concerned, garbage.
//  2. A goroutine's stack moves. When a goroutine needs more stack than
//     it has, the runtime allocates a bigger one and copies every frame
//     to a new address, rewriting the Go pointers it can find. It
//     cannot rewrite a uintptr already handed to a COM method, and it
//     cannot rewrite one the WebView2 runtime is holding at all. A
//     stack-allocated object handed to foreign code is therefore valid
//     for exactly as long as nothing on this goroutine calls a function
//     that needs to grow the stack — which is not a property any code
//     can rely on.
//
// runtime.Pinner answers both at once: a pinned object is guaranteed
// not to be moved and not to be freed until Unpin. Pinning also forces
// the object onto the heap in the first place (escape analysis follows
// the pointer into Pin), which is the half that actually fixed the
// intermittent hang D-101 measured — the completion handlers were
// stack-allocated, and a stack copy while WebView2 still held their
// address made the completion land in the abandoned copy.
//
// Two shapes are used:
//
//   - pinPtr, for memory a single call reads or writes and is done with
//     by the time that call returns (out-parameters, GUIDs, RECTs,
//     string buffers). The caller owns a runtime.Pinner on its own
//     stack and unpins on the way out.
//   - pinHandler and (*comBase).release, for the handler objects
//     WebView2 keeps a reference to and calls back into later. Those
//     cannot be unpinned when a function returns; they are unpinned
//     when their COM reference count reaches zero, which is what the
//     reference count was always for.

// pinPtr pins v for as long as p is not unpinned, and returns v's
// address in the form a COM or Win32 parameter takes.
//
// The type parameter is not decoration: it keeps the address and the
// pinned object provably the same value, so no call site can pin one
// thing and pass the address of another.
func pinPtr[T any](p *runtime.Pinner, v *T) uintptr {
	p.Pin(v)
	return uintptr(unsafe.Pointer(v))
}

// handlerPins holds one runtime.Pinner per live handler object this
// package has handed to WebView2, keyed by the object's own address —
// the same value WebView2 passes back as `this`. An entry exists for
// exactly as long as that object's COM reference count is above zero.
var (
	handlerPinsMu sync.Mutex
	handlerPins   = map[uintptr]*runtime.Pinner{}
)

// pinHandler pins h — a handler object whose first field is comBase —
// and returns its address, which is both its COM interface pointer and
// its key in handlerPins. Every handler is created with a reference
// count of 1, this package's own reference, so it stays pinned until
// that reference is dropped with (*comBase).release and every reference
// WebView2 took has been released too.
func pinHandler[T any](h *T) uintptr {
	p := new(runtime.Pinner)
	p.Pin(h)
	this := uintptr(unsafe.Pointer(h))
	handlerPinsMu.Lock()
	handlerPins[this] = p
	handlerPinsMu.Unlock()
	return this
}

// unpinHandler drops the pin on the handler at this, if any. Called
// only from releaseHandlerRef, when the reference count has reached
// zero: after it returns, the object is ordinary garbage.
func unpinHandler(this uintptr) {
	handlerPinsMu.Lock()
	p := handlerPins[this]
	delete(handlerPins, this)
	handlerPinsMu.Unlock()
	if p != nil {
		p.Unpin()
	}
}

// handlerPinCount reports how many handler objects are currently pinned.
// Only TestHandlersArePinnedUntilReleased reads it — it is the one
// observable that distinguishes "the object is reachable by luck" from
// "the object is pinned for as long as WebView2 may touch it".
func handlerPinCount() int {
	handlerPinsMu.Lock()
	defer handlerPinsMu.Unlock()
	return len(handlerPins)
}

// releaseHandlerRef is IUnknown::Release's actual behaviour for every
// handler this package implements, shared by the vtable thunk WebView2
// calls and by (*comBase).release, which this package calls for its own
// reference. Reaching zero unpins the object — the one moment at which
// it becomes safe for the collector to reclaim it, because it is the
// one moment at which nothing outside Go holds its address.
func releaseHandlerRef(this uintptr) int32 {
	b := (*comBase)(unsafe.Pointer(this))
	n := atomic.AddInt32(&b.refs, -1)
	if n <= 0 {
		unpinHandler(this)
	}
	return n
}

// comBase is the first field of every COM object this package
// implements (see the package doc comment, direction 2). Its address
// equals the address of the whole struct, which is what makes an
// *T holding comBase as field 0 a valid COM interface pointer.
type comBase struct {
	vtbl uintptr
	refs int32
}

// addr is the object's COM interface pointer: comBase is field 0, so
// its address is the address of the whole handler. Call sites pass
// h.addr() rather than converting with unsafe at each one, which keeps
// every such conversion in this file, next to the pinning that makes it
// safe.
func (b *comBase) addr() uintptr { return uintptr(unsafe.Pointer(b)) }

// release drops this package's own reference to a handler it created
// and pinned — exactly once per pinHandler. The WebView2 runtime's own
// references are dropped through releaseThunk.
func (b *comBase) release() { releaseHandlerRef(b.addr()) }

// simpleHandlerVtbl is the vtable shape shared by every completion/event
// handler this package implements: IUnknown's three methods plus one
// Invoke. WebView2's various *CompletedHandler and *EventHandler
// interfaces (ICoreWebView2CreateCoreWebView2EnvironmentCompletedHandler,
// ICoreWebView2CreateCoreWebView2ControllerCompletedHandler,
// ICoreWebView2WebMessageReceivedEventHandler,
// ICoreWebView2ExecuteScriptCompletedHandler) all have exactly this
// shape in WebView2.idl — three IUnknown methods, one Invoke — even
// though Invoke's two non-this parameters mean different things per
// interface. One Go struct type serves all of them; only the Invoke
// function pointer differs per kind (see the vtable singletons below).
type simpleHandlerVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	Invoke         uintptr
}

// queryInterfaceThunk, addRefThunk and releaseThunk implement IUnknown
// generically for every object in this package: comBase is always field
// 0, so `this` reinterpreted as *comBase is valid regardless of which
// concrete handler type actually owns it.
//
// queryInterfaceThunk only ever answers for IUnknown. None of the
// handler objects in this package are, in practice, queried by the
// WebView2 runtime for any interface other than IUnknown (each is
// handed to exactly one method that already knows its concrete type
// statically) — answering only IUnknown here is the simplest correct
// implementation for that usage pattern, not a general-purpose COM
// object.
func queryInterfaceThunk(this, riid, ppv uintptr) uintptr {
	b := (*comBase)(unsafe.Pointer(this))
	id := (*windows.GUID)(unsafe.Pointer(riid))
	out := (*uintptr)(unsafe.Pointer(ppv))
	if *id == iidIUnknown {
		atomic.AddInt32(&b.refs, 1)
		*out = this
		return sOK
	}
	*out = 0
	return eNoInterface
}

func addRefThunk(this uintptr) uintptr {
	b := (*comBase)(unsafe.Pointer(this))
	return uintptr(atomic.AddInt32(&b.refs, 1))
}

// releaseThunk decrements the reference count and reports it, exactly
// like a real COM object. It frees nothing — these are ordinary Go
// values, and freeing Go memory explicitly is not a thing — but
// reaching zero does do something now: it unpins the object, which is
// what finally allows the collector to reclaim it (see the pinning
// commentary above). Before D-101 this was a bare decrement whose
// result nothing acted on, and the handler objects were kept alive only
// by whatever local variable happened to still reference them — which
// for the completion handlers was a stack slot that could move, and for
// the navigation-completed handler was nothing at all once
// setUpWebView2 returned.
func releaseThunk(this uintptr) uintptr {
	return uintptr(releaseHandlerRef(this))
}

// Vtable singletons: one per Invoke shape, created once and shared by
// every instance of that handler kind. Creating the vtable once, rather
// than once per handler instance, keeps the number of
// syscall.NewCallback trampolines fixed and tiny (four) regardless of
// how many windows or ExecuteScript calls happen over the agent's
// lifetime — NewCallback trampolines are never released by the Go
// runtime, so allocating one per call would be a real, unbounded leak
// over a long-running tray process; allocating four for the life of the
// program is not.
var (
	environmentCompletedVtbl = &simpleHandlerVtbl{
		QueryInterface: syscall.NewCallback(queryInterfaceThunk),
		AddRef:         syscall.NewCallback(addRefThunk),
		Release:        syscall.NewCallback(releaseThunk),
		Invoke:         syscall.NewCallback(environmentCompletedInvoke),
	}
	controllerCompletedVtbl = &simpleHandlerVtbl{
		QueryInterface: syscall.NewCallback(queryInterfaceThunk),
		AddRef:         syscall.NewCallback(addRefThunk),
		Release:        syscall.NewCallback(releaseThunk),
		Invoke:         syscall.NewCallback(controllerCompletedInvoke),
	}
	webMessageReceivedVtbl = &simpleHandlerVtbl{
		QueryInterface: syscall.NewCallback(queryInterfaceThunk),
		AddRef:         syscall.NewCallback(addRefThunk),
		Release:        syscall.NewCallback(releaseThunk),
		Invoke:         syscall.NewCallback(webMessageReceivedInvoke),
	}
	navigationCompletedVtbl = &simpleHandlerVtbl{
		QueryInterface: syscall.NewCallback(queryInterfaceThunk),
		AddRef:         syscall.NewCallback(addRefThunk),
		Release:        syscall.NewCallback(releaseThunk),
		Invoke:         syscall.NewCallback(navigationCompletedInvoke),
	}
	executeScriptCompletedVtbl = &simpleHandlerVtbl{
		QueryInterface: syscall.NewCallback(queryInterfaceThunk),
		AddRef:         syscall.NewCallback(addRefThunk),
		Release:        syscall.NewCallback(releaseThunk),
		Invoke:         syscall.NewCallback(executeScriptCompletedInvoke),
	}
)

// comCall invokes the method at vtable slot index (0 = QueryInterface)
// on obj, an interface pointer to an object the WebView2 runtime
// implements (direction 1 in the package doc comment). obj's first
// machine word is read as the vtable array's address, per the
// universal C++/COM ABI — the same layout comBase gives our own
// objects, just on the other end of the call.
func comCall(obj uintptr, slot int, args ...uintptr) (uintptr, error) {
	if obj == 0 {
		return 0, fmt.Errorf("ui: comCall on a nil interface pointer (slot %d)", slot)
	}
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r1, _, _ := syscall.SyscallN(fn, all...)
	if int32(r1) < 0 {
		return r1, fmt.Errorf("ui: HRESULT 0x%08X (slot %d)", uint32(r1), slot)
	}
	return r1, nil
}

// comRelease calls IUnknown::Release (always slot 2) on obj, ignoring
// the returned refcount — every interface pointer this package holds
// onto (Environment, Controller, CoreWebView2, the QueryInterface'd
// CoreWebView2_3) is released exactly once when the window closes.
// Release's return value is a plain refcount, not an HRESULT, but it is
// never negative in practice, so comCall's HRESULT-shaped error check
// never misfires on it; the error is discarded either way since there
// is nothing a caller could do about a failed Release.
func comRelease(obj uintptr) {
	if obj != 0 {
		_, _ = comCall(obj, 2)
	}
}

// queryInterface calls IUnknown::QueryInterface (always slot 0) on obj
// for iid, returning the resulting interface pointer. Used once, to
// reach ICoreWebView2_3 (for SetVirtualHostNameToFolderMapping) from
// the base ICoreWebView2 pointer CoreWebView2Controller::CoreWebView2
// returns — see D-080 for why this project trusts the two pointers may
// legitimately differ and always QueryInterfaces explicitly rather than
// assuming they are numerically equal.
func queryInterface(obj uintptr, iid windows.GUID) (uintptr, error) {
	var pin runtime.Pinner
	defer pin.Unpin()
	var out uintptr
	iidCopy := iid
	_, err := comCall(obj, 0, pinPtr(&pin, &iidCopy), pinPtr(&pin, &out))
	if err != nil {
		return 0, err
	}
	return out, nil
}

// utf16Buf encodes s as a NUL-terminated UTF-16 buffer, which callers
// hand to WebView2 through pinUTF16 rather than by converting the slice
// themselves: whether such a buffer lands on the heap or the stack is
// an escape-analysis outcome no call site should be depending on, and a
// stack buffer moves (see the pinning commentary at the top of this
// file). Pinning is strictly stronger than the runtime.KeepAlive these
// call sites used to rely on — KeepAlive stops memory being collected
// and does nothing at all about it being copied to a new address.
func utf16Buf(s string) ([]uint16, error) {
	return windows.UTF16FromString(s)
}

// pinUTF16 pins buf and returns the address of its first element, which
// is the LPCWSTR a COM or Win32 parameter takes.
func pinUTF16(p *runtime.Pinner, buf []uint16) uintptr {
	return pinPtr(p, &buf[0])
}

// stringFromLPWSTR reads a NUL-terminated UTF-16 string the runtime
// allocated with CoTaskMemAlloc (an `[out, retval] LPWSTR*` parameter,
// e.g. WebMessageAsJson) and frees it, per the WebView2 API convention
// documented directly above every such parameter in WebView2.idl ("The
// caller must free the returned string with CoTaskMemFree").
func stringFromLPWSTR(p uintptr) string {
	if p == 0 {
		return ""
	}
	defer coTaskMemFree(p)
	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(p)))
}
