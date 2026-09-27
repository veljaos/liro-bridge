//go:build linux

package ui

// The PIN dialog on Linux. **Its input is not a GTK text widget** (D-385).
//
// Until D-385 it was a GtkPasswordEntry, and a GtkPasswordEntry told the
// accessibility bus what was typed into it: AT-SPI TextChanged "insert" and
// "delete", payload the whole text, from this process, on a bus gnome-session
// starts for every session and any process running as the same user may
// subscribe to (D-384). That is a second boundary for the PIN, and SPEC
// §6.5.1 clause 2 permits exactly one — the inherited pipe to the worker.
//
// So the field is this program's own: a page it maps and locks, filled from
// key events by this file, drawn as dots, and described to assistive
// technology only by how many characters it holds. What the password entry
// gave, the field keeps, and says where:
//
//   - **locked**: the page is mlocked, and marked not to be dumped;
//   - **overwritten through the control's own interface**: the control is
//     this program's now, so its wipe is this program overwriting its own
//     page, before the window is destroyed;
//   - **no undo**: none is implemented.
//
// What it gives up, deliberately: an input method (a PIN needs none), paste
// (the clipboard is another exposure), and any editing but Backspace.

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"golang.org/x/sys/unix"
)

// fieldAction is what a key did to the field.
type fieldAction int

const (
	fieldIgnored fieldAction = iota // not the field's: let GTK have it (Tab moves focus)
	fieldChanged                    // a character added or removed
	fieldAccept                     // Enter
	fieldCancel                     // Escape
)

// fieldKey applies one key press to the typed bytes in page[:n] and returns
// the new length. Pure, and taken out of the dialog's closure so a test can
// drive it with keyvals rather than with anybody's hands (D-094: calling this
// program's own handler forges nothing at the compositor).
//
// A character is written straight into the locked page as UTF-8 — there is no
// intermediate buffer to wipe. Backspace removes the last whole character and
// overwrites its bytes. A key with Control, Alt or Super held is not the
// field's: Ctrl+V is not a way in.
func fieldKey(page []byte, n int, keyval uint, state gdk.ModifierType) (int, fieldAction) {
	switch keyval {
	case gdk.KEY_Return, gdk.KEY_KP_Enter:
		return n, fieldAccept
	case gdk.KEY_Escape:
		return n, fieldCancel
	case gdk.KEY_BackSpace:
		if n == 0 {
			return n, fieldIgnored
		}
		_, size := utf8.DecodeLastRune(page[:n])
		wipeField(page[n-size : n])
		return n - size, fieldChanged
	}
	if state&(gdk.ControlMask|gdk.AltMask|gdk.SuperMask) != 0 {
		return n, fieldIgnored
	}
	r := rune(gdk.KeyvalToUnicode(keyval))
	if r == 0 || !unicode.IsPrint(r) {
		return n, fieldIgnored
	}
	if n+utf8.RuneLen(r) > len(page) {
		return n, fieldIgnored
	}
	return n + utf8.EncodeRune(page[n:], r), fieldChanged
}

// lockedPage maps one page of this program's own for the field, locks it,
// and asks for it to be left out of any core. A page that cannot be locked
// is an error rather than a silent fallback: locked is what the password
// entry gave, and the field does not quietly give less.
func lockedPage() ([]byte, error) {
	page, err := unix.Mmap(-1, 0, unix.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	if err != nil {
		return nil, fmt.Errorf("ui: mapping the PIN field's page: %w", err)
	}
	if err := unix.Mlock(page); err != nil {
		_ = unix.Munmap(page)
		return nil, fmt.Errorf("ui: locking the PIN field's page: %w", err)
	}
	_ = unix.Madvise(page, unix.MADV_DONTDUMP)
	return page, nil
}

// releasePage overwrites the page, then unlocks and unmaps it.
func releasePage(page []byte) {
	wipeField(page)
	_ = unix.Munlock(page)
	_ = unix.Munmap(page)
}

// wipeField overwrites b and keeps it alive across the write. The same shape
// as internal/keysource/pkcs11's wipe, which this package may not import:
// with no use after the loop the stores have no reader, and a compiler is
// entitled to drop them (pinmem's own selftest was caught by exactly that,
// D-383).
func wipeField(b []byte) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}

// fieldCaret is shown after the dots while the field has keyboard focus.
// The owner found the first field gave no sign it had focus — no caret, no
// highlight — so a person could not tell they were typing into it until a
// dot appeared (D-385). The caret answers that without depending on anybody
// seeing a colour; the outline in pinFieldCSS is the other half.
const fieldCaret = "\u2502"

// fieldDisplay is the field's only text: one dot per character typed, and the
// caret while it has focus. Never a character.
func fieldDisplay(count int, focused bool) string {
	text := strings.Repeat("\u25CF", count)
	if focused {
		text += fieldCaret
	}
	return text
}

// pinFieldCSS draws the field's frame and, while it has keyboard focus, an
// outline in the text's own colour — so it is visible in any theme and uses
// no colour this program chose.
const pinFieldCSS = `
frame.liro-pin-field { border-radius: 6px; }
frame.liro-pin-field:focus { outline: 2px solid alpha(currentColor, 0.8); outline-offset: 2px; }
`

// pinFieldStyle installs pinFieldCSS once per process, on the UI thread.
var pinFieldStyle sync.Once

func installPINFieldStyle() {
	pinFieldStyle.Do(func() {
		provider := gtk.NewCSSProvider()
		provider.LoadFromString(pinFieldCSS)
		if display := gdk.DisplayGetDefault(); display != nil {
			gtk.StyleContextAddProviderForDisplay(display, provider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)
		}
	})
}

// errFieldUnlocked is the one way the field refuses to open.
var errFieldUnlocked = errors.New("ui: the PIN field's memory could not be locked")

// CollectPIN shows the native PIN dialog and writes what was typed into
// dst (SPEC §10).
//
// **Native, not a page**, and §6.5.1 clause 2 is why: the PIN must exist
// only for the length of one C_Login and be overwritten, and a page
// rendered in a browser view is not this program's memory. WebKitGTK
// runs the page in a separate process exactly as WebView2 does, so
// D-277's reasoning carries here unchanged and the window is drawn by
// this program.
//
// The signature is the Windows one, byte for byte, and that is the
// point: **a callback filling a caller-owned buffer rather than a
// function returning a []byte.** There is then exactly one copy of the
// PIN in this program and the function that will pass it to C_Login
// owns it, pins it and overwrites it.
//
// owner is ignored here. On Windows it is the HWND that gets disabled
// while the dialog is up; on Wayland a client cannot raise or parent
// itself onto an arbitrary toplevel (F12 §4, D-337), and the dialog is
// modal to the application instead. The parameter is kept so that one
// caller compiles for both platforms.
//
// # This function does four things and must never do a fifth
//
//   - shows the dialog, once
//   - copies the typed bytes from its own locked page into the caller's buffer
//   - overwrites that page before the window goes
//   - answers ok=false for a cancellation
//
// It does not loop. SPEC §6.5.1 clause 5: nothing retries a PIN
// automatically, ever, for any reason.
func CollectPIN(owner uintptr, prompt PINPrompt, maxLen int, dst []byte) (n int, ok bool, err error) {
	_ = owner
	if maxLen <= 0 || maxLen > len(dst) {
		return 0, false, fmt.Errorf("ui: a PIN buffer of %d bytes cannot hold %d characters", len(dst), maxLen)
	}

	type result struct {
		n   int
		ok  bool
		err error
	}
	done := make(chan result, 1)

	page, err := lockedPage()
	if err != nil {
		return 0, false, fmt.Errorf("%w: %v", errFieldUnlocked, err)
	}

	if err := theUIThread.do(func() {
		win := gtk.NewWindow()
		win.SetTitle(prompt.Title)
		win.SetModal(true)
		win.SetResizable(false)
		win.SetDefaultSize(420, -1)

		box := gtk.NewBox(gtk.OrientationVertical, 12)
		box.SetMarginTop(18)
		box.SetMarginBottom(18)
		box.SetMarginStart(18)
		box.SetMarginEnd(18)

		// Clause 6: a person must be able to tell they are giving their
		// PIN to Liro Bridge rather than to the card or to the desktop.
		// It is drawn first and in the heavier face, as on Windows.
		heading := gtk.NewLabel("")
		heading.SetMarkup(`<span weight="bold" size="large">` + glib.MarkupEscapeText(prompt.Heading) + `</span>`)
		heading.SetXAlign(0)
		heading.SetWrap(true)
		box.Append(heading)

		subject := gtk.NewLabel(prompt.Subject)
		subject.SetXAlign(0)
		subject.SetWrap(true)
		box.Append(subject)

		label := gtk.NewLabel(prompt.Label)
		label.SetXAlign(0)
		box.Append(label)

		// The field: a frame the keyboard can focus, holding a label of
		// dots. The label is the only text it has, and its text is one dot
		// per character, never a character.
		dots := gtk.NewLabel("")
		dots.SetXAlign(0)
		dots.SetMarginStart(8)
		dots.SetMarginEnd(8)
		dots.SetMarginTop(6)
		dots.SetMarginBottom(6)
		installPINFieldStyle()
		field := gtk.NewFrame("")
		field.AddCSSClass("liro-pin-field")
		field.SetChild(dots)
		field.SetFocusable(true)
		field.SetFocusOnClick(true)
		field.SetHExpand(true)
		field.UpdateProperty([]gtk.AccessibleProperty{gtk.AccessiblePropertyLabel}, []coreglib.Value{*coreglib.NewValue(prompt.Label)})
		box.Append(field)

		if prompt.Hint != "" {
			hint := gtk.NewLabel(prompt.Hint)
			hint.SetXAlign(0)
			hint.SetWrap(true)
			hint.AddCSSClass("dim-label")
			box.Append(hint)
		}

		buttons := gtk.NewBox(gtk.OrientationHorizontal, 8)
		buttons.SetHAlign(gtk.AlignEnd)
		cancel := gtk.NewButtonWithLabel(prompt.Cancel)
		accept := gtk.NewButtonWithLabel(prompt.OK)
		accept.AddCSSClass("suggested-action")
		buttons.Append(cancel)
		buttons.Append(accept)
		box.Append(buttons)
		win.SetChild(box)

		// typed is the number of bytes in page; page is the field's locked
		// page. Both live in this closure, which the window's handlers
		// capture and which ends with the window — before CollectPIN
		// returns, so nothing here outlives the call.
		typed := 0

		// answered guards against a second answer: a click and a close
		// can both arrive, and the second must not write into a buffer
		// the caller has already been told about.
		answered := false
		finish := func(r result) {
			if answered {
				return
			}
			answered = true
			// The field is overwritten before the window is destroyed
			// rather than by destroying it — clause 2's first exception,
			// and the control is this program's own now.
			wipeField(page)
			typed = 0
			done <- r
			win.Destroy()
			releasePage(page)
		}

		take := func() {
			// The length is checked first and nothing is copied unless
			// all of it fits — the same rule, and the same reason, as
			// encodePINInto's: a prefix of somebody's PIN left in the
			// caller's buffer on a path the caller is being told produced
			// nothing. Nothing typed comes back as a zero-length accept
			// rather than as a cancellation: pkcs11's login() turns it
			// into a PINLengthError, which says what happened.
			if typed > maxLen {
				finish(result{0, false, ErrPINTooLong})
				return
			}
			got := copy(dst[:maxLen], page[:typed])
			finish(result{got, true, nil})
		}

		focused := false
		show := func(changed bool) {
			count := utf8.RuneCount(page[:typed])
			dots.SetText(fieldDisplay(count, focused))
			if changed && prompt.Entered != "" {
				said := fmt.Sprintf(prompt.Entered, count)
				field.UpdateProperty([]gtk.AccessibleProperty{gtk.AccessiblePropertyDescription}, []coreglib.Value{*coreglib.NewValue(said)})
				field.Announce(said, gtk.AccessibleAnnouncementPriorityLow)
			}
		}

		keys := gtk.NewEventControllerKey()
		keys.ConnectKeyPressed(func(keyval, _ uint, state gdk.ModifierType) bool {
			next, action := fieldKey(page, typed, keyval, state)
			typed = next
			switch action {
			case fieldChanged:
				show(true)
			case fieldAccept:
				take()
			case fieldCancel:
				finish(result{0, false, nil})
			default:
				return false
			}
			return true
		})
		field.AddController(keys)

		focus := gtk.NewEventControllerFocus()
		focus.ConnectEnter(func() { focused = true; show(false) })
		focus.ConnectLeave(func() { focused = false; show(false) })
		field.AddController(focus)

		accept.ConnectClicked(take)
		cancel.ConnectClicked(func() { finish(result{0, false, nil}) })
		win.ConnectCloseRequest(func() bool {
			finish(result{0, false, nil})
			return false
		})

		win.Present()
		field.GrabFocus()
	}); err != nil {
		releasePage(page)
		return 0, false, err
	}

	r := <-done
	return r.n, r.ok, r.err
}
