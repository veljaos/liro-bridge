//go:build !windows

package main

import (
	"io"
	"log/slog"
	"runtime"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// The two startup concerns that are Windows-only in substance, and
// their answers on a platform that has neither an installer nor a
// runtime to detect.
//
// requireWebView2 answers true rather than false: there is nothing to
// detect and nothing to say about it, and returning false would refuse
// every window command for a reason that is not this machine's. On
// Linux the web view is WebKitGTK, which the package declares and the
// distribution installs (SPEC §1.1) — a dependency resolved before the
// program ever runs, rather than a runtime a person might be missing.
//
// clearStaleSigningGuard used to be stubbed here too and is not any
// more: signingguard.go is platform-neutral, because the flag it marks
// is platform.NewSigningFlag, whose non-Windows form is already an
// honest no-op. The seam belongs one level down, where it is.
func requireWebView2(*i18n.Catalogue) bool { return true }

func runUninstallNotice(_ []string, out io.Writer, _ config.Config) int {
	fprintln(out, "liro-bridge: uninstall-notice is only meaningful on Windows, where the installer runs it.")
	return 1
}

// applyStartupRegistrations has a body here now, because `open` is a
// command on this platform: F12 §3 wired the window host into the
// program, and D-111's rule cuts the other way once a caller exists.
//
// Each half is a decision rather than an omission:
//
//   - **The context menu.** There is nothing to register.
//     applyExplorerMenu is already the no-op of explorermenu_other.go,
//     for F12 §8's reason: there is no universal context menu on
//     Linux, and the desktop entry is what replaces it.
//
//   - **Autostart** is registered, as an XDG `.desktop` file in
//     ~/.config/autostart (F12 §8) — deliberately not a systemd user
//     unit: "a GUI process started before the graphical session exists
//     is a defect with no good symptom". It goes through
//     platform.EnsureAutostart rather than SetEnabled, because on
//     Linux the desktop's own "switch this off at login" lives in the
//     same file, and an agent that rewrote the file at every start
//     would switch itself back on behind the person's back. Settings
//     is where an explicit yes overrides that. On macOS the call
//     refuses, and that is logged at debug for the reason it always
//     was: a warning at every start about a platform nobody has built.
//
// And the sweep, because that half is not a registration and is as
// true here as anywhere: pictures of somebody's
// documents that a crash left behind are worth collecting on a
// platform that has just learned to draw them.
func applyStartupRegistrations(cfg config.Config) {
	if err := platform.EnsureAutostart(cfg.StartWithWindows, exePath()); err != nil {
		if runtime.GOOS == "linux" {
			slog.Warn("startup: could not apply the autostart setting", "error", err)
		} else {
			slog.Debug("startup: this platform has no autostart entry yet", "error", err)
		}
	}
	sweepStalePreviews(platform.CacheDir(runtime.GOOS, platform.OSEnv), time.Now())
}
