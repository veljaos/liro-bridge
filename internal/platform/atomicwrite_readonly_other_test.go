//go:build !windows

package platform

import "testing"

// markReadOnly reports that this platform has no read-only file
// attribute to confuse with a sharing violation. The test that uses it
// skips.
func markReadOnly(t *testing.T, path string) (restore func(), marked bool) {
	t.Helper()
	_ = path
	return func() {}, false
}
