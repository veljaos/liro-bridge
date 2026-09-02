package windowscng

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

// TestIsNoPrivateKey is the F1 §3.6 test: "CRYPT_E_NOT_FOUND results in
// a skipped certificate, not an error." windows.Errno is a plain
// uintptr-based type, so the real return code can be exercised here
// without any actual syscall.
func TestIsNoPrivateKey(t *testing.T) {
	if !isNoPrivateKey(windows.Errno(cryptENotFound)) {
		t.Fatal("isNoPrivateKey(CRYPT_E_NOT_FOUND) = false, want true")
	}
	if isNoPrivateKey(windows.Errno(0x80090016)) { // an unrelated NTE_ code
		t.Fatal("isNoPrivateKey matched an unrelated error code")
	}
	if isNoPrivateKey(errors.New("not even an Errno")) {
		t.Fatal("isNoPrivateKey matched a non-Errno error")
	}
}
