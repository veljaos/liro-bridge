//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
	"github.com/veljaos/liro-bridge/internal/update"
)

// The update window's size. Small: it carries three sentences and two
// buttons, and it is measured in a real window at this size by
// TestTheUpdateWindowFitsInEveryLocale rather than chosen by eye.
const (
	updateWindowWidth  = 460
	updateWindowHeight = 300
)

// newUpdateChecker is a package variable so a test can put a checker
// in front of a server it controls. Nothing in production ever chooses
// where a release comes from — that is what the constants in
// internal/update are for.
var newUpdateChecker = func() *update.Checker { return update.NewChecker(nil) }

// runUpdateWindow asks the person about a release that has already
// been verified, and does what they say.
//
// It is the whole of SPEC §15.2's "asks the user, never installs by
// itself". There is no other path in this program that runs an
// installer, and this one cannot be reached without a person pressing
// the button in it: the daily check's only power is to open this
// window, and the manual check in Settings has exactly the same power.
//
// Returns true when the installer was launched, which is also when the
// agent is about to be replaced and should stop.
func runUpdateWindow(cfg config.Config, owner uintptr, res update.Result) bool {
	c := i18n.Load(cfg.Locale)
	messages := make(chan ui.Message, 8)
	closed := make(chan struct{})

	win, err := ui.NewWindow(ui.Options{
		Title:       c.T("update.window_title"),
		Width:       updateWindowWidth,
		Height:      updateWindowHeight,
		Owner:       owner,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/update.html",
		// Not topmost. A release is never urgent, and a window that
		// took the foreground from somebody mid-sentence to offer them
		// an upgrade would be this program deciding its own news
		// matters more than what they are doing.
		AlwaysOnTop: false,
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { close(closed) },
	})
	if err != nil {
		slog.Error("update: the window could not be opened", "error", err)
		return false
	}
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(updateInitPayload(c, res)); err != nil {
		if errors.Is(err, ui.ErrWindowClosed) {
			// Closed before the first payload landed, which is how a
			// person cancels every other window in this program
			// (D-145).
			return false
		}
		slog.Error("update: the window would not take its payload", "error", err)
		return false
	}

	for {
		select {
		case <-closed:
			return false
		case msg := <-messages:
			switch msg.Type {
			case ui.MessageTypeCancel:
				// "Not now" is an answer, and it is remembered: the
				// daily check does not raise this version again by
				// itself. Settings' own check still reports it,
				// because that is somebody asking.
				rememberDismissed(res.Manifest.Version)
				return false
			case ui.MessageTypeApprove:
				return downloadAndLaunch(c, win, res)
			}
		}
	}
}

// downloadAndLaunch is everything after the person said yes: fetch the
// installer, check it against the digest inside the manifest that has
// already verified, write it beside the agent's own state, and hand it
// to msiexec.
//
// Nothing here trusts the download. The signature covers the manifest,
// the manifest covers the digest, and the digest covers the file — so a
// server that serves different bytes than the release published is
// refused here rather than run.
func downloadAndLaunch(c *i18n.Catalogue, win ui.Window, res update.Result) bool {
	_ = win.PostJSON(map[string]any{"type": "working"})

	artefact, ok := res.Manifest.Artefact(update.KindMSI)
	if !ok {
		updateFailed(c, win, update.ErrNoMSI)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	body, err := newUpdateChecker().Download(ctx, res.Manifest, artefact)
	if err != nil {
		updateFailed(c, win, err)
		return false
	}

	dir := filepath.Join(platform.ConfigDir(runtime.GOOS, platform.OSEnv), "release-update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		updateFailed(c, win, err)
		return false
	}
	path := filepath.Join(dir, artefact.Name)
	if err := platform.WriteFileAtomic(path, body, 0o600); err != nil {
		updateFailed(c, win, err)
		return false
	}

	// msiexec, not the MSI by association: running the file would go
	// through whatever the machine has associated with .msi, and this
	// is the one place in this program that starts an installer.
	//
	// /qb rather than /qn: the person just asked for this and should
	// see it happen. The installer's own launch condition refuses if a
	// batch is in flight, which cannot be the case here — the tray is
	// what is running — but is the same guard either way.
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "msiexec.exe"), "/i", path, "/qb")
	if err := cmd.Start(); err != nil {
		updateFailed(c, win, err)
		return false
	}
	slog.Info("update: the installer was launched and this agent is stopping",
		"version", res.Manifest.Version, "installer", artefact.Name, "pid", cmd.Process.Pid)
	// The process is deliberately not waited for: it is about to
	// replace this binary, and waiting would mean the file being
	// replaced is held open by the process waiting for the thing
	// replacing it.
	_ = cmd.Process.Release()
	return true
}

func updateFailed(c *i18n.Catalogue, win ui.Window, err error) {
	slog.Error("update: the release was not installed", "error", err)
	_ = win.PostJSON(map[string]any{
		"type": "failed",
		"model": map[string]any{
			"body": fmt.Sprintf(c.T("update.failed_body"), update.ReleasesPageURL),
		},
	})
}

func updateInitPayload(c *i18n.Catalogue, res update.Result) map[string]any {
	released := ""
	if !res.Manifest.Released.IsZero() {
		released = fmt.Sprintf(c.T("update.released_line"), res.Manifest.Released.Local().Format("02.01.2006."))
	}
	return map[string]any{
		"type":    "init",
		"strings": updateStrings(c),
		"model": map[string]any{
			"versionLine":  fmt.Sprintf(c.T("update.version_line"), version, res.Manifest.Version),
			"releasedLine": released,
		},
	}
}

func updateStrings(c *i18n.Catalogue) map[string]string {
	keys := []string{
		"update.available_title", "update.what_happens", "update.install",
		"update.later", "update.close", "update.working", "update.failed_title",
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = c.T(k)
	}
	return out
}

// rememberDismissed records that this version was offered and refused.
func rememberDismissed(version string) {
	path := update.StateFile()
	st := update.LoadState(path)
	st.DismissedVersion = version
	if err := update.SaveState(path, st); err != nil {
		slog.Warn("update: could not remember that this version was declined", "error", err)
	}
}

// checkForUpdates runs one check and records that it happened —
// whether or not it worked. A machine with no internet must not retry
// every minute, and SPEC §6.8 makes offline normal rather than an
// error.
func checkForUpdates(ctx context.Context) (update.Result, error) {
	res, err := newUpdateChecker().Check(ctx, version)

	path := update.StateFile()
	st := update.LoadState(path)
	st.LastCheck = time.Now().UTC()
	if err == nil && res.Manifest.Version != "" {
		st.LastSeenVersion = res.Manifest.Version
	}
	if saveErr := update.SaveState(path, st); saveErr != nil {
		slog.Warn("update: could not record that a check ran", "error", saveErr)
	}
	return res, err
}
