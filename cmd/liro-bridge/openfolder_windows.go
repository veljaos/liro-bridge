//go:build windows

package main

import "os/exec"

// openFolder shows a directory in Explorer.
//
// The path is an argument to the process, never a command line a shell
// parses, so a folder name with a space or a quote in it is a folder
// name rather than two arguments and a syntax error.
func openFolder(dir string) error {
	return exec.Command("explorer.exe", dir).Start()
}
