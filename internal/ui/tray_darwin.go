//go:build darwin && cgo

package ui

import (
	"strings"
	"sync"
)

// The status-bar item (F13): the tray of the other two platforms, in the
// menu bar where macOS keeps such things. One per process.
var trayState struct {
	sync.Mutex
	opts *TrayOptions
}

type darwinTray struct{}

func newTray(opts TrayOptions) (Tray, error) {
	trayState.Lock()
	trayState.opts = &opts
	trayState.Unlock()
	if err := nativeTrayNew(trayIconPNG(), "Liro Bridge "+opts.Version); err != nil {
		return nil, err
	}
	return darwinTray{}, nil
}

func (darwinTray) Close() error {
	trayState.Lock()
	trayState.opts = nil
	trayState.Unlock()
	return nativeTrayClose()
}

// trayLabels is the menu, top to bottom, one label a line; the last is
// Quit, which the menu puts below a separator.
func trayLabels() string {
	trayState.Lock()
	opts := trayState.opts
	trayState.Unlock()
	if opts == nil || opts.Labels == nil {
		return "Liro Bridge"
	}
	l := opts.Labels()
	return strings.Join([]string{l.Open, l.Settings, l.Certificates, l.AuditLog, l.Quit}, "\n")
}

func trayAction(item int) {
	trayState.Lock()
	opts := trayState.opts
	trayState.Unlock()
	if opts == nil {
		return
	}
	actions := []func(){opts.OnOpen, opts.OnSettings, opts.OnCertificates, opts.OnAuditLog, opts.OnQuit}
	if item >= 0 && item < len(actions) && actions[item] != nil {
		// Not on the main thread: every action opens a window, and a
		// window waits for its page on the thread that asked.
		go actions[item]()
	}
}

// trayIconPNG is the status-bar image: the project's own icon at 36
// pixels, drawn at 18 points on a Retina display.
func trayIconPNG() []byte { return icoPNGEntry(trayIconICO, 36) }
