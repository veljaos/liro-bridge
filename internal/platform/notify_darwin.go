//go:build darwin

package platform

import (
	"context"
	"os/exec"
	"time"
)

// Desktop notifications on macOS (F13, [[D-463]]), through osascript's
// `display notification`.
//
// **Not UNUserNotificationCenter**, which is the proper API and needs a
// signed bundle with a notification entitlement — this program is
// unsigned (F13 §6). The cost is that the notification is attributed to
// Script Editor rather than to Liro Bridge, and that it cannot be taken
// down: Close does nothing, so a notification about a request answered a
// minute later stays in Notification Centre until the person clears it.
// Both go away with signing; until then this is said rather than hidden.
//
// The text is passed to the script as arguments, never written into it,
// so nothing in a summary or a body — an origin, a name — is ever
// AppleScript.
const notifyScript = `on run argv
display notification (item 2 of argv) with title (item 1 of argv)
end run`

type osascriptNotifier struct{}

func newNotifier() Notifier { return osascriptNotifier{} }

type postedNotification struct{}

// Close cannot take a notification down from osascript; see above.
func (postedNotification) Close() {}

func (osascriptNotifier) Notify(summary, body string) (Notification, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", notifyScript, summary, body).Run(); err != nil {
		return nil, err
	}
	return postedNotification{}, nil
}
