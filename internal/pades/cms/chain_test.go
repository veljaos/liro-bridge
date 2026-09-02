package cms

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// buildChainCert creates a certificate signed by issuer (or self-signed
// if issuer is nil), optionally carrying AIA caIssuers URLs.
func buildChainCert(t *testing.T, cn string, issuer *x509.Certificate, issuerKey *rsa.PrivateKey, aiaURLs []string) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		IssuingCertificateURL: aiaURLs,
	}
	parent := tmpl
	signerKey := key
	if issuer != nil {
		parent = issuer
		signerKey = issuerKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatalf("CreateCertificate(%s): %v", cn, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(%s): %v", cn, err)
	}
	return cert, key
}

func TestCompleteChainFromTrustStoreExcludesRoot(t *testing.T) {
	root, rootKey := buildChainCert(t, "Root CA", nil, nil, nil)
	intermediate, intermediateKey := buildChainCert(t, "Intermediate CA", root, rootKey, nil)
	signer, _ := buildChainCert(t, "Signer", intermediate, intermediateKey, nil)

	chain := CompleteChain(context.Background(), signer, []*x509.Certificate{intermediate, root}, nil)
	if len(chain) != 1 {
		t.Fatalf("chain has %d certificates, want 1 (intermediate only, root excluded)", len(chain))
	}
	if chain[0].Subject.CommonName != "Intermediate CA" {
		t.Fatalf("chain[0] = %q, want Intermediate CA", chain[0].Subject.CommonName)
	}
}

func TestCompleteChainFallsBackToAIA(t *testing.T) {
	root, rootKey := buildChainCert(t, "Root CA", nil, nil, nil)
	intermediate, intermediateKey := buildChainCert(t, "Intermediate CA", root, rootKey, nil)
	signer, _ := buildChainCert(t, "Signer", intermediate, intermediateKey, []string{"http://ca.example.rs/intermediate.crt"})

	fetch := func(_ context.Context, url string) ([]byte, error) {
		if url != "http://ca.example.rs/intermediate.crt" {
			t.Fatalf("unexpected AIA URL %q", url)
		}
		return intermediate.Raw, nil
	}

	// Empty trust store forces the AIA path.
	chain := CompleteChain(context.Background(), signer, nil, fetch)
	if len(chain) != 1 || chain[0].Subject.CommonName != "Intermediate CA" {
		t.Fatalf("chain = %#v, want [Intermediate CA] via AIA", chain)
	}
}

// TestCompleteChainToleratesHalcomAIADefect reproduces SPEC §11.8's
// Halcom defect: caIssuers points at a .crl file, not a certificate.
// CompleteChain must not crash and must degrade to no chain rather than
// embedding garbage.
func TestCompleteChainToleratesHalcomAIADefect(t *testing.T) {
	root, rootKey := buildChainCert(t, "Root CA", nil, nil, nil)
	signer, _ := buildChainCert(t, "Signer", root, rootKey, []string{"http://ca.example.rs/not-a-cert.crl"})

	fetch := func(_ context.Context, url string) ([]byte, error) {
		return []byte("this is a CRL, not a certificate"), nil
	}

	chain := CompleteChain(context.Background(), signer, nil, fetch)
	if len(chain) != 0 {
		t.Fatalf("chain = %#v, want empty (unparseable AIA content must be discarded, not embedded)", chain)
	}
}

func TestCompleteChainSelfSignedSignerYieldsNoChain(t *testing.T) {
	selfSigned, _ := buildChainCert(t, "Self-Signed", nil, nil, nil)
	chain := CompleteChain(context.Background(), selfSigned, nil, nil)
	if len(chain) != 0 {
		t.Fatalf("chain = %#v, want empty for a self-signed certificate", chain)
	}
}
