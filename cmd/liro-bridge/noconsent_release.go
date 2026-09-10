//go:build !softtoken

package main

import (
	"context"
	"io"

	"github.com/veljaos/liro-bridge/internal/config"
)

// A release binary has no signing path that skips the consent window,
// so it has no command for one either. SPEC §18.2: no signature without
// human approval, and no flag, configuration or header bypasses the
// consent screen.
//
// The absence is proved by inspecting a release-shaped binary rather
// than by trusting this build tag — see
// TestTheNoConsentPathIsAbsentFromAReleaseBinary and CI's own
// binary-inspection step.

// hasNoConsentPaths is what this build is. It exists so that one test
// can assert the right thing in both build configurations rather than
// two tests each asserting half of it in a build the other never sees —
// the Windows CI job builds the suite *with* the tag, so a check that
// only compiles without it would never run there (D-221, D-222).
const hasNoConsentPaths = false

// buildOnlyCommands reports none: this build has no subcommand a
// release build does not.
func buildOnlyCommands() []command { return nil }

// runBuildOnlyCommand handles one thing, and handles it by refusing:
// `sign-digest`, which was a release binary's command until F9b.
//
// A name that used to work and now silently prints the usage text and
// exits 0 is a trap — a script calling it would report success and
// produce no signature. This is the same answer D-222 gave `sign
// --interactive`: say what happened, in a sentence, and fail. English,
// like every other argument-parsing diagnostic (D-092, SPEC §9.2).
//
// Knowing the name is not knowing the path. internal/cli.RunSignDigest
// is absent from this binary, which is the property that matters and
// the one the symbol table is read to prove.
func runBuildOnlyCommand(_ context.Context, args []string, _ io.Writer, stderr io.Writer, _ config.Config) (int, bool) {
	if len(args) == 0 || args[0] != "sign-digest" {
		return 0, false
	}
	fprintln(stderr, "liro-bridge: sign-digest is not in a release build: it signs whatever digest it is handed, with no consent window (SPEC §18.2).")
	fprintln(stderr, "  A local batch: liro-bridge sign --in <file>. An application: POST /v2/sign (see docs/PROTOCOL.md).")
	return 2, true
}
