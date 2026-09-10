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

// AutostartSubcommand is the argument the autostart entry passes, and
// the reason this constant exists at all.
//
// Until F10 the Run value was the quoted executable path and nothing
// else. `liro-bridge` with no arguments prints its usage and exits 0
// (F0's own tested contract, D-082) — so what the autostart entry
// actually did at every sign-in was start the program, print a page of
// help to a console that does not exist, and exit. Measured on the
// binary this phase began from. The agent has never once started with
// Windows.
//
// It lives here beside ShellMenuVerbFlag rather than in
// cmd/liro-bridge for the same reason that one does: the string is
// half of a registration this package writes, and a registration whose
// two halves are written in two packages is one that can disagree with
// itself.
const AutostartSubcommand = "tray"

// AutostartCommand builds the command line the Run entry launches:
// the quoted executable and the subcommand that actually starts the
// agent.
//
// Quoted for the same reason ShellMenuCommand quotes its own: a
// per-user install lives under a profile directory whose name is the
// person's own, spaces and all. A Windows path cannot contain a double
// quote, so the surrounding pair needs no escaping.
func AutostartCommand(exePath string) string {
	return shellQuote + exePath + shellQuote + " " + AutostartSubcommand
}
