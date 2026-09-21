package main

// The placement window's rendered pages, and collecting the ones a
// crash left behind.
//
// This lived in uninstall_windows.go, where the uninstall notice needed
// the same directory listing — but the sweep is not an uninstall and it
// is not Windows: it runs once at every agent start, on any platform
// that can open a placement window, and what it removes is pictures of
// somebody's documents sitting in their profile (D-243). F12 §3 gives
// this platform such a window, so it gives it this too.

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// previewPrefix is named on its own because the startup sweep takes
// only this one, and an uninstall takes them all.
//
// The difference is not cosmetic. An uninstall runs when the program is
// going away, so everything it can remake is fair game; the sweep runs
// while the program is starting, and the extracted icon is a file it is
// about to load. A sweep over the whole list would have deleted it —
// and would have got away with it today only because that entry happens
// to be a file and the sweep happens to skip files, which is a guard
// nobody wrote on purpose and nothing would have kept true.
const previewPrefix = "preview-"

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
	for _, name := range matchingPrefixes(dir, []string{previewPrefix}) {
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
