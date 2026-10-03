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
	code, _ := runAgent(cfg, version, false)
	return code
}

// runAgent is the agent, and withWindow is D27's option 1: `open` on a
// desktop where no agent is running becomes the agent, and opens the
// agent's own window at once rather than a window with nothing behind it.
// On stock GNOME that was a window whose web applications could not reach
// Liro until the next login, and nothing said so (D-407). A tray launched
// from the Shell outlives its window (D-421), so the agent it becomes keeps
// serving after the person closes the window, as it would after a login.
//
// started is false when the agent itself could not be brought up — the
// one case option 3's sentence is kept for — so that the caller can still
// give the person a window, and say what is missing.
func runAgent(cfg config.Config, version string, withWindow bool) (code int, started bool) {
	// F12 §7.1: one agent per user session. A second one would take a
	// second port, write its own discovery file over the first one's,
	// and leave the first running and unreachable by the only mechanism
	// SPEC §14 permits — which is D-323, observed rather than imagined.
	if _, live := liveAgent(); live {
		c := i18n.Load(cfg.Locale)
		if err := handOver(); err != nil {
			slog.Warn("tray: an agent is already running and this launch could not reach it", "error", err)
			fmt.Println(c.T("startup.handover_failed"))
			return 1, true
		}
		slog.Info("tray: an agent is already running, so this launch handed it the request and stopped")
		fmt.Println(c.T("startup.already_running"))
		return 0, true
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
	// A logout's SIGTERM takes the same way out as Quit, so the deferred
	// cleanup below runs rather than dying with the process (D-393, D20).
	defer quitOnTerminate(quitOnce)()
	openedAWindow := false
	windows := &oneWindow{}

	// The tray menu's three windows, as one set of doors: the menu opens
	// them, and so does the main window, on every platform (D31).
	doors := windowDoors{
		settings: func(owner uintptr) {
			openedAWindow = true
			if err := runSettingsWindow(cfg, owner, pairings, secretStore); err != nil {
				slog.Warn("tray: settings window failed", "error", err)
			}
		},
		certificates: func() {
			openedAWindow = true
			if err := runCertificatesWindow(currentConfig(cfg).Locale); err != nil {
				slog.Warn("tray: certificates window failed", "error", err)
			}
		},
		auditLog: func() {
			openedAWindow = true
			if err := runAuditLogWindow(currentConfig(cfg).Locale); err != nil {
				slog.Warn("tray: audit log window failed", "error", err)
			}
		},
	}

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
	// Option 3's sentence (D27): every window of an agent whose protocol
	// did not start says that web applications cannot reach it, which
	// nothing else would tell a person on a desktop with no tray.
	unreachable := protocolErr != nil

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
			if !windows.run(func() { openAgentWindow(cfg, nil, quitOnce, doors, unreachable) }) {
				slog.Debug("tray: Open was clicked while the window was already up")
			}
		},
		OnSettings:     func() { doors.settings(0) },
		OnCertificates: doors.certificates,
		OnAuditLog:     doors.auditLog,
		OnQuit:         func() { quitOnce() },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "liro-bridge: tray:", err)
		return 1, false
	}
	defer func() { _ = t.Close() }()

	// F12 §7.1's other half: the agent listens for a launch that handed
	// its request over, and for documents the Explorer verb or a second
	// launch left in the inbox. Without this the handover would be a
	// message nobody reads.
	go watchForHandovers(cfg, windows, quit, &openedAWindow, quitOnce, doors, unreachable)

	if withWindow {
		openedAWindow = true
		slog.Info("tray: started by a launch that asked for the window, so it opens it (D27)")
		go windows.run(func() { openAgentWindow(cfg, nil, quitOnce, doors, unreachable) })
	}

	// SPEC §15.2's daily check. It runs for the life of the tray, it
	// never installs anything, and the only thing it can do on its own
	// is open the window that asks (F10 §5).
	go watchForUpdates(cfg, quit, quitOnce)

	<-quit
	if openedAWindow {
		time.Sleep(trayExitGrace)
	}
	return 0, true
}

// openAgentWindow shows the agent's own window, with any documents that
// came with the request.
func openAgentWindow(cfg config.Config, paths []string, quitAgent func(), doors windowDoors, unreachable bool) {
	now := currentConfig(cfg)
	box := jobs.NewInbox(shellInboxDir())
	if code := runAgentWindow(context.Background(), now, now.Locale, paths, box, quitAgent, doors, unreachable); code != 0 {
		slog.Warn("tray: the main window returned an error", "code", code)
	}
}

// watchForHandovers opens the window when another launch asks for it.
//
// **Only an open-request asks.** It used to be two things — an open-request
// from a second launch (F12 §7.1), or any document sitting in the inbox — and
// the second was a race with a collector that was never meant to share the
// inbox with it: an "Open with" launch appends its document and then spends
// jobs.CoalesceWindow gathering the rest of the selection, and this loop,
// polling every inboxPollInterval, saw the document first and opened an empty
// window beside the one the launch then opened with it. Two windows for one
// PDF, seen by the owner and reproduced by exact PID (D-355). A launch that
// wants this agent to show documents now says so with an open-request of its
// own (runShellVerb), and the window this opens takes the documents from the
// inbox while it is up, so they reach the window already on screen rather
// than a second one.
func watchForHandovers(cfg config.Config, windows *oneWindow, quit <-chan struct{}, opened *bool, quitAgent func(), doors windowDoors, unreachable bool) {
	box := jobs.NewInbox(shellInboxDir())
	ticker := time.NewTicker(inboxPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-quit:
			return
		case <-ticker.C:
			asked, err := agentWindowWanted(box)
			if err != nil {
				slog.Warn("tray: reading the handover request failed", "error", err)
			}
			if !asked {
				continue
			}
			*opened = true
			if !windows.run(func() { openAgentWindow(cfg, nil, quitAgent, doors, unreachable) }) {
				slog.Debug("tray: a launch handed over while the window was already up")
			}
		}
	}
}

// agentWindowWanted reports whether a launch has asked the running agent for
// its window, and consumes the request. Documents in the inbox are not a
// request; see watchForHandovers.
func agentWindowWanted(box *jobs.Inbox) (bool, error) {
	return box.TakeOpenRequest()
}
