//go:build !windows

package main

import (
	"context"
	"io"

	"github.com/veljaos/liro-bridge/internal/config"
)

// runSignInteractive is Windows-only this phase (see the windows.go
// file's doc comment) — internal/ui itself has no other-platform
// implementation yet either.
func runSignInteractive(_ context.Context, _ []string, out io.Writer, _ string, _ config.Config) int {
	fprintln(out, "liro-bridge: sign --interactive is only supported on Windows in this phase.")
	return 1
}
