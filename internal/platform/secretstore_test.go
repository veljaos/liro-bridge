package platform

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// xorProtector is a stand-in for the OS facility, so every property of
// the file-backed store — the format, the entropy file's lifecycle, the
// atomic write, the not-found behaviour — is exercised on every
// platform this project builds for, not only on the one that has a real
// protector. It is deliberately not cryptography: what it has to model
// is "this value cannot be read back without the same entropy", which
// is the only property the store itself depends on.
type xorProtector struct{}

func (xorProtector) transform(b, entropy []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[i] ^ entropy[i%len(entropy)]
	}
	return out
}

func (p xorProtector) Protect(plaintext, entropy []byte) ([]byte, error) {
	return p.transform(plaintext, entropy), nil
}

func (p xorProtector) Unprotect(ciphertext, entropy []byte) ([]byte, error) {
	return p.transform(ciphertext, entropy), nil
}

func newTestStore(t *testing.T) (*fileSecretStore, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := newFileSecretStore(dir, xorProtector{})
	if err != nil {
		t.Fatalf("newFileSecretStore: %v", err)
	}
	return s, dir
}

func TestSecretStoreRoundTrip(t *testing.T) {
	s, _ := newTestStore(t)

	want := []byte("thirty-two bytes of device secret")
	if err := s.Set("pairing.abc", want); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("pairing.abc")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get returned %q, want %q", got, want)
	}
}

func TestSecretStoreMissingSecretIsNotFound(t *testing.T) {
	s, _ := newTestStore(t)

	_, err := s.Get("nothing.here")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Get of an absent secret returned %v, want ErrSecretNotFound", err)
	}
}

func TestSecretStoreDeleteRemovesTheSecretAndIsIdempotent(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.Set("pairing.abc", []byte("secret")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Delete("pairing.abc"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("pairing.abc"); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Get after Delete returned %v, want ErrSecretNotFound", err)
	}
	// Deleting again is not an error: the caller's intent — there is no
	// such secret afterwards — is satisfied either way.
	if err := s.Delete("pairing.abc"); err != nil {
		t.Fatalf("Delete of an absent secret: %v", err)
	}
}

func TestSecretStoreRefusesAnEmptyValue(t *testing.T) {
	s, _ := newTestStore(t)

	if err := s.Set("pairing.abc", nil); !errors.Is(err, errEmptySecret) {
		t.Fatalf("Set of an empty value returned %v, want errEmptySecret", err)
	}
}

// The whole point of SPEC §6.4's additional entropy: the encrypted blob
// on its own is not enough. Copying secrets.json somewhere the entropy
// file is different must not decrypt.
func TestSecretStoreBlobAloneDoesNotDecryptWithDifferentEntropy(t *testing.T) {
	source, sourceDir := newTestStore(t)
	if err := source.Set("pairing.abc", []byte("the device secret")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// A second store, with its own entropy, receiving a copy of the
	// first one's secrets.json and nothing else.
	target, targetDir := newTestStore(t)
	if err := target.Set("unrelated", []byte("something")); err != nil {
		t.Fatalf("Set on the target store: %v", err)
	}
	blob, err := os.ReadFile(filepath.Join(sourceDir, secretsFileName))
	if err != nil {
		t.Fatalf("reading the source store: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, secretsFileName), blob, 0o600); err != nil {
		t.Fatalf("copying the source store: %v", err)
	}

	got, err := target.Get("pairing.abc")
	if err == nil && bytes.Equal(got, []byte("the device secret")) {
		t.Fatal("a secrets file copied without its entropy file decrypted anyway; " +
			"SPEC §6.4's additional entropy is not doing anything")
	}
}

func TestSecretStoreWritesTheEntropyFileOnceAndReusesIt(t *testing.T) {
	s, dir := newTestStore(t)

	if err := s.Set("first", []byte("a")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(dir, entropyFileName))
	if err != nil {
		t.Fatalf("reading the entropy file: %v", err)
	}
	if len(before) != entropyLength {
		t.Fatalf("entropy file is %d bytes, want %d", len(before), entropyLength)
	}

	if err := s.Set("second", []byte("b")); err != nil {
		t.Fatalf("Set: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, entropyFileName))
	if err != nil {
		t.Fatalf("re-reading the entropy file: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the entropy file changed between two writes; every secret stored " +
			"before the change would have become undecryptable")
	}
}

// A store re-opened over the same directory reads what the previous one
// wrote — which is the whole reason the entropy lives in a file rather
// than in memory.
func TestSecretStoreSurvivesReopening(t *testing.T) {
	dir := t.TempDir()
	first, err := newFileSecretStore(dir, xorProtector{})
	if err != nil {
		t.Fatalf("newFileSecretStore: %v", err)
	}
	if err := first.Set("pairing.abc", []byte("the device secret")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	second, err := newFileSecretStore(dir, xorProtector{})
	if err != nil {
		t.Fatalf("re-opening: %v", err)
	}
	got, err := second.Get("pairing.abc")
	if err != nil {
		t.Fatalf("Get after re-opening: %v", err)
	}
	if string(got) != "the device secret" {
		t.Fatalf("Get after re-opening returned %q", got)
	}
}

// The stored form must be unreadable by inspection: a device secret
// that survives a grep of the store file is not encrypted at rest.
func TestSecretStoreFileDoesNotContainThePlaintext(t *testing.T) {
	s, dir := newTestStore(t)

	secret := []byte("PLAINTEXT-DEVICE-SECRET-MARKER")
	if err := s.Set("pairing.abc", secret); err != nil {
		t.Fatalf("Set: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, secretsFileName))
	if err != nil {
		t.Fatalf("reading the store: %v", err)
	}
	if bytes.Contains(raw, secret) {
		t.Fatalf("the secret store contains the plaintext secret verbatim:\n%s", raw)
	}

	// And what it does contain is the protected blob, base64-encoded,
	// under the name it was stored with — the format this code claims.
	var f secretsFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("the secret store is not valid JSON: %v", err)
	}
	if f.Version != secretsFileVersion {
		t.Fatalf("secrets.json version is %d, want %d", f.Version, secretsFileVersion)
	}
	encoded, ok := f.Secrets["pairing.abc"]
	if !ok {
		t.Fatalf("secrets.json has no entry for the stored name: %+v", f.Secrets)
	}
	if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
		t.Fatalf("the stored value is not base64: %v", err)
	}
}

func TestSecretStoreRejectsATruncatedEntropyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, entropyFileName), []byte("short"), 0o600); err != nil {
		t.Fatalf("writing a truncated entropy file: %v", err)
	}
	s, err := newFileSecretStore(dir, xorProtector{})
	if err != nil {
		t.Fatalf("newFileSecretStore: %v", err)
	}
	// Replacing it silently would make every already-stored secret
	// undecryptable while looking like a fresh start.
	if err := s.Set("pairing.abc", []byte("x")); err == nil {
		t.Fatal("Set accepted a truncated entropy file instead of refusing")
	}
}
