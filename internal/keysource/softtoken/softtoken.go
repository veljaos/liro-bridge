//go:build softtoken

// Package softtoken is a test-only keysource.Source backed by a
// PKCS#12 file (F2 §3). It exists so the entire signing pipeline — F2
// through F6 — can be developed and tested with no smart card at all.
//
// Every file in this package carries the "softtoken" build constraint
// (SPEC §16.6, F2 §3.1): a release build never passes this tag, so the
// package — and every symbol in it — is absent from the binary
// entirely. internal/keysource/softtoken/buildtag_test.go in this
// package, plus the CI step that builds without the tag and greps the
// resulting binary, are what turn "must be impossible to enable" into a
// check that actually fails if this ever regresses.
package softtoken

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // certificate thumbprint identifier, not a signature — see the identical note in internal/keysource/windowscng/thumbprint.go
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

// Environment variables the soft token is configured by — never the
// config file (F2 §3.2): a file a user can edit with a text editor must
// never be able to turn this on.
const (
	envP12      = "LIRO_SOFTTOKEN_P12"
	envPassword = "LIRO_SOFTTOKEN_PASSWORD"
)

// Source implements keysource.Source over a PKCS#12 file (F2 §3).
type Source struct{}

// NewSource returns the soft token Source. Its List and Open report an
// empty/not-found result until LIRO_SOFTTOKEN_P12 is set — there is no
// separate "enabled" switch beyond the environment variable's presence.
func NewSource() Source { return Source{} }

// Name implements keysource.Source.
func (Source) Name() string { return "softtoken" }

// configured reads the two environment variables this package may be
// configured by (F2 §3.2).
func configured() (p12Path, password string, ok bool) {
	p12Path = os.Getenv(envP12)
	if p12Path == "" {
		return "", "", false
	}
	return p12Path, os.Getenv(envPassword), true
}

// thumbprint computes the SHA-1 hash of the DER certificate, uppercase
// hex — the identifier used everywhere in the agent (SPEC §11).
// Duplicated rather than imported from internal/keysource/windowscng,
// which this package must not depend on (the two are independent
// keysource.Source implementations, not layered on each other).
func thumbprint(der []byte) keysource.Thumbprint {
	sum := sha1.Sum(der) //nolint:gosec
	return keysource.Thumbprint(strings.ToUpper(hex.EncodeToString(sum[:])))
}

// load reads and decodes the configured PKCS#12 file.
func load(p12Path, password string) (*rsa.PrivateKey, *x509.Certificate, error) {
	data, err := os.ReadFile(p12Path)
	if err != nil {
		return nil, nil, errs.New(errs.CodeCertNotFound, fmt.Errorf("reading %s: %w", p12Path, err))
	}
	rawKey, cert, err := pkcs12.Decode(data, password)
	if err != nil {
		return nil, nil, errs.New(errs.CodeCertNotFound, fmt.Errorf("decoding %s: %w", p12Path, err))
	}
	key, ok := rawKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errs.New(errs.CodeCertNotUsable, fmt.Errorf("%s does not contain an RSA private key", p12Path))
	}
	return key, cert, nil
}

// List implements keysource.Source. It returns a single certificate
// when the soft token is configured, none otherwise — there is no error
// for "not configured": an agent without LIRO_SOFTTOKEN_P12 set simply
// has no soft-token certificates, exactly like a machine with no smart
// card reader has none from windowscng.
func (Source) List(_ context.Context) ([]keysource.Certificate, error) {
	p12Path, password, ok := configured()
	if !ok {
		return nil, nil
	}
	_, cert, err := load(p12Path, password)
	if err != nil {
		return nil, err
	}
	return []keysource.Certificate{{
		Thumbprint: thumbprint(cert.Raw),
		DER:        cert.Raw,
		IsTestKey:  true, // SPEC §16.6: every soft-token signature is visibly marked as a test signature
	}}, nil
}

// Open implements keysource.Source. There is no PIN: SPEC §16.6/F2 §3.2
// says so explicitly, and the PKCS#12 password is not one — it protects
// a file on disk, not a hardware device, and never crosses into a
// signing API the way a card PIN would.
func (Source) Open(ctx context.Context, thumb keysource.Thumbprint) (keysource.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p12Path, password, ok := configured()
	if !ok {
		return nil, errs.New(errs.CodeCertNotFound, fmt.Errorf("softtoken: %s is not set", envP12))
	}
	key, cert, err := load(p12Path, password)
	if err != nil {
		return nil, err
	}
	if got := thumbprint(cert.Raw); got != thumb {
		return nil, errs.New(errs.CodeCertNotFound, fmt.Errorf("softtoken: no certificate with thumbprint %s", thumb))
	}
	return &session{
		key: key,
		cert: keysource.Certificate{
			Thumbprint: thumb,
			DER:        cert.Raw,
			IsTestKey:  true,
		},
	}, nil
}

// session implements keysource.Session over an in-memory RSA key. Not
// safe for concurrent use (SPEC §8.5), same as every other Session.
type session struct {
	key    *rsa.PrivateKey
	cert   keysource.Certificate
	closed bool
}

// SignDigest implements keysource.Session using rsa.SignPKCS1v15 (F2
// §3.2) — the same padding scheme the real card uses (BCRYPT_PAD_PKCS1,
// F2 §2.2), so a caller cannot tell the two apart from the signature
// shape alone.
func (s *session) SignDigest(ctx context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.closed {
		return nil, fmt.Errorf("softtoken: session is closed")
	}
	size := alg.Size()
	if size == 0 {
		return nil, errs.New(errs.CodeSignFailed, fmt.Errorf("unsupported digest algorithm %v", alg))
	}
	if len(digest) != size {
		return nil, errs.New(errs.CodeSignFailed, fmt.Errorf("digest length %d, want %d for %v", len(digest), size, alg))
	}
	var hash crypto.Hash
	switch alg {
	case keysource.DigestSHA256:
		hash = crypto.SHA256
	default:
		return nil, errs.New(errs.CodeSignFailed, fmt.Errorf("unsupported digest algorithm %v", alg))
	}
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, hash, digest)
	if err != nil {
		return nil, errs.New(errs.CodeSignFailed, err)
	}
	return sig, nil
}

// Certificate implements keysource.Session.
func (s *session) Certificate() keysource.Certificate { return s.cert }

// Chain implements keysource.Session. The soft token's certificate is
// self-signed test fixture data; there is no chain to complete.
func (s *session) Chain() [][]byte { return nil }

// Close implements keysource.Session. There is no hardware handle to
// release, but Close still marks the session unusable afterwards.
func (s *session) Close() error {
	s.closed = true
	return nil
}
