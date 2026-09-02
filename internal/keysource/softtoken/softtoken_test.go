//go:build softtoken

package softtoken

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"software.sslmate.com/src/go-pkcs12"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

// generateP12 builds a throwaway self-signed RSA-2048 certificate with
// contentCommitment set (F2 §3.3) and writes it, PKCS#12-encoded, to a
// temporary file. Mirrors what scripts/gentestkeys does for real use,
// but inline so this test needs no external binary.
func generateP12(t *testing.T, password string) (path string, key *rsa.PrivateKey, cert *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Liro Bridge Test Signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageContentCommitment,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	p12, err := pkcs12.Modern.Encode(key, cert, nil, password)
	if err != nil {
		t.Fatalf("pkcs12 Encode: %v", err)
	}
	path = filepath.Join(t.TempDir(), "test.p12")
	if err := os.WriteFile(path, p12, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path, key, cert
}

func setEnv(t *testing.T, p12Path, password string) {
	t.Helper()
	t.Setenv(envP12, p12Path)
	t.Setenv(envPassword, password)
}

func TestListReturnsNoneWithoutEnvVar(t *testing.T) {
	t.Setenv(envP12, "")
	certs, err := (Source{}).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v, want nil (unconfigured, like a machine with no reader)", err)
	}
	if certs != nil {
		t.Fatalf("List() = %v, want nil", certs)
	}
}

func TestListReturnsOneCertificateMarkedAsTestKey(t *testing.T) {
	p12Path, _, cert := generateP12(t, "secret")
	setEnv(t, p12Path, "secret")

	certs, err := (Source{}).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(certs) != 1 {
		t.Fatalf("len(certs) = %d, want 1", len(certs))
	}
	if !certs[0].IsTestKey {
		t.Fatal("a soft-token certificate must have IsTestKey == true (SPEC §16.6)")
	}
	if string(certs[0].DER) != string(cert.Raw) {
		t.Fatal("DER does not match the generated certificate")
	}
}

func TestOpenAndSignDigestProducesVerifiableSignature(t *testing.T) {
	p12Path, _, cert := generateP12(t, "secret")
	setEnv(t, p12Path, "secret")

	certs, err := (Source{}).List(context.Background())
	if err != nil || len(certs) != 1 {
		t.Fatalf("List: %v", err)
	}

	sess, err := (Source{}).Open(context.Background(), certs[0].Thumbprint)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	digest := sha256.Sum256([]byte("liro-bridge F2 soft token test"))
	sig, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, digest[:])
	if err != nil {
		t.Fatalf("SignDigest: %v", err)
	}
	// Verify against the certificate's own public key, exactly what
	// sign-digest's external OpenSSL verification does (F2 §6.1) — this
	// proves the padding and digest algorithm are wired correctly,
	// independent of OpenSSL.
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatal("certificate does not carry an RSA public key")
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("rsa.VerifyPKCS1v15: %v", err)
	}
	if !sess.Certificate().IsTestKey {
		t.Fatal("session's own certificate must be marked IsTestKey")
	}
}

func TestSignDigestRejectsWrongLength(t *testing.T) {
	p12Path, _, _ := generateP12(t, "secret")
	setEnv(t, p12Path, "secret")

	certs, err := (Source{}).List(context.Background())
	if err != nil || len(certs) != 1 {
		t.Fatalf("List: %v", err)
	}
	sess, err := (Source{}).Open(context.Background(), certs[0].Thumbprint)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = sess.Close() }()

	_, err = sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 31))
	if err == nil {
		t.Fatal("SignDigest with a 31-byte digest must fail")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeSignFailed {
		t.Fatalf("error = %v, want SIGN_FAILED", err)
	}
}

func TestOpenRejectsWrongThumbprint(t *testing.T) {
	p12Path, _, _ := generateP12(t, "secret")
	setEnv(t, p12Path, "secret")

	_, err := (Source{}).Open(context.Background(), keysource.Thumbprint("0000000000000000000000000000000000AAAA"))
	if err == nil {
		t.Fatal("Open with a mismatched thumbprint must fail")
	}
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeCertNotFound {
		t.Fatalf("error = %v, want CERT_NOT_FOUND", err)
	}
}

func TestOpenRejectsWrongPassword(t *testing.T) {
	p12Path, _, cert := generateP12(t, "secret")
	setEnv(t, p12Path, "not-the-password")

	_, err := (Source{}).Open(context.Background(), thumbprint(cert.Raw))
	if err == nil {
		t.Fatal("Open with the wrong password must fail")
	}
}

func TestSignDigestFailsAfterClose(t *testing.T) {
	p12Path, _, _ := generateP12(t, "secret")
	setEnv(t, p12Path, "secret")
	certs, err := (Source{}).List(context.Background())
	if err != nil || len(certs) != 1 {
		t.Fatalf("List: %v", err)
	}
	sess, err := (Source{}).Open(context.Background(), certs[0].Thumbprint)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := sess.SignDigest(context.Background(), keysource.DigestSHA256, make([]byte, 32)); err == nil {
		t.Fatal("SignDigest after Close must fail")
	}
}
