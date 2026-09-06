//go:build !windows

package audit

import (
	"os"
	"testing"
)

// holdFileUnreadable makes path impossible for this process to open, by
// clearing every permission bit on it. It is restored afterwards.
//
// A process running as root can read it anyway, which is why this
// reports whether it actually worked rather than assuming it did.
func holdFileUnreadable(t *testing.T, path string) (release func(), held bool) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	mode := info.Mode().Perm()
	if err := os.Chmod(path, 0); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	restore := func() { _ = os.Chmod(path, mode) }
	if f, err := os.Open(path); err == nil {
		_ = f.Close()
		restore()
		return func() {}, false
	}
	return restore, true
}
