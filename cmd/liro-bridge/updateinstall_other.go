//go:build !windows

package main

import (
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
	"github.com/veljaos/liro-bridge/internal/update"
)

// installRelease says where updates come from on a platform that has a
// package manager, and installs nothing.
//
// **It is a notice and not a failure, and the difference is the whole
// point of this file.** The window's other refusal state says "The
// update did not work", which would be false twice over here: nothing
// was attempted and nothing is broken. SPEC §15.2's in-app updater is
// Windows-shaped — it downloads an MSI and runs msiexec — and F12 §8 is
// explicit that on this platform "a package manager is the expected
// route", leaving open only whether the agent should offer to download
// at all or merely say a version exists.
//
// Until §8 answers that, the offer is still on the screen and pressing
// it lands here, where a person is told the route rather than shown an
// error. The check itself is untouched: knowing a new version exists is
// useful on any platform, and it is the installing that belongs to
// apt and dnf.
func installRelease(c *i18n.Catalogue, win ui.Window, res update.Result) bool {
	slog.Info("update: a newer version exists and this platform installs through its package manager",
		"version", res.Manifest.Version)

	_ = win.PostJSON(map[string]any{
		"type": "notice",
		"model": map[string]any{
			"body": c.T("update.package_route_body"),
		},
	})
	// False: nothing was installed, so the agent is not about to be
	// replaced and must not stop.
	return false
}
