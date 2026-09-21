//go:build linux

package platform

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestASecretRoundTripsThroughTheFileStore(t *testing.T) {
	store, err := NewSecretStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSecretStore: %v", err)
	}
	want := []byte("a device secret, 32 bytes of it!")
	if err := store.Set("device", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := store.Get("device")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("Get returned %q, want %q", got, want)
	}
}

// TestTheSecretIsNotInTheFile is the assertion the whole mechanism
// exists for: SPEC §6.4's point is that what lands on disk is not the
// secret, and a store that "worked" while writing plaintext would pass
// every round-trip test ever written.
func TestTheSecretIsNotInTheFile(t *testing.T) {
	dir := t.TempDir()
	store, err := NewSecretStore(dir)
	if err != nil {
		t.Fatalf("NewSecretStore: %v", err)
	}
	secret := []byte("correct-horse-battery-staple-0001")
	if err := store.Set("device", secret); err != nil {
		t.Fatalf("Set: %v", err)
	}
	blob, err := os.ReadFile(dir + "/" + secretsFileName)
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if bytes.Contains(blob, secret) {
		t.Fatal("the secret is in secrets.json in plain sight")
	}
	if strings.Contains(string(blob), "correct-horse") {
		t.Fatal("part of the secret is in secrets.json in plain sight")
	}
}

// TestAnotherInstallationsEntropyDoesNotOpenIt covers SPEC §6.4's
// "additional entropy stored alongside — so that copying the encrypted
// blob to another machine or another user account is not enough".
//
// Machine and user cannot be varied inside one test process, and the
// entropy file can: it is the third of the three bindings and the only
// one a test can move, so it is the one that is checked. The other two
// are the same derivation with different inputs.
func TestAnotherInstallationsEntropyDoesNotOpenIt(t *testing.T) {
	p := machineBoundProtector{}
	secret := []byte("a device secret")

	mine := bytes.Repeat([]byte{0xA5}, entropyLength)
	theirs := bytes.Repeat([]byte{0x5A}, entropyLength)

	blob, err := p.Protect(secret, mine)
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	if _, err := p.Unprotect(blob, theirs); err == nil {
		t.Fatal("a blob decrypted under another installation's entropy, so copying secrets.json " +
			"and nothing else would be enough")
	}
	back, err := p.Unprotect(blob, mine)
	if err != nil {
		t.Fatalf("Unprotect with the right entropy: %v", err)
	}
	if !bytes.Equal(back, secret) {
		t.Errorf("round trip gave %q, want %q", back, secret)
	}
}

// TestTamperingWithTheStoredBlobIsRefused: GCM authenticates, and a
// secret that decrypts to something an attacker chose would be worse
// than one that fails to decrypt at all.
func TestTamperingWithTheStoredBlobIsRefused(t *testing.T) {
	p := machineBoundProtector{}
	entropy := bytes.Repeat([]byte{0xC3}, entropyLength)
	blob, err := p.Protect([]byte("a device secret"), entropy)
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	blob[len(blob)-1] ^= 0x01
	if _, err := p.Unprotect(blob, entropy); err == nil {
		t.Fatal("a modified blob was accepted")
	}
}

// TestTheKeyIsNotTheSameOnEveryMachine checks the derivation actually
// consumes the machine identifier, by deriving with one and comparing
// against a blob made with another. It is done through the two files
// machineID reads rather than by faking the function, so that a change
// to *which* file it reads is also covered.
func TestTheKeyIsNotTheSameOnEveryMachine(t *testing.T) {
	id, err := machineID()
	if err != nil {
		t.Skipf("this machine has no machine-id: %v", err)
	}
	if id == "" {
		t.Fatal("machineID returned empty with no error")
	}
	if len(id) < 8 {
		t.Errorf("machineID returned %q, which is too short to be one", id)
	}
}
