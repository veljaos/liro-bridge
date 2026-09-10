//go:build !windows

package platform

// ProcessRunning is Windows-only in substance: its one caller is the
// installer guard (F10 §3.2), and there is no installer on any other
// platform yet. Reporting false everywhere else means a mark that
// somehow exists there is treated as stale, which is the behaviour
// noSigningFlag already implies.
func ProcessRunning(int) bool { return false }
