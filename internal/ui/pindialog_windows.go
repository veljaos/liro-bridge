//go:build windows

package ui

// The PIN dialog: the one window in this product that is not HTML.
//
// SPEC §10 carries the reason and D-277 carries the argument. In one line:
// §6.5.1's second clause requires the PIN to exist only for the length of one
// C_Login and to be overwritten afterwards, and that is a statement about this
// program's memory — a page rendered in WebView2 is not this program's memory,
// because the browser runs it in another process whose heap this program can
// neither reach nor wipe, and because the only way a page hands data back
// (Eval) returns a Go string, which cannot be overwritten at all.
//
// So the PIN is collected by a window this program draws, in controls this
// program owns, into a buffer the caller allocated. Nothing here ever makes a
// Go string out of it.
//
// It is also the familiar shape rather than the novel one: Windows already
// collects the PIN in its own window on the CNG path, and so does every other
// program that touches a Serbian card. Which is exactly why the labels below
// say Liro Bridge — §6.5.1's sixth clause matters *more* on a window that
// looks like a system dialog, not less.
//
// # What this file must never do
//
//   - make a string, a fmt argument or a log field out of the characters typed
//   - keep them anywhere after Collect returns
//   - retry, or offer to
//
// internal/ui/pin_test.go is the mechanical half of the first two.

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	// gdi32 is not otherwise used by this package: the PIN dialog is the one
	// window whose text this program draws rather than a browser.
	gdi32DLL = windows.NewLazySystemDLL("gdi32.dll")

	procCreateFontW      = gdi32DLL.NewProc("CreateFontW")
	procDeleteObject     = gdi32DLL.NewProc("DeleteObject")
	procIsDialogMessageW = user32DLL.NewProc("IsDialogMessageW")
)

const (
	// Control identifiers. Cancel is IDCANCEL so that IsDialogMessageW turns
	// Escape into a click on it without this file handling Escape at all.
	idPINEdit   = 1001
	idPINOK     = 1 // IDOK
	idPINCancel = 2 // IDCANCEL

	// Window and control styles used here.
	wsChild         = 0x40000000
	wsTabStop       = 0x00010000
	wsBorder        = 0x00800000
	wsGroup         = 0x00020000
	wsExClientEdge  = 0x00000200
	wsExControlPar  = 0x00010000 // WS_EX_CONTROLPARENT: Tab crosses into children
	wsExDlgModal    = 0x00000001 // WS_EX_DLGMODALFRAME
	esPassword      = 0x00000020
	esAutoHScroll   = 0x00000080
	bsDefPushButton = 0x00000001
	bsPushButton    = 0x00000000
	ssLeft          = 0x00000000

	// Messages. wmCommand is declared in tray_windows.go, which got there
	// first for the tray menu, and means the same thing here.
	wmSetFont      = 0x0030
	wmGetText      = 0x000D
	wmSetText      = 0x000C
	emSetLimitText = 0x00C5

	// SetWindowPos flags, named rather than spelled at each call site.
	swpNoSize   = 0x0001
	swpNoMove   = 0x0002
	swpNoZOrder = 0x0004

	// Layout, in unscaled points at 96 DPI. Every one of these is multiplied
	// by the window's own DPI before use.
	pinDlgWidth   = 380
	pinDlgPadding = 16
	pinRowGap     = 8
	pinLineHeight = 18
	pinEditHeight = 26
	pinButtonW    = 92
	pinButtonH    = 28
)

// pinDialog is one instance's state, reachable from the window procedure
// through GWLP_USERDATA.
//
// It holds the UTF-16 buffer the edit control is read into and the destination
// the caller owns. It never holds a string: `dst` and `wide` are the only two
// places the characters exist, both are this program's, and both are wiped
// before Collect returns.
type pinDialog struct {
	hwnd   uintptr
	edit   uintptr
	font   uintptr
	bold   uintptr
	dst    []byte
	wide   []uint16
	n      int
	ok     bool
	closed bool

	// tooLong separates "what was typed does not fit the token's buffer" from
	// "the person pressed Cancel", which were one answer until the dialog
	// acquired its first caller. See ErrPINTooLong.
	tooLong bool
}

// CollectPIN shows the dialog and writes what was typed into dst, UTF-8
// encoded, returning how many bytes it wrote.
//
// owner is the window this one belongs to; it is disabled for as long as the
// dialog is up and re-enabled before the dialog is destroyed, so activation
// returns to it rather than to whatever else is on the desktop — D-129's
// finding, where a window opened from another with no owner was created
// underneath it and the one behind then froze.
//
// ok is false when the person cancelled. That is not an error and must not be
// reported as one.
//
// The one other way ok is false is with err set to ErrPINTooLong: what was
// typed fitted the edit control, which counts characters, and did not fit the
// token's own buffer, which counts bytes. It is a distinct answer rather than
// a cancellation because a person who typed something and pressed OK has not
// cancelled anything, and because the two call for different sentences.
//
// It runs on its own OS thread, locked, with its own message loop. Not on the
// shared UI thread (D-207) on purpose: this dialog blocks until it is
// answered, and blocking the thread every other window in the process is
// pumped by would freeze all of them — which is the defect D-129 measured from
// the other direction.
func CollectPIN(owner uintptr, prompt PINPrompt, maxLen int, dst []byte) (n int, ok bool, err error) {
	if maxLen <= 0 || maxLen > len(dst) {
		return 0, false, fmt.Errorf("ui: a PIN buffer of %d bytes cannot hold %d characters", len(dst), maxLen)
	}
	ensureDPIAware()

	type result struct {
		n   int
		ok  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		rn, rok, rerr := runPINDialog(owner, prompt, maxLen, dst)
		done <- result{rn, rok, rerr}
	}()
	r := <-done
	return r.n, r.ok, r.err
}

func runPINDialog(owner uintptr, prompt PINPrompt, maxLen int, dst []byte) (int, bool, error) {
	d := &pinDialog{
		dst: dst,
		// One extra for the terminating NUL WM_GETTEXT always writes.
		wide: make([]uint16, maxLen+1),
	}
	// The UTF-16 buffer is pinned for the same reason the PIN buffer in
	// internal/keysource/pkcs11 is: its address is handed to foreign code
	// (WM_GETTEXT writes through it), and a stack that grows moves a frame
	// while KeepAlive says nothing about a copy (D-101).
	var pin runtime.Pinner
	defer func() {
		wipeUTF16(d.wide)
		pin.Unpin()
	}()
	pin.Pin(&d.wide[0])

	title, err := utf16Buf(prompt.Title)
	if err != nil {
		return 0, false, err
	}

	ensurePINClass()
	dpi := 96
	hwnd, _, _ := procCreateWindowExW.Call(
		wsExDlgModal|wsExControlPar,
		uintptr(unsafe.Pointer(pinClassName)),
		uintptr(unsafe.Pointer(&title[0])),
		wsPopup|wsCaption|wsSysMenu,
		0, 0, 100, 100,
		owner, 0, moduleHandle(), 0,
	)
	if hwnd == 0 {
		return 0, false, fmt.Errorf("ui: CreateWindowExW for the PIN dialog returned 0")
	}
	d.hwnd = hwnd
	// Attached after creation rather than through WM_NCCREATE's lpCreateParams:
	// this package already has setWindowUserData and no CREATESTRUCT binding,
	// and the window procedure's own nil check covers the handful of messages
	// that arrive before this line (D-080's discipline — do not add a struct
	// layout that nothing has measured).
	setWindowUserData(hwnd, unsafe.Pointer(d))
	// d is reachable from foreign code for as long as the window lives, so it
	// is pinned rather than merely kept alive (D-101).
	pin.Pin(d)

	if r, _, _ := procGetDpiForWindow.Call(hwnd); r != 0 {
		dpi = int(r)
	}
	if err := d.build(prompt, maxLen, dpi); err != nil {
		_, _, _ = procDestroyWindow.Call(hwnd)
		return 0, false, err
	}
	d.centreOn(owner, dpi)

	if owner != 0 {
		_, _, _ = procEnableWindow.Call(owner, 0)
	}
	_, _, _ = procShowWindow.Call(hwnd, swShow)
	_, _, _ = procSetForegroundWindow.Call(hwnd)
	_, _, _ = procSetFocus.Call(d.edit)

	d.pump()

	if owner != 0 {
		// Before DestroyWindow, so activation goes back to the owner rather
		// than to whatever else is on the desktop (D-129).
		_, _, _ = procEnableWindow.Call(owner, 1)
	}
	// Both fonts are GDI objects this window created and therefore owns. A
	// window that leaks one leaks it per window, which is invisible until
	// somebody counts across many (D-169 measured exactly that, six GDI
	// objects per window, for the two icons).
	for _, f := range []*uintptr{&d.font, &d.bold} {
		if *f != 0 {
			_, _, _ = procDeleteObject.Call(*f)
			*f = 0
		}
	}
	if d.tooLong {
		return 0, false, ErrPINTooLong
	}
	if !d.ok {
		return 0, false, nil
	}
	return d.n, true, nil
}

// build creates the controls. Every coordinate is scaled from the constants
// above by this window's own DPI, so the dialog is the same size in points on
// a scaled display rather than the same size in pixels.
func (d *pinDialog) build(prompt PINPrompt, maxLen, dpi int) error {
	s := func(v int) int { return v * dpi / 96 }

	d.font = createUIFont(dpi, 400)
	// The heading is SPEC §6.5.1's sixth clause and nothing else on this
	// window is. Measured by looking at the first capture: at one weight it
	// read as a caption rather than as the thing the window is for.
	d.bold = createUIFont(dpi, 700)
	w := s(pinDlgWidth)
	pad := s(pinDlgPadding)
	gap := s(pinRowGap)
	line := s(pinLineHeight)
	inner := w - 2*pad

	y := pad
	mkFont := func(class string, text string, style uintptr, exStyle uintptr, x, top, cw, ch int, id int, font uintptr) (uintptr, error) {
		cls, err := utf16Buf(class)
		if err != nil {
			return 0, err
		}
		txt, err := utf16Buf(text)
		if err != nil {
			return 0, err
		}
		h, _, _ := procCreateWindowExW.Call(
			exStyle,
			uintptr(unsafe.Pointer(&cls[0])),
			uintptr(unsafe.Pointer(&txt[0])),
			wsChild|wsVisible|style,
			uintptr(x), uintptr(top), uintptr(cw), uintptr(ch),
			d.hwnd, uintptr(id), moduleHandle(), 0,
		)
		if h == 0 {
			return 0, fmt.Errorf("ui: creating the PIN dialog's %s control returned 0", class)
		}
		if font != 0 {
			_, _, _ = procSendMessageW.Call(h, wmSetFont, font, 1)
		}
		return h, nil
	}
	mk := func(class string, text string, style uintptr, exStyle uintptr, x, top, cw, ch int, id int) (uintptr, error) {
		return mkFont(class, text, style, exStyle, x, top, cw, ch, id, d.font)
	}

	// The heading is SPEC §6.5.1 clause 6 and is drawn first for that reason:
	// the first thing read on this window says which program is asking.
	if _, err := mkFont("STATIC", prompt.Heading, ssLeft, 0, pad, y, inner, line, 0, d.bold); err != nil {
		return err
	}
	y += line + gap

	if prompt.Subject != "" {
		if _, err := mk("STATIC", prompt.Subject, ssLeft, 0, pad, y, inner, 2*line, 0); err != nil {
			return err
		}
		y += 2*line + gap
	}

	if _, err := mk("STATIC", prompt.Label, ssLeft, 0, pad, y, inner, line, 0); err != nil {
		return err
	}
	y += line + s(4)

	edit, err := mk("EDIT", "",
		esPassword|esAutoHScroll|wsBorder|wsTabStop|wsGroup, wsExClientEdge,
		pad, y, inner, s(pinEditHeight), idPINEdit)
	if err != nil {
		return err
	}
	d.edit = edit
	// The token's own maximum, enforced by the control itself so that a
	// person cannot type past it and then be refused. This is the near half of
	// SPEC §6.5.1's seventh clause; the far half is in the backend, which
	// checks the length again before C_Login because a control is a courtesy
	// and the clause is a requirement.
	_, _, _ = procSendMessageW.Call(edit, emSetLimitText, uintptr(maxLen), 0)
	y += s(pinEditHeight) + s(4)

	if prompt.Hint != "" {
		if _, err := mk("STATIC", prompt.Hint, ssLeft, 0, pad, y, inner, line, 0); err != nil {
			return err
		}
		y += line + gap
	}
	y += gap

	bw, bh := s(pinButtonW), s(pinButtonH)
	if _, err := mk("BUTTON", prompt.Cancel, bsPushButton|wsTabStop, 0,
		w-pad-2*bw-gap, y, bw, bh, idPINCancel); err != nil {
		return err
	}
	if _, err := mk("BUTTON", prompt.OK, bsDefPushButton|wsTabStop, 0,
		w-pad-bw, y, bw, bh, idPINOK); err != nil {
		return err
	}
	y += bh + pad

	// Size the frame to the content rather than choosing a height and hoping.
	// D-208's own finding, one window over: reasoning a height out instead of
	// measuring one is how a scrollbar ships.
	rect := rect{Left: 0, Top: 0, Right: int32(w), Bottom: int32(y)}
	_, _, _ = procAdjustWindowRectEx.Call(
		uintptr(unsafe.Pointer(&rect)), wsPopup|wsCaption|wsSysMenu, 0, wsExDlgModal|wsExControlPar)
	_, _, _ = procSetWindowPos.Call(d.hwnd, 0, 0, 0,
		uintptr(rect.Right-rect.Left), uintptr(rect.Bottom-rect.Top), swpNoMove|swpNoZOrder)
	return nil
}

// centreOn puts the dialog in the middle of its owner, or of the monitor under
// the cursor when it has none.
func (d *pinDialog) centreOn(owner uintptr, _ int) {
	var self rect
	_, _, _ = procGetWindowRect.Call(d.hwnd, uintptr(unsafe.Pointer(&self)))
	w, h := self.Right-self.Left, self.Bottom-self.Top

	var host rect
	if owner != 0 {
		_, _, _ = procGetWindowRect.Call(owner, uintptr(unsafe.Pointer(&host)))
	} else {
		host = cursorMonitorRect()
	}
	x := host.Left + (host.Right-host.Left-w)/2
	y := host.Top + (host.Bottom-host.Top-h)/2
	_, _, _ = procSetWindowPos.Call(d.hwnd, 0, uintptr(x), uintptr(y), 0, 0, swpNoSize|swpNoZOrder)
}

// pump runs the dialog's own message loop.
//
// IsDialogMessageW first is what makes this window behave like a dialog
// without a dialog template: Tab moves between the controls, Enter presses the
// default button, and Escape presses IDCANCEL — all of it Windows' own
// behaviour, which is also how SPEC §10.3's keyboard reachability and visible
// focus come for free here rather than having to be built.
func (d *pinDialog) pump() {
	var msg winMsg
	for !d.closed {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if ok, _, _ := procIsDialogMessageW.Call(d.hwnd, uintptr(unsafe.Pointer(&msg))); ok != 0 {
			continue
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// accept reads the edit control into the caller's buffer.
//
// The characters go from the control into d.wide (UTF-16, this program's, and
// wiped), and from there rune by rune into dst (UTF-8, the caller's). They are
// never a Go string: windows.UTF16ToString would make one, and a Go string
// cannot be overwritten, which is the whole reason this window exists rather
// than a page (D-277).
func (d *pinDialog) accept() {
	for i := range d.wide {
		d.wide[i] = 0
	}
	got, _, _ := procSendMessageW.Call(d.edit, wmGetText,
		uintptr(len(d.wide)), uintptr(unsafe.Pointer(&d.wide[0])))

	runes := utf16.Decode(d.wide[:got])
	n := encodePINInto(d.dst, runes)
	for i := range runes {
		runes[i] = 0
	}
	runtime.KeepAlive(runes)

	if n < 0 {
		// More bytes than the token's maximum, which EM_SETLIMITTEXT counts in
		// characters rather than in UTF-8 bytes. Refused here rather than
		// truncated: half a PIN is a wrong PIN, and a wrong PIN is an attempt.
		//
		// It is reported as ErrPINTooLong rather than as ok=false, which is
		// what Cancel means. Those were one answer until this dialog acquired
		// its first caller, and a person who typed something and pressed OK
		// would have been recorded as having cancelled.
		d.n, d.ok, d.tooLong = 0, false, true
	} else {
		d.n, d.ok = n, true
	}
	d.clearEdit()
	d.closed = true
	_, _, _ = procDestroyWindow.Call(d.hwnd)
}

// clearEdit overwrites the control's own copy of what was typed.
//
// Windows keeps the text inside the edit control, which is memory this program
// does not own but can overwrite through the control's own interface. Setting
// it to empty is the nearest thing available to wiping it, and it happens
// before the window is destroyed rather than relying on destruction to do it.
func (d *pinDialog) clearEdit() {
	if d.edit == 0 {
		return
	}
	empty := []uint16{0}
	_, _, _ = procSendMessageW.Call(d.edit, wmSetText, 0, uintptr(unsafe.Pointer(&empty[0])))
}

func (d *pinDialog) cancel() {
	d.n, d.ok = 0, false
	d.clearEdit()
	d.closed = true
	_, _, _ = procDestroyWindow.Call(d.hwnd)
}

// wipeUTF16 overwrites a UTF-16 buffer. KeepAlive for the reason the backend's
// own wipe has one: with no use after the loop the stores have no reader.
func wipeUTF16(b []uint16) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

// ---------------------------------------------------------------- the class

// Both of these name a PIN and neither holds one, which is the case D-270
// measured across this tree. The explicit types are what say so to
// pin_test.go: a var with no type expression and a call for an initialiser is
// one the checker cannot see through, and it is right to be conservative
// there. Naming the type answers its question truthfully — a *uint16 class
// name and a callback trampoline cannot hold a PIN's characters — where
// renaming them would be the tail wagging the dog (D-270).
var (
	pinClassName *uint16 = windows.StringToUTF16Ptr("LiroBridgePINDialog")
	pinClassOnce sync.Once
)

func ensurePINClass() {
	pinClassOnce.Do(func() {
		var wc wndClassExW
		wc.Size = uint32(unsafe.Sizeof(wc))
		wc.WndProc = pinWndProcCallback
		wc.Instance = moduleHandle()
		wc.Cursor = loadArrowCursor()
		// COLOR_3DFACE+1. Measured by looking at it: with COLOR_WINDOW the
		// frame is white and every STATIC label paints itself against
		// COLOR_3DFACE anyway, so each label rendered as a grey block on a
		// white ground. A Win32 dialog's own background is 3DFACE, and
		// matching it is what makes this look like a dialog rather than like
		// a thing this program drew (D-087's rule, on the screen it matters
		// most on).
		wc.Background = 16
		wc.ClassName = pinClassName
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			panic(fmt.Sprintf("ui: RegisterClassExW for the PIN dialog failed: %v", err))
		}
	})
}

var pinWndProcCallback uintptr = syscall.NewCallback(pinWndProc)

func pinWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	ptr := getWindowUserData(hwnd)
	if ptr == nil {
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
		return r
	}
	d := (*pinDialog)(ptr)

	switch msg {
	case wmCommand:
		switch wparam & 0xFFFF {
		case idPINOK:
			d.accept()
			return 0
		case idPINCancel:
			d.cancel()
			return 0
		}
	case wmClose:
		// The title bar's close box means the same thing as Cancel. A window
		// closed is a person declining, not a failure (D-145).
		d.cancel()
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

// createUIFont makes the dialog's font.
//
// Segoe UI at 9pt, scaled to the window's DPI. The alternative was
// SystemParametersInfoW(SPI_GETNONCLIENTMETRICS), which is the properly
// correct answer and needs the 500-byte NONCLIENTMETRICSW laid out by hand —
// a struct layout measured by nobody, in a file whose whole subject is not
// getting layouts wrong. Naming the font is an assumption this comment can
// state; a wrong offset into a struct would be one nothing could see. If Segoe
// UI is absent, CreateFontW substitutes and the dialog still reads.
func createUIFont(dpi, weight int) uintptr {
	name, err := utf16Buf("Segoe UI")
	if err != nil {
		return 0
	}
	height := -(9 * dpi / 72)
	h, _, _ := procCreateFontW.Call(
		uintptr(int32(height)), 0, 0, 0,
		uintptr(weight),
		0, 0, 0,
		1, // DEFAULT_CHARSET
		0, // OUT_DEFAULT_PRECIS
		0, // CLIP_DEFAULT_PRECIS
		5, // CLEARTYPE_QUALITY
		0, // DEFAULT_PITCH | FF_DONTCARE
		uintptr(unsafe.Pointer(&name[0])),
	)
	return h
}
