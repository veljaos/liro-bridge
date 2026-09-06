//go:build !windows

package platform

import "testing"

// holdFileOpen reports that this platform has no such condition:
// rename(2) replaces a file other processes hold open, and they go on
// reading the inode they already have. The test that uses this skips.
func holdFileOpen(t *testing.T, path string) (release func(), held bool) {
	t.Helper()
	_ = path
	return func() {}, false
}
