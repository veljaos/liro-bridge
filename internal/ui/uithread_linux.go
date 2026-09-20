//go:build linux

package ui

// One UI thread, one GLib main loop, for the whole process — the Linux
// counterpart of uithread_windows.go, and for the same reason stated
// the other way round.
//
// On Windows the constraint is COM: an ICoreWebView2Environment belongs
// to the apartment that created it, so sharing an environment between
// windows means sharing a thread. On Linux the constraint is GTK
// itself, which is not thread-safe and must be called only from the
// thread that called gtk_init. Neither platform gets a choice, and both
// arrive at the same shape:
//
//   - Exactly one OS thread touches GTK, and it is the thread that
//     initialised it.
//   - PostJSON, Eval, Navigate, Resize and Close marshal onto it.
//   - Nothing a caller supplied ever runs on it — OnMessage,
//     OnFilesDropped and OnClosed keep their own per-window goroutine,
//     exactly as on Windows, so a handler that blocks delays later
//     callbacks and nothing else.
//
// The thread is locked with runtime.LockOSThread and never unlocked.
// That is deliberate: an unlocked goroutine can be rescheduled onto
// another thread between two GTK calls, which is the failure this whole
// file exists to make impossible, and it would not announce itself —
// GTK does not check.

import (
	"errors"
	"runtime"
	"sync"
	"syscall"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ErrNoDisplay is returned when GTK cannot reach a windowing system:
// no Wayland compositor, no X server, or a DISPLAY that names neither.
//
// It is an error and not a panic, and that is the entire reason
// gtk.InitCheck is used below rather than gtk.Init. **gtk_init()
// terminates the process** when the windowing system cannot be
// initialised — it prints to stderr and calls exit(). In this program
// that would mean the agent dying without an audit record on a machine
// where someone had merely logged out, and every test binary in this
// package dying on a headless CI runner. gtk_init_check() returns false
// instead, which is a thing a caller can report.
//
// It is the Linux sibling of ErrUnsupportedPlatform's role on the other
// platforms: the window cannot be shown, and the caller is told rather
// than surprised.
var ErrNoDisplay = errors.New("ui: GTK could not connect to a windowing system (no Wayland or X11 display)")

// uiThread is the one OS thread every GTK object in this process lives
// on.
type uiThread struct {
	// startOnce guards run, so that the first caller to need a window
	// starts the thread and every later one joins it.
	startOnce sync.Once

	// ready is closed once run has either brought the loop up or failed
	// to. Every field below is written before it is closed and read
	// only after it, so closing it is the happens-before edge that
	// makes them safe to read without a lock.
	ready chan struct{}

	// err is ErrNoDisplay if GTK could not start, nil otherwise.
	err error

	// loop is the GLib main loop this thread runs.
	loop *glib.MainLoop

	// tid is this thread's kernel thread id, for the "am I already on
	// it" question do asks.
	tid int
}

var theUIThread = &uiThread{ready: make(chan struct{})}

// start brings the UI thread up if it is not up, and blocks until it
// either is or has failed. Safe to call from any goroutine, any number
// of times; every call after the first returns the first one's verdict.
func (t *uiThread) start() error {
	t.startOnce.Do(func() { go t.run() })
	<-t.ready
	return t.err
}

// run is the UI thread. It never returns while the loop is running.
func (t *uiThread) run() {
	// Locked and never unlocked — see the file comment. The goroutine
	// does not exit, so the thread is not released either, which is
	// what we want: it is GTK's for the life of the process.
	runtime.LockOSThread()
	t.tid = syscall.Gettid()

	// F12 §3.2's two variables, set here because here is the only place
	// that is provably before gtk_init — the one ordering requirement
	// they have (D-329). Its return value is deliberately not logged
	// from this package: internal/ui has no logger and no i18n
	// dependency (SPEC §4.2 rule 4), and a caller that wants to record
	// what changed can call it itself beforehand, which is idempotent.
	PrepareWebKitEnvironment()

	if !gtk.InitCheck() {
		t.err = ErrNoDisplay
		close(t.ready)
		return
	}

	t.loop = glib.NewMainLoop(glib.MainContextDefault(), false)
	close(t.ready)
	t.loop.Run()
}

// do runs f on the UI thread and waits for it to finish.
//
// A caller already on the UI thread runs f directly rather than queuing
// it. That is not an optimisation: a signal handler or an idle callback
// is *on* this thread, and queuing from there and then waiting would
// wait for a loop iteration that cannot happen until the current one
// returns. It is the same re-entrancy guard the Windows side needs for
// the same reason.
func (t *uiThread) do(f func()) error {
	if err := t.start(); err != nil {
		return err
	}

	// A goroutine that is not locked to the UI thread cannot be on it,
	// because that thread is locked to run's goroutine — so this
	// comparison is only ever true for a callback GTK itself invoked.
	if syscall.Gettid() == t.tid {
		f()
		return nil
	}

	done := make(chan struct{})
	glib.IdleAdd(func() {
		defer close(done)
		f()
	})
	<-done
	return nil
}
