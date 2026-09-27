//go:build !windows && !linux

package main

// filesDroppedHandler is nil on a platform whose window host does not
// implement drops, and asking anyway would be a refusal rather than a
// silence. Linux had this until D-371, when D-338's reason for it — that
// the binding could not read a drop — turned out not to be true.
func filesDroppedHandler(func([]string)) func([]string) { return nil }
