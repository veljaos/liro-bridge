package platform

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
)

// This is the one test that exercises DPAPI itself rather than the file
// store around it. Everything above it in secretstore_test.go runs on
// every platform with a stand-in protector; nothing there would notice
// if CryptProtectData were never called at all.
func TestDPAPIRoundTripOnThisMachine(t *testing.T) {
	entropy := make([]byte, entropyLength)
	if _, err := rand.Read(entropy); err != nil {
		t.Fatalf("rand: %v", err)
	}
	secret := []byte("a 32-byte device secret goes here")

	p := dpapiProtector{}
	blob, err := p.Protect(secret, entropy)
	if err != nil {
		t.Fatalf("CryptProtectData: %v", err)
	}
	if bytes.Contains(blob, secret) {
		t.Fatal("the DPAPI blob contains the plaintext verbatim")
	}

	got, err := p.Unprotect(blob, entropy)
	if err != nil {
		t.Fatalf("CryptUnprotectData: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("round trip returned %q, want %q", got, secret)
	}
}

// SPEC §6.4's additional entropy, checked against the real API rather
// than against the stand-in: the same user on the same machine cannot
// decrypt the blob without the entropy it was bound to.
func TestDPAPIRefusesTheWrongEntropy(t *testing.T) {
	right := make([]byte, entropyLength)
	wrong := make([]byte, entropyLength)
	if _, err := rand.Read(right); err != nil {
		t.Fatalf("rand: %v", err)
	}
	if _, err := rand.Read(wrong); err != nil {
		t.Fatalf("rand: %v", err)
	}

	p := dpapiProtector{}
	blob, err := p.Protect([]byte("the device secret"), right)
	if err != nil {
		t.Fatalf("CryptProtectData: %v", err)
	}
	if _, err := p.Unprotect(blob, wrong); err == nil {
		t.Fatal("CryptUnprotectData accepted the wrong entropy")
	}
}

// The real store, on the real platform, end to end.
func TestNewSecretStoreOnWindowsRoundTrips(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Liro")
	s, err := NewSecretStore(dir)
	if err != nil {
		t.Fatalf("NewSecretStore: %v", err)
	}
	secret := []byte("PLAINTEXT-DEVICE-SECRET-MARKER")
	if err := s.Set("pairing.abc", secret); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("pairing.abc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, secret) {
		t.Fatalf("Get returned %q", got)
	}
	raw, err := os.ReadFile(filepath.Join(dir, secretsFileName))
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if bytes.Contains(raw, secret) {
		t.Fatal("the real Windows secret store contains the plaintext secret verbatim")
	}
}
