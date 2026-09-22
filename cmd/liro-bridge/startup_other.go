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
// It registers nothing, and each half is a decision rather than an
// omission:
//
//   - **The context menu.** There is nothing to register.
//     applyExplorerMenu is already the no-op of explorermenu_other.go,
//     for F12 §8's reason: there is no universal context menu on
//     Linux, and the desktop entry is what replaces it.
//
//   - **Autostart.** F12 §8's answer is an XDG `.desktop` file in
//     ~/.config/autostart, written and removed by this program when
//     the setting changes — deliberately not a systemd user unit,
//     "a GUI process started before the graphical session exists is a
//     defect with no good symptom". It is not built yet, and
//     platform.NewAutostart's non-Windows form refuses rather than
//     pretending. Calling it here would put a warning in the log at
//     every single start about a feature nobody has written, so this
//     says it once, at debug, and only when the person has actually
//     asked for it.
//
// What it does keep is the sweep, because that half is not a
// registration and is as true here as anywhere: pictures of somebody's
// documents that a crash left behind are worth collecting on a
// platform that has just learned to draw them.
func applyStartupRegistrations(cfg config.Config) {
	if cfg.StartWithWindows {
		slog.Debug("startup: start-at-login is set, and this platform has no autostart entry yet (F12 §8)")
	}
	sweepStalePreviews(platform.CacheDir(runtime.GOOS, platform.OSEnv), time.Now())
}
