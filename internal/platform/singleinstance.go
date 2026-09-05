package platform

// Leader decides which of several simultaneously-started processes does
// a piece of work that only one of them should do.
//
// F6 §2 is the case it exists for: Explorer invokes a classic shell
// verb once per selected file, in a separate process, so twenty
// selected documents start twenty copies of this program. Exactly one
// of them must open a window; the rest hand their file over and exit.
type Leader interface {
	// Acquire reports whether this process is the one that should do
	// the work. It never blocks and never waits for another process.
	// A process that acquires holds the claim until Release.
	Acquire() (bool, error)

	// Release gives up the claim. Safe to call whether or not Acquire
	// succeeded.
	Release()
}

// ShellBatchLeaderName is the claim the Explorer-invocation collector
// takes. Per user, not per machine: SPEC §14.1 makes several users on
// one machine a supported configuration, and one of them signing must
// not stop another from doing the same.
const ShellBatchLeaderName = "LiroBridge.ShellBatch"
