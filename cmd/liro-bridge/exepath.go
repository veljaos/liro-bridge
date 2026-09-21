package main

import "os"

// exePath is this program's own path on disk, for the two things that
// have to name it to the operating system: the autostart registration
// and, on Windows, the Explorer context-menu entry.
//
// Neutral, and it always was — os.Executable is. It lived in
// tray_windows.go because its callers did.
func exePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}
