// Command b22probe is the holder half of B22's measurement: it raises the
// shipped PIN dialog and two controls, and a separate process (probe.cs) reads
// them. It ships in nothing. See README.md; the reading it took is D-395.
//
//	go build -o b22probe.exe ./scripts/b22probe
//
// It is here rather than in a temp directory for the reason the CI comment
// beside scripts/pinmem gives about the first pinmem: that one lived in /tmp
// and went with a reboot. B24 is the open item that would want this again.
//
// During the measurement itself this file was outside the repository and built
// through `go build -overlay=`, which maps a path into the module for one
// build, so that a session under "do not change any code" could still import
// internal/ui. overlay.json is kept next to this file as the record of that;
// it is not needed now and its paths are that session's scratchpad.
//
// It raises three windows, in this order, and each is the subject of a
// measurement taken by an entirely separate process (probe.cs):
//
//	A - a plain EDIT control, no ES_PASSWORD.        THE INSTRUMENT'S CONTROL.
//	    If the probe cannot read this one, every absence it reports afterwards
//	    is worthless. On Linux the first control failed silently and the
//	    absence read as clean; this is that control, built first.
//	B - a bare EDIT control WITH ES_PASSWORD, this program's own window.
//	    Separates "ES_PASSWORD is what stops it" from "something about Liro's
//	    dialog stops it". Neither answer is available without it.
//	C - ui.CollectPIN. The real shipped dialog, the real code path, no
//	    substitute and no replica.
//
// Nothing here sends input. The owner types. D-094.
//
// There is a second control, at the subject rather than at the instrument: in
// every phase this program reads its own control at the moment OK is pressed
// and says whether the needle was actually in it. An instrument's silence
// about an empty box is not a finding.
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetForegroundWin = user32.NewProc("SetForegroundWindow")
	procSetFocus         = user32.NewProc("SetFocus")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procIsDialogMessageW = user32.NewProc("IsDialogMessageW")
	procSendMessageW     = user32.NewProc("SendMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procLoadCursorW      = user32.NewProc("LoadCursorW")
	procGetSysColorBrush = user32.NewProc("GetSysColorBrush")
	procGetStockObject   = gdi32.NewProc("GetStockObject")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	wsChild         = 0x40000000
	wsVisible       = 0x10000000
	wsPopup         = 0x80000000
	wsCaption       = 0x00C00000
	wsSysMenu       = 0x00080000
	wsTabStop       = 0x00010000
	wsBorder        = 0x00800000
	wsGroup         = 0x00020000
	wsExClientEdge  = 0x00000200
	wsExControlPar  = 0x00010000
	wsExDlgModal    = 0x00000001
	esPassword      = 0x00000020
	esAutoHScroll   = 0x00000080
	bsDefPushButton = 0x00000001
	ssLeft          = 0x00000000

	wmDestroy      = 0x0002
	wmClose        = 0x0010
	wmSetFont      = 0x0030
	wmGetText      = 0x000D
	wmCommand      = 0x0111
	emSetLimitText = 0x00C5

	swShow         = 5
	idcArrow       = 32512
	colorBtnFace   = 15
	defaultGUIFont = 17
	idOK           = 1
	idEdit         = 1001

	maxLen = 15 // characters - the same bound the real dialog is given below
)

type wndClassExW struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

type point struct{ X, Y int32 }

type msgT struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

// window is one control phase: a popup with a single EDIT and an OK button.
type window struct {
	hwnd   uintptr
	edit   uintptr
	needle string
	held   bool // the needle was in the control when OK was pressed
	n      int
}

var current *window

func wndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmCommand:
		if uint16(wparam) == idOK && current != nil && current.hwnd == hwnd {
			current.readAndCompare()
			_, _, _ = procDestroyWindow.Call(hwnd)
			return 0
		}
	case wmClose:
		_, _, _ = procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		_, _, _ = procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

// readAndCompare reads this program's own control and records whether the
// needle was in it. It never makes a Go string of what it read, and it wipes
// the buffer, because the habit is the point even in a probe.
func (w *window) readAndCompare() {
	wide := make([]uint16, maxLen+1)
	got, _, _ := procSendMessageW.Call(w.edit, wmGetText,
		uintptr(len(wide)), uintptr(unsafe.Pointer(&wide[0])))
	runes := utf16.Decode(wide[:got])
	want := []rune(w.needle)
	w.n = len(runes)
	w.held = len(runes) == len(want)
	if w.held {
		for i := range want {
			if runes[i] != want[i] {
				w.held = false
				break
			}
		}
	}
	for i := range runes {
		runes[i] = 0
	}
	for i := range wide {
		wide[i] = 0
	}
}

var classRegistered = map[string]bool{}

func ensureClass(name string) *uint16 {
	n := u16(name)
	if classRegistered[name] {
		return n
	}
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	brush, _, _ := procGetSysColorBrush.Call(colorBtnFace)
	mod, _, _ := procGetModuleHandleW.Call(0)
	c := wndClassExW{
		Style:      0x0002 | 0x0001, // CS_HREDRAW|CS_VREDRAW
		WndProc:    windows.NewCallback(wndProc),
		Instance:   syscall.Handle(mod),
		Cursor:     syscall.Handle(cursor),
		Background: syscall.Handle(brush),
		ClassName:  n,
	}
	c.Size = uint32(unsafe.Sizeof(c))
	r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&c)))
	if r == 0 {
		panic(fmt.Sprintf("RegisterClassExW(%s): %v", name, e))
	}
	classRegistered[name] = true
	return n
}

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

// showEdit runs one control phase and returns whether the needle was in the
// control when OK was pressed.
func showEdit(class, title, label, needle string, password bool) bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cls := ensureClass(class)
	mod, _, _ := procGetModuleHandleW.Call(0)
	hwnd, _, e := procCreateWindowExW.Call(
		wsExDlgModal|wsExControlPar,
		uintptr(unsafe.Pointer(cls)),
		uintptr(unsafe.Pointer(u16(title))),
		wsPopup|wsCaption|wsSysMenu,
		220, 220, 460, 210,
		0, 0, mod, 0,
	)
	if hwnd == 0 {
		fmt.Printf("  CreateWindowExW failed: %v\n", e)
		return false
	}
	w := &window{hwnd: hwnd, needle: needle}
	current = w

	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	mk := func(c, text string, style, ex uintptr, x, y, cw, ch, id int) uintptr {
		h, _, _ := procCreateWindowExW.Call(ex,
			uintptr(unsafe.Pointer(u16(c))),
			uintptr(unsafe.Pointer(u16(text))),
			wsChild|wsVisible|style,
			uintptr(x), uintptr(y), uintptr(cw), uintptr(ch),
			hwnd, uintptr(id), mod, 0)
		if h != 0 && font != 0 {
			_, _, _ = procSendMessageW.Call(h, wmSetFont, font, 1)
		}
		return h
	}

	mk("STATIC", label, ssLeft, 0, 16, 14, 410, 38, 0)
	style := uintptr(esAutoHScroll | wsBorder | wsTabStop | wsGroup)
	if password {
		style |= esPassword
	}
	w.edit = mk("EDIT", "", style, wsExClientEdge, 16, 62, 410, 26, idEdit)
	if w.edit == 0 {
		fmt.Println("  the EDIT control could not be created")
		return false
	}
	_, _, _ = procSendMessageW.Call(w.edit, emSetLimitText, maxLen, 0)
	mk("BUTTON", "OK", bsDefPushButton|wsTabStop, 0, 330, 106, 96, 30, idOK)

	_, _, _ = procShowWindow.Call(hwnd, swShow)
	_, _, _ = procSetForegroundWin.Call(hwnd)
	_, _, _ = procSetFocus.Call(w.edit)

	var msg msgT
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if ok, _, _ := procIsDialogMessageW.Call(hwnd, uintptr(unsafe.Pointer(&msg))); ok != 0 {
			continue
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	current = nil
	return w.held
}

type handover struct {
	RunID   string `json:"run_id"`
	PID     int    `json:"pid"`
	Started string `json:"started"`
	NeedleA string `json:"needle_a"`
	NeedleB string `json:"needle_b"`
	NeedleC string `json:"needle_c"`
	ClassA  string `json:"class_a"`
	ClassB  string `json:"class_b"`
	ClassC  string `json:"class_c"`
}

const (
	classA = "LiroProbeControlPlain"
	classB = "LiroProbeControlPassword"
	classC = "LiroBridgePINDialog" // the real dialog's own class, not ours
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	h := handover{
		RunID:   randNeedle(6),
		PID:     os.Getpid(),
		Started: time.Now().Format(time.RFC3339),
		NeedleA: randNeedle(8),
		NeedleB: randNeedle(8),
		NeedleC: randNeedle(8),
		ClassA:  classA, ClassB: classB, ClassC: classC,
	}
	hp := filepath.Join(dir, "handover.json")
	dp := filepath.Join(dir, "phases-done")
	_ = os.Remove(dp)
	b, _ := json.MarshalIndent(h, "", "  ")
	if err := os.WriteFile(hp, b, 0o600); err != nil {
		panic(err)
	}

	fmt.Printf("\n  HOLDER  pid=%d  run=%s  started=%s\n", h.PID, h.RunID, h.Started)
	fmt.Printf("  handover: %s\n\n", hp)
	fmt.Println("  Three windows, in order. Type the needle for that window, WAIT TWO")
	fmt.Println("  SECONDS so a poller cannot miss it, then press Enter.")
	fmt.Println()
	fmt.Printf("    A  plain edit, no ES_PASSWORD  (instrument control) : %s\n", h.NeedleA)
	fmt.Printf("    B  bare ES_PASSWORD edit, this program's window     : %s\n", h.NeedleB)
	fmt.Printf("    C  the real Liro Bridge PIN dialog                  : %s\n", h.NeedleC)
	fmt.Println()
	fmt.Println("  Needles are case-sensitive and random per run. Start the probe in the")
	fmt.Println("  other terminal now, then press Enter here.")
	fmt.Print("  > ")
	in := bufio.NewScanner(os.Stdin)
	in.Scan()

	fmt.Println("\n  --- A: plain edit, no ES_PASSWORD -------------------------------")
	report("A", showEdit(classA, "Probe A - plain edit",
		"Type needle A, wait 2 s, then press Enter.\nThis control has no ES_PASSWORD.", h.NeedleA, false))
	time.Sleep(1500 * time.Millisecond)

	fmt.Println("\n  --- B: bare ES_PASSWORD edit ------------------------------------")
	report("B", showEdit(classB, "Probe B - password edit",
		"Type needle B, wait 2 s, then press Enter.\nThe same control, with ES_PASSWORD set.", h.NeedleB, true))
	time.Sleep(1500 * time.Millisecond)

	fmt.Println("\n  --- C: the real dialog, ui.CollectPIN ---------------------------")
	report("C", realDialog(h.NeedleC))

	_ = os.WriteFile(dp, []byte(h.RunID), 0o600)
	fmt.Println("\n  All three phases done. The probe prints its report and exits.")
	fmt.Println("  Press Enter to close this window.")
	fmt.Print("  > ")
	in.Scan()
}

func report(phase string, held bool) {
	if held {
		fmt.Printf("  phase %s: the needle WAS in the control when OK was pressed.\n", phase)
		fmt.Printf("           So any silence the probe reports for %s is about the probe.\n", phase)
		return
	}
	fmt.Printf("  phase %s: the needle was NOT in the control - MISTYPED OR CANCELLED.\n", phase)
	fmt.Printf("           Nothing the probe says about phase %s means anything. Re-run.\n", phase)
}

// realDialog calls the shipped CollectPIN with a prompt built from the real
// catalogue, and reports only whether what came back was the needle.
func realDialog(needle string) bool {
	c := i18n.Load("sr-Latn")
	prompt := ui.PINPrompt{
		Title:   c.T("pindialog.title"),
		Heading: c.T("pindialog.heading"),
		Subject: fmt.Sprintf(c.T("pindialog.subject"), "B22 PROBE - NOT A REAL CARD"),
		Label:   c.T("pindialog.label"),
		Hint:    fmt.Sprintf(c.T("pindialog.hint"), 4, maxLen),
		OK:      c.T("pindialog.ok"),
		Cancel:  c.T("pindialog.cancel"),
	}
	dst := make([]byte, 64)
	defer func() {
		for i := range dst {
			dst[i] = 0
		}
	}()
	n, ok, err := ui.CollectPIN(0, prompt, maxLen, dst)
	if err != nil {
		fmt.Printf("  CollectPIN returned an error: %v\n", err)
		return false
	}
	if !ok {
		fmt.Println("  CollectPIN: cancelled.")
		return false
	}
	want := []byte(needle)
	if n != len(want) {
		return false
	}
	for i := range want {
		if dst[i] != want[i] {
			return false
		}
	}
	return true
}

// randNeedle: crypto/rand, an alphabet with no lookalikes, so a mistyped
// needle is the owner's hand rather than the font's.
func randNeedle(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	out := make([]byte, n)
	for i := range b {
		out[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(out)
}
