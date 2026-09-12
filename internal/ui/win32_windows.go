//go:build windows

package ui

// The Win32 primitives the window host needs: class registration, the
// message loop, DPI awareness, and centring on the monitor under the
// cursor (F5 §2.3). Nothing here is unit-testable — it is pure syscall
// glue, the same role conn_windows.go plays for windowscng.
import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32DLL   = windows.NewLazySystemDLL("user32.dll")
	kernel32DLL = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW      = user32DLL.NewProc("RegisterClassExW")
	procCreateWindowExW       = user32DLL.NewProc("CreateWindowExW")
	procDefWindowProcW        = user32DLL.NewProc("DefWindowProcW")
	procDestroyWindow         = user32DLL.NewProc("DestroyWindow")
	procShowWindow            = user32DLL.NewProc("ShowWindow")
	procSetWindowPos          = user32DLL.NewProc("SetWindowPos")
	procGetWindowLongPtrW     = user32DLL.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW     = user32DLL.NewProc("SetWindowLongPtrW")
	procGetMessageW           = user32DLL.NewProc("GetMessageW")
	procTranslateMessage      = user32DLL.NewProc("TranslateMessage")
	procDispatchMessageW      = user32DLL.NewProc("DispatchMessageW")
	procPostQuitMessage       = user32DLL.NewProc("PostQuitMessage")
	procPostMessageW          = user32DLL.NewProc("PostMessageW")
	procAdjustWindowRectEx    = user32DLL.NewProc("AdjustWindowRectEx")
	procGetCursorPos          = user32DLL.NewProc("GetCursorPos")
	procMonitorFromPoint      = user32DLL.NewProc("MonitorFromPoint")
	procMonitorFromWindow     = user32DLL.NewProc("MonitorFromWindow")
	procGetMonitorInfoW       = user32DLL.NewProc("GetMonitorInfoW")
	procMessageBoxW           = user32DLL.NewProc("MessageBoxW")
	procSetProcessDPIAwareCtx = user32DLL.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForWindow       = user32DLL.NewProc("GetDpiForWindow")
	procLoadCursorW           = user32DLL.NewProc("LoadCursorW")
	procSetForegroundWindow   = user32DLL.NewProc("SetForegroundWindow")
	procSetFocus              = user32DLL.NewProc("SetFocus")
	procEnumChildWindows      = user32DLL.NewProc("EnumChildWindows")
	procGetClassNameW         = user32DLL.NewProc("GetClassNameW")
	procSetTimer              = user32DLL.NewProc("SetTimer")
	procKillTimer             = user32DLL.NewProc("KillTimer")
	procWindowFromPoint       = user32DLL.NewProc("WindowFromPoint")
	procGetWindowThreadPID    = user32DLL.NewProc("GetWindowThreadProcessId")
	procIsIconic              = user32DLL.NewProc("IsIconic")
	procEnableWindow          = user32DLL.NewProc("EnableWindow")
	procGetWindowRect         = user32DLL.NewProc("GetWindowRect")

	procGetModuleHandleW   = kernel32DLL.NewProc("GetModuleHandleW")
	procGetCurrentThreadID = kernel32DLL.NewProc("GetCurrentThreadId")

	// shell32's drag-and-drop trio (F6 §1). DragAcceptFiles marks a
	// window as a drop target for Explorer; WM_DROPFILES then arrives
	// with an HDROP that DragQueryFileW reads and DragFinish releases.
	// shell32DLL itself is declared in tray_windows.go, which got there
	// first for Shell_NotifyIconW.
	procDragAcceptFile = shell32DLL.NewProc("DragAcceptFiles")
	procDragQueryFileW = shell32DLL.NewProc("DragQueryFileW")
	procDragFinish     = shell32DLL.NewProc("DragFinish")
)

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type monitorInfo struct {
	Size     uint32
	Monitor  rect
	WorkArea rect
	Flags    uint32
}

const (
	wsPopup   = 0x80000000
	wsCaption = 0x00C00000
	wsSysMenu = 0x00080000
	wsVisible = 0x10000000

	wsExTopMost = 0x00000008

	swShow = 5

	wmDestroy    = 0x0002
	wmClose      = 0x0010
	wmTimer      = 0x0113
	wmDropFiles  = 0x0233
	wmDPIChanged = 0x02E0
	wmApp        = 0x8000

	gwlpUserData = -21

	monitorDefaultToNearest = 2

	csHRedraw = 0x0002
	csVRedraw = 0x0001

	idcArrow = 32512 // IDC_ARROW, a MAKEINTRESOURCE ordinal

	mbOK              = 0x00000000
	mbIconWarning     = 0x00000030
	mbIconInformation = 0x00000040
	mbTopMost         = 0x00040000

	// wmRunFunc is a private WM_APP message used to marshal a Window
	// method call (PostJSON, Close) onto the OS thread that owns the
	// window's COM apartment — every COM call in this package must run
	// on that thread (F5 §2.1's STA model); see window_windows.go.
	wmRunFunc = wmApp + 1
)

// dpiAwareOnce declares per-monitor-v2 DPI awareness for the whole
// process (F5 §2.3). This project has no application manifest pipeline
// yet (SPEC §5's rsrc.syso arrives with packaging, F10) so the
// programmatic API is used instead of a manifest entry — see D-081.
// It must run before any window is created, and only once per process.
var dpiAwareOnce sync.Once

// uintptrFromInt32 converts a negative 32-bit value (a Win32 constant
// such as GWLP_USERDATA or DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2,
// both defined as small negative numbers reinterpreted as handle- or
// index-sized values) to the sign-extended uintptr the syscall
// convention expects. Going through a variable rather than a bare
// negative constant conversion is required here: Go rejects
// uintptr(-21) as a constant expression (negative values are not
// representable in an unsigned type), but the same conversion on a
// runtime int32 value is well-defined two's-complement sign extension —
// exactly the bit pattern the Win32 API expects back.
func uintptrFromInt32(v int32) uintptr { return uintptr(v) }

func ensureDPIAware() {
	dpiAwareOnce.Do(func() {
		// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 is defined as
		// ((DPI_AWARENESS_CONTEXT)-4); SetProcessDpiAwarenessContext
		// takes it as a handle-sized value, so -4 is passed
		// sign-extended into a uintptr.
		_, _, _ = procSetProcessDPIAwareCtx.Call(uintptrFromInt32(-4))
	})
}

func getDpiForWindow(hwnd uintptr) uint32 {
	r, _, _ := procGetDpiForWindow.Call(hwnd)
	if r == 0 {
		return 96
	}
	return uint32(r)
}

// scaleForDPI converts a DPI-independent point value (96 dpi baseline)
// to physical pixels at dpi.
func scaleForDPI(v int, dpi uint32) int32 {
	return int32(v * int(dpi) / 96)
}

func getMessage(m *winMsg) bool {
	r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(m)), 0, 0, 0)
	return int32(r) > 0
}

func translateMessage(m *winMsg) { _, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(m))) }
func dispatchWinMessage(m *winMsg) {
	_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(m)))
}
func postQuitMessage(code int32) { _, _, _ = procPostQuitMessage.Call(uintptr(code)) }

func postMessage(hwnd uintptr, msg uint32, wparam, lparam uintptr) {
	_, _, _ = procPostMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
}

// currentThreadID identifies the OS thread this goroutine is running
// on. window_windows.go records it for the thread that creates a
// window's WebView2 controller, and compares against it to answer the
// only question that matters for a single-threaded-apartment object:
// am I the thread allowed to touch this? See (*window).Close.
func currentThreadID() uintptr {
	id, _, _ := procGetCurrentThreadID.Call()
	return id
}

// sendMessage dispatches msg synchronously. Used only by
// (*window).Close, for the one case where the caller is already on the
// window's own thread and so cannot wait for it — SendMessageW to a
// window the calling thread owns calls its window procedure directly,
// without going near the message queue.
func sendMessage(hwnd uintptr, msg uint32, wparam, lparam uintptr) {
	_, _, _ = procSendMessageW.Call(hwnd, uintptr(msg), wparam, lparam)
}

// pumpUntil runs the Win32 message loop until done reports true or the
// loop receives WM_QUIT. It is used both during setup (waiting for the
// environment/controller completion handlers, which fire from inside
// DispatchMessage on this very thread — see webview2_windows.go) and,
// with a never-true done, as the window's steady-state event loop.
func pumpUntil(done func() bool) {
	var m winMsg
	for !done() {
		if !getMessage(&m) {
			return
		}
		translateMessage(&m)
		dispatchWinMessage(&m)
	}
}

func defWindowProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func setWindowUserData(hwnd uintptr, p unsafe.Pointer) {
	_, _, _ = procSetWindowLongPtrW.Call(hwnd, uintptrFromInt32(gwlpUserData), uintptr(p))
}

func getWindowUserData(hwnd uintptr) unsafe.Pointer {
	r, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptrFromInt32(gwlpUserData))
	return unsafe.Pointer(r)
}

// hwndTopMost, passed as the "insert after" handle, keeps a window
// above all non-topmost windows (F5 §2.3's always-on-top requirement
// for the consent window).
const hwndTopMost = ^uintptr(0) // (HWND)(-1), sign-extended

func setWindowPos(hwnd, after uintptr, x, y, w, h int32, flags uintptr) {
	_, _, _ = procSetWindowPos.Call(hwnd, after, uintptr(x), uintptr(y), uintptr(w), uintptr(h), flags)
}

func showAndFocusWindow(hwnd uintptr, alwaysOnTop bool) {
	_, _, _ = procShowWindow.Call(hwnd, swShow)
	if alwaysOnTop {
		const swpNoMove, swpNoSize = 0x0002, 0x0001
		setWindowPos(hwnd, hwndTopMost, 0, 0, 0, 0, swpNoMove|swpNoSize)
	}
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	_, _, _ = procSetFocus.Call(hwnd)
}

// cursorMonitorWorkRect returns the work-area rect (screen coordinates)
// of the monitor currently under the mouse cursor (F5 §2.3: "Centred on
// the display containing the cursor, not always the primary display").
func cursorMonitorRect() rect {
	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// MonitorFromPoint takes its POINT argument by value; on the amd64
	// calling convention a struct this small (two 32-bit fields) is
	// packed into a single 64-bit argument, x in the low 32 bits and y
	// in the high 32 bits — not two separate uintptr arguments. Passing
	// them separately silently shifts every later argument by one slot,
	// which is exactly what produced a bogus zero-value monitor rect
	// (and so a window centred on screen coordinate (0,0) instead of the
	// cursor's monitor) before this fix.
	packedPt := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	hMonitor, _, _ := procMonitorFromPoint.Call(packedPt, monitorDefaultToNearest)
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	_, _, _ = procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
	return mi.Monitor
}

// workAreaFor is the usable area — the monitor less the taskbar — of
// the monitor a window is on. Used by Resize to keep a window that
// changes shape between steps from putting its own buttons under the
// taskbar.
func workAreaFor(hwnd uintptr) (rect, bool) {
	hMonitor, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if hMonitor == 0 {
		return rect{}, false
	}
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	if ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi))); ret == 0 {
		return rect{}, false
	}
	return mi.WorkArea, true
}

func moduleHandle() uintptr {
	r, _, _ := procGetModuleHandleW.Call(0)
	return r
}

func loadArrowCursor() uintptr {
	r, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))
	return r
}

var registerClassOnce sync.Once
var windowClassName = windows.StringToUTF16Ptr("LiroBridgeWindow")

// wndProcCallback is the single, process-wide WndProc for every window
// this package creates. It dispatches by looking up the *window Go
// value stored in GWLP_USERDATA (set right after CreateWindowExW) —
// the standard technique for associating Win32 window state with a
// managed-language object.
var wndProcCallback = syscall.NewCallback(wndProc)

func registerWindowClass() {
	registerClassOnce.Do(func() {
		var wc wndClassExW
		wc.Size = uint32(unsafe.Sizeof(wc))
		wc.Style = csHRedraw | csVRedraw
		wc.WndProc = wndProcCallback
		wc.Instance = moduleHandle()
		wc.Cursor = loadArrowCursor()
		wc.ClassName = windowClassName
		r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if r == 0 {
			panic(fmt.Sprintf("ui: RegisterClassExW failed: %v", err))
		}
	})
}

// messageBox shows a native, blocking MessageBox. It is what this
// package has to say something with when it cannot open a window of
// its own: the WebView2-runtime-absent path (F5 §2.2), which by
// definition cannot use a WebView2 window to explain itself, and the
// uninstall notice (F10 §3.3), which runs while the installer is
// removing the program.
//
// title and text are pre-localised by the caller (see ShowWarning and
// ShowNotice in window.go): internal/ui has no i18n dependency,
// matching SPEC §4.2 rule 4's independence between internal/ui,
// internal/api and internal/cli.
func messageBox(title, text string, icon uintptr) {
	t, _ := windows.UTF16PtrFromString(title)
	b, _ := windows.UTF16PtrFromString(text)
	_, _, _ = procMessageBoxW.Call(0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), mbOK|icon|mbTopMost)
}

// setDragAcceptFiles marks hwnd as willing to receive files dropped
// from Explorer, which is what makes WM_DROPFILES arrive at all.
//
// On its own this is not enough for a WebView2 host: the control's own
// child windows are registered drop targets by Chromium and consume the
// drop before the parent frame ever sees it. The parent only receives
// WM_DROPFILES once ICoreWebView2Controller4::put_AllowExternalDrop has
// been set FALSE (webview2_windows.go). Both halves are required; see
// D-114.
func setDragAcceptFiles(hwnd uintptr, accept bool) {
	v := uintptr(0)
	if accept {
		v = 1
	}
	_, _, _ = procDragAcceptFile.Call(hwnd, v)
}

// droppedFiles reads every path out of an HDROP and releases it.
//
// DragQueryFileW with an index of 0xFFFFFFFF returns the count rather
// than a path; with a real index and a nil buffer it returns the length
// in characters, not counting the terminating NUL — which is why each
// buffer is allocated one larger than the reported length.
//
// DragFinish runs from a defer so the HDROP is released even if a path
// is malformed enough to make the loop give up early: leaking it would
// leak the shell's own memory for the drop, once per drop, for the life
// of the process.
func droppedFiles(hdrop uintptr) []string {
	defer func() { _, _, _ = procDragFinish.Call(hdrop) }()
	return readDropPaths(hdrop)
}

// readDropPaths reads every path out of an HDROP without releasing it.
//
// Split out of droppedFiles because the two callers own the handle
// differently: WM_DROPFILES hands the HDROP over outright, so that path
// must call DragFinish, while IDropTarget::Drop reads it out of a
// STGMEDIUM the data object still owns, where ReleaseStgMedium is what
// releases it and DragFinish would be a double free
// (droptarget_windows.go).
func readDropPaths(hdrop uintptr) []string {
	countR, _, _ := procDragQueryFileW.Call(hdrop, 0xFFFFFFFF, 0, 0)
	count := int(countR)
	if count <= 0 {
		return nil
	}
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		lenR, _, _ := procDragQueryFileW.Call(hdrop, uintptr(i), 0, 0)
		n := int(lenR)
		if n <= 0 {
			continue
		}
		buf := make([]uint16, n+1)
		gotR, _, _ := procDragQueryFileW.Call(hdrop, uintptr(i),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if int(gotR) == 0 {
			continue
		}
		out = append(out, windows.UTF16ToString(buf))
	}
	return out
}

// descendantWindows lists every window beneath hwnd, in no particular
// order. EnumChildWindows is already recursive, so one call reaches the
// whole subtree.
func descendantWindows(hwnd uintptr) []uintptr {
	var out []uintptr
	cb := syscall.NewCallback(func(child uintptr, _ uintptr) uintptr {
		out = append(out, child)
		return 1
	})
	_, _, _ = procEnumChildWindows.Call(hwnd, cb, 0)
	return out
}

// windowUnderPoint is the window at a screen point, with the process
// that owns it. Used to ask whether a drop aimed at this window would
// actually reach it (dropwatch_windows.go).
//
// WindowFromPoint takes its POINT by value and is packed into one
// 64-bit argument exactly as MonitorFromPoint is — see
// cursorMonitorRect, where passing the two fields separately silently
// shifted every later argument by a slot.
func windowUnderPoint(pt point) (hwnd uintptr, pid uint32) {
	packedPt := uintptr(uint32(pt.X)) | uintptr(uint32(pt.Y))<<32
	h, _, _ := procWindowFromPoint.Call(packedPt)
	if h == 0 {
		return 0, 0
	}
	var owner uint32
	_, _, _ = procGetWindowThreadPID.Call(h, uintptr(unsafe.Pointer(&owner)))
	return h, owner
}

// isIconic reports whether a window is minimised, in which case its
// rectangle says nothing about where it is on screen.
func isIconic(hwnd uintptr) bool {
	r, _, _ := procIsIconic.Call(hwnd)
	return r != 0
}

// windowClass is the window's registered class name, for log lines that
// have to be readable months later — "Chrome_WidgetWin_1" says
// something an HWND alone does not.
func windowClass(hwnd uintptr) string {
	buf := make([]uint16, 128)
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// enableWindow enables or disables a window's mouse and keyboard input.
// Used to make an owner window inert for as long as the window it
// opened is up (window_windows.go's Options.Owner).
func enableWindow(hwnd uintptr, enable bool) {
	v := uintptr(0)
	if enable {
		v = 1
	}
	_, _, _ = procEnableWindow.Call(hwnd, v)
}

// windowRect is a window's bounding rectangle in screen coordinates.
// ok is false when the window is gone, in which case the caller falls
// back to whatever it would have used without one.
func windowRect(hwnd uintptr) (rect, bool) {
	var r rect
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 {
		return rect{}, false
	}
	return r, true
}
