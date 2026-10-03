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
