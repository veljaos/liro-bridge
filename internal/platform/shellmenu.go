package platform

// ShellMenu controls the "sign this" entry Explorer shows when a person
// right-clicks a PDF (F6 §2). Registered per user, so it needs no
// administrator rights, and removed cleanly when switched off.
type ShellMenu interface {
	// IsRegistered reports whether the entry is currently present.
	IsRegistered() (bool, error)

	// Register adds or updates the entry. label is the already-localised
	// menu text, exePath the binary to invoke, and iconPath where the
	// menu icon comes from (normally exePath itself).
	Register(label, exePath, iconPath string) error

	// Unregister removes the entry and everything it owns.
	Unregister() error
}

// ShellMenuVerbFlag is the argument the context-menu command passes, so
// an invocation arriving from Explorer is recognisable as one. It is
// what tells the agent to coalesce this file with the others Explorer
// is about to invoke it for, rather than opening a window per file.
const ShellMenuVerbFlag = "--shell-verb"

// shellQuote is the double quote the command string is built from,
// named because a bare one inside a Go string next to another quote is
// unreadable.
const shellQuote = `"`

// ShellMenuCommand builds the command string Explorer runs for one
// selected file.
//
// "%1" is the file Explorer substitutes, quoted because Serbian
// document names contain spaces constantly and an unquoted %1 hands the
// agent half a name. exePath is quoted for the same reason: a per-user
// install lives under a profile directory whose name is the user's own,
// spaces and all.
//
// The quoting is a pair of literal double quotes rather than
// strconv.Quote. Quote produces *Go source* syntax, which escapes every
// backslash — an ordinary path came out as
// "C:\\Users\\Petar Petrović\\..." and would have reached Windows with
// each separator doubled. A Windows path cannot contain a double quote
// (the filesystem forbids it), so surrounding quotes need no escaping
// of their own.
//
// Windows invokes a classic shell verb once per selected file, so this
// command runs N times for N files. Coalescing them back into one batch
// is the agent's job, not the registration's — see internal/jobs's
// coalescing window.
func ShellMenuCommand(exePath string) string {
	return shellQuote + exePath + shellQuote + " " + ShellMenuVerbFlag + " " + shellQuote + "%1" + shellQuote
}
