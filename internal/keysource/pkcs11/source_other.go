//go:build !windows

package pkcs11

import (
	"context"
	"errors"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// ErrPlatform is what every method answers on a platform with no binding. The
// layouts this package reads are measured for Windows x64, where CK_ULONG is 4
// bytes, and are wrong here rather than merely unavailable — F12 and F13 are
// where that changes.
var ErrPlatform = errors.New("pkcs11: not supported on this platform yet (see F12/F13)")

// Source implements keysource.Source over one PKCS#11 module.
type Source struct {
	modulePath string
	entry      PINEntry
}

// NewSource returns a Source over the module at path.
func NewSource(modulePath string) Source { return Source{modulePath: modulePath} }

// WithPINEntry returns a copy of this Source that collects PINs with entry.
//
// It exists here so that the type a caller has to satisfy, and the method they
// call, are the same on every platform — wiring written once compiles for
// F12's macOS and F13's Linux without being written again. Nothing on this
// platform will call it, because Open refuses before it could.
func (s Source) WithPINEntry(entry PINEntry) Source {
	s.entry = entry
	return s
}

// ModulePath is which module this Source speaks to.
func (s Source) ModulePath() string { return s.modulePath }

// Name implements keysource.Source.
func (Source) Name() string { return "pkcs11" }

// Enumerate implements the Windows behaviour's shape and refuses here.
func (s Source) Enumerate(ctx context.Context) ([]CertificateInfo, error) {
	return nil, ErrPlatform
}

// List implements keysource.Source.
func (s Source) List(ctx context.Context) ([]keysource.Certificate, error) {
	return nil, ErrPlatform
}

// Open implements keysource.Source.
func (s Source) Open(ctx context.Context, thumbprint keysource.Thumbprint) (keysource.Session, error) {
	return nil, ErrPlatform
}

// ChainFor returns the chain a token carries for one of its certificates.
func (s Source) ChainFor(ctx context.Context, want keysource.Thumbprint) ([][]byte, error) {
	return nil, ErrPlatform
}
