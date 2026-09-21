//go:build !windows

package main

import "github.com/veljaos/liro-bridge/internal/config"

// runTray is the one window command this platform still refuses, and
// after F12 §3 it is the only one: `sign` and `open` are real here now
// (interactive.go, open.go), on the GTK4 and WebKitGTK host D-331
// built.
//
// The tray is not a missing implementation, it is F12 §6's undecided
// question. A stock GNOME desktop has no tray at all; D-326 measured
// that GTK4 has no replacement for GtkStatusIcon and that the usual
// remedy, libayatana-appindicator, is a GTK3 library that cannot be
// loaded into a GTK4 process; and what the agent should do on a
// desktop with no tray — how a person then reaches Settings, the
// certificate list and the audit log — is a product decision F12 §6
// reserves and nobody has made.
func runTray(_ config.Config, _ string) int {
	return 1
}
