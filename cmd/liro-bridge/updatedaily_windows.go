//go:build windows

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/update"
)

// updatePollInterval is how often the agent asks whether a check is
// due — not how often it checks. update.Due is what decides that, and
// it says no more than once a day (SPEC §15.2).
//
// An hour, because a tray agent runs for days and a check that only
// ever happened at startup would never happen on a machine nobody
// signs out of.
const updatePollInterval = time.Hour

// updateStartDelay is how long the agent waits before its first look.
// A person who has just started the agent is about to sign something;
// they are not waiting for it to talk to GitHub, and the check has all
// day.
var updateStartDelay = 45 * time.Second

// watchForUpdates is the daily check (SPEC §15.2, F10 §5). It runs for
// the life of the tray and its only power is to open the window that
// asks.
//
// Everything it might do, it does not do here: it does not download,
// it does not install, and it does not raise a version the person has
// already said "not now" to. What it does is ask update.Due, run one
// check when the answer is yes, and — for a verified release newer
// than this build — open runUpdateWindow.
//
// Every failure is a log line and nothing else. SPEC §6.8 makes
// offline normal rather than an error, and a person who is not looking
// at anything must not be shown a window because GitHub was slow.
func watchForUpdates(fallback config.Config, quit <-chan struct{}, stop func()) {
	select {
	case <-quit:
		return
	case <-time.After(updateStartDelay):
	}

	for {
		// The setting is read from the file each time round rather than
		// captured once: turning the check off in Settings has to stop
		// it now, not at the next sign-in (D-134's rule).
		cfg := currentConfig(fallback)
		st := update.LoadState(update.StateFile())

		if update.Due(st, cfg.UpdateCheckEnabled, time.Now()) {
			runOneScheduledCheck(cfg, st, stop)
		}

		select {
		case <-quit:
			return
		case <-time.After(updatePollInterval):
		}
	}
}

func runOneScheduledCheck(cfg config.Config, before update.State, stop func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	res, err := checkForUpdates(ctx)
	if err != nil {
		slog.Info("update: the daily check did not complete", "error", err)
		return
	}
	if !res.Available {
		slog.Info("update: nothing newer", "current", version, "latest", res.Manifest.Version)
		return
	}
	if before.DismissedVersion == res.Manifest.Version {
		// They were asked about this one and said not now. Asking again
		// every day is how a person learns to dismiss a window without
		// reading it.
		slog.Info("update: a newer version is available and was already declined", "version", res.Manifest.Version)
		return
	}

	slog.Info("update: a newer version is available", "current", version, "version", res.Manifest.Version)
	if runUpdateWindow(cfg, 0, res) {
		// The installer is running and is about to replace this
		// binary. Stopping is not optional: an agent still holding its
		// own executable is an upgrade that needs a reboot.
		stop()
	}
}
