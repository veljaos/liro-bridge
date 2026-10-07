//go:build !linux

package main

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/config"
)

// openWithoutAgent is `open` when no agent is running, everywhere but Linux:
// the window, with nothing behind it, as before. On Windows the installer
// starts the agent (LaunchAgent), and D27's option 1 was measured only on
// GNOME (D-421).
func openWithoutAgent(ctx context.Context, cfg config.Config, _ string) int {
	applyStartupRegistrations(cfg)
	return runMainWindow(ctx, cfg, cfg.Locale, nil)
}

// runMainWindow opens the signing window and runs it until it is
// closed. initialPaths seeds the queue — the Explorer context menu and
// the command line both arrive that way; an empty slice opens the empty
// state. It lives here because nothing on Linux calls it: since dev.15
// (D-422) Linux's open becomes the agent and opens its window from there.
func runMainWindow(ctx context.Context, cfg config.Config, locale string, initialPaths []string) int {
	return runMainWindowWatching(ctx, cfg, locale, initialPaths, nil)
}
