//go:build linux

package ui

/*
#cgo pkg-config: gtk4

#include <gtk/gtk.h>
#include <string.h>

// The PIN never becomes a Go string, and these three functions are how
// that is arranged.
//
// gtk_editable_get_text returns a const char * into GtkPasswordEntry's
// own buffer — which is mlocked, measured (D-350) — and the only thing
// Go is ever handed is its length and a copy of its bytes into a buffer
// the caller owns. A Go string cannot be overwritten, and SPEC §6.5.1
// clause 2 requires these bytes to be, which is the whole reason this
// window is not a page (SPEC §10, D-277).

// liro_pin_len reports how many bytes the entry holds, without copying
// any of them.
static size_t liro_pin_len(GtkWidget *entry) {
	const char *t = gtk_editable_get_text(GTK_EDITABLE(entry));
	return t == NULL ? 0 : strlen(t);
}

// liro_pin_copy copies at most cap bytes of the entry's text into dst
// and returns how many it wrote. dst is the caller's own buffer, all
// the way up to the one login() pins and passes to C_Login.
static size_t liro_pin_copy(GtkWidget *entry, void *dst, size_t cap) {
	const char *t = gtk_editable_get_text(GTK_EDITABLE(entry));
	if (t == NULL) return 0;
	size_t n = strlen(t);
	if (n > cap) return 0;
	memcpy(dst, t, n);
	return n;
}

// liro_pin_clear overwrites the entry through the widget's own
// interface.
//
// This is SPEC §6.5.1 clause 2's **first** exception in its GTK
// spelling, and it carries across unchanged: the characters are in
// GTK's memory rather than this program's, so this program cannot wipe
// them — but it can overwrite them through the control's own interface,
// and it does, before the window is destroyed rather than by destroying
// it.
//
// Measured (D-350): after this call the buffer's own copy is gone from
// every writable mapping of the process, and the page it was in is
// mlocked while it is there.
//
// **That is a statement about the buffer and not about the process**
// (D-351, D-352). What this call touches is the widget's own copy.
// What it does not touch, and what nothing here has yet measured, is
// whatever the layers in front of the widget do with a keystroke on the
// way in — GdkEvent, and whatever input method is between the
// compositor and this process. An attempt to measure that found seven
// copies and they were all coincidences of a short needle (D-352), so
// the honest state is that **the path has not been looked at**, not
// that it is clean.
//
// Clause 2's first exception is a sentence about the destination and is
// silent about the path, on this platform and on Windows alike. Do not
// read this call as making the PIN unreadable.
static void liro_pin_clear(GtkWidget *entry) {
	gtk_editable_set_text(GTK_EDITABLE(entry), "");
}
*/
import "C"

import (
	"fmt"
	"unsafe"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

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
//   - copies the typed bytes straight into the caller's buffer
//   - overwrites the entry before the window goes
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

		entry := gtk.NewPasswordEntry()
		entry.SetShowPeekIcon(false)
		entry.SetHExpand(true)
		box.Append(entry)

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

		// answered guards against a second answer: a click and a close
		// can both arrive, and the second must not write into a buffer
		// the caller has already been told about.
		answered := false
		finish := func(r result) {
			if answered {
				return
			}
			answered = true
			// The entry is overwritten before the window is destroyed
			// rather than by destroying it — exception 1's remedy, in
			// GTK's spelling.
			C.liro_pin_clear((*C.GtkWidget)(unsafe.Pointer(entry.Widget.Native())))
			done <- r
			win.Destroy()
		}

		take := func() {
			w := (*C.GtkWidget)(unsafe.Pointer(entry.Widget.Native()))
			// The length is asked for first and nothing is copied
			// unless all of it fits — the same rule, and the same
			// reason, as encodePINInto's: a prefix of somebody's PIN
			// left in the caller's buffer on a path the caller is being
			// told produced nothing.
			if int(C.liro_pin_len(w)) > maxLen {
				finish(result{0, false, ErrPINTooLong})
				return
			}
			// Nothing typed comes back as a zero-length accept rather
			// than as a cancellation, and the difference matters: the
			// length check in pkcs11's login() turns it into a
			// PINLengthError, which says what happened, where a
			// cancellation would report something the person did not
			// do (D-145's converse, which pin.go's own sentinel is
			// about). Nothing empty reaches the card either way.
			got := C.liro_pin_copy(w, unsafe.Pointer(&dst[0]), C.size_t(maxLen))
			finish(result{int(got), true, nil})
		}

		accept.ConnectClicked(take)
		entry.ConnectActivate(take)
		cancel.ConnectClicked(func() { finish(result{0, false, nil}) })
		win.ConnectCloseRequest(func() bool {
			finish(result{0, false, nil})
			return false
		})

		win.Present()
		entry.GrabFocus()
	}); err != nil {
		return 0, false, err
	}

	r := <-done
	return r.n, r.ok, r.err
}
