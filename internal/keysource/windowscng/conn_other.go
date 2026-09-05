//go:build !windows

package windowscng

import (
	"fmt"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// unsupportedConn is the ncryptConn used on platforms other than
// Windows. CNG is a Windows-only API; macOS and Linux get their own key
// sources in phases 12 and 13 (SPEC §11.11). Enumerate already reports
// no certificates on these platforms (F1), so nothing here can ever
// succeed — this exists so the package, and anything built on top of
// it, cross-compiles.
type unsupportedConn struct{}

func newConn() ncryptConn { return unsupportedConn{} }

// findAndAcquire reports CERT_NOT_FOUND, the same code the Windows
// implementation returns when the store holds no such certificate
// (D-033). That is not a euphemism here: there is no CNG store on this
// platform, so no thumbprint is in it, and "not found" is the literally
// correct answer to the question asked.
//
// The code matters beyond tidiness. cmd/liro-bridge falls back to the
// soft token exactly when CNG answers CERT_NOT_FOUND, and never on any
// other error, so that a real failure (card removed, PIN blocked) is
// not masked by a confusing second attempt against an unrelated
// backend. Returning a bare "not supported on this platform" here meant
// that fallback could not fire at all on Linux or macOS, so
// `sign-digest` and `sign` against the soft token failed outright with
// that message — which is exactly what F2 §6.1's own CI step does, and
// what it did the first time that step ever ran (D-113).
func (unsupportedConn) findAndAcquire(string) ([]byte, ncryptKeyHandle, bool, error) {
	return nil, 0, false, errs.New(errs.CodeCertNotFound,
		fmt.Errorf("windowscng: no CNG certificate store on this platform"))
}

func (unsupportedConn) setWindowHandle(ncryptKeyHandle, uintptr) error {
	return fmt.Errorf("windowscng: not supported on this platform")
}

func (unsupportedConn) signHash(ncryptKeyHandle, []byte) ([]byte, error) {
	return nil, fmt.Errorf("windowscng: not supported on this platform")
}

func (unsupportedConn) freeKey(ncryptKeyHandle) error { return nil }

func (unsupportedConn) probePresence(string) (bool, error) {
	return false, fmt.Errorf("windowscng: not supported on this platform")
}
