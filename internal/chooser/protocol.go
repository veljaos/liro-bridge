// Package chooser is the file-chooser helper: a short-lived process of this
// binary that asks the desktop's portal for files or a folder on a window's
// behalf and hands back the paths chosen (D-408 option 1, D-410).
//
// # Why it is a process of its own
//
// Every other process of this binary clears its dumpable flag at start
// (platform.ForbidCoreDumps, D-376), and the kernel then gives its /proc
// entries to root. xdg-desktop-portal identifies a caller by opening
// /proc/PID/root, cannot, and refuses it; GTK 4.17.1 and later then never
// answer the dialog, and the window waits for ever (D-408). The portal
// checks whichever process is on the other end of the D-Bus connection, so
// the one route that exists is a different process, which is dumpable and
// holds nothing: this one.
//
// # What it may hold, and where that is enforced
//
// Its title, labels and filters, and the paths the person chose. Nothing
// else: no configuration, no key source, no audit log, no pairing, no window.
// guards_test.go enforces that on its imports, with the reason written there.
//
// It keeps RLIMIT_CORE at zero and does not clear the dumpable flag, so a
// crash leaves no core, but systemd-coredump still writes the process's
// command line, working directory and whole environment into the system
// journal (D-409). So everything it is given, and everything it returns,
// travels on stdin and stdout. Its command line is this binary and
// Subcommand and nothing more. Its environment is the session bus address
// and nothing more. Its working directory is /.
//
// This file is the protocol, shared by the helper (run_linux.go) and the
// parent that spawns it (internal/ui/filedialog_linux.go). It has no build
// tag so that the package exists on every platform; everything that runs is
// Linux only.
package chooser

import "time"

// Subcommand is the helper's whole command line after the binary's path.
//
// main dispatches it before anything else, before ForbidCoreDumps, and only
// when it is the one and only argument. It is not in --help: a person who
// runs it from a shell gets a process that reads a request from their own
// terminal, which is harmless and useless.
const Subcommand = "file-chooser"

// Kind is what the person is asked to choose.
type Kind string

const (
	// KindFiles is one or more files (the Izaberi… button).
	KindFiles Kind = "files"
	// KindFolder is one folder (the output folder, a report, an audit
	// export).
	KindFolder Kind = "folder"
)

// Filter is one entry in the dialog's file-type list.
type Filter struct {
	Name      string   `json:"name"`
	MIMETypes []string `json:"mimeTypes,omitempty"`
	Patterns  []string `json:"patterns,omitempty"`
}

// Request is the one line the parent writes to the helper's stdin.
//
// After that line the parent keeps stdin open, and closing it means "stop":
// the window was closed or the parent is gone. The helper then closes the
// portal's dialog and exits.
type Request struct {
	Kind  Kind   `json:"kind"`
	Title string `json:"title"`
	// Filters, the first shown and chosen. Files only.
	Filters []Filter `json:"filters,omitempty"`
	// InitialFolder is the folder to start in; empty lets the portal choose.
	InitialFolder string `json:"initialFolder,omitempty"`
	// Parent is the portal's parent-window identifier: "wayland:<handle>",
	// "x11:<xid in hex>", or empty for none.
	Parent string `json:"parent,omitempty"`
}

// Outcome is how a chooser ended.
type Outcome string

const (
	// OutcomeChosen is the person choosing something. Paths holds it.
	OutcomeChosen Outcome = "chosen"
	// OutcomeCancelled is the person cancelling or closing the dialog, or
	// the parent asking the helper to stop. The ordinary outcome, not a
	// failure.
	OutcomeCancelled Outcome = "cancelled"
	// OutcomeRefused is the portal answering the call with an error that
	// says it will not serve this caller, which is D-408's refusal.
	OutcomeRefused Outcome = "refused"
	// OutcomeNoPortal is nothing on the session bus answering the
	// FileChooser interface at all, or answering one too old for what was
	// asked.
	OutcomeNoPortal Outcome = "no-portal"
	// OutcomeTimeout is the portal not answering the call within
	// CallTimeout. It is not a person taking their time: that wait begins
	// only once the call has been answered.
	OutcomeTimeout Outcome = "timeout"
	// OutcomeExpired is the dialog open for longer than Ceiling, closed by
	// the helper.
	OutcomeExpired Outcome = "expired"
	// OutcomeError is anything else.
	OutcomeError Outcome = "error"
)

// Result is the one line the helper writes to its stdout.
//
// Detail says what went wrong, for the parent's log. It never carries a
// path, so a failure logged by the parent does not put a person's file
// names into bridge.log.
type Result struct {
	Outcome Outcome  `json:"outcome"`
	Paths   []string `json:"paths,omitempty"`
	Detail  string   `json:"detail,omitempty"`
}

// The two waits, and they are different things (D-408, D-410).
const (
	// CallTimeout bounds the portal answering the call: connecting to the
	// session bus, reading the interface's version, and OpenFile returning
	// a request handle or an error. The answer is immediate either way
	// (D-408 measured the refusal in 1 ms). Ten seconds is chosen, not
	// measured, and allows for the portal being started by D-Bus
	// activation on this call, which GTK's settings read at a window's
	// start has usually done already.
	CallTimeout = 10 * time.Second

	// Ceiling bounds a person choosing, once the portal has taken the call.
	// Minutes are ordinary, so this is not a timeout on a person. It exists
	// because without it a helper whose dialog never appeared, or that
	// nobody closes, would outlive everything, and this project spent two
	// weeks learning what unbounded waits do. **Thirty minutes is chosen,
	// not measured** (the owner's ruling, D-410). The helper closes the
	// dialog when it fires, and the window says why.
	Ceiling = 30 * time.Minute

	// StopGrace is how long the helper is given to close the dialog and
	// exit once its parent has asked it to stop, before the parent kills
	// it.
	StopGrace = 2 * time.Second
)
