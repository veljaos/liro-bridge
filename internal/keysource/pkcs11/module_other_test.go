//go:build !windows && !linux

package pkcs11

import (
	"strings"
	"testing"
)

// The stub must refuse clearly rather than silently doing nothing, and it must
// say why it is refusing — the reason is not "unsupported" in general but that
// the layouts this package reads are measured for Windows x64, where CK_ULONG
// is 4 bytes, and would be wrong here rather than merely unavailable.
func TestThePlatformStubRefusesAndSaysWhy(t *testing.T) {
	m, err := openModule("/usr/lib/opensc-pkcs11.so")
	if err == nil {
		t.Fatal("openModule succeeded on a platform with no binding")
	}
	if m != nil {
		t.Error("openModule returned a module alongside its error")
	}
	if !strings.Contains(err.Error(), "F12") && !strings.Contains(err.Error(), "F13") {
		t.Errorf("%q does not say where the binding for this platform will come from", err)
	}

	// close on the zero value must not panic: the orchestration layer above
	// will defer it whether or not the open succeeded.
	var zero module
	if err := zero.close(); err != nil {
		t.Errorf("close on an unopened module: %v", err)
	}
}
