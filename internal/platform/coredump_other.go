//go:build !linux

package platform

// ForbidCoreDumps does nothing off Linux. On Windows, D-292 measured that
// nothing available to this program keeps a secret out of a crash dump, and
// SPEC §6.5.1 clause 3 says so rather than promising it; macOS is F13's.
func ForbidCoreDumps() error { return nil }
