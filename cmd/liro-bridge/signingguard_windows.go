//go:build windows

package main

import (
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// signingFlag is the process's one mark. A package variable so a test
// can point it at a scratch registry key rather than at the real one —
// the same seam auditStore and interactiveGather already are.
var signingFlag = platform.NewSigningFlag()

// beginSigningGuard marks a batch as in flight and returns the flag it
// marked, so the caller's defer gives it back to endSigningGuard
// without either of them having to know which flag it was.
//
// A failure to mark is logged and not fatal. The mark exists so that
// an installer refuses rather than interrupting; a registry write that
// fails is a reason to lose the guard, never a reason to refuse to
// sign — the person is standing at the card.
func beginSigningGuard() platform.SigningFlag {
	f := signingFlag
	if err := f.Begin(); err != nil {
		slog.Warn("signing: could not mark the batch as in flight; an upgrade would not be blocked by it", "error", err)
	}
	return f
}

// endSigningGuard clears the mark.
func endSigningGuard(f platform.SigningFlag) {
	if err := f.End(); err != nil {
		slog.Warn("signing: could not clear the in-flight mark; the next agent to start will", "error", err)
	}
}

// clearStaleSigningGuard is called once when the agent starts. It is
// what bounds how long a mark left behind by a process that died
// mid-batch can refuse somebody's installer: until the agent next
// runs, which for a tray agent is the next sign-in.
//
// Nothing else clears it unconditionally. A batch that ends normally
// clears its own; this is only for the batch that did not end at all.
func clearStaleSigningGuard() {
	held, pid, err := signingFlag.Held()
	if err != nil {
		slog.Warn("signing: could not read the in-flight mark", "error", err)
		return
	}
	if !held {
		return
	}
	if platform.ProcessRunning(pid) {
		// Somebody is signing right now — in this session, in another
		// process. Clearing their mark would be this invocation
		// deciding, on no evidence, that their batch is over.
		return
	}
	slog.Warn("signing: clearing an in-flight mark left behind by a process that did not finish its batch", "pid", pid)
	if err := signingFlag.Clear(); err != nil {
		slog.Warn("signing: could not clear the stale in-flight mark", "error", err)
	}
}
