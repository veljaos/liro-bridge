//go:build windows

package main

// The weakest level a batch reached, and **nothing calls either of
// these.**
//
// They are here rather than in interactive.go because that file is
// compiled on every platform now and these have no caller on any of
// them: the only reference in the tree is
// tsa_choice_windows_test.go, a Windows-only test. So the build tag
// records where the single reference lives and is not a claim that the
// arithmetic is Windows-shaped — it is not.
//
// **That is worth someone's attention rather than a quiet move.** The
// doc comment below states a rule of SPEC §18.11 — report the weakest
// level a batch actually reached, so that a batch where only some
// documents got a timestamp is not reported as though all of them did
// — and if nothing calls this, nothing in the program applies that
// rule. It is D-247's shape exactly: implemented, tested, never
// invoked. Either the reporting path computes the level some other way
// and these are dead, or it does not and a mixed batch reports a level
// it did not reach. Deciding which needs the Windows reporting path in
// front of somebody, which this platform does not have.

import "github.com/veljaos/liro-bridge/internal/pades"

// levelRank orders the three PAdES levels so lowerLevel can pick the
// weakest one a batch actually reached. Reporting the weakest — not the
// strongest, and not the last — is what keeps the reported level honest
// for a batch where only some documents got a timestamp (SPEC §18.11).
func levelRank(level string) int {
	switch pades.Level(level) {
	case pades.LevelBB:
		return 1
	case pades.LevelBT:
		return 2
	case pades.LevelBLT:
		return 3
	default:
		return 0
	}
}

func lowerLevel(a, b string) string {
	if levelRank(a) == 0 {
		return b
	}
	if levelRank(b) == 0 {
		return a
	}
	if levelRank(b) < levelRank(a) {
		return b
	}
	return a
}
