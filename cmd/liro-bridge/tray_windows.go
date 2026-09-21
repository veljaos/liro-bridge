//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// webView2ExitGrace is a brief pause before the tray subcommand returns
// (and the process reaches ExitProcess), given only after at least one
// WebView2 window has actually been opened and closed during this
// session (Task 8, F5 first-real-run review). ui.Window.Close already
// waits for this process's own COM teardown to finish before returning
// (D-0xx), but the WebView2 runtime's own browser process — a separate
// executable — tears itself down asynchronously once that happens, on
// its own schedule; this is what gives it a little more of that
// schedule to run before this process disappears out from under it,
// which is what actually produces its "Failed to unregister class
// Chrome_WidgetWin_0" console line (emitted by that browser process,
// not by any code in this repository) when the two race. Not applied
// when no such window was ever opened this session — a user who only
// ever used the tray menu's Quit should never wait for anything.
const webView2ExitGrace = 400 * time.Millisecond

// runTray implements F5 §3's product shape: the agent starts minimised
// to tray with no window, and stays running until Quit.
//
// cfg is what the configuration said when the process started, and is
// used as nothing but a fallback from here on. The tray outlives every
// window it opens and every save those windows make, so each of them
// asks the file what the configuration is *now* (currentConfig) rather
// than being handed a copy taken at startup. Handing that copy on is
// what made a saved language come back as the old one: the value
// reached disk correctly and was then never read again.
func runTray(cfg config.Config, version string) int {
	// One pairing store for the life of the process (see openPairings):
	// the settings window revokes through it, and the protocol
	// authenticates against it, and F7 2.4 makes revoking immediate.
	pairings := openPairingsOrNil()
	quit := make(chan struct{})
	// Closed from two places — the tray's Quit item and the update
	// check, when it has just launched an installer that is about to
	// replace this binary — so closing it twice must not panic.
	quitOnce := sync.OnceFunc(func() { close(quit) })
	openedAWindow := false

	// F6 §2: the entry is on by default, so it is registered when the
	// agent starts rather than only when Settings is opened and saved.
	// Registering is idempotent and also refreshes the label after a
	// language change. The autostart entry goes with it (F10 §3.1):
	// until now nothing applied that one at any point except a Save,
	// so StartWithWindows defaulting to true meant nothing.
	applyStartupRegistrations(cfg)

	// F7 §4: the loopback listener, the discovery file and the
	// protocol's own routes. It lives here because the tray is the
	// agent — the process that is running when nobody is looking at a
	// window — and because one machine's user session must have exactly
	// one of it (SPEC §14.1).
	protocol, protocolErr := startProtocol(cfg, version, pairings)
	if protocolErr != nil {
		// Not fatal. An agent that cannot serve the protocol can still
		// sign for the person sitting at it, and saying so in the log is
		// more use than refusing to start.
		slog.Error("tray: the protocol could not be started", "error", protocolErr)
	}
	defer protocol.stop()

	t, err := ui.NewTray(ui.TrayOptions{
		Version: version,
		Labels: func() ui.TrayLabels {
			c := i18n.Load(currentConfig(cfg).Locale)
			return ui.TrayLabels{
				Open:         c.T("tray.open_window"),
				Settings:     c.T("tray.settings"),
				Certificates: c.T("tray.certificates"),
				AuditLog:     c.T("tray.audit_log"),
				Quit:         c.T("tray.quit"),
			}
		},
		OnOpen: func() {
			// F5 §3's "left click opens the main window", finally
			// pointing at one (F6 §1). Opened empty: the drop zone and
			// Browse are how documents get in from here.
			openedAWindow = true
			now := currentConfig(cfg)
			if code := runMainWindow(context.Background(), now, now.Locale, nil); code != 0 {
				slog.Warn("tray: the main window returned an error", "code", code)
			}
		},
		OnSettings: func() {
			openedAWindow = true
			if err := runSettingsWindow(cfg, 0, pairings); err != nil {
				slog.Warn("tray: settings window failed", "error", err)
			}
		},
		OnCertificates: func() {
			openedAWindow = true
			if err := runCertificatesWindow(currentConfig(cfg).Locale); err != nil {
				slog.Warn("tray: certificates window failed", "error", err)
			}
		},
		OnAuditLog: func() {
			openedAWindow = true
			if err := runAuditLogWindow(currentConfig(cfg).Locale); err != nil {
				slog.Warn("tray: audit log window failed", "error", err)
			}
		},
		OnQuit: func() { quitOnce() },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "liro-bridge: tray:", err)
		return 1
	}
	defer func() { _ = t.Close() }()

	// SPEC §15.2's daily check. It runs for the life of the tray, it
	// never installs anything, and the only thing it can do on its own
	// is open the window that asks (F10 §5).
	go watchForUpdates(cfg, quit, quitOnce)

	<-quit
	if openedAWindow {
		time.Sleep(webView2ExitGrace)
	}
	return 0
}
