//go:build !linux

package platform

import "errors"

// errNoNotifier is returned where this program does not post desktop
// notifications, which is every platform whose window host can raise
// its own window.
//
// **Windows is not missing this.** SPEC §6.5.2 exists because a
// compositor refuses to let a client come to the front; WebView2's
// window is brought forward by the agent and the call is reliable, so
// there is nothing for a notification to compensate for and §6.5.2
// "permits nothing for it". A no-op that reported success would make
// the caller's log say a notification had been posted on a platform
// that posts none.
var errNoNotifier = errors.New("platform: this platform does not post desktop notifications")

type noNotifier struct{}

func newNotifier() Notifier { return noNotifier{} }

func (noNotifier) Notify(string, string) (Notification, error) { return nil, errNoNotifier }
