//go:build linux

package ui

// Dropping documents onto a window, on Linux (F6 §1; open-items D2): a
// GtkDropTarget for GdkFileList, which is what a file manager offers when
// files are dragged out of it.
//
// D-338 recorded that gdk.FileList had no methods in the binding and that
// reading a drop would need hand-written cgo. It has one, Files(), over
// gdk_file_list_get_files, in the same v0.3.1 go.mod named then (D-371).

import (
	"log/slog"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// connectDrop makes the window a drop target for files. On the UI thread.
//
// **On the window, in the capture phase, and that is the part not yet
// watched.** The web view is a drop target of its own — WebKit hands a
// drop to the page, which can see a file's name and contents but never its
// path, and this program opens paths. Capture runs from the toplevel down,
// before anything on the view, so a drop of files is answered here first;
// whether WebKit's controller then sees it at all is for a real drag to
// show (D-094: a drag is a person's hands).
//
// The Windows host switches WebView2's own drop handling off for the same
// reason and registers its own target (droptarget_windows.go).
func (w *linuxWindow) connectDrop(onDropped func(paths []string)) {
	target := gtk.NewDropTarget(gdk.GTypeFileList, gdk.ActionCopy)
	target.SetPropagationPhase(gtk.PhaseCapture)
	target.ConnectDrop(func(value *coreglib.Value, _, _ float64) bool {
		list, ok := value.GoValue().(*gdk.FileList)
		if !ok || list == nil {
			slog.Warn("ui: a drop arrived that was not a list of files")
			return false
		}
		paths := localPaths(list.Files())
		if len(paths) == 0 {
			return false
		}
		// Through the window's own ordered channel, like every other
		// callback: the handler may block, and the UI thread must not.
		w.emit(func() { onDropped(paths) })
		return true
	})
	w.win.AddController(target)
}

// localPaths is the local path of each file, in order. A file with none —
// a location the desktop can browse but that is not a file on this disk —
// is left out and logged: every caller opens what it is given as a path,
// and an empty string would reach one as a path.
func localPaths(files []*gio.File) []string {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		if p := f.Path(); p != "" {
			paths = append(paths, p)
			continue
		}
		slog.Warn("ui: an item has no local path and was left out", "uri", f.URI())
	}
	return paths
}
