//go:build !linux

package main

// The file-chooser helper exists on Linux only (D-410): Windows' choosers
// are the system's own dialogs, called in process.

func chooserHelperRequested([]string) bool { return false }

func runChooserHelper() int { return 2 }

func enableChooserHelper() {}
