//go:build !windows

package main

import (
	"context"
	"io"

	"github.com/veljaos/liro-bridge/internal/config"
)

// runSignCommand is Windows-only this phase (see the windows.go file's
// doc comment) — internal/ui itself has no other-platform
// implementation yet either, and `sign` is the consent window (SPEC
// §18.2), so there is nothing it could honestly do here instead.
func runSignCommand(_ context.Context, _ []string, out io.Writer, _ string, _ config.Config) int {
	fprintln(out, "liro-bridge: sign is only supported on Windows in this phase.")
	return 1
}
