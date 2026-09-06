// Everything in this file exists to feed a WebView2 window, which is
// Windows-only this phase (SPEC §11.11) — the filename says so now.
// Without the suffix these symbols still compiled on Linux, where every
// one of their callers is behind a `windows` constraint and therefore
// absent, so `unused` was right to flag them. The two output helpers
// that genuinely are cross-platform moved to print.go.

package main

import (
	"io/fs"

	"github.com/veljaos/liro-bridge/internal/ui"
)

// liroVirtualHost is the hostname every window's assets are mapped to
// (F5 §2.4). It resolves to nothing outside the WebView2 instance that
// mapped it.
const liroVirtualHost = "liro.invalid"

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
