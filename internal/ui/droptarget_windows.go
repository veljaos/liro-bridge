//go:build windows

package ui

// The drop targets that make dragging a document onto a window work.
//
// What was measured, in order, because two plausible explanations were
// wrong before the third one held (D-114 recorded half of this and got
// the other half backwards).
//
// A window hosting WebView2 is not one window. The frame this package
// creates owns a Chrome_WidgetWin_0, which owns a Chrome_WidgetWin_1,
// which owns Chrome_RenderWidgetHostHWND and an Intermediate D3D
// Window — every one of them exactly covering the client area.
//
//  1. Integrity level, ruled out first: this process and explorer.exe
//     both measure medium (0x2000), so UIPI was never blocking a drop.
//  2. DragAcceptFiles is called on the right window. The frame's
//     WS_EX_ACCEPTFILES bit is set, read back with GetWindowLongPtr.
//     It does not help: the bit is consulted for the window the drop
//     lands on, and the drop does not land on the frame.
//  3. Where it does land was found by walking the tree and reading
//     each window's "OleDropTargetInterface" property. With the drop
//     still switched on, the one window carrying a registered
//     IDropTarget is Chrome_WidgetWin_1 — and
//     put_AllowExternalDrop(FALSE) makes Chromium revoke exactly that,
//     leaving nothing anywhere under the cursor that accepts anything.
//     That is what Windows draws the no-entry cursor for.
//  4. Registering an IDropTarget on the frame alone was tried next, on
//     the theory that the search walks up the parent chain. Measured
//     against a real drag: it does not. The cursor stayed no-entry.
//  5. Registering one on every window in the tree works. The log from
//     a real drag says which window actually received it, and the
//     answer is Chrome_RenderWidgetHostHWND — four levels below the
//     frame, and WS_EX_TRANSPARENT, so not even the window
//     WindowFromPoint returns.
//
// Hence: switch WebView2's own handling off, then register a target on
// the frame and on every window underneath it. Which of them a given
// drop arrives at is Windows' business, not something this code needs
// a theory about; every one of them answers the same way.
//
// WM_DROPFILES is still handled (window_windows.go): DragAcceptFiles
// stays on and covers a drop landing on the frame itself. The two
// cannot both fire for one drop — OLE uses a registered target when it
// finds one and only falls back to WS_EX_ACCEPTFILES when it does not.

import (
	"fmt"
	"log/slog"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procRegisterDragDrop = ole32DLL.NewProc("RegisterDragDrop")
	procRevokeDragDrop   = ole32DLL.NewProc("RevokeDragDrop")
	procReleaseStgMedium = ole32DLL.NewProc("ReleaseStgMedium")
)

const (
	// cfHDROP is CF_HDROP, the clipboard format Explorer puts a list of
	// file paths in. Its medium is an HGLOBAL that DragQueryFileW reads
	// directly — the same handle WM_DROPFILES delivers.
	cfHDROP = 15

	tymedHGlobal    = 1
	dvAspectContent = 1

	dropEffectNone = 0
	dropEffectCopy = 1 << 0

	// dragDropEAlreadyRegistered is DRAGDROP_E_ALREADYREGISTERED, worth
	// naming because it is the one RegisterDragDrop failure that means
	// "this is a bug in our own bookkeeping" rather than "OLE is not
	// available on this thread".
	dragDropEAlreadyRegistered = 0x80040101
)

// formatEtc mirrors FORMATETC. The explicit padding is not decoration:
// CLIPFORMAT is 16 bits and the pointer that follows it is 8-byte
// aligned on amd64, so a Go struct without it would put ptd at offset 2
// and every field after it in the wrong place.
type formatEtc struct {
	cfFormat uint16
	_        [3]uint16
	_        uintptr // ptd, the target device: always nil here
	dwAspect uint32
	lindex   int32
	tymed    uint32
	_        uint32
}

// stgMedium mirrors STGMEDIUM: a tag, a union this code only ever reads
// as an HGLOBAL, and the release interface ReleaseStgMedium needs.
type stgMedium struct {
	tymed  uint32
	_      uint32
	handle uintptr
	_      uintptr // pUnkForRelease, read by ReleaseStgMedium, never here
}

// dropTargetVtbl is IDropTarget's layout: IUnknown's three methods then
// DragEnter, DragOver, DragLeave, Drop, in that order (objidl.h).
type dropTargetVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	DragEnter      uintptr
	DragOver       uintptr
	DragLeave      uintptr
	Drop           uintptr
}

// dropTarget is this package's IDropTarget. comBase is field 0, so the
// object's address is its interface pointer, exactly as for every other
// COM object here.
type dropTarget struct {
	comBase
	hwnd    uintptr
	onFiles func([]string)
	// accept records what DragEnter decided, so DragOver answers the
	// same thing without re-inspecting the data object on every mouse
	// move.
	accept atomic.Bool
}

// iidIDropTarget is IDropTarget's IID, from objidl.h.
var iidIDropTarget = mustGUID("00000122-0000-0000-c000-000000000046")

// dropTargetQueryInterface answers for IUnknown and for IDropTarget.
// Unlike the WebView2 handlers (com_windows.go's queryInterfaceThunk,
// which only ever needs to answer IUnknown because each handler is
// passed to one method that already knows its type), this object is
// handed to OLE, which does query it for IDropTarget by IID.
func dropTargetQueryInterface(this, riid, ppv uintptr) uintptr {
	b := (*comBase)(unsafe.Pointer(this))
	id := (*windows.GUID)(unsafe.Pointer(riid))
	out := (*uintptr)(unsafe.Pointer(ppv))
	if *id == iidIUnknown || *id == iidIDropTarget {
		atomic.AddInt32(&b.refs, 1)
		*out = this
		return sOK
	}
	*out = 0
	return eNoInterface
}

// The parameter lists below are objidl.h's, counted in machine words
// rather than in C parameters, because POINTL is passed *by value*:
// two LONGs pack into one 64-bit argument on amd64, so it occupies one
// slot, not two. Miscounting it shifts pdwEffect by a slot and hands
// OLE a garbage pointer to write the drop effect into — the same trap
// MonitorFromPoint's POINT sprang on this package once already (D-080).
//
//	DragEnter(this, IDataObject*, DWORD grfKeyState, POINTL, DWORD* pdwEffect)  5
//	DragOver (this,               DWORD grfKeyState, POINTL, DWORD* pdwEffect)  4
//	DragLeave(this)                                                             1
//	Drop     (this, IDataObject*, DWORD grfKeyState, POINTL, DWORD* pdwEffect)  5
func dropTargetDragEnter(this, dataObj, _, _, effect uintptr) uintptr {
	t := (*dropTarget)(unsafe.Pointer(this))
	has := dataObjectHasFiles(dataObj)
	slog.Info("ui: drag entered a registered window",
		"hwnd", fmt.Sprintf("%#x", t.hwnd), "class", windowClass(t.hwnd), "carriesFiles", has)
	t.accept.Store(has)
	setDropEffect(effect, has)
	return sOK
}

func dropTargetDragOver(this, _, _, effect uintptr) uintptr {
	t := (*dropTarget)(unsafe.Pointer(this))
	setDropEffect(effect, t.accept.Load())
	return sOK
}

func dropTargetDragLeave(this uintptr) uintptr {
	t := (*dropTarget)(unsafe.Pointer(this))
	t.accept.Store(false)
	return sOK
}

func dropTargetDrop(this, dataObj, _, _, effect uintptr) uintptr {
	t := (*dropTarget)(unsafe.Pointer(this))
	t.accept.Store(false)

	paths := dataObjectFiles(dataObj)
	slog.Info("ui: drop received",
		"hwnd", fmt.Sprintf("%#x", t.hwnd), "class", windowClass(t.hwnd), "paths", len(paths))
	if len(paths) == 0 {
		setDropEffect(effect, false)
		return sOK
	}
	setDropEffect(effect, true)
	if t.onFiles != nil {
		t.onFiles(paths)
	}
	return sOK
}

// setDropEffect writes *pdwEffect. Copy, never Move: a signing agent
// reads the documents it is given and must never be the reason a file
// leaves the folder it was dragged from.
func setDropEffect(p uintptr, accept bool) {
	if p == 0 {
		return
	}
	e := (*uint32)(unsafe.Pointer(p))
	if accept {
		*e = dropEffectCopy
		return
	}
	*e = dropEffectNone
}

// hdropFormat is the FORMATETC every query below uses: CF_HDROP, whole
// content, no target device, in an HGLOBAL.
func hdropFormat() formatEtc {
	return formatEtc{
		cfFormat: cfHDROP,
		dwAspect: dvAspectContent,
		lindex:   -1,
		tymed:    tymedHGlobal,
	}
}

// dataObjectHasFiles asks whether the drag carries file paths at all,
// through IDataObject::QueryGetData (vtable slot 5) — cheaper than
// fetching the data, and DragEnter is called for every drag that passes
// over the window, including ones carrying text or an image.
func dataObjectHasFiles(dataObj uintptr) bool {
	if dataObj == 0 {
		return false
	}
	var pin runtime.Pinner
	defer pin.Unpin()
	fe := hdropFormat()
	r, err := comCall(dataObj, 5, pinPtr(&pin, &fe))
	return err == nil && r == sOK
}

// dataObjectFiles reads CF_HDROP out of the drag's data object through
// IDataObject::GetData (vtable slot 3) and returns the paths.
//
// The medium is released with ReleaseStgMedium, not DragFinish: the
// handle belongs to the data object here, unlike the WM_DROPFILES path
// where the HDROP is handed over outright.
func dataObjectFiles(dataObj uintptr) []string {
	if dataObj == 0 {
		return nil
	}
	var pin runtime.Pinner
	defer pin.Unpin()
	fe := hdropFormat()
	var med stgMedium
	if _, err := comCall(dataObj, 3, pinPtr(&pin, &fe), pinPtr(&pin, &med)); err != nil {
		slog.Warn("ui: the dropped data could not be read as a file list", "error", err)
		return nil
	}
	defer func() { _, _, _ = procReleaseStgMedium.Call(pinPtr(&pin, &med)) }()
	if med.tymed != tymedHGlobal || med.handle == 0 {
		return nil
	}
	return readDropPaths(med.handle)
}

var dropTargetVtblSingleton = &dropTargetVtbl{
	QueryInterface: syscall.NewCallback(dropTargetQueryInterface),
	AddRef:         syscall.NewCallback(addRefThunk),
	Release:        syscall.NewCallback(releaseThunk),
	DragEnter:      syscall.NewCallback(dropTargetDragEnter),
	DragOver:       syscall.NewCallback(dropTargetDragOver),
	DragLeave:      syscall.NewCallback(dropTargetDragLeave),
	Drop:           syscall.NewCallback(dropTargetDrop),
}

// registerDropTarget makes hwnd a real OLE drop target for file drags.
// The returned object must be handed back to revokeDropTarget before
// the window is destroyed.
//
// It must be called on the window's own thread: RegisterDragDrop binds
// the target to the calling thread's apartment.
func registerDropTarget(hwnd uintptr, onFiles func([]string)) (*dropTarget, error) {
	t := &dropTarget{hwnd: hwnd, onFiles: onFiles}
	t.vtbl = uintptr(unsafe.Pointer(dropTargetVtblSingleton))
	t.refs = 1
	this := pinHandler(t)

	r, _, _ := procRegisterDragDrop.Call(hwnd, this)
	if int32(r) < 0 {
		t.release()
		if uint32(r) == dragDropEAlreadyRegistered {
			return nil, fmt.Errorf("ui: RegisterDragDrop: this window already has a drop target")
		}
		return nil, fmt.Errorf("ui: RegisterDragDrop: HRESULT 0x%08X", uint32(r))
	}
	return t, nil
}

// revokeDropTarget undoes registerDropTarget and drops this package's
// own reference to the target. OLE releases its reference inside
// RevokeDragDrop, so the object is unpinned once both are gone.
func revokeDropTarget(hwnd uintptr, t *dropTarget) {
	if t == nil {
		return
	}
	_, _, _ = procRevokeDragDrop.Call(hwnd)
	t.release()
}

// ---- registering the whole hosted subtree ---------------------------

// registerDropTargets registers a drop target on the frame and on every
// window WebView2 has created underneath it.
//
// A drop lands on one specific window — the one the hit test resolves
// to — and for a WebView2 host that is Chrome_WidgetWin_1, four levels
// down and not ours to change. Registering only the frame leaves that
// window with no target of its own; whether OLE then searches upward is
// not something this project could establish by reading, so it does not
// depend on the answer. Every window in the tree gets a target, and
// each one says which window a drag actually reached.
//
// Chromium's own registrations are already gone by this point
// (put_AllowExternalDrop(FALSE)), so nothing is being displaced;
// a window that somehow still has one is skipped rather than fought
// over.
func registerDropTargets(frame uintptr, onFiles func([]string)) []*dropTarget {
	var out []*dropTarget
	for _, hwnd := range append([]uintptr{frame}, descendantWindows(frame)...) {
		t, err := registerDropTarget(hwnd, onFiles)
		if err != nil {
			slog.Warn("ui: this window will not accept dropped files",
				"hwnd", fmt.Sprintf("%#x", hwnd), "class", windowClass(hwnd), "error", err)
			continue
		}
		slog.Info("ui: registered a drop target",
			"hwnd", fmt.Sprintf("%#x", hwnd), "class", windowClass(hwnd))
		out = append(out, t)
	}
	return out
}

// revokeDropTargets undoes registerDropTargets.
func revokeDropTargets(targets []*dropTarget) {
	for _, t := range targets {
		revokeDropTarget(t.hwnd, t)
	}
}
