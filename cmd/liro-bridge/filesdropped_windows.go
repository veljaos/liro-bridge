//go:build windows

package main

// filesDroppedHandler is the handler itself on a platform whose window
// host implements drops (F6 §1).
func filesDroppedHandler(h func([]string)) func([]string) { return h }
