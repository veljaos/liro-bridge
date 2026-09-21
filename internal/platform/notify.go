package platform

// Desktop notifications: the "look at the window" half of SPEC §6.5.2.
//
// A consent window on a platform that will not let a program raise it is
// a window a person may not notice. SPEC §6.5.2 answers that with a
// notification posted alongside the window, and is exact about what it
// is for: **"a prompt to go and look rather than a way to reach the
// window"**. It is not a second consent surface, it carries no Approve,
// and nothing about the security argument rests on it — an unanswered
// request times out as CONSENT_TIMEOUT and nothing is signed (D-288).
//
// **It is best-effort and its absence is never a refusal.** A desktop
// with no notification service is a desktop where the window still
// exists and the timeout still protects, so a caller logs that it could
// not post one and carries on. Refusing to sign because a notification
// failed would block a person for no security gain, which the owner
// ruled and §6.5.2 records.

// Notification is one posted notification. The only thing a caller can
// do with it is take it down, which is what happens when the request it
// was about has been answered — a notification still sitting there
// saying a signature is waiting, for a batch signed ten minutes ago, is
// worse than none.
type Notification interface {
	Close()
}

// Notifier posts desktop notifications.
type Notifier interface {
	// Notify posts one and returns a handle to it. An error means
	// nothing was posted, which is a thing to log and not a thing to
	// stop for.
	Notify(summary, body string) (Notification, error)
}

// NewNotifier returns this platform's notifier.
func NewNotifier() Notifier { return newNotifier() }
