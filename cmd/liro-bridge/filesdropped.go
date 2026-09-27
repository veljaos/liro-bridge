//go:build windows || linux

package main

// filesDroppedHandler is the handler itself on a platform whose window
// host implements drops: Windows (F6 §1) and Linux (D-371).
func filesDroppedHandler(h func([]string)) func([]string) { return h }
