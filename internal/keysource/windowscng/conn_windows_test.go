package windowscng

import (
	"errors"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// TestFindAndAcquireUnknownThumbprintIsCertNotFound is the D-033
// regression test: a thumbprint that is not in the current user's "MY"
// store must map to CERT_NOT_FOUND, not SIGN_FAILED. Every other test in
// this package drives session logic through a fake ncryptConn
// (session_core_test.go), which is exactly why the original bug survived
// a fully green test suite — nothing exercised the real
// CertFindCertificateInStore call path. This test calls realConn{}
// directly, against the real Windows store, so it cannot pass for the
// same wrong reason.
func TestFindAndAcquireUnknownThumbprintIsCertNotFound(t *testing.T) {
	// A syntactically valid SHA-1 hex thumbprint that is vanishingly
	// unlikely to exist in any real "MY" store.
	const absentThumbprint = "0000000000000000000000000000000000BEEF"

	_, _, _, err := realConn{}.findAndAcquire(absentThumbprint)
	if err == nil {
		t.Fatal("findAndAcquire(absent thumbprint) returned no error, want CERT_NOT_FOUND")
	}

	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("findAndAcquire(absent thumbprint) error is not *errs.Error: %v", err)
	}
	if e.Code != errs.CodeCertNotFound {
		t.Fatalf("findAndAcquire(absent thumbprint).Code = %q, want %q (this is exactly the D-033 regression: an unmapped CertFindCertificateInStore failure falling through to SIGN_FAILED)", e.Code, errs.CodeCertNotFound)
	}
}
