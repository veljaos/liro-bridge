//go:build linux

package ui

// Dropping documents onto a window, on Linux (F6 §1; open-items D2, D25;
// D-408 option 4, D-410).
//
// **The drop is read as text/uri-list, asynchronously, and nothing else is
// read.** GTK's GdkFileList conversion, which this used before, goes through
// the Documents portal's FileTransfer, and the portal refuses this process
// because it is not dumpable (D-376): on Fedora 44 a drop from Files did
// nothing (D-408). Files offers text/uri-list beside the portal's formats,
// and reading it asks no portal (D-409, measured in a non-dumpable process).
// A source that does not offer it is not accepted.
//
// Asynchronously because D-409's first probe read the stream synchronously
// in the drop callback, on GTK's own thread — the thread that has to pass
// the request to the compositor before the source writes a byte — and hung
// itself. Every step below is a callback on the UI thread's main loop, and
// none of them waits.

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/veljaos/liro-bridge/internal/chooser"
)

const (
	uriListMIME = "text/uri-list"

	// dropReadTimeout bounds reading one drop. The data is a few lines
	// from a process on this desktop and arrived in the same second in
	// D-409; ten seconds is chosen, not measured.
	dropReadTimeout = 10 * time.Second

	// dropReadLimit bounds its size. A thousand long paths are well under
	// it; past it the drop is refused rather than truncated, because a
	// truncated list would silently lose documents.
	dropReadLimit = 1 << 20
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
//
// **The page is told when the drag is over, by this target** (D28). The
// page draws its "drop here" outline on DOM dragover and clears it on a DOM
// drop or a final dragleave — but this target takes the drop before the
// view sees it, so the page's drop never fires, and whether WebKit sends a
// final dragleave varies: on Ubuntu's Xorg it did not, and the outline
// stayed until the window closed (D-415); on Fedora's Wayland something
// cleared it (D-417). So on the target's own drop and drag-leave the page
// is told, whatever WebKit does.
func (w *linuxWindow) connectDrop(onDropped func(paths []string)) {
	w.win.AddController(newDropTarget(func(paths []string) {
		// Through the window's own ordered channel, like every other
		// callback: the handler may block, and the UI thread must not.
		w.emit(func() { onDropped(paths) })
	}, w.tellPageDragEnded))
}

// tellPageDragEnded runs the page's drag-ended hook. Through the window's
// ordered channel, so it is ordered with the drop, and off the UI thread,
// because Eval waits for the page's answer.
func (w *linuxWindow) tellPageDragEnded() {
	w.emit(func() {
		if _, err := w.Eval(dragEndedScript); err != nil && !errors.Is(err, ErrWindowClosed) {
			slog.Warn("ui: telling the page the drag is over failed", "error", err)
		}
	})
}

// dragEndedScript calls the page's own hook, if it has one. Only the main
// page takes drops; any other page simply has no hook.
const dragEndedScript = `window.__liroDragEnded && window.__liroDragEnded(); void 0`

// newDropTarget is the window's drop target for text/uri-list, calling
// deliver with the local paths of each drop. On the UI thread.
//
// **The formats are set after construction and never given to the
// constructor** (D-412). gtk_drop_target_async_new takes its formats
// transfer full — GTK keeps the caller's reference — and gotk4 v0.3.1
// hands it the pointer without taking one and leaves
// gdk.NewContentFormats' finalizer in place. Once Go's collector had run,
// the target held freed memory, and the first drag over the window read
// it: dev.13's agent died of SIGSEGV in GTK's code (D-412).
// gtk_drop_target_async_set_formats takes a reference of its own (read
// in libgtk-4's machine code, not only in the GIR), so the finalizer
// drops only Go's.
//
// **Nothing here may call the target's Formats().** GTK 4.14's GIR says
// gtk_drop_target_async_get_formats returns transfer full; the C returns
// its own field without a reference (read in libgtk-4's machine code),
// and gotk4 v0.3.1 follows the GIR, so each call's finalizer drops one of
// GTK's references. The drop's own formats, gdk_drop_get_formats, are
// transfer none in both and the binding takes a reference: below is safe.
//
// drop_linux_test.go builds the target through this function and forces
// the collector before asking it for its formats.
//
// ended is called, on the UI thread, when a drag over the window is over —
// dropped, whatever came of reading it, or left — so the page can stop
// saying it is a target (D28).
func newDropTarget(deliver func(paths []string), ended func()) *gtk.DropTargetAsync {
	target := gtk.NewDropTargetAsync(nil, gdk.ActionCopy)
	target.SetFormats(gdk.NewContentFormats([]string{uriListMIME}))
	target.SetPropagationPhase(gtk.PhaseCapture)
	// The drop argument is not used: it is wrapped as the drop signal's
	// is, coreglib.Take on a transfer-none GdkDrop (gotk4 v0.3.1, read).
	target.ConnectDragLeave(func(gdk.Dropper) { ended() })
	target.ConnectDrop(func(dropper gdk.Dropper, _, _ float64) bool {
		ended()
		drop := gdk.BaseDrop(dropper)
		if !drop.Formats().ContainMIMEType(uriListMIME) {
			return false
		}
		readDrop(drop, deliver)
		return true
	})
	return target
}

// readDrop reads drop's text/uri-list and calls deliver with the local
// paths in it, then finishes the drop. On the UI thread; returns at once.
//
// The source is told the drop is finished only once everything has been
// read, as GTK requires, and it is always told, whatever went wrong: a
// source left waiting is a drag stuck on the person's screen.
func readDrop(drop *gdk.Drop, deliver func([]string)) {
	ctx, cancel := context.WithTimeout(context.Background(), dropReadTimeout)
	var data bytes.Buffer

	finish := func(ok bool, why string) {
		cancel()
		if !ok {
			slog.Warn("ui: a drop could not be read", "reason", why)
			drop.Finish(0)
			return
		}
		paths, skipped := chooser.LocalPaths(parseURIList(data.String()))
		if skipped > 0 {
			slog.Warn("ui: dropped items with no local path were left out", "count", skipped)
		}
		if len(paths) == 0 {
			drop.Finish(0)
			return
		}
		drop.Finish(gdk.ActionCopy)
		deliver(paths)
	}

	drop.ReadAsync(ctx, []string{uriListMIME}, int(glib.PRIORITY_DEFAULT), func(res gio.AsyncResulter) {
		_, streamer, err := drop.ReadFinish(res)
		if err != nil {
			finish(false, "opening the stream: "+err.Error())
			return
		}
		stream := gio.BaseInputStream(streamer)
		var next func()
		next = func() {
			stream.ReadBytesAsync(ctx, 64<<10, int(glib.PRIORITY_DEFAULT), func(res gio.AsyncResulter) {
				b, err := stream.ReadBytesFinish(res)
				if err != nil {
					closeStream(stream)
					finish(false, "reading: "+err.Error())
					return
				}
				chunk := b.Data()
				if len(chunk) == 0 {
					closeStream(stream)
					finish(true, "")
					return
				}
				if data.Len()+len(chunk) > dropReadLimit {
					closeStream(stream)
					finish(false, "more than the limit of text/uri-list")
					return
				}
				data.Write(chunk)
				next()
			})
		}
		next()
	})
}

// closeStream closes a drop's stream without waiting on the UI thread.
func closeStream(stream *gio.InputStream) {
	stream.CloseAsync(context.Background(), int(glib.PRIORITY_DEFAULT), func(res gio.AsyncResulter) {
		_ = stream.CloseFinish(res)
	})
}

// parseURIList is RFC 2483's text/uri-list: one URI per line, lines ended
// by CRLF (LF accepted), lines starting with '#' are comments, blank lines
// ignored.
func parseURIList(s string) []string {
	var uris []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		uris = append(uris, line)
	}
	return uris
}
