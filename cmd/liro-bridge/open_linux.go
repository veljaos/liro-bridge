package main

import (
	"context"
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/config"
)

// openWithoutAgent is `open` when no agent is running, on Linux: the launch
// becomes the agent and opens the agent's own window (D27, option 1, the
// owner's ruling in D-421).
//
// A package cannot start a graphical agent in a person's session (session
// 14 §G, option 2), and autostart waits for the next login, so on a stock
// GNOME the first launch after installing used to open a window with no
// agent behind it: a person could sign there, but their web application
// could not reach Liro, and nothing told them. A tray started from a Shell
// launch outlives its window (D-421), so this one goes on serving after the
// window is closed, as it would after the next login.
//
// If the agent cannot be brought up at all, the person still gets a window,
// and it says what is missing (option 3's sentence).
func openWithoutAgent(ctx context.Context, cfg config.Config, version string) int {
	code, started := runAgent(cfg, version, true)
	if started {
		return code
	}
	slog.Warn("open: the agent could not be started, so this window has none behind it")
	return runAgentWindow(ctx, cfg, cfg.Locale, nil, nil, nil, ownDoors(cfg), true)
}
