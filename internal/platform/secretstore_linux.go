//go:build linux

package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// The Linux secret store: the file branch of SPEC §6.4.
//
// §6.4 names two mechanisms for this platform and prefers the first:
// "Secret Service (libsecret) where available, otherwise an encrypted
// file with a key derived from machine-id + user, with a clear warning
// in the log". **Only the second is built**, so the selection below
// takes it every time and says so — the warning is §6.4's own
// requirement rather than decoration, and it is the thing that stops a
// staged implementation from looking like a finished one (D-343).
//
// # Why a file is defensible at all, since it looks weaker than a keyring
//
// F12 §7 asks for the reasoning rather than the choice: **an unlocked
// keyring does not protect against malware running as the same user
// either.** Both mechanisms protect the same thing — another user of
// the machine, and a copy of the file taken elsewhere — and neither
// protects against code already running as the person whose secret it
// is. That is the same boundary SPEC §6.5 draws for the PIN, and it is
// why the consent screen rather than storage is this product's gate.
//
// What the key binds to, and what each one costs an attacker:
//
//   - **the machine**, through /etc/machine-id: the blob does not
//     decrypt on another computer;
//   - **the user**, through the numeric uid: it does not decrypt in
//     another account on this computer;
//   - **this installation**, through the entropy file beside it, which
//     is SPEC §6.4's "additional entropy stored alongside" and is what
//     makes a copy of secrets.json on its own worth nothing.
func newSecretStore(dir string) (SecretStore, error) {
	slog.Warn("platform: storing secrets in an encrypted file rather than the desktop's secret service, " +
		"which is not implemented yet (SPEC §6.4)")
	return newFileSecretStore(dir, machineBoundProtector{})
}

// machineBoundProtector is AES-256-GCM under a key derived from the
// machine, the user and the entropy file.
type machineBoundProtector struct{}

// secretStoreKeyInfo is HKDF's info string: it names the program, the
// purpose and the version of this scheme, so that a future change to
// how secrets are encrypted produces a different key rather than a
// confusing plaintext.
const secretStoreKeyInfo = "liro-bridge secret store v1"

func (machineBoundProtector) Protect(plaintext, entropy []byte) ([]byte, error) {
	if len(plaintext) == 0 || len(entropy) == 0 {
		return nil, errors.New("platform: refusing to encrypt with an empty input")
	}
	aead, err := secretAEAD(entropy)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("platform: nonce: %w", err)
	}
	// nonce || ciphertext||tag, which is what Seal's append form
	// produces when it is given the nonce as the destination.
	return aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (machineBoundProtector) Unprotect(ciphertext, entropy []byte) ([]byte, error) {
	if len(ciphertext) == 0 || len(entropy) == 0 {
		return nil, errors.New("platform: refusing to decrypt an empty input")
	}
	aead, err := secretAEAD(entropy)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < aead.NonceSize() {
		return nil, errors.New("platform: the stored secret is too short to hold a nonce")
	}
	nonce, body := ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():]
	out, err := aead.Open(nil, nonce, body, nil)
	if err != nil {
		// Deliberately not "wrong key" or "corrupt": GCM cannot tell
		// those apart and neither can this. What a caller needs to
		// know is that the bytes did not come from this machine, this
		// account and this entropy file together.
		return nil, fmt.Errorf("platform: this secret was not written by this user on this machine: %w", err)
	}
	return out, nil
}

func secretAEAD(entropy []byte) (cipher.AEAD, error) {
	id, err := machineID()
	if err != nil {
		return nil, err
	}
	// The machine and the user are the salt; the entropy file is the
	// keying material. Either way round would derive the same strength,
	// and this way says which part is the secret.
	salt := []byte(id + "\x00" + strconv.Itoa(os.Getuid()))
	key, err := hkdf.Key(sha256.New, entropy, salt, secretStoreKeyInfo, 32)
	if err != nil {
		return nil, fmt.Errorf("platform: deriving the secret store key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("platform: secret store cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

// machineIDPaths are where systemd and D-Bus keep the machine's own
// identifier, in the order the specification for it gives.
var machineIDPaths = []string{"/etc/machine-id", "/var/lib/dbus/machine-id"}

// machineID reads this machine's identifier.
//
// **A missing one is an error rather than a default.** Falling back to
// a constant would produce a key that is the same on every machine,
// which is the one property this derivation exists to deny — and it
// would do it silently, on exactly the minimal systems where nobody is
// watching. A system with no machine-id gets no secret store, and the
// agent says so.
func machineID() (string, error) {
	var errs []string
	for _, p := range machineIDPaths {
		b, err := os.ReadFile(p)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if id := strings.TrimSpace(string(b)); id != "" {
			return id, nil
		}
		errs = append(errs, p+" is empty")
	}
	return "", fmt.Errorf("platform: no machine-id, so a secret cannot be bound to this machine: %s",
		strings.Join(errs, "; "))
}
