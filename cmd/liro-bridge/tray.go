package main

// The agent mode: the process that is running when nobody is looking at
// a window.
//
// **It is not "the tray", although it is reached by a command with that
// name, and F12 §6 is where that distinction had to be made.** A stock
// GNOME desktop draws no tray at all, and requiring an extension to fix
// that is the one answer §6 rules out. So what this function does is
// run the agent — the protocol listener, the discovery file, the daily
// update check — and *offer* an icon to whatever is drawing trays. On a
// desktop with one, that is the icon and menu this program has had
// since F5. On a desktop without, the agent is still there and still
// serving, and the way to it is the desktop entry that opens the main
// window (F12 §8) rather than a menu nobody can see.
//
// internal/ui.NewTray is what makes that shape possible rather than a
// branch here: on linux it exports its objects whether or not anything
// is listening, and registers if and when something appears (D-342).

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/ui"
)

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
// oneWindow keeps the agent to one main window at a time.
//
// SPEC §10.2 is about the steps of one flow — "the signing window is one
// window" — and this is the same idea one level up: the tray's Open
// item, a handover from a second launch, and documents arriving in the
// inbox are three ways to ask for the window, and a person who asks
// twice wants the window they already have rather than two of them.
type oneWindow struct {
	mu   sync.Mutex
	open bool
}

// run calls f unless a window is already up, and reports whether it did.
func (o *oneWindow) run(f func()) bool {
	o.mu.Lock()
	if o.open {
		o.mu.Unlock()
		return false
	}
	o.open = true
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.open = false
		o.mu.Unlock()
	}()
	f()
	return true
}

func runTray(cfg config.Config, version string) int {
	// F12 §7.1: one agent per user session. A second one would take a
	// second port, write its own discovery file over the first one's,
	// and leave the first running and unreachable by the only mechanism
	// SPEC §14 permits — which is D-323, observed rather than imagined.
	if _, live := liveAgent(); live {
		c := i18n.Load(cfg.Locale)
		if err := handOver(); err != nil {
			slog.Warn("tray: an agent is already running and this launch could not reach it", "error", err)
			fmt.Println(c.T("startup.handover_failed"))
			return 1
		}
		slog.Info("tray: an agent is already running, so this launch handed it the request and stopped")
		fmt.Println(c.T("startup.already_running"))
		return 0
	}

	// One pairing store for the life of the process (see openPairings):
	// the settings window revokes through it, and the protocol
	// authenticates against it, and F7 2.4 makes revoking immediate.
	pairings, secretStore := openPairingsOrNil()
	quit := make(chan struct{})
	// Closed from two places — the tray's Quit item and the update
	// check, when it has just launched an installer that is about to
	// replace this binary — so closing it twice must not panic.
	quitOnce := sync.OnceFunc(func() { close(quit) })
	openedAWindow := false
	windows := &oneWindow{}

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
			if !windows.run(func() { openAgentWindow(cfg, nil, quitOnce) }) {
				slog.Debug("tray: Open was clicked while the window was already up")
			}
		},
		OnSettings: func() {
			openedAWindow = true
			if err := runSettingsWindow(cfg, 0, pairings, secretStore); err != nil {
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

	// F12 §7.1's other half: the agent listens for a launch that handed
	// its request over, and for documents the Explorer verb or a second
	// launch left in the inbox. Without this the handover would be a
	// message nobody reads.
	go watchForHandovers(cfg, windows, quit, &openedAWindow, quitOnce)

	// SPEC §15.2's daily check. It runs for the life of the tray, it
	// never installs anything, and the only thing it can do on its own
	// is open the window that asks (F10 §5).
	go watchForUpdates(cfg, quit, quitOnce)

	<-quit
	if openedAWindow {
		time.Sleep(trayExitGrace)
	}
	return 0
}

// openAgentWindow shows the agent's own window, with any documents that
// came with the request.
func openAgentWindow(cfg config.Config, paths []string, quitAgent func()) {
	now := currentConfig(cfg)
	box := jobs.NewInbox(shellInboxDir())
	if code := runAgentWindow(context.Background(), now, now.Locale, paths, box, quitAgent); code != 0 {
		slog.Warn("tray: the main window returned an error", "code", code)
	}
}

// watchForHandovers opens the window when another launch asks for it.
//
// Two things arrive the same way and mean the same thing — show the
// person the window — so they are polled together: an open-request from
// a second launch (F12 §7.1), and documents dropped in the inbox by the
// shell integration. The window this opens watches the inbox itself
// while it is up, so documents arriving after it opens reach the window
// already on screen rather than a second one.
func watchForHandovers(cfg config.Config, windows *oneWindow, quit <-chan struct{}, opened *bool, quitAgent func()) {
	box := jobs.NewInbox(shellInboxDir())
	ticker := time.NewTicker(inboxPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
			asked, err := box.TakeOpenRequest()
			if err != nil {
				slog.Warn("tray: reading the handover request failed", "error", err)
			}
			if !asked {
				if n, err := box.Count(); err != nil || n == 0 {
					continue
				}
			}
			*opened = true
			if !windows.run(func() { openAgentWindow(cfg, nil, quitAgent) }) {
				slog.Debug("tray: a launch handed over while the window was already up")
			}
		}
	}
}
