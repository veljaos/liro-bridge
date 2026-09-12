//go:build !windows

package main

// detachAllocatedConsole has nothing to do on a platform whose loader
// does not hand a process a console it did not ask for. Unlike
// applyStartupRegistrations, which deliberately has no stub here
// because its only callers are Windows-only (startup_other.go), this
// one is called from main on every platform, so it needs a body.
func detachAllocatedConsole() {}
