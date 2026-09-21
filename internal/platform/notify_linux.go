//go:build linux

package platform

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/godbus/dbus/v5"
)

// The freedesktop notification service, spoken directly over the
// session bus.
//
// **No C library, and that is SPEC §1.1's rule rather than a
// preference**: "a dependency that can be satisfied in Go is not a
// reason to declare one". libnotify would put a second C library in
// `Depends:` for one method call. This is a pure-Go D-Bus client, so
// the package declares nothing new and this file compiles at
// CGO_ENABLED=0 (D-341).
const (
	notifyDest  = "org.freedesktop.Notifications"
	notifyPath  = "/org/freedesktop/Notifications"
	notifyIface = "org.freedesktop.Notifications"

	// notifyCallTimeout bounds the one blocking call this makes.
	//
	// The consent window is already on screen by the time this runs, so
	// every millisecond spent here is a millisecond a person is looking
	// at a window this program has not finished reacting to. A
	// notification daemon that does not answer in three seconds is a
	// daemon this program waits no longer for: the notification is
	// best-effort and the window is not.
	notifyCallTimeout = 3 * time.Second

	// notifyNeverExpires is expire_timeout = 0, which the freedesktop
	// specification defines as "never expire". A banner that vanished
	// after four seconds would be exactly the notification SPEC §6.5.2
	// does not want — the person it is for is, by assumption, not
	// looking at this screen. It stays in the desktop's notification
	// list until they look or until Close takes it down.
	notifyNeverExpires = int32(0)
)

type dbusNotifier struct{}

func newNotifier() Notifier { return dbusNotifier{} }

type dbusNotification struct {
	conn *dbus.Conn
	id   uint32
}

// Notify posts one notification and returns a handle to it.
//
// **It offers no actions, and that is measured rather than austere.**
// The freedesktop interface can carry buttons, and clicking one sends
// the program an activation token it can raise a window with — on this
// desktop, measured, no token arrives and the window is not raised
// (D-337, D-339). A button that did nothing when pressed would be a
// worse thing than no button, so until a packaged desktop entry makes
// that measurement come out differently (F12 §8), this is text.
func (dbusNotifier) Notify(summary, body string) (Notification, error) {
	// The shared session connection: this program may post one of these
	// per request for the life of an agent, and a private connection
	// per notification would be a socket, an authentication handshake
	// and a Hello for one method call.
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, fmt.Errorf("platform: no session bus: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), notifyCallTimeout)
	defer cancel()

	var id uint32
	err = conn.Object(notifyDest, notifyPath).
		CallWithContext(ctx, notifyIface+".Notify", 0,
			// app_name, replaces_id, app_icon, summary, body
			appName, uint32(0), "", summary, body,
			// actions: none, for the reason above
			[]string{},
			// hints: none yet. **A desktop-entry hint belongs here and
			// cannot be written honestly until there is a package to
			// name** — F12 §8 carries that as a measurement waiting for
			// one, because whether an installed entry changes the
			// activation-token answer is the thing it would settle.
			map[string]dbus.Variant{},
			notifyNeverExpires,
		).Store(&id)
	if err != nil {
		return nil, fmt.Errorf("platform: posting a notification: %w", err)
	}
	return dbusNotification{conn: conn, id: id}, nil
}

// Close takes the notification down, because the request it announced
// has been answered.
//
// A failure is logged and nothing more: the caller is on its way to
// reporting a signature or a refusal, and a notification this program
// could not withdraw is not a reason to interrupt that.
func (n dbusNotification) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), notifyCallTimeout)
	defer cancel()

	if err := n.conn.Object(notifyDest, notifyPath).
		CallWithContext(ctx, notifyIface+".CloseNotification", 0, n.id).Err; err != nil {
		slog.Debug("platform: could not withdraw the notification", "id", n.id, "error", err)
	}
}

// appName is what the desktop shows as the sender. It is this program's
// own name and never anything a caller supplied — SPEC §6.6 makes that
// point about the consent screen's application name, and a notification
// is more exposed than the screen rather than less: it is the one
// surface here that appears without anybody opening it.
const appName = "Liro Bridge"
