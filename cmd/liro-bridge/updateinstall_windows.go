//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
	"github.com/veljaos/liro-bridge/internal/update"
)

// installRelease is the in-app updater on the platform that has one: an
// MSI this program downloads, checks and hands to the installer (SPEC
// §15.2).
// downloadAndLaunch is everything after the person said yes: fetch the
// installer, check it against the digest inside the manifest that has
// already verified, write it beside the agent's own state, and hand it
// to msiexec.
//
// Nothing here trusts the download. The signature covers the manifest,
// the manifest covers the digest, and the digest covers the file — so a
// server that serves different bytes than the release published is
// refused here rather than run.
func installRelease(c *i18n.Catalogue, win ui.Window, res update.Result) bool {
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
