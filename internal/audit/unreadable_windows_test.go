package audit

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// holdFileUnreadable makes path impossible for this process to open,
// and returns a function that lets go again.
//
// An exclusive handle rather than an ACL change: it reproduces exactly
// what a network drive that has gone away or a file somebody else has
// locked looks like from inside os.Open, and it cannot leave the
// machine in a state a failed test would have to clean up. Denying
// oneself access through an ACL, by contrast, is a change to a real
// object with a real chance of outliving the test.
func holdFileUnreadable(t *testing.T, path string) (release func(), held bool) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ,
		0 /* no sharing at all */, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	if _, err := os.Open(path); err == nil {
		_ = windows.CloseHandle(h)
		t.Skip("an exclusive handle did not make the file unreadable on this system")
	}
	return func() { _ = windows.CloseHandle(h) }, true
}
