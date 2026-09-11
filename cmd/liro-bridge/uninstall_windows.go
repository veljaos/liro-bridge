//go:build windows

package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// derivedState is everything under the agent's own per-user directory
// that the agent produced for itself and can produce again: extracted
// assets, a browser profile, and the file that says which port it is
// listening on. None of it is anybody's data, and all of it is
// meaningless once the binary is gone.
//
// It is a list here rather than in the installer's authoring for two
// reasons. One is that the layout is this package's to know, and a
// path written down in two languages is a path that drifts — this
// project has recorded that four times (D-108, D-124, D-138, D-183).
// The other is that Windows Installer cannot express half of it:
// ui-assets is a directory whose subdirectory is named after a content
// hash, and MSI's RemoveFile can only name directories it already
// knows.
var derivedState = []string{
	"bridge.json",      // SPEC §14's discovery file: a port that is no longer listening
	"webview2",         // the extracted WebView2Loader.dll (D-080)
	"webview2-profile", // the browser profile the runtime keeps for us (D-207)
	"ui-assets",        // the extracted HTML/CSS/JS the virtual host serves (D-082)
	"icon.ico",         // the extracted tray and window icon (D-090)
	"release-update",   // a downloaded installer, if one was ever fetched
}

// derivedStatePrefixes is the same list for entries whose names this
// program does not know in advance.
//
// There is one, and it is not bookkeeping: a preview directory holds
// rendered pages of the documents somebody was about to sign (D-141),
// and D-141's own reasoning for deleting it when the window closes —
// "a rendered page of somebody's contract is a different kind of thing
// from a stylesheet, and it has no reason to outlive the window that
// needed it" — applies with more force to an uninstall than to a
// window close. The program is being removed; its pictures of somebody
// else's contracts should not be what is left behind.
//
// They are normally removed by the window that made them. One survives
// only when that never happened: a crash, a kill, a machine turned off
// mid-signature. D-243 found one on this machine, left by a session
// weeks earlier, and recorded that it was in neither list.
var derivedStatePrefixes = []string{
	"preview-", // page images for the placement window (D-141)
}

// stalePreviewAge is how old a preview directory must be before a
// starting agent will collect it.
//
// It is a day rather than an hour because a second agent in the same
// session may have a placement window open right now, and that window
// is serving images out of a directory whose modification time stopped
// changing when its last page was drawn. Collecting one out from under
// it would fill somebody's screen with broken images while they were
// deciding where to put a signature. A day is longer than any window
// stays open and shorter than "never", which is what this was before.
const stalePreviewAge = 24 * time.Hour

// keptState is what an uninstall leaves, and what the notice names. It
// is a list so that the message and the behaviour cannot disagree: a
// name in one and not the other would be a sentence about a file that
// was deleted, or a file kept in silence.
var keptState = []string{
	"audit",             // F10 §3.3: never removed, and said so
	"config.json",       // the person's own settings
	"pairings.json",     // which applications may ask
	"secrets.json",      // and the secrets that let them
	"secrets.entropy",   //
	"update-state.json", // when the update check last ran
	"tsl-cache.xml",     // the Trusted List, which is not ours either
	"logs",              //
}

// runUninstallNotice is the installer's last act before it removes the
// binary: it deletes the derived state above and then says, in the
// person's own language, that the audit log is still there and where.
//
// F10 §3.3 requires the audit log to survive an uninstall *and*
// requires the uninstall to say so, "so nobody thinks it vanished".
// This program's installer has no authored dialogs at all — Windows'
// own basic UI does the talking, in the machine's own language, which
// is a better installer UI than this project could translate (see
// build/msi/README.md). The one thing Windows' UI cannot say is this,
// because it is about this program. So the program says it.
//
// It is scheduled only when somebody is watching: the installer's own
// condition is UILevel >= 4, so a silent or GPO uninstall removes the
// same files and shows nothing.
func runUninstallNotice(args []string, out io.Writer, cfg config.Config) int {
	quiet := false
	for _, a := range args {
		if a == "--quiet" {
			quiet = true
		}
	}

	dir := platform.ConfigDir(runtime.GOOS, platform.OSEnv)

	removed, failed := removeDerivedState(dir)
	slog.Info("uninstall: derived state removed", "removed", removed, "failed", failed, "dir", dir)
	fprintf(out, "removed %d of %d derived items from %s\n", removed, removed+failed, dir)

	// The two registrations the agent makes for itself. It is the only
	// thing that knows how to unmake them, and after RemoveFiles it
	// will not be here to be asked — so this is where they go. The
	// per-user installer also removes them declaratively, because a
	// custom action that fails must not leave a Run entry pointing at a
	// binary that no longer exists.
	if err := platform.NewShellMenu().Unregister(); err != nil {
		slog.Warn("uninstall: could not remove the Explorer menu entry", "error", err)
	}
	if err := platform.NewAutostart().SetEnabled(false, ""); err != nil {
		slog.Warn("uninstall: could not remove the autostart entry", "error", err)
	}
	if err := platform.NewSigningFlag().Clear(); err != nil {
		slog.Warn("uninstall: could not clear the in-flight mark", "error", err)
	}

	if quiet {
		return 0
	}
	c := i18n.Load(cfg.Locale)
	ui.ShowNotice(c.T("uninstall.notice_title"), fmt.Sprintf(c.T("uninstall.notice_body"), dir))
	return 0
}

// removeDerivedState deletes each entry in derivedState, and nothing
// else. It never removes the directory itself: what is left in it is
// the point.
//
// A file that will not delete is counted and not reported as a
// failure of the uninstall. An uninstall that fails because a log file
// was open is worse than one that leaves a log file behind.
func removeDerivedState(dir string) (removed, failed int) {
	names := append([]string{}, derivedState...)
	names = append(names, matchingPrefixes(dir, derivedStatePrefixes)...)

	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Lstat(path); err != nil {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("uninstall: could not remove derived state", "path", name, "error", err)
			failed++
			continue
		}
		removed++
	}
	return removed, failed
}

// matchingPrefixes lists the entries of dir whose names begin with one
// of the given prefixes.
//
// A directory that cannot be read produces nothing rather than an
// error: an uninstall's job is to remove what it can find, and the
// caller already treats a thing it cannot remove as a count rather
// than a failure.
func matchingPrefixes(dir string, prefixes []string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// A directory that is not there yet is the ordinary state of a
		// profile the agent has never run in, and the startup sweep is
		// one of the first things to look at it. That is not worth a
		// line in anybody's log.
		if !os.IsNotExist(err) {
			slog.Warn("uninstall: could not list the agent's own directory", "dir", dir, "error", err)
		}
		return nil
	}
	var names []string
	for _, e := range entries {
		for _, p := range prefixes {
			if strings.HasPrefix(e.Name(), p) {
				names = append(names, e.Name())
				break
			}
		}
	}
	return names
}

// sweepStalePreviews removes preview directories nothing came back
// for, and is called once when the agent starts.
//
// The placement window deletes its own on every path it can take
// (D-141), including the ones that end in an error. What it cannot
// cover is not taking a path at all — a crash, a kill, a machine
// switched off while somebody was choosing where a signature goes —
// and what is left then is a folder of rendered pages of that person's
// documents, sitting in their profile until something removes it.
// Before this, nothing did: D-243 found one on this machine that a
// session weeks earlier had left.
//
// It reports what it removed and swallows everything else. An agent
// that cannot tidy up is still an agent that can sign, and a person
// waiting to sign a document is not served by being told about a
// directory.
func sweepStalePreviews(dir string, now time.Time) int {
	removed := 0
	for _, name := range matchingPrefixes(dir, derivedStatePrefixes) {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		if now.Sub(info.ModTime()) < stalePreviewAge {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("startup: could not remove a stale preview directory", "path", name, "error", err)
			continue
		}
		slog.Info("startup: removed a preview directory nothing came back for", "path", name, "age", now.Sub(info.ModTime()).Truncate(time.Hour))
		removed++
	}
	return removed
}
