//go:build !(darwin && cgo)

package ui

func runMain(f func() int) int { return f() }
