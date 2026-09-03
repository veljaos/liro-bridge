package main

import (
	"fmt"
	"io"
	"io/fs"

	"github.com/veljaos/liro-bridge/internal/ui"
)

// fprintln mirrors internal/cli's own small helper (render.go) —
// duplicated here rather than exported across the internal/cli
// boundary for one two-line function.
func fprintln(w io.Writer, a ...any)               { _, _ = fmt.Fprintln(w, a...) }
func fprintf(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

// liroVirtualHost is the hostname every window's assets are mapped to
// (F5 §2.4). It resolves to nothing outside the WebView2 instance that
// mapped it.
const liroVirtualHost = "liro.local"

// assetsFS is the subtree ui.Assets serves at the virtual host root —
// computed once so every window creation reuses the same fs.FS value.
var assetsFS = mustAssetsSub()

func mustAssetsSub() fs.FS {
	sub, err := fs.Sub(ui.Assets, "assets")
	if err != nil {
		panic("cmd/liro-bridge: internal/ui.Assets has no \"assets\" subdirectory: " + err.Error())
	}
	return sub
}
