package platform

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// SecretStore keeps small secrets encrypted at rest using the operating
// system's own facilities (SPEC §6.4). The rest of the agent calls Get,
// Set and Delete and knows nothing about DPAPI, the Keychain or the
// Secret Service.
//
// The only secret this project stores today is a paired application's
// 32-byte device secret (F7 §2.1), one entry per pairing.
type SecretStore interface {
	// Get returns the secret stored under name, or ErrSecretNotFound if
	// there is none.
	Get(name string) ([]byte, error)

	// Set stores value under name, replacing whatever was there.
	// An empty value is rejected: see errEmptySecret.
	Set(name string, value []byte) error

	// Delete removes the secret stored under name. Deleting a secret
	// that is not there is not an error — the caller's intent (there is
	// no such secret afterwards) is satisfied either way.
	Delete(name string) error
}

// ErrSecretNotFound is returned by SecretStore.Get when name has no
// secret. Callers branch on it with errors.Is.
var ErrSecretNotFound = errors.New("platform: no such secret")

// ErrSecretStoreUnsupported is returned by NewSecretStore on a platform
// with no implementation yet. macOS (Keychain) is phase 12 and Linux
// (Secret Service) is phase 13; SPEC §6.4 already names the mechanism
// for each. Returning an error rather than silently falling back to an
// unencrypted file is deliberate — a device secret sitting in plain
// JSON on disk is exactly what §6.4 exists to prevent, and a fallback
// nobody notices is how it would get there.
var ErrSecretStoreUnsupported = errors.New("platform: encrypted secret storage is not implemented on this platform")

// errEmptySecret is returned by Set for a zero-length value. It is not
// a limitation worth working around: an empty secret is either a bug at
// the call site or a way of writing "delete", and Delete already says
// that unambiguously.
var errEmptySecret = errors.New("platform: refusing to store an empty secret")

// NewSecretStore opens (creating it if necessary) the secret store kept
// in dir — normally the agent's own per-user configuration directory,
// so that two users on one machine have two stores (SPEC §14.1).
func NewSecretStore(dir string) (SecretStore, error) { return newSecretStore(dir) }

// secretsFileName and entropyFileName are the two files a file-backed
// secret store keeps side by side in its directory.
//
// They are separate files on purpose. SPEC §6.4 requires "additional
// entropy stored alongside — so that copying the encrypted blob to
// another machine or another user account is not enough": the entropy
// is the second input DPAPI needs, and a blob lifted out of
// secrets.json on its own decrypts to nothing anywhere.
const (
	secretsFileName = "secrets.json"
	entropyFileName = "secrets.entropy"
)

// entropyLength is how many random bytes the additional-entropy file
// holds. 32 is the same size as the secrets it protects and the same
// size as every other random value this protocol generates.
const entropyLength = 32

// protector encrypts and decrypts one value using an OS facility. It is
// the whole of what differs between platforms; everything else about a
// file-backed secret store — the file format, the atomic write, the
// entropy file's lifecycle — is shared, and is therefore testable on
// any platform with a fake protector (secretstore_test.go).
type protector interface {
	// Protect encrypts plaintext, binding it to the current user
	// account and to entropy.
	Protect(plaintext, entropy []byte) ([]byte, error)

	// Unprotect reverses Protect. It fails if the blob was produced by
	// another user, on another machine, or with different entropy.
	Unprotect(ciphertext, entropy []byte) ([]byte, error)
}

// secretsFile is the on-disk shape of secrets.json: a version, so a
// future format change is recognisable rather than a parse error, and a
// name -> base64(protected blob) map.
type secretsFile struct {
	Version int               `json:"version"`
	Secrets map[string]string `json:"secrets"`
}

// secretsFileVersion is the only version this code writes or reads.
const secretsFileVersion = 1

// fileSecretStore is a SecretStore backed by two files in one
// directory: the protected values, and the additional entropy every one
// of them is bound to.
type fileSecretStore struct {
	mu        sync.Mutex
	dir       string
	protect   protector
	entropy   []byte
	entropyOK bool
}

// newFileSecretStore returns a store over dir using p. The directory is
// created if it does not exist; nothing is read or written until the
// first Get, Set or Delete, so constructing a store never fails for a
// reason the caller cannot yet act on.
func newFileSecretStore(dir string, p protector) (*fileSecretStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("platform: creating the secret store directory: %w", err)
	}
	return &fileSecretStore{dir: dir, protect: p}, nil
}

// loadEntropy reads the additional-entropy file, generating it on first
// use. Held under the caller's lock.
func (s *fileSecretStore) loadEntropy() ([]byte, error) {
	if s.entropyOK {
		return s.entropy, nil
	}
	path := filepath.Join(s.dir, entropyFileName)
	b, err := os.ReadFile(path)
	switch {
	case err == nil && len(b) == entropyLength:
		s.entropy, s.entropyOK = b, true
		return s.entropy, nil
	case err == nil:
		// A truncated or overwritten entropy file cannot be repaired:
		// every blob in secrets.json is bound to the bytes that used to
		// be here, so replacing it would silently make every stored
		// secret undecryptable while looking like a fresh start. Say so
		// instead.
		return nil, fmt.Errorf("platform: the secret store's entropy file is %d bytes, expected %d: %s",
			len(b), entropyLength, path)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("platform: reading the secret store's entropy file: %w", err)
	}

	fresh := make([]byte, entropyLength)
	if _, err := rand.Read(fresh); err != nil {
		return nil, fmt.Errorf("platform: generating secret store entropy: %w", err)
	}
	if err := WriteFileAtomic(path, fresh, 0o600); err != nil {
		return nil, fmt.Errorf("platform: writing the secret store's entropy file: %w", err)
	}
	s.entropy, s.entropyOK = fresh, true
	return s.entropy, nil
}

// readFile returns the parsed secrets.json, or an empty one when the
// file does not exist yet. Held under the caller's lock.
func (s *fileSecretStore) readFile() (secretsFile, error) {
	out := secretsFile{Version: secretsFileVersion, Secrets: map[string]string{}}
	b, err := os.ReadFile(filepath.Join(s.dir, secretsFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("platform: reading the secret store: %w", err)
	}
	var parsed secretsFile
	if err := json.Unmarshal(b, &parsed); err != nil {
		return out, fmt.Errorf("platform: the secret store is not valid JSON: %w", err)
	}
	if parsed.Version != secretsFileVersion {
		return out, fmt.Errorf("platform: the secret store is version %d, this build understands %d",
			parsed.Version, secretsFileVersion)
	}
	if parsed.Secrets == nil {
		parsed.Secrets = map[string]string{}
	}
	return parsed, nil
}

// writeFile replaces secrets.json with f, atomically. Held under the
// caller's lock.
func (s *fileSecretStore) writeFile(f secretsFile) error {
	f.Version = secretsFileVersion
	// Sorted keys, so two runs that store the same secrets produce the
	// same bytes and a diff of this file is readable.
	names := make([]string, 0, len(f.Secrets))
	for name := range f.Secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	ordered := make(map[string]string, len(names))
	for _, name := range names {
		ordered[name] = f.Secrets[name]
	}
	f.Secrets = ordered

	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := WriteFileAtomic(filepath.Join(s.dir, secretsFileName), b, 0o600); err != nil {
		return fmt.Errorf("platform: writing the secret store: %w", err)
	}
	return nil
}

// Get implements SecretStore.
func (s *fileSecretStore) Get(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.readFile()
	if err != nil {
		return nil, err
	}
	encoded, ok := f.Secrets[name]
	if !ok {
		return nil, ErrSecretNotFound
	}
	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("platform: the stored secret is not valid base64: %w", err)
	}
	entropy, err := s.loadEntropy()
	if err != nil {
		return nil, err
	}
	value, err := s.protect.Unprotect(blob, entropy)
	if err != nil {
		return nil, fmt.Errorf("platform: decrypting a stored secret: %w", err)
	}
	return value, nil
}

// Set implements SecretStore.
func (s *fileSecretStore) Set(name string, value []byte) error {
	if len(value) == 0 {
		return errEmptySecret
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entropy, err := s.loadEntropy()
	if err != nil {
		return err
	}
	blob, err := s.protect.Protect(value, entropy)
	if err != nil {
		return fmt.Errorf("platform: encrypting a secret: %w", err)
	}
	f, err := s.readFile()
	if err != nil {
		return err
	}
	f.Secrets[name] = base64.StdEncoding.EncodeToString(blob)
	return s.writeFile(f)
}

// Delete implements SecretStore.
func (s *fileSecretStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.readFile()
	if err != nil {
		return err
	}
	if _, ok := f.Secrets[name]; !ok {
		return nil
	}
	delete(f.Secrets, name)
	return s.writeFile(f)
}
