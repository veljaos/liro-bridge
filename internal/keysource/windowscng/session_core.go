package windowscng

import (
	"context"
	"fmt"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

// ncryptKeyHandle is an opaque handle to an acquired NCrypt private key.
// It is a plain uintptr — the same underlying type as windows.Handle —
// so the session logic below has no compile-time dependency on
// golang.org/x/sys/windows and can be unit-tested on any platform. The
// handle only has real meaning inside conn_windows.go.
type ncryptKeyHandle uintptr

// ncryptConn is the thin interface over the actual Windows API calls
// that session logic is tested against — mirroring the scardConn split
// in internal/platform/scard_core.go (F1 §2.3): conn_windows.go backs
// this with the real crypt32.dll/ncrypt.dll calls (F2 §2.1/§2.2), which
// cannot be meaningfully unit-tested; everything above this interface
// can be, with a fake.
type ncryptConn interface {
	// findAndAcquire locates the certificate with the given SHA-1
	// thumbprint in the current user's certificate store and acquires
	// its private key via CryptAcquireCertificatePrivateKey with
	// CRYPT_ACQUIRE_ONLY_NCRYPT_KEY_FLAG (F2 §2.1). It owns the
	// certificate store and context for the duration of the call and
	// closes both before returning — nothing above this layer needs
	// them once the key handle exists.
	findAndAcquire(thumbprint string) (der []byte, key ncryptKeyHandle, callerFree bool, err error)

	// setWindowHandle sets NCRYPT_WINDOW_HANDLE_PROPERTY on key. A
	// failure here is not fatal to the session (F2 §2.3): without it
	// the OS PIN dialog can appear unparented, not fail to appear.
	setWindowHandle(key ncryptKeyHandle, hwnd uintptr) error

	// signHash calls NCryptSignHash with PKCS#1 v1.5 padding over an
	// already-length-validated digest.
	signHash(key ncryptKeyHandle, digest []byte) ([]byte, error)

	// freeKey calls NCryptFreeObject. The caller (session.Close) must
	// call this only when fCallerFree was true (F2 §2.1) — a cached
	// handle must never be freed.
	freeKey(key ncryptKeyHandle) error
}

// session implements keysource.Session over a Windows CNG key handle
// (F2 §2). Not safe for concurrent use (SPEC §8.5) — internal/signing
// owns serialisation.
type session struct {
	conn       ncryptConn
	key        ncryptKeyHandle
	callerFree bool
	cert       keysource.Certificate
	closed     bool
}

// openSession opens a signing session for the certificate with the
// given thumbprint (F2 §2). windowHandle becomes
// NCRYPT_WINDOW_HANDLE_PROPERTY; F2 has no agent window yet, so callers
// pass 0 here — F5 supplies the real value once a window exists.
func openSession(conn ncryptConn, thumbprint keysource.Thumbprint, windowHandle uintptr) (keysource.Session, error) {
	der, key, callerFree, err := conn.findAndAcquire(string(thumbprint))
	if err != nil {
		return nil, err
	}
	// NCRYPT_SILENT_FLAG is never set on this key: silent mode fails
	// instead of prompting, which is the opposite of what a desktop
	// agent wants (F2 §2.3). NCRYPT_PIN_PROPERTY is never set either —
	// the agent does not collect, transport or store a PIN; the smart
	// card KSP shows the operating system's own dialog (SPEC §6.5).
	if err := conn.setWindowHandle(key, windowHandle); err != nil {
		// Non-fatal: the PIN dialog opens unparented rather than the
		// session failing to open (F2 §2.3).
		_ = err
	}
	return &session{
		conn:       conn,
		key:        key,
		callerFree: callerFree,
		cert:       keysource.Certificate{Thumbprint: thumbprint, DER: der},
	}, nil
}

// SignDigest implements keysource.Session.
func (s *session) SignDigest(ctx context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.closed {
		return nil, fmt.Errorf("windowscng: session is closed")
	}
	// Validated before any backend call (F2 §2.2): a wrong-length digest
	// reaches NCryptSignHash as NTE_INVALID_PARAMETER with no useful
	// message otherwise.
	if err := validateDigest(alg, digest); err != nil {
		return nil, err
	}
	return s.conn.signHash(s.key, digest)
}

// Certificate implements keysource.Session.
func (s *session) Certificate() keysource.Certificate { return s.cert }

// Chain implements keysource.Session. It returns no certificates: SPEC
// §11.6 makes chain completion the caller's job, and in this phase
// there is no document to embed a chain into (that arrives in F3).
func (s *session) Chain() [][]byte { return nil }

// Close implements keysource.Session.
func (s *session) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	if !s.callerFree {
		// fCallerFree was false: Windows caches this handle. Freeing it
		// here would corrupt the cache for every other process using
		// the same key (F2 §2.1) — so it is deliberately leaked from
		// this session's point of view, not actually leaked from the
		// process's, since Windows owns and reuses it.
		return nil
	}
	return s.conn.freeKey(s.key)
}

// validateDigest checks digest's length against alg before any backend
// call (F2 §2.2).
func validateDigest(alg keysource.DigestAlgorithm, digest []byte) error {
	size := alg.Size()
	if size == 0 {
		return errs.New(errs.CodeSignFailed, fmt.Errorf("unsupported digest algorithm %v", alg))
	}
	if len(digest) != size {
		return errs.New(errs.CodeSignFailed, fmt.Errorf("digest length %d, want %d for %v", len(digest), size, alg))
	}
	return nil
}
