//go:build windows

package platform

import (
	"os"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

// windowsSigningFlag writes one value under HKCU. Per-user, like
// everything else this agent registers (autostart, the Explorer verb),
// so it needs no administrator rights — and per-user is also the right
// scope: two people signed in over RDP each have their own agent (SPEC
// §14.1), and one of them signing is not a reason to block the other's
// installer.
//
// keyPath is a field rather than a constant for the same reason
// windowsAutostart's is: so a test can point it at a key that does not
// exist yet, which is the case CreateKey-versus-OpenKey gets wrong and
// which cannot be exercised against the real key without deleting
// something.
type windowsSigningFlag struct{ keyPath string }

// NewSigningFlag returns the Windows implementation.
func NewSigningFlag() SigningFlag { return windowsSigningFlag{keyPath: SigningFlagKeyPath} }

func (f windowsSigningFlag) Begin() error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, f.keyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return k.SetStringValue(SigningFlagValueName, strconv.Itoa(os.Getpid()))
}

func (f windowsSigningFlag) End() error { return f.Clear() }

func (f windowsSigningFlag) Clear() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, f.keyPath, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return err
	}
	defer func() { _ = k.Close() }()
	if err := k.DeleteValue(SigningFlagValueName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

func (f windowsSigningFlag) Held() (bool, int, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, f.keyPath, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, 0, nil
		}
		return false, 0, err
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue(SigningFlagValueName)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, 0, nil
		}
		return false, 0, err
	}
	// A value that is present but not a number still means held: the
	// mark's presence is the signal and the pid is only ever
	// diagnostic. Reading it the other way round would let a garbled
	// value silently unblock an installer mid-batch.
	pid, _ := strconv.Atoi(v)
	return true, pid, nil
}
