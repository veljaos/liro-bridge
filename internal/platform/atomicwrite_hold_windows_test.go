package platform

import (
	"testing"

	"golang.org/x/sys/windows"
)

// holdFileOpen opens path the way an ordinary reader does — a PDF
// viewer with the signed document on screen — and returns a function
// that closes it again.
//
// FILE_SHARE_READ|FILE_SHARE_WRITE and deliberately *not*
// FILE_SHARE_DELETE, which is what makes this the real reproduction
// rather than a contrived one: it is what a C runtime's fopen("rb")
// and most Windows programs that are merely displaying a file do. Such
// a handle permits the truncate-and-overwrite os.WriteFile performs —
// which is how a previously good signed file used to be destroyed — and
// forbids the delete a rename over the destination needs. That is
// exactly the state J-8 measured: os.Rename returned "Access is
// denied" while os.WriteFile succeeded.
//
// A Go reader (os.Open) would not reproduce it: Go passes
// FILE_SHARE_DELETE as well, so the rename goes through.
func holdFileOpen(t *testing.T, path string) (release func(), held bool) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0)
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	return func() { _ = windows.CloseHandle(h) }, true
}
