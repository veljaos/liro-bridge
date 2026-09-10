//go:build windows

package main

import (
	"log/slog"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// detectWebView2 is a package variable so a test can put this process
// into the state a machine without the runtime is in, which is the one
// state this machine cannot be put into any other way — the runtime is
// installed here and removing it to test a message box is not a thing
// to do to somebody's computer.
var detectWebView2 = ui.DetectRuntime

// requireWebView2 is what every command that opens a window calls
// before it opens one. It reports whether to carry on.
//
// F10 §2: every window this program has is WebView2, so a machine
// without the Evergreen Runtime is a machine on which this program can
// do nothing at all — and what it did about that, measured before this
// was written, was exit 1 in a tenth of a second with nothing on
// stdout, nothing on stderr and nothing on screen. `sign` is what the
// Explorer verb runs, so there is no console for a message to go to
// even if one had been printed. A person double-clicked a PDF and
// nothing happened.
//
// ui.DetectRuntime and ui.ShowRuntimeMissingMessage were both written
// in F5 for exactly this and had no caller in the entire program. This
// is that caller.
//
// The message is a native MessageBox rather than a window, because the
// thing that cannot be created is windows. It is localised from the
// same catalogue as everything else, so a person reads it in their own
// language, which is more than an installer's own error dialog can do.
func requireWebView2(c *i18n.Catalogue) bool {
	available, version, err := detectWebView2()
	if err != nil {
		// The loader itself could not be reached — the DLL could not be
		// extracted, or extracted and could not be loaded. That is not
		// "the runtime is missing", and it is not something a person
		// can act on by installing the runtime, so it is reported as
		// what it is and the caller carries on to fail where it would
		// have failed anyway, with the real error.
		slog.Error("webview2: the runtime could not be detected", "error", err)
		return true
	}
	if available {
		slog.Info("webview2: runtime present", "version", version)
		return true
	}
	slog.Error("webview2: the Evergreen Runtime is not installed; every window this program has needs it")
	ui.ShowWarning(c.T("runtime.missing_title"), c.T("runtime.missing_body"))
	return false
}
