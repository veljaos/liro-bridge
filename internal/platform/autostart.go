package platform

// Autostart controls whether the agent launches automatically when the
// user signs in (F5 §3). Windows registers this per-user, under
// HKCU\Software\Microsoft\Windows\CurrentVersion\Run — never HKLM,
// which would need administrator rights the target user (a bookkeeper
// on a machine they do not administer) does not have.
type Autostart interface {
	// IsEnabled reports whether the agent is currently registered to
	// start automatically.
	IsEnabled() (bool, error)

	// SetEnabled registers or unregisters the agent for autostart.
	// exePath is the full path to the binary to launch.
	SetEnabled(enabled bool, exePath string) error
}
