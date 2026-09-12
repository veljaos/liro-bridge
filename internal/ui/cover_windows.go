//go:build windows

package ui

// Saying when something is in front of this window.
//
// A drop is delivered to the window under the cursor. If that window is
// not one of ours, no IDropTarget of ours is ever consulted and nothing
// reaches this process — no refusal, no error, no log line, and a
// registration that looks perfect. It is the one state that makes a
// dragged document vanish leaving the agent's own account of itself
// indistinguishable from a working one, and D-256 records three
// sessions with exactly that shape.
//
// It is not hypothetical. Two agent windows are centred on the same
// point of the same monitor, so a second one sits exactly on the first
// (D-129 measured WindowFromPoint returning the wrong one of a pair);
// and any other program's window over this one does the same thing.
//
// So the window asks, once a second, what is actually under its own
// centre, and says so when the answer changes — one line when it is
// covered, one when it is not. A person switching to another program
// produces a pair of those lines, which is intended: the question they
// answer is asked afterwards, by somebody reading a log, and it is
// "was this window the one under the cursor at that moment".
//
// No window title is ever logged. A title carries the name of the
// document somebody is signing (SPEC §18.3); a class name, an HWND and
// a process id carry nothing about a person.

import (
	"fmt"
	"log/slog"
	"unsafe"

	"golang.org/x/sys/windows"
)

// coverState is what the window under this window's centre turned out
// to be.
type coverState int

const (
	// coverNone: the window under the centre is this window or one of
	// the browser's windows underneath it. A drop there reaches us.
	coverNone coverState = iota
	// coverByOwnProcess: another window belonging to this same agent
	// process — the two-windows-on-one-monitor arrangement.
	coverByOwnProcess
	// coverByOtherProcess: somebody else's window. A drop there is
	// theirs, and this process never hears of it.
	coverByOtherProcess
)

// classifyCover decides, from four facts a test can supply, whether a
// drop aimed at the middle of this window would reach it.
//
// ours is the frame plus every window underneath it — the same set the
// drop targets are on. Membership of that set is the test, and the
// owning process is only consulted afterwards, because
// Chrome_RenderWidgetHostHWND — the window every measured drop actually
// arrives at — belongs to the msedgewebview2 process rather than to
// this one. A check that compared process ids would report this
// window's own surface as somebody else's, every second, for ever.
func classifyCover(under uintptr, underPID uint32, ours []uintptr, ourPID uint32) coverState {
	for _, hwnd := range ours {
		if hwnd == under {
			return coverNone
		}
	}
	if underPID == ourPID {
		return coverByOwnProcess
	}
	return coverByOtherProcess
}

// reportCover samples what is under this window's centre and logs the
// transitions.
func (w *window) reportCover(tree []uintptr) {
	if isIconic(w.hwnd) {
		return
	}
	var r rect
	if ok, _, _ := procGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return
	}
	if r.Right <= r.Left || r.Bottom <= r.Top {
		return
	}
	centre := point{X: (r.Left + r.Right) / 2, Y: (r.Top + r.Bottom) / 2}
	under, underPID := windowUnderPoint(centre)
	if under == 0 {
		return
	}

	state := classifyCover(under, underPID, tree, uint32(windows.GetCurrentProcessId()))
	if state == w.cover {
		return
	}
	w.cover = state

	switch state {
	case coverNone:
		slog.Info("ui: this window is in front again where a drop would land")
	case coverByOwnProcess:
		slog.Info("ui: another window of this agent is in front of this one; a drop here would go to that one",
			"hwnd", fmt.Sprintf("%#x", under), "class", windowClass(under))
	case coverByOtherProcess:
		slog.Info("ui: another program's window is in front of this one; a drop here would go to it and this agent would never hear of it",
			"hwnd", fmt.Sprintf("%#x", under), "class", windowClass(under), "pid", underPID)
	}
}
