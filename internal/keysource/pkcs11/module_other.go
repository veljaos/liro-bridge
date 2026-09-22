//go:build !windows && !linux

package pkcs11

import "fmt"

// A PKCS#11 module is reached through LoadLibrary here and would be reached
// through dlopen elsewhere, and the structures it returns are laid out
// differently: CK_ULONG is 4 bytes on Windows x64 (LLP64) and 8 on Linux
// (LP64), so every offset in module_windows.go is wrong on this platform
// rather than merely unavailable.
//
// F12 and F13 are where that changes. When they do, the dlopen half belongs
// here — with its own measured offsets, not with the Windows ones widened by
// hand.

type module struct{}

func openModule(path string) (*module, error) {
	return nil, fmt.Errorf("pkcs11: not supported on this platform yet (see F12/F13): %s", path)
}

func (m *module) close() error { return nil }

// There is deliberately no info() and no moduleInfo here.
//
// Both existed briefly, added "so that the probe child compiles for every
// platform" and documented as unreachable — openModule refuses first, and a
// module value is the only way to call them. A stub whose stated purpose is to
// satisfy a compiler is a sign that the caller is pretending to be
// platform-neutral over something that is not, and it was: RunProbe's load step
// is now describeModule, split per platform like everything else in this
// package. See probechild_other.go.
