package main

import (
	"fmt"
	"io"
)

// fprintln and fprintf mirror internal/cli's own small helpers
// (render.go) — duplicated here rather than exported across the
// internal/cli boundary for two one-line functions. Both deliberately
// discard the write error: there is no recovery action for a failed
// write to stdout, and letting every call site ignore it individually
// is what errcheck would otherwise flag repeatedly for no benefit.
//
// They live in their own file, with no build constraint, because
// main.go and interactive_other.go use them on every platform. They
// were previously in ui_assets.go, which is Windows-only in everything
// but its filename — see that file's own note.
func fprintln(w io.Writer, a ...any)               { _, _ = fmt.Fprintln(w, a...) }
func fprintf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
