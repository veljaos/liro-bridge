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

// buildOnlyCommands reports none: this build has no subcommand a
// release build does not.
func buildOnlyCommands() []command { return nil }

// runBuildOnlyCommand handles nothing.
func runBuildOnlyCommand(context.Context, []string, io.Writer, io.Writer, config.Config) (int, bool) {
	return 0, false
}
