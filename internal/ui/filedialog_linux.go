//go:build linux

package ui

// The file and folder choosers on Linux (open-items D2): GTK4's
// GtkFileDialog, the same widget every GTK program on the desktop shows.
// The contract is window.go's ChooseFiles and ChooseFolder, which block the
// caller until the person answers; GtkFileDialog is asynchronous like
// everything else in this binding, so the dialog is started on the UI
// thread and waited for off it.

import (
	"context"
	"errors"
	"syscall"

	"github.com/diamondburned/gotk4/pkg/core/gerror"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// errChooserOnUIThread is returned when a chooser is asked for from the UI
// thread itself. The answer is delivered by that thread's main loop, so
// waiting for it there would wait for ever. No caller does this — every
// chooser is opened from a window's event goroutine — and it is an error
// rather than a deadlock so that one that starts to is named at once.
var errChooserOnUIThread = errors.New("ui: a file chooser cannot be waited for on the UI thread")

// chooserResult is what a dialog's callback hands back to the waiting
// caller.
type chooserResult struct {
	paths []string
	ok    bool
	err   error
}

func pickFiles(owner uintptr, title, filterLabel, allFilesLabel string) ([]string, bool, error) {
	r, err := runChooser(func(parent *gtk.Window, done func(chooserResult)) {
		d := gtk.NewFileDialog()
		d.SetTitle(title)
		d.SetModal(true)

		// F6 §1: a PDF filter first and chosen, and "all files" beside
		// it, because a file a person chooses deliberately is not
		// something to filter away — the signing step names a non-PDF.
		pdf := gtk.NewFileFilter()
		pdf.SetName(filterLabel)
		pdf.AddMIMEType("application/pdf")
		pdf.AddSuffix("pdf")
		all := gtk.NewFileFilter()
		all.SetName(allFilesLabel)
		all.AddPattern("*")
		filters := gio.NewListStore(gtk.GTypeFileFilter)
		filters.Append(coreglib.BaseObject(pdf))
		filters.Append(coreglib.BaseObject(all))
		d.SetFilters(filters)
		d.SetDefaultFilter(pdf)

		d.OpenMultiple(context.Background(), parent, func(res gio.AsyncResulter) {
			list, err := d.OpenMultipleFinish(res)
			if err != nil {
				done(chooserFailed(err))
				return
			}
			var files []*gio.File
			for i := uint(0); i < list.NItems(); i++ {
				if item := list.Item(i); item != nil {
					files = append(files, &gio.File{Object: item})
				}
			}
			done(chosen(files))
		})
	}, owner)
	if err != nil {
		return nil, false, err
	}
	return r.paths, r.ok, r.err
}

func pickFolder(owner uintptr, title, initial string) (string, bool, error) {
	r, err := runChooser(func(parent *gtk.Window, done func(chooserResult)) {
		d := gtk.NewFileDialog()
		d.SetTitle(title)
		d.SetModal(true)
		if initial != "" {
			// window.go: starting on the folder the caller already holds
			// is what keeps an accidental OK from meaning somewhere else.
			d.SetInitialFolder(gio.NewFileForPath(initial))
		}
		d.SelectFolder(context.Background(), parent, func(res gio.AsyncResulter) {
			f, err := d.SelectFolderFinish(res)
			if err != nil {
				done(chooserFailed(err))
				return
			}
			done(chosen([]*gio.File{f}))
		})
	}, owner)
	if err != nil || !r.ok || r.err != nil {
		return "", r.ok, firstErr(err, r.err)
	}
	return r.paths[0], true, nil
}

// runChooser starts a dialog on the UI thread, parented to owner's window
// when owner names one, and waits for its callback.
func runChooser(start func(parent *gtk.Window, done func(chooserResult)), owner uintptr) (chooserResult, error) {
	if err := theUIThread.start(); err != nil {
		return chooserResult{}, err
	}
	if syscall.Gettid() == theUIThread.tid {
		return chooserResult{}, errChooserOnUIThread
	}
	answer := make(chan chooserResult, 1)
	if err := theUIThread.do(func() {
		var parent *gtk.Window
		if w := windowByHandle(owner); w != nil {
			parent = w.win
		}
		start(parent, func(r chooserResult) { answer <- r })
	}); err != nil {
		return chooserResult{}, err
	}
	return <-answer, nil
}

// chosen is what the dialog returned, as paths (localPaths). Nothing chosen
// that is a local file means nothing was chosen.
func chosen(files []*gio.File) chooserResult {
	paths := localPaths(files)
	return chooserResult{paths: paths, ok: len(paths) > 0}
}

// chooserFailed is a dialog that ended without an answer. The person
// cancelling or closing it is the ordinary outcome — ok false, no error,
// as window.go promises; anything else is an error.
func chooserFailed(err error) chooserResult {
	if isChooserDismissal(err) {
		return chooserResult{}
	}
	return chooserResult{err: err}
}

// isChooserDismissal reports whether err is GtkDialogError's "dismissed"
// (the person pressed Cancel or closed it) or "cancelled" (the operation
// was cancelled, which here only the dialog itself can do).
func isChooserDismissal(err error) bool {
	var ge *gerror.GError
	if !errors.As(err, &ge) {
		return false
	}
	if ge.Quark() != uint32(gtk.DialogErrorQuark()) {
		return false
	}
	code := gtk.DialogError(ge.ErrorCode())
	return code == gtk.DialogErrorDismissed || code == gtk.DialogErrorCancelled
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
