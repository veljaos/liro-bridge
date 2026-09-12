//go:build windows

package ui

// Registering a drop target on a window the browser creates after the
// registration snapshot.
//
// registerDropTargets walks the tree once, after the page has finished
// loading. That is a snapshot, and D-123 recorded its limit at the
// time: "A window that navigated elsewhere later could grow a child
// nobody registered, and would refuse drops over it with no error."
//
// The compositor's own window is exactly that case, and it is
// measurable rather than theoretical: on a freshly opened window
// Intermediate D3D Window carries no target, and after any navigation
// it does, because the navigation rebuilds the tree and the second
// registration finds it. Which windows can receive a drop should not
// depend on how far through a flow somebody is.
//
// This does not change which window a drop actually arrives at. Every
// drop measured in this project, in every configuration, has arrived at
// Chrome_RenderWidgetHostHWND — including drops onto fresh windows
// where the compositor window had no target at all. It closes an
// inconsistency nobody chose. It is not a fix for a drop that does not
// arrive (D-257).

import (
	"fmt"
	"log/slog"
)

// dropWatchTimerID identifies this window's watch timer. Timer ids are
// per window, so one constant serves every window.
const dropWatchTimerID = 1

// dropWatchInterval is how often the tree is looked at again.
//
// One second is short enough that a log line lands beside the drag it
// explains, and cheap enough to be free: one EnumChildWindows over a
// tree of five. It is not a wait for anything and nothing is timed
// against it (D-201) — a slower machine asks the same question later,
// never fewer times.
const dropWatchInterval = 1000

// newDropTargetCandidates returns the windows in current that have no
// target registered by this package yet.
//
// Split out and pure because it is the whole of the "register what
// appeared later" decision; everything around it is syscalls a test can
// make no assertion about.
func newDropTargetCandidates(registered []*dropTarget, current []uintptr) []uintptr {
	have := make(map[uintptr]struct{}, len(registered))
	for _, t := range registered {
		if t != nil {
			have[t.hwnd] = struct{}{}
		}
	}
	var out []uintptr
	for _, hwnd := range current {
		if _, ok := have[hwnd]; ok {
			continue
		}
		have[hwnd] = struct{}{} // EnumChildWindows can list a window twice
		out = append(out, hwnd)
	}
	return out
}

// startDropWatch begins the watch. Called once, after the first
// registration, on the UI thread.
func (w *window) startDropWatch() {
	if w.onFilesDropped == nil || w.dropWatchOn {
		return
	}
	r, _, err := procSetTimer.Call(w.hwnd, dropWatchTimerID, dropWatchInterval, 0)
	if r == 0 {
		// Not fatal: the window still takes drops on everything already
		// registered. What is lost is the noticing.
		slog.Warn("ui: could not start the drop watch timer", "error", err)
		return
	}
	w.dropWatchOn = true
}

// stopDropWatch ends it, on the same thread, before the window goes.
func (w *window) stopDropWatch() {
	if !w.dropWatchOn {
		return
	}
	_, _, _ = procKillTimer.Call(w.hwnd, dropWatchTimerID)
	w.dropWatchOn = false
}

// watchDropSurface is the timer body.
//
// It runs on the UI thread, which is also the thread that registers and
// revokes drop targets and the thread that tears the window down
// (D-101's ownership rule, D-207's one thread), so w.dropTargets needs
// no lock.
func (w *window) watchDropSurface() {
	if w.tearingDown || w.onFilesDropped == nil {
		return
	}

	tree := append([]uintptr{w.hwnd}, descendantWindows(w.hwnd)...)

	for _, hwnd := range newDropTargetCandidates(w.dropTargets, tree) {
		t, err := registerDropTarget(hwnd, func(paths []string) {
			w.queueEvent(windowEvent{dropped: paths})
		})
		if err != nil {
			// Chromium putting one back, or a window that went away
			// between the enumeration and here. Debug rather than Warn:
			// unlike the first registration this runs every second, and
			// a window that will never take a target would otherwise say
			// so every second for the life of the window.
			slog.Debug("ui: a window that appeared later will not accept dropped files",
				"hwnd", fmt.Sprintf("%#x", hwnd), "class", windowClass(hwnd), "error", err)
			continue
		}
		w.dropTargets = append(w.dropTargets, t)
		slog.Info("ui: registered a drop target on a window that appeared later",
			"hwnd", fmt.Sprintf("%#x", hwnd), "class", windowClass(hwnd))
	}

	w.reportCover(tree)
}
