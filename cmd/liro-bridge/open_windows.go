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
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// runOpen opens the main window, seeded with any paths given on the
// command line. Every remaining argument is a path; there are no flags,
// because there is nothing here to configure that Settings does not
// already own.
func runOpen(ctx context.Context, args []string, out io.Writer, cfg config.Config) int {
	var paths []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			fprintln(out, "liro-bridge: open: unrecognised argument", a)
			return 2
		}
		paths = append(paths, a)
	}
	return runMainWindow(ctx, cfg, cfg.Locale, paths)
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
