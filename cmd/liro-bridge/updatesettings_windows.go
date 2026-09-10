//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
	"github.com/veljaos/liro-bridge/internal/update"
)

// checkForUpdatesFromSettings is what the Settings button does after
// it has said it is checking. It runs on its own goroutine (see the
// caller) and reports on the same status line.
//
// Five outcomes, five sentences, because each of them calls for
// something different from the person: nothing to do, a version to
// consider, a network to look at, a build that cannot be compared, and
// a release this build will not trust. Folding the last three into one
// "the check failed" would send somebody to look at their internet
// connection for a signature problem — the objection D-066, D-104,
// D-118 and D-165 have each made once already about error codes,
// applied here to a status line.
func checkForUpdatesFromSettings(win ui.Window, c *i18n.Catalogue, cfg config.Config) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	res, err := checkForUpdates(ctx)
	switch {
	case errors.Is(err, update.ErrUntrusted), errors.Is(err, update.ErrNoTrustedKeys),
		errors.Is(err, update.ErrManifestInvalid):
		postWindowStatus(win, c.T("settings.update_unverified"), ui.IntentNegative)
		return
	case err != nil:
		slog.Info("settings: the update check did not complete", "error", err)
		postWindowStatus(win, c.T("settings.update_check_failed"), ui.IntentWarning)
		return
	case !res.Comparable:
		// A development build. There is no order between "dev" and any
		// release, so this says what the newest release is and stops
		// short of calling it an update.
		postWindowStatus(win, fmt.Sprintf(c.T("settings.update_dev_build"), res.Manifest.Version), ui.IntentCaution)
		return
	case !res.Available:
		postWindowStatus(win, fmt.Sprintf(c.T("settings.update_current"), version), ui.IntentPositive)
		return
	}

	postWindowStatus(win, fmt.Sprintf(c.T("settings.update_available"), res.Manifest.Version), ui.IntentCaution)

	// And then ask. A person who pressed "check for updates" and was
	// told one exists has asked to be shown it; the window is opened
	// over Settings, owned by it, so it cannot end up behind the window
	// that produced it (D-129).
	if runUpdateWindow(cfg, win.Handle(), res) {
		slog.Info("update: the installer was launched from Settings")
	}
}
