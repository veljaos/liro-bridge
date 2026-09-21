//go:build !windows

package main

import "os/exec"

// openFolder shows a directory in whatever the desktop uses for one.
//
// `xdg-open` is the freedesktop way to ask: it consults the desktop's
// own association for `inode/directory` and hands the path to Nautilus,
// Dolphin, Thunar or whatever else is there, without this program
// having to know which. It is the same shape as the Explorer call on
// the other side rather than a special case.
//
// **The path is an argument and never a shell command**, which is the
// whole of the care this function needs: a folder called `Q3 "final"`
// or one with a space in it is one argument here and would be several
// words and an unbalanced quote through `sh -c`. There is no shell in
// this call at all.
//
// An absent xdg-open is an error like any other and the caller logs it:
// this runs because somebody pressed a button on the report screen, and
// the documents are signed and written whether or not a file manager
// opens.
func openFolder(dir string) error {
	return exec.Command("xdg-open", dir).Start()
}
