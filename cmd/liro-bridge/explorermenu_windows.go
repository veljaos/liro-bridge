//go:build windows

package main

import (
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// applyExplorerMenu makes the Explorer context-menu entry match the
// configuration (F6 §2). Registering is idempotent, so this is also
// what keeps the menu label in the user's current language after they
// change it.
func applyExplorerMenu(cfg config.Config, c *i18n.Catalogue) error {
	menu := platform.NewShellMenu()
	if !cfg.ExplorerMenuEnabled {
		return menu.Unregister()
	}
	exe := exePath()
	// The real Liro mark, not the executable: the binary carries no
	// icon resource of its own until F10 builds one, so pointing the
	// menu at it would show the generic Windows application icon.
	// A failure to extract is not a reason to leave the entry
	// unregistered — an entry with the default icon still works.
	icon, err := ui.IconFilePath()
	if err != nil {
		slog.Warn("settings: could not extract the menu icon, falling back to the executable", "error", err)
		icon = exe
	}
	return menu.Register(c.T("settings.explorer_menu_verb"), exe, icon)
}
