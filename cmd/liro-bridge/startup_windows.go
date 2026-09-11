//go:build windows

package main

import (
	"log/slog"
	"runtime"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// applyStartupRegistrations makes the two things this agent registers
// about itself match the configuration and point at this binary.
//
// It is called by the two commands that mean "the agent is being used
// as an agent" — `tray` and `open` — and by neither `sign` nor
// `certs`, which a person may well run once from a shell without
// meaning to install anything about themselves into their profile.
//
// Both registrations are idempotent, both are under HKCU so neither
// needs administrator rights (SPEC §15's per-user constraint), and
// both are written with os.Executable(), so after an install they
// point at the installed binary rather than at whatever build output
// happened to run last. F9b overwrote the repository-root
// liro-bridge.exe that the Explorer verb pointed at, because `go build
// ./...` writes into the working directory; the trap was the
// registered path being a development path, and an install is what
// stops it being one.
//
// Neither failure is fatal. An agent that cannot write to the Run key
// is an agent that will not start itself next time, which is a thing
// to say in the log and not a reason to refuse to sign today.
func applyStartupRegistrations(cfg config.Config) {
	if err := applyExplorerMenu(cfg, i18n.Load(cfg.Locale)); err != nil {
		slog.Warn("startup: could not apply the Explorer context menu setting", "error", err)
	}
	if err := applyAutostart(cfg); err != nil {
		slog.Warn("startup: could not apply the autostart setting", "error", err)
	}
	// Not a registration, and here because this is the one place both
	// commands that mean "the agent is being used as an agent" already
	// go through. What it collects is pictures of somebody's documents
	// that a crash left behind — see sweepStalePreviews.
	sweepStalePreviews(platform.ConfigDir(runtime.GOOS, platform.OSEnv), time.Now())
}

// applyAutostart makes HKCU\...\Run agree with cfg.StartWithWindows.
//
// Until F10 nothing ever called this at startup: the value was written
// only when somebody pressed Save in the settings window, so
// StartWithWindows defaulting to true meant nothing at all, and a
// fresh install never started with Windows. Measured, on the binary
// this phase began from: no LiroBridge value existed after a first run.
//
// What it writes is autostartCommand(exe) rather than the bare path,
// which is the second half of the same defect — see there.
func applyAutostart(cfg config.Config) error {
	return platform.NewAutostart().SetEnabled(cfg.StartWithWindows, exePath())
}
