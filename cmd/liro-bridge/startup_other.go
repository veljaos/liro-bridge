//go:build !windows

package main

import (
	"io"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
)

// The three startup concerns that are Windows-only in substance, and
// their answers on a platform that has no windows, no installer and no
// registry yet (SPEC §19: macOS is phase 12, Linux phase 13).
//
// requireWebView2 answers true rather than false: there is nothing to
// detect and nothing to say about it, and returning false would refuse
// every window command for a reason that is not this machine's.
// Whatever these commands do on such a platform, they already say so
// themselves (interactive_other.go).
func requireWebView2(*i18n.Catalogue) bool { return true }

func clearStaleSigningGuard() {}

func runUninstallNotice(_ []string, out io.Writer, _ config.Config) int {
	fprintln(out, "liro-bridge: uninstall-notice is only meaningful on Windows, where the installer runs it.")
	return 1
}

// applyStartupRegistrations has no stub here on purpose. Its only
// callers are the two Windows-only commands that mean "the agent is
// being used as an agent" (open and tray), so a stub would be a
// function nothing on this platform can reach — which is what
// golangci-lint's unused check said the moment it was written, and
// D-111's own lesson: name a thing for the platform it is for rather
// than giving it a body somewhere it has no caller.
