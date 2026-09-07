//go:build !windows

package platform

// newSecretStore has no implementation off Windows yet. SPEC §6.4 names
// the mechanism for each platform — the Keychain on macOS (phase 12),
// the Secret Service or a key derived from machine-id and user on Linux
// (phase 13) — and F7 is explicitly Windows-only.
//
// It refuses rather than falling back to an unencrypted file: a device
// secret in plain JSON is the one outcome §6.4 exists to prevent, and a
// fallback nobody notices is how it would arrive.
func newSecretStore(dir string) (SecretStore, error) {
	_ = dir
	return nil, ErrSecretStoreUnsupported
}
