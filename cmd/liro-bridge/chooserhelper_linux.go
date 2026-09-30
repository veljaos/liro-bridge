//go:build linux

package main

import (
	"fmt"
	"os"

	"github.com/veljaos/liro-bridge/internal/chooser"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// chooserHelperRequested reports whether this process was started as the
// file-chooser helper: this binary's path and chooser.Subcommand, and
// nothing else (D-410).
//
// Exact, and the whole command line, because it decides whether this
// process keeps its dumpable flag. A desktop entry, the autostart file and
// every spawner in this program put something else in args[1], so none of
// them can arrive here by accident.
func chooserHelperRequested(args []string) bool {
	return len(args) == 2 && args[1] == chooser.Subcommand
}

// runChooserHelper is the helper, and the only thing that process does.
func runChooserHelper() int {
	return chooser.Run(os.Stdin, os.Stdout)
}

// enableChooserHelper tells the windows which binary to run as the helper.
//
// Only this program's main does this, so a test binary, which is not this
// program and would not dispatch the subcommand, never spawns itself as one:
// the recursion D-293 measured is bounded by construction here rather than
// by a marker in the environment, which the helper's must not carry.
func enableChooserHelper() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "liro-bridge: the file chooser will not be available:", err)
		return
	}
	ui.SetChooserExecutable(exe)
}
