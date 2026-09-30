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
func (w *linuxWindow) connectDrop(onDropped func(paths []string)) {
	target := gtk.NewDropTargetAsync(gdk.NewContentFormats([]string{uriListMIME}), gdk.ActionCopy)
	target.SetPropagationPhase(gtk.PhaseCapture)
	target.ConnectDrop(func(dropper gdk.Dropper, _, _ float64) bool {
		drop := gdk.BaseDrop(dropper)
		if !drop.Formats().ContainMIMEType(uriListMIME) {
			return false
		}
		readDrop(drop, func(paths []string) {
			// Through the window's own ordered channel, like every other
			// callback: the handler may block, and the UI thread must not.
			w.emit(func() { onDropped(paths) })
		})
		return true
	})
	w.win.AddController(target)
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
