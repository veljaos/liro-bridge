//go:build windows

package main

import "time"

// webView2ExitGrace is a brief pause before the tray subcommand returns
// (and the process reaches ExitProcess), given only after at least one
// WebView2 window has actually been opened and closed during this
// session (Task 8, F5 first-real-run review). ui.Window.Close already
// waits for this process's own COM teardown to finish before returning
// (D-0xx), but the WebView2 runtime's own browser process — a separate
// executable — tears itself down asynchronously once that happens, on
// its own schedule; this is what gives it a little more of that
// schedule to run before this process disappears out from under it,
// which is what actually produces its "Failed to unregister class
// Chrome_WidgetWin_0" console line (emitted by that browser process,
// not by any code in this repository) when the two race. Not applied
// when no such window was ever opened this session — a user who only
// ever used the tray menu's Quit should never wait for anything.
const trayExitGrace = 400 * time.Millisecond
