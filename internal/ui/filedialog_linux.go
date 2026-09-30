//go:build linux

package ui

// The file and folder choosers on Linux (open-items D2, D25; D-408, D-410).
//
// The dialog is the desktop portal's, asked for by a helper process of this
// binary (internal/chooser), not by GTK in this process. This process is
// not dumpable (D-376), the portal refuses a caller whose /proc/PID/root it
// cannot open, and GTK 4.17.1 and later then never answer the dialog, which
// is how a window came to wait for ever on Fedora 44 (D-408). The helper is
// dumpable, holds only what the person chooses, and is told everything on
// its stdin.
//
// The contract is window.go's ChooseFiles and ChooseFolder, which block the
// caller until the person answers. Here that wait ends on the helper's
// answer, on the window it was opened from closing, or on the parent's own
// bound, whichever is first. None of them waits for ever.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/veljaos/liro-bridge/internal/chooser"
)

// errChooserOnUIThread is returned when a chooser is asked for from the UI
// thread itself. The parent window's handle is exported there, so waiting
// on that thread would wait for ever. No caller does this — every chooser is
// opened from a window's event goroutine — and it is an error rather than a
// deadlock so that one that starts to is named at once.
var errChooserOnUIThread = errors.New("ui: a file chooser cannot be waited for on the UI thread")

// chooserWatchdog is the parent's own bound on the helper: its two waits
// and a grace. The helper keeps both waits itself; this is for a helper
// that does not.
const chooserWatchdog = chooser.CallTimeout + chooser.Ceiling + 5*time.Second

var chooserExe struct {
	sync.Mutex
	path string
}

// SetChooserExecutable names the binary to run as the chooser helper. Only
// cmd/liro-bridge's main calls it; until it is called, every chooser fails
// with ErrChooserFailed. So a test binary, which does not dispatch the
// helper's subcommand, never spawns itself as one (D-293's recursion).
func SetChooserExecutable(path string) {
	chooserExe.Lock()
	chooserExe.path = path
	chooserExe.Unlock()
}

func chooserExecutable() string {
	chooserExe.Lock()
	defer chooserExe.Unlock()
	return chooserExe.path
}

func pickFiles(owner uintptr, title, filterLabel, allFilesLabel string) ([]string, bool, error) {
	// F6 §1: a PDF filter first and chosen, and "all files" beside it,
	// because a file a person chooses deliberately is not something to
	// filter away — the signing step names a non-PDF.
	r, err := runChooser(owner, chooser.Request{
		Kind:  chooser.KindFiles,
		Title: title,
		Filters: []chooser.Filter{
			{Name: filterLabel, MIMETypes: []string{"application/pdf"}, Patterns: []string{"*.pdf", "*.PDF"}},
			{Name: allFilesLabel, Patterns: []string{"*"}},
		},
	})
	if err != nil {
		return nil, false, err
	}
	return r.Paths, len(r.Paths) > 0, nil
}

func pickFolder(owner uintptr, title, initial string) (string, bool, error) {
	// window.go: starting on the folder the caller already holds is what
	// keeps an accidental OK from meaning somewhere else.
	r, err := runChooser(owner, chooser.Request{
		Kind:          chooser.KindFolder,
		Title:         title,
		InitialFolder: initial,
	})
	if err != nil || len(r.Paths) == 0 {
		return "", false, err
	}
	return r.Paths[0], true, nil
}

// runChooser runs one helper for owner's window and turns its Result into
// the contract's: chosen paths, a cancel (no paths, no error), or an error
// wrapping ErrChooserFailed or ErrChooserExpired. Every failure is logged
// here, with the helper's outcome and detail and never a path.
func runChooser(owner uintptr, req chooser.Request) (chooser.Result, error) {
	if err := theUIThread.start(); err != nil {
		return chooser.Result{}, err
	}
	if syscall.Gettid() == theUIThread.tid {
		return chooser.Result{}, errChooserOnUIThread
	}

	w := windowByHandle(owner)
	var closed <-chan struct{}
	if w != nil {
		closed = w.closed
		parent, release := exportParent(w)
		defer release()
		req.Parent = parent
	}

	r, err := spawnChooser(chooserExecutable(), req, closed, chooserWatchdog)
	if err != nil {
		slog.Warn("ui: the file chooser could not be run", "kind", req.Kind, "error", err)
		return chooser.Result{}, fmt.Errorf("%w: %w", ErrChooserFailed, err)
	}
	switch r.Outcome {
	case chooser.OutcomeChosen, chooser.OutcomeCancelled:
		if r.Detail != "" {
			slog.Info("ui: the file chooser", "kind", req.Kind, "outcome", r.Outcome, "detail", r.Detail)
		}
		return r, nil
	case chooser.OutcomeExpired:
		// Loudly: nobody will ever watch this happen, so the log is the
		// only place it will be seen (the owner's instruction, D-410).
		slog.Error("ui: THE FILE CHOOSER WAS CLOSED BY ITS CEILING — open for longer than chooser.Ceiling with no answer",
			"kind", req.Kind, "ceiling", chooser.Ceiling, "detail", r.Detail)
		return chooser.Result{}, fmt.Errorf("%w: %s", ErrChooserExpired, r.Detail)
	default:
		slog.Warn("ui: the file chooser failed", "kind", req.Kind, "outcome", r.Outcome, "detail", r.Detail)
		return chooser.Result{}, fmt.Errorf("%w: %s: %s", ErrChooserFailed, r.Outcome, r.Detail)
	}
}

// spawnChooser runs the helper at exe with req and waits for its Result, for
// closed to close, or for watchdog. A window closing is a cancel; the others
// that end without a Result are errors.
//
// What the helper is given, and nothing else (D-409): its command line is
// exe and the subcommand; its environment is the session bus address; its
// working directory is /; its only files are the three pipes. A crash puts
// all of that in the journal, and none of it is anything a person chose or
// anything of this process's.
func spawnChooser(exe string, req chooser.Request, closed <-chan struct{}, watchdog time.Duration) (chooser.Result, error) {
	if exe == "" {
		return chooser.Result{}, errors.New("no helper binary was set (SetChooserExecutable)")
	}
	bus, err := sessionBusAddress()
	if err != nil {
		return chooser.Result{}, err
	}

	cmd := exec.Command(exe, chooser.Subcommand)
	cmd.Env = []string{"DBUS_SESSION_BUS_ADDRESS=" + bus}
	cmd.Dir = "/"
	// A second route to the helper's end if this process dies without
	// closing its stdin; the first is that stdin closing. Go documents that
	// Pdeathsig follows the thread that started the child rather than the
	// process, which is why it is the second route and not the only one.
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	var stderr boundedBuffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return chooser.Result{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return chooser.Result{}, err
	}
	if err := cmd.Start(); err != nil {
		return chooser.Result{}, fmt.Errorf("starting the helper: %w", err)
	}
	pid := cmd.Process.Pid

	line, err := json.Marshal(req)
	if err == nil {
		_, err = stdin.Write(append(line, '\n'))
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return chooser.Result{}, fmt.Errorf("giving the helper its request: %w", err)
	}

	type read struct {
		r   chooser.Result
		err error
	}
	answer := make(chan read, 1)
	go func() {
		b, err := bufio.NewReader(io.LimitReader(stdout, 1<<20)).ReadBytes('\n')
		if err != nil {
			answer <- read{err: fmt.Errorf("reading the helper's answer: %w", err)}
			return
		}
		var r chooser.Result
		if err := json.Unmarshal(b, &r); err != nil {
			answer <- read{err: fmt.Errorf("decoding the helper's answer: %w", err)}
			return
		}
		answer <- read{r: r}
	}()

	timer := time.NewTimer(watchdog)
	defer timer.Stop()

	var got read
	select {
	case got = <-answer:
		_ = stdin.Close()
	case <-closed:
		// The window this chooser belongs to has gone. Ask the helper to
		// take its dialog down, give it StopGrace, then end it.
		_ = stdin.Close()
		select {
		case <-answer:
		case <-time.After(chooser.StopGrace):
			slog.Warn("ui: the file chooser helper did not stop when asked; killing it", "pid", pid)
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return chooser.Result{Outcome: chooser.OutcomeCancelled, Detail: "the window was closed"}, nil
	case <-timer.C:
		slog.Error("ui: the file chooser helper outlived its watchdog; killing it", "pid", pid, "watchdog", watchdog)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return chooser.Result{}, fmt.Errorf("the helper did not answer within %s", watchdog)
	}

	waitErr := waitBounded(cmd, chooser.StopGrace)
	if got.err != nil {
		if s := stderr.String(); s != "" {
			slog.Warn("ui: the file chooser helper wrote to stderr", "pid", pid, "stderr", s)
		}
		if waitErr != nil {
			return chooser.Result{}, fmt.Errorf("%w (%v)", got.err, waitErr)
		}
		return chooser.Result{}, got.err
	}
	return got.r, nil
}

// waitBounded reaps the helper, killing it if it has not exited within
// grace of answering.
func waitBounded(cmd *exec.Cmd, grace time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(grace):
		_ = cmd.Process.Kill()
		return <-done
	}
}

// sessionBusAddress is the address the helper is given: this process's own,
// or the systemd user bus's socket if this process was started without one.
func sessionBusAddress() (string, error) {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" && a != "autolaunch:" {
		return a, nil
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		sock := filepath.Join(dir, "bus")
		if fi, err := os.Stat(sock); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return "unix:path=" + sock, nil
		}
	}
	return "", errors.New("no session bus: DBUS_SESSION_BUS_ADDRESS is unset and $XDG_RUNTIME_DIR/bus is not a socket")
}

// boundedBuffer keeps the first 4 KiB written to it: enough of a Go panic's
// first lines to say what happened, not a whole trace in the log.
type boundedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := 4096 - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
