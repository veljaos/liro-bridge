//go:build linux

package ui

import (
	"errors"
	"reflect"
	"testing"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
)

// localPaths keeps what is a file on this disk, in order, and leaves out
// what is not — a location the desktop can browse is not a path a caller
// can open, and an empty string must never reach one as a path. No display
// needed: GFile is GIO's, not GTK's.
func TestLocalPathsKeepsFilesOnThisDiskAndNothingElse(t *testing.T) {
	files := []*gio.File{
		gio.NewFileForPath("/home/someone/ugovor.pdf"),
		gio.NewFileForURI("https://example.invalid/faktura.pdf"),
		gio.NewFileForPath("/tmp/Čitač kartica/račun.pdf"),
	}
	got := localPaths(files)
	want := []string{"/home/someone/ugovor.pdf", "/tmp/Čitač kartica/račun.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("localPaths = %q, want %q", got, want)
	}
	if r := chosen(files[1:2]); r.ok || len(r.paths) != 0 {
		t.Errorf("a choice with no local file = %+v, want nothing chosen", r)
	}
}

// A chooser that fails for a reason other than the person dismissing it is
// an error, not a cancel: only GtkDialogError's two codes are a cancel.
func TestOnlyADismissedChooserIsACancel(t *testing.T) {
	r := chooserFailed(errors.New("the portal is not running"))
	if r.err == nil || r.ok {
		t.Errorf("an ordinary error = %+v, want it reported as an error", r)
	}
	if isChooserDismissal(nil) {
		t.Error("nil is not a dismissal")
	}
}
