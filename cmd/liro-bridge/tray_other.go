//go:build !windows

package main

import "github.com/veljaos/liro-bridge/internal/config"

// runTray is Windows-only this phase — see interactive_other.go's doc
// comment; internal/ui has no other-platform tray implementation yet.
func runTray(_ config.Config, _, _ string) int {
	return 1
}
