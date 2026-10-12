package main

import "strings"

// launchArgs is what a launch means when the system, not a person, chose
// the arguments (F13). On macOS, Finder and LaunchServices start an
// application bundle with no arguments of this program's — at most an
// old -psn_ process serial number — and a bundle started that way is the
// agent: it is how a person opens Liro Bridge from Applications, and how
// a liro-bridge:// link starts it when it is not running. Everywhere
// else, and for a binary run from a shell, the arguments are what they
// say.
func launchArgs(goos, exe string, args []string) []string {
	if goos != "darwin" {
		return args
	}
	var kept []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-psn_") {
			kept = append(kept, a)
		}
	}
	if len(kept) == 0 && strings.Contains(exe, ".app/Contents/MacOS/") {
		return []string{"tray"}
	}
	return kept
}
