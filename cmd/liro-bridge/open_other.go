//go:build !windows

package main

import (
	"context"
	"io"

	"github.com/veljaos/liro-bridge/internal/config"
)

// runOpen and runShellVerb are Windows-only this phase, like every
// other window entry point here (SPEC §11.11) — see
// interactive_other.go's doc comment.
func runOpen(_ context.Context, _ []string, out io.Writer, _ config.Config) int {
	fprintln(out, "liro-bridge: open is only supported on Windows in this phase.")
	return 1
}

func runShellVerb(_ context.Context, _ []string, _ config.Config) int {
	return 1
}
