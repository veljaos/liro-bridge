package main

import (
	"log/slog"
	"sync/atomic"

	"github.com/veljaos/liro-bridge/internal/config"
)

// windowDoors are the three windows the tray's menu opens — Podešavanja,
// Sertifikati, Prikaži dnevnik revizije — as the main window reaches them.
//
// **On every platform, and not because of a tray** (D31; the owner's
// ruling, D-419). On a stock GNOME desktop there is no tray host (D-405),
// so a person had no route to Settings, the certificates or the audit log
// at all (D-417) — and "is there a tray here" is not a question the
// program can answer reliably: Ubuntu's tray is an extension a person can
// turn off, and a watcher being present says nothing about whether
// anybody can see it. So the main window has the doors wherever it runs.
//
// The agent passes its own (runTray): Settings revokes pairings through
// the agent's one pairing store, the one the protocol authenticates
// against, so a revocation is immediate (F7 §2.4, D-182). A window with no
// agent behind it opens its own, as `sign`'s Settings does.
type windowDoors struct {
	settings     func(owner uintptr)
	certificates func()
	auditLog     func()
}

// ownDoors are the doors of a window that is not the agent's.
func ownDoors(cfg config.Config) windowDoors {
	return windowDoors{
		settings: func(owner uintptr) {
			// Its own pairing store: nothing else in this process holds
			// one, so there is no second copy to disagree with.
			pairings, secretStore := openPairingsOrNil()
			if err := runSettingsWindow(currentConfig(cfg), owner, pairings, secretStore); err != nil {
				slog.Warn("signing window: settings window failed", "error", err)
			}
		},
		certificates: func() {
			if err := runCertificatesWindow(currentConfig(cfg).Locale); err != nil {
				slog.Warn("signing window: certificates window failed", "error", err)
			}
		},
		auditLog: func() {
			if err := runAuditLogWindow(currentConfig(cfg).Locale); err != nil {
				slog.Warn("signing window: audit log window failed", "error", err)
			}
		},
	}
}

// doorsOpen says which of the three a window has open, so a second click
// on a door whose window is still up opens nothing rather than a second
// copy of it.
type doorsOpen struct {
	settings, certificates, auditLog atomic.Bool
}

// openDoor opens one door's window on a goroutine of its own, so the main
// window keeps answering while it is up, and signals after when it has
// closed. A nil door — a window built without doors — is a log line.
func openDoor(name string, busy *atomic.Bool, open func(), after chan<- struct{}) {
	if open == nil {
		slog.Warn("signing window: a door with nothing behind it was clicked", "door", name)
		return
	}
	if !busy.CompareAndSwap(false, true) {
		slog.Info("signing window: that window is already open", "door", name)
		return
	}
	slog.Info("signing window: opening from the main window", "door", name)
	go func() {
		defer busy.Store(false)
		open()
		if after != nil {
			select {
			case after <- struct{}{}:
			default:
			}
		}
	}()
}
