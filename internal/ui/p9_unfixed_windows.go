//go:build windows

package ui

// p9Unfixed switches C21's fixes in this package back to the behaviour
// before them, for ONE CI run: P9 measures the new tests red on the old
// behaviour before the next commit removes this file and every use of
// it (worklist 0.1–0.3, D-473). Not a setting; never shipped.
var p9Unfixed = true
