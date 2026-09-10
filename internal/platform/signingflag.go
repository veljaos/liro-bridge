package platform

// SigningFlag is how a batch in flight is visible to something that is
// not this process — specifically, to the installer.
//
// F10 §3.2: an upgrade must not interrupt a signature in progress. The
// installer replaces liro-bridge.exe, and Windows Installer's Restart
// Manager would otherwise close or terminate the running agent to do
// it. Restart Manager asks a GUI application to close by posting
// WM_QUERYENDSESSION to its visible top-level windows; the tray's own
// window is message-only, so there is nothing for it to ask and the
// fallback is TerminateProcess — the card session gone mid-batch, the
// document half-written, the audit entry never made.
//
// So the agent says out loud, in a place an MSI can read with no code
// running, when it must not be interrupted. The installer reads it
// with a RegistrySearch and refuses, which is one of the three answers
// F10 §3.2 allows and the only one that needs no custom action, no DLL
// and no cooperation from a process that may be blocked on a card.
//
// The value is a string rather than a DWORD because MSI's
// AppSearch/RegLocator reads a string most simply, and because what it
// holds is diagnostic rather than arithmetic: the PID of the process
// that set it, so a stale flag names what left it behind.
type SigningFlag interface {
	// Begin marks a batch as in flight. Idempotent.
	Begin() error

	// End clears the mark. Clearing a mark that is not there is not an
	// error: the requested state is already the actual one.
	End() error

	// Held reports whether the mark is currently set, and by which
	// process id.
	Held() (held bool, pid int, err error)

	// Clear removes the mark unconditionally. The agent calls this once
	// at startup, which is what bounds how long a flag left behind by a
	// process that died mid-batch can block an installer: until the
	// agent next runs. Nothing else in this program calls it.
	Clear() error
}

// SigningFlagKeyPath and SigningFlagValueName are where the mark lives.
// They are exported because the installer's own authoring
// (build/msi/liro-bridge.wxs) has to name the same key, and a path
// written down twice in two languages is a path that drifts — this
// project has recorded that four times (D-108, D-124, D-138, D-183).
// A test reads the .wxs and compares.
const (
	SigningFlagKeyPath   = `Software\Liro\Bridge`
	SigningFlagValueName = "SigningInProgress"
)
