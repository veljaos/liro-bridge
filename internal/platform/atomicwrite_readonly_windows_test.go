package platform

import (
	"testing"

	"golang.org/x/sys/windows"
)

// markReadOnly sets FILE_ATTRIBUTE_READONLY on path and returns a
// function that clears it again.
func markReadOnly(t *testing.T, path string) (restore func(), marked bool) {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	before, err := windows.GetFileAttributes(p)
	if err != nil {
		t.Fatalf("GetFileAttributes: %v", err)
	}
	if err := windows.SetFileAttributes(p, before|windows.FILE_ATTRIBUTE_READONLY); err != nil {
		t.Fatalf("SetFileAttributes: %v", err)
	}
	return func() { _ = windows.SetFileAttributes(p, before) }, true
}
