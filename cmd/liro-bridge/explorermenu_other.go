//go:build !windows

package main

import (
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
)

// applyExplorerMenu has nothing to do on this platform, and that is a
// decision rather than a gap.
//
// F12 §8: "There is no universal context menu on Linux — Nautilus has
// scripts, Dolphin has service menus — so do not try to reproduce the
// Explorer verb." The place a person reaches this program's own
// integration on Linux is the desktop entry and its MimeType, which is
// §8's work and is not a per-setting toggle.
//
// It returns nil rather than an error on purpose. Settings calls this
// whenever it saves, and platform.NewShellMenu's non-Windows stub
// refuses with errShellMenuUnsupported — correctly, since it is asked
// to register something real. Reporting that to a person as a failed
// save would be telling them that something went wrong when nothing
// did: there is simply no Explorer here to keep in step.
func applyExplorerMenu(_ config.Config, _ *i18n.Catalogue) error { return nil }
