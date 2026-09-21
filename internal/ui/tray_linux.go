//go:build linux

package ui

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// The tray, as StatusNotifierItem over D-Bus.
//
// **GTK4 has no tray and the usual remedy cannot be used** (D-326):
// GtkStatusIcon was removed, gotk4's gtk/v4 has no replacement, and
// libayatana-appindicator — the library everybody reaches for — is
// built against GTK3 and cannot be loaded into a GTK4 process at all.
// What is left is the protocol underneath all of them, which is
// StatusNotifierItem: an object on the session bus, a menu object
// beside it, and a *watcher* that panels run. SPEC §1.1 prefers that
// anyway — "a dependency that can be satisfied in Go is not a reason to
// declare one" — so this links nothing and adds nothing to F12 §8's
// Depends line.
//
// # What a tray is on this platform, measured
//
// `org.kde.StatusNotifierWatcher` is owned by gnome-shell here, with
// Ubuntu's AppIndicator extension active, and it is **not
// D-Bus-activatable**: it exists while something is running that draws
// trays and not otherwise. Fedora's stock GNOME runs no such thing, and
// F12 §6 is explicit that requiring an extension is not an answer.
//
// **So this never fails for want of a watcher.** It exports its objects,
// registers if there is somewhere to register, and watches for one
// appearing — which is not a nicety: the agent starts at login and
// gnome-shell loads its extensions when it gets to them, so an agent
// that looked once at startup would be an agent with no icon on exactly
// the desktop that has a tray (D-342).
const (
	sniIface       = "org.kde.StatusNotifierItem"
	sniPath        = dbus.ObjectPath("/StatusNotifierItem")
	sniWatcher     = "org.kde.StatusNotifierWatcher"
	sniWatcherPath = dbus.ObjectPath("/StatusNotifierWatcher")
)

// ErrNoSessionBus is returned when there is no session bus to put a
// tray on. It is distinct from having no watcher, which is not an
// error: a desktop with no tray is a supported desktop (F12 §6), and a
// session with no bus is a program that cannot do this at all.
var ErrNoSessionBus = errors.New("ui: no session bus, so there is nowhere to put a tray")

type linuxTray struct {
	conn *dbus.Conn
	name string

	mu       sync.Mutex
	closed   bool
	stop     chan struct{}
	closeOne sync.Once
}

func newTray(opts TrayOptions) (Tray, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoSessionBus, err)
	}

	// The name the watcher is given. The specification's own shape:
	// one item per process, numbered, so that two agents in one session
	// (SPEC §14.1 makes that a supported configuration) do not collide.
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil {
		return nil, fmt.Errorf("ui: requesting %s: %w", name, err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return nil, fmt.Errorf("ui: %s is already owned in this session", name)
	}

	t := &linuxTray{conn: conn, name: name, stop: make(chan struct{})}

	menu := newTrayMenu(opts)
	if err := conn.Export(menu, dbusMenuPath, dbusMenuIface); err != nil {
		_, _ = conn.ReleaseName(name)
		return nil, fmt.Errorf("ui: exporting the menu: %w", err)
	}

	item := &sniItem{onActivate: opts.OnOpen}
	if err := conn.Export(item, sniPath, sniIface); err != nil {
		_, _ = conn.ReleaseName(name)
		return nil, fmt.Errorf("ui: exporting the tray item: %w", err)
	}

	pixmaps, err := trayIconPixmaps(trayIconICO)
	if err != nil {
		// An icon that will not decode is not a reason to have no
		// tray: a panel draws a blank or its own fallback, and every
		// menu item still works.
		slog.Warn("ui: the tray icon could not be decoded, so the item has no picture", "error", err)
	}

	title := "Liro Bridge"
	if _, err := prop.Export(conn, sniPath, prop.Map{
		sniIface: {
			"Category":   {Value: "ApplicationStatus", Emit: prop.EmitFalse},
			"Id":         {Value: "liro-bridge", Emit: prop.EmitFalse},
			"Title":      {Value: title, Emit: prop.EmitTrue},
			"Status":     {Value: "Active", Emit: prop.EmitTrue},
			"WindowId":   {Value: int32(0), Emit: prop.EmitFalse},
			"IconName":   {Value: "", Emit: prop.EmitFalse},
			"IconPixmap": {Value: pixmaps, Emit: prop.EmitTrue},
			"ItemIsMenu": {Value: true, Emit: prop.EmitFalse},
			"Menu":       {Value: dbusMenuPath, Emit: prop.EmitFalse},
			"ToolTip": {Value: sniToolTip{
				IconName: "",
				Pixmaps:  pixmaps,
				Title:    title,
				Text:     opts.Version,
			}, Emit: prop.EmitTrue},
		},
	}); err != nil {
		_, _ = conn.ReleaseName(name)
		return nil, fmt.Errorf("ui: exporting the tray item's properties: %w", err)
	}

	if _, err := prop.Export(conn, dbusMenuPath, prop.Map{
		dbusMenuIface: {
			"Version":       {Value: uint32(3), Emit: prop.EmitFalse},
			"Status":        {Value: "normal", Emit: prop.EmitTrue},
			"TextDirection": {Value: "ltr", Emit: prop.EmitFalse},
			"IconThemePath": {Value: []string{}, Emit: prop.EmitFalse},
		},
	}); err != nil {
		_, _ = conn.ReleaseName(name)
		return nil, fmt.Errorf("ui: exporting the menu's properties: %w", err)
	}

	t.registerWithWatcher()
	go t.followWatcher()
	return t, nil
}

// registerWithWatcher offers this item to whatever is drawing trays.
//
// A failure is logged at debug and nothing more: the overwhelmingly
// common reason is that nothing is drawing trays, which on a stock
// GNOME desktop is the ordinary state of the world and not a fault
// (F12 §6).
func (t *linuxTray) registerWithWatcher() {
	call := t.conn.Object(sniWatcher, sniWatcherPath).
		Call(sniWatcher+".RegisterStatusNotifierItem", 0, t.name)
	if call.Err != nil {
		slog.Debug("ui: no tray host accepted the item, so this desktop shows no tray icon",
			"item", t.name, "error", call.Err)
		return
	}
	slog.Info("ui: the tray icon was accepted by this desktop's tray host", "item", t.name)
}

// followWatcher registers again whenever a watcher appears.
//
// This is the part a single check at startup gets wrong. The watcher
// here is owned by gnome-shell and appears when the shell has loaded
// the extension that provides it; an agent started by autostart is
// racing that, and the race is not one it should have to win.
func (t *linuxTray) followWatcher() {
	if err := t.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"),
		dbus.WithMatchArg(0, sniWatcher),
	); err != nil {
		slog.Debug("ui: could not watch for a tray host appearing", "error", err)
		return
	}

	signals := make(chan *dbus.Signal, 8)
	t.conn.Signal(signals)
	defer t.conn.RemoveSignal(signals)

	for {
		select {
		case <-t.stop:
			return
		case sig, ok := <-signals:
			if !ok {
				return
			}
			if sig == nil || sig.Name != "org.freedesktop.DBus.NameOwnerChanged" || len(sig.Body) < 3 {
				continue
			}
			nameArg, _ := sig.Body[0].(string)
			newOwner, _ := sig.Body[2].(string)
			if nameArg != sniWatcher || newOwner == "" {
				// Either a different name, or the watcher going away.
				// Nothing to do for a departure: the item stays
				// exported, and whatever appears next is offered it.
				continue
			}
			slog.Info("ui: a tray host appeared, offering it the icon")
			t.registerWithWatcher()
		}
	}
}

func (t *linuxTray) Close() error {
	t.closeOne.Do(func() { close(t.stop) })

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true

	_ = t.conn.Export(nil, sniPath, sniIface)
	_ = t.conn.Export(nil, dbusMenuPath, dbusMenuIface)
	if _, err := t.conn.ReleaseName(t.name); err != nil {
		return fmt.Errorf("ui: releasing %s: %w", t.name, err)
	}
	return nil
}

// sniToolTip is the specification's (s a(iiay) s s).
type sniToolTip struct {
	IconName string
	Pixmaps  []trayPixmap
	Title    string
	Text     string
}

// sniItem carries the item's methods. The properties are prop.Export's;
// these are the four things a panel can *do* to an icon.
type sniItem struct {
	onActivate func()
}

// Activate is a left click. It opens the window, which is what the
// same click does on the other platform.
//
// On its own goroutine for the reason the menu's Event gives: this is a
// method the panel is waiting on, and opening a window means waiting
// for a person.
func (s *sniItem) Activate(x, y int32) *dbus.Error {
	if s.onActivate != nil {
		go s.onActivate()
	}
	return nil
}

// SecondaryActivate is a middle click. The same as a left click: this
// program has one thing to show.
func (s *sniItem) SecondaryActivate(x, y int32) *dbus.Error { return s.Activate(x, y) }

// ContextMenu is a right click on a host that wants the item to put its
// own menu up. Nothing here does that — ItemIsMenu is true and Menu
// names the object a panel should draw — so this is the honest no-op
// rather than a second menu implementation.
func (s *sniItem) ContextMenu(x, y int32) *dbus.Error { return nil }

// Scroll is the wheel over the icon. This program has nothing that a
// wheel should change.
func (s *sniItem) Scroll(delta int32, orientation string) *dbus.Error { return nil }
