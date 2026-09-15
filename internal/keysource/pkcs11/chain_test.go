package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"testing"
	"time"
)

// issueCert builds one certificate signed by parent, or self-signed when
// parent is nil. Synthetic throughout: a real chain from a real card carries a
// real person's name and national identity number, which D-021 and D-038
// already established do not belong in this repository's fixtures.
func issueCert(t *testing.T, cn string, isCA bool, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) ([]byte, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  isCA,
		BasicConstraintsValid: true,
	}
	signerCert, signerKey := tmpl, key
	if parent != nil {
		signerCert, signerKey = parent, parentKey
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signerCert, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatalf("CreateCertificate(%s): %v", cn, err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate(%s): %v", cn, err)
	}
	return der, parsed, key
}

func TestIssuersForReturnsWhatTheTokenCarriesAndNothingElse(t *testing.T) {
	rootDER, root, rootKey := issueCert(t, "Test Root CA", true, nil, nil)
	interDER, inter, interKey := issueCert(t, "Test Intermediate CA", true, root, rootKey)
	leafDER, _, _ := issueCert(t, "Signer", false, inter, interKey)
	strangerDER, _, _ := issueCert(t, "Somebody Else's CA", true, nil, nil)

	t.Run("the whole chain is on the token", func(t *testing.T) {
		got := issuersFor(leafDER, [][]byte{leafDER, interDER, rootDER, strangerDER})
		if len(got) != 2 {
			t.Fatalf("got %d issuers, want 2 (intermediate then root)", len(got))
		}
		if !equalBytes(got[0], interDER) {
			t.Error("the first issuer is not the intermediate; the chain is not ordered upward")
		}
		if !equalBytes(got[1], rootDER) {
			t.Error("the second issuer is not the root")
		}
	})

	t.Run("only the intermediate is on the token", func(t *testing.T) {
		got := issuersFor(leafDER, [][]byte{leafDER, interDER})
		if len(got) != 1 || !equalBytes(got[0], interDER) {
			t.Fatalf("got %d issuers, want just the intermediate — the walk must stop "+
				"where the token stops rather than inventing the rest", len(got))
		}
	})

	// This is the case that actually happens. Measured on a MUP e-ID card
	// through both NetSeT modules: two certificates on the token, no CA among
	// them, and both issued by "MUP Gradjani CA 4", which is not there.
	t.Run("a Serbian card: the signer and its twin, and no issuer at all", func(t *testing.T) {
		twinDER, _, _ := issueCert(t, "Signer Auth", false, inter, interKey)
		got := issuersFor(leafDER, [][]byte{leafDER, twinDER})
		if len(got) != 0 {
			t.Fatalf("got %d issuers, want none — the token carries no CA, and a chain "+
				"this layer cannot supply must come back empty rather than guessed", len(got))
		}
	})

	t.Run("the signer is never its own chain", func(t *testing.T) {
		if got := issuersFor(rootDER, [][]byte{rootDER}); len(got) != 0 {
			t.Errorf("a self-signed certificate alone produced %d issuers, want none", len(got))
		}
	})

	t.Run("an unrelated CA is not adopted", func(t *testing.T) {
		if got := issuersFor(leafDER, [][]byte{leafDER, strangerDER}); len(got) != 0 {
			t.Errorf("got %d issuers from a token carrying only an unrelated CA; the match "+
				"must be an exact RawSubject/RawIssuer comparison, never a near one", len(got))
		}
	})

	t.Run("an object that is not a certificate is skipped", func(t *testing.T) {
		got := issuersFor(leafDER, [][]byte{leafDER, []byte("not a certificate"), interDER})
		if len(got) != 1 {
			t.Errorf("got %d issuers; unparseable token objects must be skipped rather "+
				"than ending the walk", len(got))
		}
	})

	t.Run("a signer that does not parse yields nothing", func(t *testing.T) {
		if got := issuersFor([]byte("not a certificate"), [][]byte{interDER}); got != nil {
			t.Error("issuersFor invented a chain for something that is not a certificate")
		}
	})
}

// TestIssuersForIsBounded guards the depth bound with a chain deeper than it.
// No real token carries one, and that is the point: the bound exists so that a
// token which presents something strange cannot make this walk spin.
func TestIssuersForIsBounded(t *testing.T) {
	const depth = maxChainDepth * 3

	rootDER, cert, key := issueCert(t, "CA 0", true, nil, nil)
	pool := [][]byte{rootDER}
	for i := 1; i <= depth; i++ {
		var der []byte
		der, cert, key = issueCert(t, fmt.Sprintf("CA %d", i), true, cert, key)
		pool = append(pool, der)
	}
	leafDER, _, _ := issueCert(t, "Signer", false, cert, key)
	pool = append(pool, leafDER)

	got := issuersFor(leafDER, pool)
	if len(got) > maxChainDepth {
		t.Errorf("the walk returned %d issuers from a %d-deep chain, more than the "+
			"%d-deep bound", len(got), depth, maxChainDepth)
	}
	if len(got) != maxChainDepth {
		t.Errorf("the walk returned %d issuers, want exactly the bound (%d) — it should "+
			"climb as far as it is allowed and then stop", len(got), maxChainDepth)
	}
}
