//go:build windows

package ui

// The tray icon (F5 §3): a notification-area icon with a right-click
// menu, present from startup and independent of any WebView2 window's
// lifetime — it runs its own dedicated OS thread and message loop, with
// no COM involved at all (Shell_NotifyIconW is a plain shell32 call).
import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32DLL = windows.NewLazySystemDLL("shell32.dll")

	procShellNotifyIconW = shell32DLL.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu  = user32DLL.NewProc("CreatePopupMenu")
	procAppendMenuW      = user32DLL.NewProc("AppendMenuW")
	procTrackPopupMenu   = user32DLL.NewProc("TrackPopupMenu")
	procDestroyMenu      = user32DLL.NewProc("DestroyMenu")
	procLoadIconW        = user32DLL.NewProc("LoadIconW")
)

const (
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	// nifGuid tells Shell_NotifyIconW to identify this icon by
	// NOTIFYICONDATA.GUIDItem rather than by (hWnd, uID) (Task 7, F5
	// first-real-run review). Without it, Windows falls back to
	// identifying the icon by the executable's path plus that pair —
	// fragile the moment the icon's owning window is a message-only
	// window created fresh on every launch (as this tray's is), and
	// exactly why a user's choice to promote the icon out of the
	// overflow area was not being remembered across restarts: Windows
	// had no stable identity to remember it *by*.
	nifGuid = 0x00000020

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	// wmTrayCallback is the private message Shell_NotifyIconW delivers
	// mouse events on, chosen one past wmRunFunc (win32_windows.go) so
	// the two message spaces never collide even though tray windows and
	// WebView2 windows use different WndProcs entirely.
	wmTrayCallback = wmApp + 2

	mfString    = 0x00000000
	mfSeparator = 0x00000800

	// D-089's rule — a menu item that looks enabled and does nothing
	// reads as a broken program, so a placeholder is greyed out rather
	// than silently inert — needed MF_GRAYED|MF_DISABLED while Open was
	// one. F6 §1 gives Open a window, nothing else in this menu is a
	// placeholder, and the constants and their helper are deleted
	// rather than kept for a hypothetical next one: unused code that
	// documents a rule is worse at it than the rule written down, which
	// it is, here and in D-089.

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	// idiApplication is IDI_APPLICATION, a MAKEINTRESOURCE ordinal — the
	// default Windows application icon. F5 §3 requires an icon present
	// from startup, not a specific brand mark; a proper .ico asset
	// arrives with F10's packaging pipeline (D-082).
	idiApplication = 32512
)

// notifyIconDataW mirrors NOTIFYICONDATAW. The fixed-size WCHAR arrays
// match the Win32 struct exactly; Go's fixed-size array types give the
// same layout as C arrays with no padding surprises here since every
// preceding field is naturally aligned.
type notifyIconDataW struct {
	CbSize          uint32
	Hwnd            uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        windows.GUID
	BalloonIcon     uintptr
}

// trayIconGUID is this tray icon's permanent identity (Task 7): fixed
// forever, never regenerated or derived from anything build- or
// machine-specific, so Shell_NotifyIconW's NIF_GUID promotion state
// (the user having dragged the icon out of the overflow area) survives
// every future restart of the agent, not just this one.
var trayIconGUID = windows.GUID{
	Data1: 0x4fc9e32a,
	Data2: 0xd77f,
	Data3: 0x4142,
	Data4: [8]byte{0x91, 0xd3, 0xb3, 0x54, 0x75, 0xde, 0x27, 0x68},
}

const (
	trayCmdOpen = iota + 1001
	trayCmdSettings
	trayCmdCertificates
	trayCmdAuditLog
	trayCmdQuit
)

type tray struct {
	hwnd uintptr

	opts TrayOptions

	closedCh  chan struct{}
	closeOnce sync.Once
}

var trayClassOnce sync.Once
var trayClassName = windows.StringToUTF16Ptr("LiroBridgeTray")
var trayWndProcCallback = windows.NewCallback(trayWndProc)

func registerTrayClass() {
	trayClassOnce.Do(func() {
		var wc wndClassExW
		wc.Size = uint32(unsafe.Sizeof(wc))
		wc.WndProc = trayWndProcCallback
		wc.Instance = moduleHandle()
		wc.ClassName = trayClassName
		r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if r == 0 {
			panic(fmt.Sprintf("ui: RegisterClassExW (tray) failed: %v", err))
		}
	})
}

func newTray(opts TrayOptions) (Tray, error) {
	ensureDPIAware()

	t := &tray{opts: opts, closedCh: make(chan struct{})}
	ready := make(chan error, 1)
	go t.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return t, nil
}

func (t *tray) run(ready chan<- error) {
	runtime.LockOSThread()

	registerTrayClass()

	// A message-only window (HWND_MESSAGE parent, -3) never becomes
	// visible and needs no title, size or style beyond receiving
	// messages — exactly what the tray needs to host Shell_NotifyIconW's
	// callback and the popup menu's WM_COMMAND replies.
	const hwndMessage = ^uintptr(2) // (HWND)(-3), sign-extended
	hwnd, _, callErr := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(trayClassName)), 0, 0,
		0, 0, 0, 0,
		hwndMessage, 0, moduleHandle(), 0,
	)
	if hwnd == 0 {
		ready <- fmt.Errorf("ui: CreateWindowExW (tray): %v", callErr)
		return
	}
	t.hwnd = hwnd
	setWindowUserData(hwnd, unsafe.Pointer(t))

	// Task 5, F5 review: the real Liro mark (scripts/genicon), not the
	// generic Windows application icon — falls back to IDI_APPLICATION
	// only if the embedded .ico can't be extracted or loaded, so a
	// packaging problem degrades the tray icon rather than stopping the
	// agent from starting at all.
	icon := loadTrayIcon(hwnd)
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, uintptr(idiApplication))
	}

	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.Hwnd = hwnd
	nid.ID = 1
	nid.Flags = nifMessage | nifIcon | nifTip | nifGuid
	nid.CallbackMessage = wmTrayCallback
	nid.Icon = icon
	nid.GUIDItem = trayIconGUID
	copyUTF16(nid.Tip[:], trayTooltip(t.opts.Version))

	if r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); r == 0 {
		// NIM_ADD can fail for a GUID Windows still has stale state for
		// (e.g. a previous instance that crashed instead of reaching
		// NIM_DELETE) — NIM_DELETE-then-retry is Microsoft's own
		// documented recovery for exactly this, and strictly safer than
		// falling back to an unstable (hWnd, uID) identity that would
		// reintroduce the bug this GUID exists to fix.
		var stale notifyIconDataW
		stale.CbSize = uint32(unsafe.Sizeof(stale))
		stale.Flags = nifGuid
		stale.GUIDItem = trayIconGUID
		_, _, _ = procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&stale)))
		if r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); r == 0 {
			_, _, _ = procDestroyWindow.Call(hwnd)
			ready <- fmt.Errorf("ui: Shell_NotifyIconW(NIM_ADD) failed")
			return
		}
	}

	ready <- nil
	pumpUntil(func() bool { return false })

	var del notifyIconDataW
	del.CbSize = uint32(unsafe.Sizeof(del))
	del.Hwnd = hwnd
	del.ID = 1
	del.Flags = nifGuid
	del.GUIDItem = trayIconGUID
	_, _, _ = procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&del)))
}

// trayTooltip implements F5 §3's "tooltip shows the version."
func trayTooltip(version string) string {
	if version == "" {
		return "Liro Bridge"
	}
	return "Liro Bridge " + version
}

// copyUTF16 encodes s into dst as a NUL-terminated UTF-16 string,
// truncating if necessary rather than overflowing the fixed-size
// NOTIFYICONDATAW field it targets.
func copyUTF16(dst []uint16, s string) {
	u := windows.StringToUTF16(s)
	n := len(u)
	if n > len(dst) {
		n = len(dst)
	}
	copy(dst, u[:n])
	dst[len(dst)-1] = 0
}

func trayWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	p := getWindowUserData(hwnd)
	if p == nil {
		return defWindowProc(hwnd, msg, wparam, lparam)
	}
	t := (*tray)(p)

	switch msg {
	case wmTrayCallback:
		switch lparam {
		case wmLButtonUp:
			if t.opts.OnOpen != nil {
				t.opts.OnOpen()
			}
		case wmRButtonUp:
			t.showMenu()
		}
		return 0

	case wmCommand:
		switch wparam & 0xFFFF {
		case trayCmdOpen:
			if t.opts.OnOpen != nil {
				t.opts.OnOpen()
			}
		case trayCmdSettings:
			if t.opts.OnSettings != nil {
				t.opts.OnSettings()
			}
		case trayCmdCertificates:
			if t.opts.OnCertificates != nil {
				t.opts.OnCertificates()
			}
		case trayCmdAuditLog:
			if t.opts.OnAuditLog != nil {
				t.opts.OnAuditLog()
			}
		case trayCmdQuit:
			if t.opts.OnQuit != nil {
				t.opts.OnQuit()
			}
			_, _, _ = procDestroyWindow.Call(hwnd)
		}
		return 0

	case wmClose:
		_, _, _ = procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		close(t.closedCh)
		postQuitMessage(0)
		return 0

	default:
		return defWindowProc(hwnd, msg, wparam, lparam)
	}
}

const (
	wmCommand   = 0x0111
	wmLButtonUp = 0x0202
	wmRButtonUp = 0x0205
)

// showMenu implements F5 §3's right-click menu: Open, Settings,
// Certificates, View audit log, Quit.
//
// Open was shown disabled through F5, because it was a placeholder and
// an item that looks enabled and does nothing reads as a broken program
// (D-089). F6 §1 gives it a window to open, so it is an ordinary item
// again.
func (t *tray) showMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer func() { _, _, _ = procDestroyMenu.Call(hMenu) }()

	// Asked for every time the menu is built, so a language changed in
	// Settings reaches the menu that opened it.
	var labels TrayLabels
	if t.opts.Labels != nil {
		labels = t.opts.Labels()
	}
	appendMenuItem(hMenu, trayCmdOpen, labels.Open)
	appendMenuItem(hMenu, trayCmdSettings, labels.Settings)
	appendMenuItem(hMenu, trayCmdCertificates, labels.Certificates)
	appendMenuItem(hMenu, trayCmdAuditLog, labels.AuditLog)
	_, _, _ = procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	appendMenuItem(hMenu, trayCmdQuit, labels.Quit)

	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	// The window must be the foreground window before TrackPopupMenu, and
	// a follow-up WM_NULL is posted afterward — both are Microsoft's own
	// documented workaround for the menu not dismissing correctly when
	// the user clicks elsewhere (TrackPopupMenu's own reference page).
	_, _, _ = procSetForegroundWindow.Call(t.hwnd)
	_, _, _ = procTrackPopupMenu.Call(hMenu, tpmRightButton, uintptr(pt.X), uintptr(pt.Y), 0, t.hwnd, 0)
	postMessage(t.hwnd, 0 /* WM_NULL */, 0, 0)
}

func appendMenuItem(hMenu uintptr, id int, label string) {
	l, _ := windows.UTF16PtrFromString(label)
	_, _, _ = procAppendMenuW.Call(hMenu, mfString, uintptr(id), uintptr(unsafe.Pointer(l)))
}

func (t *tray) Close() error {
	t.closeOnce.Do(func() {
		postMessage(t.hwnd, wmClose, 0, 0)
	})
	return nil
}
