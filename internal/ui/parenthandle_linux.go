//go:build linux

package ui

// #cgo pkg-config: gtk4
// #include <stdint.h>
// #include <dlfcn.h>
// #include <gdk/gdk.h>
//
// /* The X11 id of a GdkX11Surface, or 0 for any other surface.
//  *
//  * gotk4 v0.3.1 does not bind gdk_x11_surface_get_xid (it returns X's
//  * Window, a type the generator has no mapping for), and including
//  * <gdk/x11/gdkx.h> would bring in Xlib's headers and put -lX11 on this
//  * binary's link line. Both halves are in libgtk-4, which is already
//  * loaded, so the type is found by name and the function by dlsym. */
// static unsigned long liro_surface_xid(uintptr_t surface) {
//     GType t = g_type_from_name("GdkX11Surface");
//     if (t == 0 || !G_TYPE_CHECK_INSTANCE_TYPE((gpointer)surface, t))
//         return 0;
//     unsigned long (*get_xid)(GdkSurface *) =
//         (unsigned long (*)(GdkSurface *))dlsym(RTLD_DEFAULT, "gdk_x11_surface_get_xid");
//     return get_xid ? get_xid((GdkSurface *)surface) : 0;
// }
import "C"

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdkwayland/v4"
)

// exportTimeout bounds the Wayland handle export, which is a round trip to
// the compositor. Chosen, not measured; past it the dialog opens with no
// parent, which still works and is not modal to the window.
const exportTimeout = 2 * time.Second

// exportParent is the portal's identifier for w's window, and what undoes
// it. Off the UI thread. An empty identifier means none could be had; the
// helper passes it on and the portal opens an unparented dialog.
//
// On Wayland, "wayland:" and a handle from xdg-foreign, exported for the
// dialog's life and dropped after it (GTK 4.12 and later keep one per call).
// On Xorg, "x11:" and the window's X id in hex, which is how GTK's own
// portal code writes it and needs nothing undone.
func exportParent(w *linuxWindow) (string, func()) {
	type exported struct {
		id      string
		release func()
	}
	got := make(chan exported, 1)

	// abandoned is set if the export's answer comes after the wait below has
	// given up; the callback then drops the handle itself, so that nothing
	// is left exported for a dialog that was never parented to it.
	var mu sync.Mutex
	abandoned := false

	if err := theUIThread.do(func() {
		if w.win == nil {
			got <- exported{}
			return
		}
		surface := w.win.Surface()
		if surface == nil {
			got <- exported{}
			return
		}
		if tl, ok := surface.(*gdkwayland.WaylandToplevel); ok {
			started := tl.ExportHandle(func(tl *gdkwayland.WaylandToplevel, handle string) {
				mu.Lock()
				late := abandoned
				mu.Unlock()
				if late {
					tl.DropExportedHandle(handle)
					return
				}
				got <- exported{
					id: "wayland:" + handle,
					release: func() {
						_ = theUIThread.do(func() { tl.DropExportedHandle(handle) })
					},
				}
			})
			if !started {
				got <- exported{}
			}
			return
		}
		if xid := C.liro_surface_xid(C.uintptr_t(coreglib.BaseObject(surface).Native())); xid != 0 {
			got <- exported{id: fmt.Sprintf("x11:%x", uint64(xid))}
			return
		}
		got <- exported{}
	}); err != nil {
		slog.Warn("ui: the chooser's parent window could not be named", "error", err)
		return "", func() {}
	}

	select {
	case e := <-got:
		if e.id == "" {
			slog.Warn("ui: the chooser's parent window could not be named; the dialog will not be modal to it")
		}
		if e.release == nil {
			e.release = func() {}
		}
		return e.id, e.release
	case <-time.After(exportTimeout):
		mu.Lock()
		abandoned = true
		mu.Unlock()
		// The answer may have landed between the timer and the flag.
		select {
		case e := <-got:
			if e.release != nil {
				e.release()
			}
		default:
		}
		slog.Warn("ui: the compositor did not export the chooser's parent window in time; the dialog will not be modal to it", "timeout", exportTimeout)
		return "", func() {}
	}
}
