//go:build darwin

package platform

// NewAutostart is the macOS LaunchAgent (launchagent.go, F13).
func NewAutostart() Autostart { return launchAgent{path: LaunchAgentFile(OSEnv)} }

// EnsureAutostart makes the LaunchAgent agree with enabled at the
// agent's start, rewriting it only when it is not already exactly right
// — so that a binary that moved is followed (F10 §3.1's reason).
func EnsureAutostart(enabled bool, exePath string) error {
	return launchAgent{path: LaunchAgentFile(OSEnv)}.ensure(enabled, exePath)
}
