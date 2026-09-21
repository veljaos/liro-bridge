//go:build !windows

package main

import "time"

// trayExitGrace is nothing here, and the reason is worth a line rather
// than a zero.
//
// On Windows the pause exists for the WebView2 runtime's *browser
// process* — a separate executable that tears itself down on its own
// schedule after this process has closed its windows, and that prints a
// complaint if this process disappears first. WebKitGTK's web process
// is this program's own child through bubblewrap: it goes when the view
// goes, and there is nothing left racing the exit.
const trayExitGrace = 0 * time.Second
