package main

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// The PIN dialog, driven in all three locales.
//
// It lives in cmd/liro-bridge with every other window test, for the reason
// D-098 recorded: go test runs packages in parallel, every window in this
// project opens against one user data folder, and keeping the window tests in
// one package is what keeps them serialised.
//
// It is driven by **window messages only** — WM_SETTEXT into the edit control
// and WM_COMMAND to the buttons. Nothing here touches the real cursor or the
// real keyboard. D-094 forbids that outright, and the reason it forbids it is
// this screen: a synthetic click that lands on the wrong window because the
// foreground moved is bad enough over a browser tab, and over a PIN dialog it
// is a signature nobody authorised.
//
// It deliberately does **not** import internal/keysource/pkcs11. Wiring the
// backend's PINEntry to this dialog is what connects a crash path to the agent
// (D-272, D-275), and cmd/liro-bridge/pkcs11reach_test.go exists to stop it.
// The dialog takes plain strings, so none of that is needed to exercise it.

var (
	user32ForPIN     = windows.NewLazySystemDLL("user32.dll")
	procSendMsgW     = user32ForPIN.NewProc("SendMessageW")
	procPostMsgW     = user32ForPIN.NewProc("PostMessageW")
	procGetClassName = user32ForPIN.NewProc("GetClassNameW")
	procIsWindowVis  = user32ForPIN.NewProc("IsWindowVisible")
	procEnumWindows  = user32ForPIN.NewProc("EnumWindows")
	procGetWinText   = user32ForPIN.NewProc("GetWindowTextW")
	procGetWinTextL  = user32ForPIN.NewProc("GetWindowTextLengthW")
	procEnumChildWin = user32ForPIN.NewProc("EnumChildWindows")
)

const (
	wmSetTextMsg = 0x000C
	wmCommandMsg = 0x0111
	idOKButton   = 1
	idCancelBtn  = 2
)

// pinPromptFor builds the dialog's text from the real catalogue, which is what
// the eventual wiring in F11 §4 will do. Building it here rather than
// hard-coding strings is the point: it is what makes this a test of three
// locales rather than of one.
func pinPromptFor(locale, cardLabel string, minLen, maxLen int) ui.PINPrompt {
	c := i18n.Load(locale)
	return ui.PINPrompt{
		Title:   c.T("pindialog.title"),
		Heading: c.T("pindialog.heading"),
		Subject: fmt.Sprintf(c.T("pindialog.subject"), cardLabel),
		Label:   c.T("pindialog.label"),
		Hint:    fmt.Sprintf(c.T("pindialog.hint"), minLen, maxLen),
		OK:      c.T("pindialog.ok"),
		Cancel:  c.T("pindialog.cancel"),
	}
}

func classOf(hwnd uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := procGetClassName.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

func textOf(hwnd uintptr) string {
	n, _, _ := procGetWinTextL.Call(hwnd)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	got, _, _ := procGetWinText.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:got])
}

// findPINDialog waits for the dialog to be on screen and returns its handle.
//
// It waits for an observable state rather than a duration (D-201): the window
// is created some milliseconds before it is shown, and a message posted into
// that gap is dispatched into a half-built window. The deadline exists only to
// turn a dialog that never appears into a failure instead of a hang.
func findPINDialog(t *testing.T) uintptr {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var found uintptr
		cb := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
			if classOf(hwnd) != "LiroBridgePINDialog" {
				return 1
			}
			if vis, _, _ := procIsWindowVis.Call(hwnd); vis == 0 {
				return 1
			}
			found = hwnd
			return 0
		})
		_, _, _ = procEnumWindows.Call(cb, 0)
		if found != 0 {
			return found
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the PIN dialog never became visible")
	return 0
}

// childrenOf returns every control in the dialog, with its class and text.
func childrenOf(t *testing.T, parent uintptr) []struct {
	Hwnd  uintptr
	Class string
	Text  string
} {
	t.Helper()
	var out []struct {
		Hwnd  uintptr
		Class string
		Text  string
	}
	cb := windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		out = append(out, struct {
			Hwnd  uintptr
			Class string
			Text  string
		}{hwnd, classOf(hwnd), textOf(hwnd)})
		return 1
	})
	_, _, _ = procEnumChildWin.Call(parent, cb, 0)
	return out
}

func editControl(t *testing.T, dlg uintptr) uintptr {
	t.Helper()
	for _, c := range childrenOf(t, dlg) {
		if strings.EqualFold(c.Class, "Edit") {
			return c.Hwnd
		}
	}
	t.Fatal("the PIN dialog has no edit control")
	return 0
}

// TestThePINDialogSaysWhoIsAskingInEveryLocale is SPEC §6.5.1's sixth clause,
// and SPEC §10.2's reason for the PIN row's last sentence: this window looks
// like a system dialog, which is why it has to say it is Liro Bridge asking.
//
// It reads the rendered controls rather than the payload that produced them.
// Six entries in this log record that a green suite is not evidence about what
// a window shows (D-087, D-122, D-161, D-172, D-219, D-247), and this is the
// screen where that matters most.
func TestThePINDialogSaysWhoIsAskingInEveryLocale(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			prompt := pinPromptFor(locale, "Savka Odžić 200100123", 5, 15)
			dst := make([]byte, 15)

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _, _ = ui.CollectPIN(0, prompt, 15, dst)
			}()

			dlg := findPINDialog(t)
			var all []string
			for _, c := range childrenOf(t, dlg) {
				all = append(all, c.Text)
			}
			joined := strings.Join(all, " | ")

			// The heading is the clause. It must name this program, on screen,
			// in the locale the person is running.
			if !strings.Contains(joined, "Liro Bridge") {
				t.Errorf("no control on the dialog names Liro Bridge; a person cannot tell "+
					"they are giving their PIN to this program rather than to Windows "+
					"(SPEC §6.5.1 clause 6). Controls: %q", joined)
			}
			// The card, so somebody with two of them knows which is being asked about.
			if !strings.Contains(joined, "Savka Odžić 200100123") {
				t.Errorf("the dialog does not name the card. Controls: %q", joined)
			}
			// The token's own limits, never this program's numbers.
			if !strings.Contains(joined, "5") || !strings.Contains(joined, "15") {
				t.Errorf("the dialog does not state the token's own 5..15 limits. Controls: %q", joined)
			}
			if title := textOf(dlg); title == "" {
				t.Error("the dialog has no caption")
			}

			_, _, _ = procPostMsgW.Call(dlg, wmCommandMsg, idCancelBtn, 0)
			wg.Wait()
		})
	}
}

// TestThePINDialogHandsBackWhatWasTypedAndKeepsNothing drives the dialog the
// way a person does — text into the edit control, then the OK button — and
// checks both halves: the characters arrive in the caller's buffer as UTF-8,
// and the control does not still hold them afterwards.
func TestThePINDialogHandsBackWhatWasTypedAndKeepsNothing(t *testing.T) {
	// Deliberately not digits only: the conversion from the control's UTF-16
	// to the caller's UTF-8 is a place a multi-byte character would be lost,
	// and a Serbian card's PIN is not guaranteed to be numeric.
	const typed = "123čž"
	prompt := pinPromptFor("sr-Latn", "Savka Odžić 200100123", 5, 15)
	dst := make([]byte, 15)

	type result struct {
		n  int
		ok bool
	}
	done := make(chan result, 1)
	go func() {
		n, ok, err := ui.CollectPIN(0, prompt, 15, dst)
		if err != nil {
			t.Errorf("CollectPIN: %v", err)
		}
		done <- result{n, ok}
	}()

	dlg := findPINDialog(t)
	edit := editControl(t, dlg)

	buf, err := windows.UTF16FromString(typed)
	if err != nil {
		t.Fatalf("UTF16FromString: %v", err)
	}
	_, _, _ = procSendMsgW.Call(edit, wmSetTextMsg, 0, uintptr(unsafe.Pointer(&buf[0])))
	_, _, _ = procPostMsgW.Call(dlg, wmCommandMsg, idOKButton, 0)

	r := <-done
	if !r.ok {
		t.Fatal("the dialog reported a cancellation for a PIN that was typed and confirmed")
	}
	if got := string(dst[:r.n]); got != typed {
		t.Errorf("the buffer holds %q, want %q", got, typed)
	}
	if r.n != len(typed) {
		t.Errorf("the dialog reported %d bytes for a %d-byte PIN", r.n, len(typed))
	}
}

// TestThePINDialogCancelsWithoutWritingAnything covers the answer that is not
// a failure. A person who closes the screen has declined, and D-145 already
// recorded that reporting a cancellation as an error is its own defect.
func TestThePINDialogCancelsWithoutWritingAnything(t *testing.T) {
	for _, how := range []struct {
		name string
		send func(dlg uintptr)
	}{
		{"the Cancel button", func(dlg uintptr) {
			_, _, _ = procPostMsgW.Call(dlg, wmCommandMsg, idCancelBtn, 0)
		}},
		{"the title bar's close box", func(dlg uintptr) {
			_, _, _ = procPostMsgW.Call(dlg, 0x0010 /* WM_CLOSE */, 0, 0)
		}},
	} {
		t.Run(how.name, func(t *testing.T) {
			prompt := pinPromptFor("en", "Savka Odžić 200100123", 5, 15)
			dst := make([]byte, 15)
			for i := range dst {
				dst[i] = 0xEE // so "wrote nothing" is distinguishable from "was already zero"
			}

			type result struct {
				n   int
				ok  bool
				err error
			}
			done := make(chan result, 1)
			go func() {
				n, ok, err := ui.CollectPIN(0, prompt, 15, dst)
				done <- result{n, ok, err}
			}()

			dlg := findPINDialog(t)
			edit := editControl(t, dlg)
			buf, _ := windows.UTF16FromString("12345")
			_, _, _ = procSendMsgW.Call(edit, wmSetTextMsg, 0, uintptr(unsafe.Pointer(&buf[0])))
			how.send(dlg)

			r := <-done
			if r.err != nil {
				t.Errorf("cancelling reported an error: %v", r.err)
			}
			if r.ok {
				t.Error("cancelling reported success")
			}
			if r.n != 0 {
				t.Errorf("cancelling reported %d bytes written", r.n)
			}
			for i, b := range dst {
				if b != 0xEE {
					t.Errorf("byte %d of the caller's buffer was written (0x%02X) by a cancelled dialog", i, b)
					break
				}
			}
		})
	}
}
