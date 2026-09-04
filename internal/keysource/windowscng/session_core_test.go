package windowscng

import (
	"context"
	"errors"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

// fakeConn is a programmable ncryptConn, mirroring the fakeStore /
// fakeSCard pattern used elsewhere in this project to test the pure
// logic layer without the real DLL calls.
type fakeConn struct {
	der        []byte
	key        ncryptKeyHandle
	callerFree bool
	findErr    error

	windowHandleSet uintptr
	windowHandleErr error

	signFunc func(digest []byte) ([]byte, error)
	freed    []ncryptKeyHandle
	freeErr  error

	presencePresent bool
	presenceErr     error
}

func (f *fakeConn) findAndAcquire(string) ([]byte, ncryptKeyHandle, bool, error) {
	if f.findErr != nil {
		return nil, 0, false, f.findErr
	}
	return f.der, f.key, f.callerFree, nil
}

func (f *fakeConn) setWindowHandle(_ ncryptKeyHandle, hwnd uintptr) error {
	f.windowHandleSet = hwnd
	return f.windowHandleErr
}

func (f *fakeConn) signHash(_ ncryptKeyHandle, digest []byte) ([]byte, error) {
	if f.signFunc != nil {
		return f.signFunc(digest)
	}
	return []byte("signature"), nil
}

func (f *fakeConn) freeKey(key ncryptKeyHandle) error {
	f.freed = append(f.freed, key)
	return f.freeErr
}

func (f *fakeConn) probePresence(string) (bool, error) {
	return f.presencePresent, f.presenceErr
}

func TestOpenSessionSetsWindowHandleToZeroForHeadlessCallers(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 1}
	if _, err := openSession(conn, "ABC", 0); err != nil {
		t.Fatalf("openSession: %v", err)
	}
	if conn.windowHandleSet != 0 {
		t.Fatalf("windowHandleSet = %d, want 0 (headless CLI paths pass no window)", conn.windowHandleSet)
	}
}

// TestOpenSessionPassesThroughRealWindowHandle proves openSession
// forwards a non-zero windowHandle to conn.setWindowHandle unchanged
// (Task 2, F2 §2.3): the consent window passes its own HWND through
// Source.WithWindowHandle so the OS PIN dialog is parented to it
// instead of appearing behind it.
func TestOpenSessionPassesThroughRealWindowHandle(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 1}
	if _, err := openSession(conn, "ABC", 0xDEADBEEF); err != nil {
		t.Fatalf("openSession: %v", err)
	}
	if conn.windowHandleSet != 0xDEADBEEF {
		t.Fatalf("windowHandleSet = %#x, want 0xDEADBEEF", conn.windowHandleSet)
	}
}

// TestOpenSessionSucceedsDespiteWindowHandleFailure proves setting
// NCRYPT_WINDOW_HANDLE_PROPERTY failing is not fatal to opening the
// session (F2 §2.3).
func TestOpenSessionSucceedsDespiteWindowHandleFailure(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 1, windowHandleErr: errors.New("boom")}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v, want success despite the window-handle failure", err)
	}
	if sess == nil {
		t.Fatal("session is nil")
	}
}

func TestOpenSessionPropagatesFindError(t *testing.T) {
	conn := &fakeConn{findErr: errs.New(errs.CodeCertNotFound, errors.New("not found"))}
	_, err := openSession(conn, "ABC", 0)
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeCertNotFound {
		t.Fatalf("openSession error = %v, want CERT_NOT_FOUND", err)
	}
}

func TestSessionCertificateCarriesThumbprintAndDER(t *testing.T) {
	conn := &fakeConn{der: []byte("cert-bytes"), key: 1}
	sess, err := openSession(conn, "AABBCC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	cert := sess.Certificate()
	if cert.Thumbprint != "AABBCC" || string(cert.DER) != "cert-bytes" {
		t.Fatalf("Certificate() = %+v, want thumbprint AABBCC and DER cert-bytes", cert)
	}
	if cert.IsTestKey {
		t.Fatal("a CNG-backed certificate must never be marked IsTestKey")
	}
}

// TestSignDigestRejectsWrongLengthBeforeBackendCall is the failing test
// for F2 §2.2's "validate before calling" rule: an implementation that
// forgot the check would call fakeConn.signHash and this test would see
// signCalled become true.
func TestSignDigestRejectsWrongLengthBeforeBackendCall(t *testing.T) {
	var signCalled bool
	conn := &fakeConn{der: []byte("cert"), key: 1, signFunc: func([]byte) ([]byte, error) {
		signCalled = true
		return []byte("sig"), nil
	}}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	_, err = sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 31))
	if err == nil {
		t.Fatal("SignDigest with a 31-byte digest must fail")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeSignFailed {
		t.Fatalf("error = %v, want SIGN_FAILED", err)
	}
	if signCalled {
		t.Fatal("the backend must never be called for a digest of the wrong length")
	}
}

func TestSignDigestAcceptsCorrectLength(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 1}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	sig, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32))
	if err != nil {
		t.Fatalf("SignDigest: %v", err)
	}
	if string(sig) != "signature" {
		t.Fatalf("SignDigest returned %q, want the fake backend's signature", sig)
	}
}

func TestSignDigestFailsAfterClose(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 1}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err == nil {
		t.Fatal("SignDigest after Close must fail")
	}
}

// TestCloseHonoursCallerFreeTrue proves the key handle IS freed when
// fCallerFree was true.
func TestCloseHonoursCallerFreeTrue(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 42, callerFree: true}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(conn.freed) != 1 || conn.freed[0] != 42 {
		t.Fatalf("freed = %v, want [42] (fCallerFree was true)", conn.freed)
	}
}

// TestCloseHonoursCallerFreeFalse is the failing test for F2 §2.1: a
// cached handle (fCallerFree == false) must never be freed, or it
// corrupts the handle cache for every other process using the same key.
func TestCloseHonoursCallerFreeFalse(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 42, callerFree: false}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(conn.freed) != 0 {
		t.Fatalf("freed = %v, want none — fCallerFree was false, freeing corrupts Windows's cache", conn.freed)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 42, callerFree: true}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if len(conn.freed) != 1 {
		t.Fatalf("freed called %d times, want exactly 1", len(conn.freed))
	}
}

func TestSignDigestRespectsContextCancellation(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 1}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sess.SignDigest(ctx, keysource.DigestSHA256, make([]byte, 32)); err == nil {
		t.Fatal("SignDigest on a cancelled context must fail")
	}
}

// TestIsCardAbsentStatus is Task 2's own measurement, pinned: exactly
// NTE_BAD_KEYSET and SCARD_W_REMOVED_CARD mean "not present" — nothing
// else, including codes mapStatus (errors.go) treats as related-but-
// different failures (e.g. NTE_NO_KEY), is misclassified as absence.
func TestIsCardAbsentStatus(t *testing.T) {
	cases := []struct {
		name   string
		status uint32
		want   bool
	}{
		{"NTE_BAD_KEYSET", nteBadKeyset, true},
		{"SCARD_W_REMOVED_CARD", scardWRemovedCard, true},
		{"NTE_NO_KEY", nteNoKey, false},
		{"SCARD_W_WRONG_CHV", scardWWrongCHV, false},
		{"anything else", 0xDEADBEEF, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isCardAbsentStatus(c.status); got != c.want {
				t.Fatalf("isCardAbsentStatus(0x%08X) = %v, want %v", c.status, got, c.want)
			}
		})
	}
}

func TestChainIsEmptyInThisPhase(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 1}
	sess, err := openSession(conn, "ABC", 0)
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	if chain := sess.Chain(); len(chain) != 0 {
		t.Fatalf("Chain() = %v, want empty (chain completion arrives in F3)", chain)
	}
}
