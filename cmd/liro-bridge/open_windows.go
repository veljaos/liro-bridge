//go:build windows

package main

// The two ways into the main window that are not the tray: `liro-bridge
// open`, and the Explorer context menu's one-process-per-file
// invocation (F6 §1, §2).

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// runOpen opens the main window. It takes no arguments at all.
//
// It used to seed the window with paths given after the command, and
// nothing ever called it that way — not the tray's Open item, which
// reaches the window in this process, not the Explorer context menu,
// which has its own verb (platform.ShellMenuVerbFlag), and not the
// --help text, which has never mentioned an argument.
//
// It is gone because it was the one place two commands answered the
// same question differently (F9b §3b). "Which documents" is `sign
// --in`, which expands a pattern — `sign --in "C:\docs\*.pdf"` is the
// ordinary case on Windows, where the shell does not expand one — and
// `open x.pdf` did not, so the same intent typed two ways gave two
// answers. That is the disagreement D-138 removed for the stamp margin
// and D-108 for the certificate filter, and the cheapest place to
// remove it here is the surface nobody used.
//
// What is left is two commands that do not overlap: `open` is the
// window with nothing in it, and `sign --in` is a batch. There is no
// third way to say either.
func runOpen(ctx context.Context, args []string, out io.Writer, cfg config.Config) int {
	if len(args) > 0 {
		fprintln(out, "liro-bridge: open takes no arguments; to sign named documents use: liro-bridge sign --in <file or pattern>")
		return 2
	}
	// F6 §2: the entry is on by default, and only the tray applied it.
	// A person who reaches the agent any other way — this command, a
	// shortcut, the first run before autostart has ever fired — would
	// have found no entry to right-click, which is a chicken and egg
	// the entry cannot solve for itself. Registering is idempotent and
	// costs two registry writes.
	//
	// The autostart entry is applied alongside it for the same reason
	// and, until F10, for a worse one: nothing applied it at all
	// (applyAutostart).
	applyStartupRegistrations(cfg)

	return runMainWindow(ctx, cfg, cfg.Locale, nil)
}

// runShellVerb handles one invocation of the Explorer context menu.
//
// Windows starts this program once per selected file, so twenty
// selected documents mean twenty of these running at nearly the same
// moment. Every one of them puts its file in the shared inbox. Exactly
// one — whichever wins the leader claim — then waits for the inbox to
// go quiet and opens a single window for the whole selection; the rest
// return immediately, having done their part.
//
// The claim is held for the whole gather, not just the moment of
// taking it: releasing early would let a later invocation elect itself
// a second collector and open a second window for the tail of the same
// selection.
func runShellVerb(ctx context.Context, args []string, cfg config.Config) int {
	paths := make([]string, 0, len(args))
	for _, a := range args {
		if a == "" {
			continue
		}
		paths = append(paths, filepath.Clean(a))
	}

	box := jobs.NewInbox(shellInboxDir())
	if err := box.Append(paths...); err != nil {
		slog.Error("shell verb: could not record the selected file", "error", err)
		return 1
	}
	// One line per invocation, with its own timestamp. This is how the
	// arrival spread of a real multiple selection was measured, and how
	// it can be measured again on a machine where a batch is splitting.
	slog.Info("shell verb: recorded a selected document", "count", len(paths))

	// An inbox nobody has touched for a long time belongs to a
	// collector that died. Discarding it is not a loss to hide: those
	// documents are gone from this batch either way, and merging them
	// into whatever is being right-clicked now would sign files the
	// person did not choose this time.
	if n, err := box.DiscardIfStale(jobs.CoalesceStaleAfter, time.Now()); err != nil {
		slog.Warn("shell verb: could not check the inbox for stale entries", "error", err)
	} else if n > 0 {
		slog.Warn("shell verb: discarded a stale inbox left by an earlier run", "documents", n)
	}

	leader := platform.NewLeader(platform.ShellBatchLeaderName)
	isLeader, err := leader.Acquire()
	if err != nil {
		slog.Error("shell verb: could not decide which invocation opens the window", "error", err)
		return 1
	}
	if !isLeader {
		// Another invocation is collecting. This one's file is already
		// in the inbox, which is the whole of its job.
		leader.Release()
		return 0
	}
	defer leader.Release()

	batch, err := jobs.CollectBatch(box, jobs.CoalesceWindow, jobs.RealClock())
	if err != nil {
		slog.Error("shell verb: could not collect the selection", "error", err)
		return 1
	}
	if len(batch) == 0 {
		return 0
	}
	slog.Info("shell verb: opening one window for a selection", "documents", len(batch))
	code := runMainWindowWatching(ctx, cfg, cfg.Locale, batch, box)

	// One last drain before the claim is released. A file that arrived
	// between the watcher's last look and the window closing would
	// otherwise sit in the inbox with nobody left to open it — which is
	// exactly the silent half-batch this whole arrangement exists to
	// prevent, just moved to the end.
	if leftover, err := box.Take(); err != nil {
		slog.Warn("shell verb: final inbox drain failed", "error", err)
	} else if len(leftover) > 0 {
		slog.Info("shell verb: opening a window for documents that arrived as the last one closed",
			"documents", len(leftover))
		code = runMainWindowWatching(ctx, cfg, cfg.Locale, leftover, box)
	}
	return code
}

// shellInboxDir is where the handover file lives: the agent's own
// per-user directory, so a second user signed in over RDP has their own
// (SPEC §14.1).
func shellInboxDir() string {
	return filepath.Dir(platform.DefaultConfigFile())
}
