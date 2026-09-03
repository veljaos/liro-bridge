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

	// mfGrayed and mfDisabled together render a menu item greyed out and
	// unselectable (Task 4, F5 review): "Open" stays a genuine
	// placeholder this phase (F6 supplies the real main window), but an
	// item that looks enabled and does nothing when clicked reads as a
	// broken program, not an unfinished feature — MF_GRAYED alone dims
	// the text but on some Windows versions still delivers WM_COMMAND on
	// click; MF_DISABLED is what actually stops that.
	mfGrayed   = 0x00000001
	mfDisabled = 0x00000002

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
	icon := loadTrayIcon()
	if icon == 0 {
		icon, _, _ = procLoadIconW.Call(0, uintptr(idiApplication))
	}

	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.Hwnd = hwnd
	nid.ID = 1
	nid.Flags = nifMessage | nifIcon | nifTip
	nid.CallbackMessage = wmTrayCallback
	nid.Icon = icon
	copyUTF16(nid.Tip[:], trayTooltip(t.opts.Version))

	if r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); r == 0 {
		_, _, _ = procDestroyWindow.Call(hwnd)
		ready <- fmt.Errorf("ui: Shell_NotifyIconW(NIM_ADD) failed")
		return
	}

	ready <- nil
	pumpUntil(func() bool { return false })

	var del notifyIconDataW
	del.CbSize = uint32(unsafe.Sizeof(del))
	del.Hwnd = hwnd
	del.ID = 1
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
// Certificates, View audit log, Quit. Open is shown disabled (Task 4):
// it stays a genuine F6 placeholder, but must not appear clickable and
// silently do nothing.
func (t *tray) showMenu() {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer func() { _, _, _ = procDestroyMenu.Call(hMenu) }()

	appendMenuItemDisabled(hMenu, trayCmdOpen, t.opts.Labels.Open)
	appendMenuItem(hMenu, trayCmdSettings, t.opts.Labels.Settings)
	appendMenuItem(hMenu, trayCmdCertificates, t.opts.Labels.Certificates)
	appendMenuItem(hMenu, trayCmdAuditLog, t.opts.Labels.AuditLog)
	_, _, _ = procAppendMenuW.Call(hMenu, mfSeparator, 0, 0)
	appendMenuItem(hMenu, trayCmdQuit, t.opts.Labels.Quit)

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

// appendMenuItemDisabled adds a menu item that is visible but neither
// clickable nor selectable (Task 4's "disable... with a 'coming soon'
// state" option, applied to Open — see showMenu).
func appendMenuItemDisabled(hMenu uintptr, id int, label string) {
	l, _ := windows.UTF16PtrFromString(label)
	_, _, _ = procAppendMenuW.Call(hMenu, mfString|mfGrayed|mfDisabled, uintptr(id), uintptr(unsafe.Pointer(l)))
}

func (t *tray) Close() error {
	t.closeOnce.Do(func() {
		postMessage(t.hwnd, wmClose, 0, 0)
	})
	return nil
}
