package tsa

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These tests call Pošta Srbije's real public test TSA endpoints
// (F3 §6.2). They are integration tests, not unit tests: they need
// working network access and a TSA that is actually up. Both are
// outside this project's control, so a failure to reach the endpoint
// skips the test rather than failing the suite — the same pattern the
// testdata/*/local directories use for material this project cannot
// guarantee is present. When the endpoint is reachable, this is
// stronger evidence than any fixture this repository could ship: a real
// qualified TSA's real response, not a synthetic stand-in.

const (
	postaBasicEndpoint  = "https://test-tsa.ca.posta.rs/timestamp1"
	postaClientEndpoint = "https://test-tsa.ca.posta.rs/timestamp2"
)

func TestIntegrationPostaBasicAuthTSA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	digest := sha256.Sum256([]byte("liro-bridge F3 integration test " + time.Now().String()))

	c := NewClient(postaBasicEndpoint, Auth{BasicUsername: "Test.Korisnik", BasicPassword: "123456"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.Timestamp(ctx, digest[:])
	if err != nil {
		t.Skipf("Pošta test TSA (Basic auth) unreachable or unavailable, skipping: %v", err)
	}

	if len(resp.TokenDER) == 0 {
		t.Fatal("response has no TimeStampToken bytes")
	}
	if time.Since(resp.GenTime) > maxSkew || time.Since(resp.GenTime) < -maxSkew {
		t.Fatalf("genTime %s outside the skew window of now (%s)", resp.GenTime, time.Now())
	}
	if len(resp.SignerCertificates) == 0 {
		t.Error("certReq was true but the response embedded no certificates")
	}
	t.Logf("real TSA response: genTime=%s serialNumber=%s certificates=%d tokenBytes=%d",
		resp.GenTime, resp.SerialNumber, len(resp.SignerCertificates), len(resp.TokenDER))
}

// TestIntegrationPostaClientCertTSA exercises F3 §6.2's second
// authentication mode: a TLS client certificate supplied as a PFX with
// password 1234. That credential is Pošta's own test material, not
// published in this specification beyond "a PFX, password 1234", so it
// is supplied the same way real signed PDFs are (see
// testdata/pdfs/local/README.md): dropped locally, gitignored, and this
// test skips cleanly when it is absent.
func TestIntegrationPostaClientCertTSA(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network integration test in -short mode")
	}
	p12Path := filepath.Join("..", "..", "..", "testdata", "tsa", "local", "posta-client.p12")
	p12, err := os.ReadFile(p12Path)
	if err != nil {
		t.Skipf("no local client-certificate PFX at %s, skipping (see testdata/tsa/local/README.md): %v", p12Path, err)
	}
	cert, err := LoadPKCS12ClientCert(p12, "1234")
	if err != nil {
		t.Fatalf("LoadPKCS12ClientCert: %v", err)
	}

	digest := sha256.Sum256([]byte("liro-bridge F3 integration test client-cert " + time.Now().String()))
	c := NewClient(postaClientEndpoint, Auth{ClientCertificate: &cert})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := c.Timestamp(ctx, digest[:])
	if err != nil {
		t.Skipf("Pošta test TSA (client cert) unreachable or unavailable, skipping: %v", err)
	}
	if len(resp.TokenDER) == 0 {
		t.Fatal("response has no TimeStampToken bytes")
	}
	t.Logf("real TSA (client cert) response: genTime=%s serialNumber=%s", resp.GenTime, resp.SerialNumber)
}
