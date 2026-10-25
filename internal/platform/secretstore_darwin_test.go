//go:build darwin && cgo

package platform

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"
)

// The Keychain store, on CI's macOS runner: one item stored, read back,
// replaced, deleted, and its absence reported as ErrSecretNotFound — under
// a service of the test's own, so a developer's real items are never
// touched.
func TestTheKeychainStoreRoundTrips(t *testing.T) {
	k := keychainStore{service: "liro-bridge-test"}
	name := fmt.Sprintf("roundtrip-%d", time.Now().UnixNano())
	t.Cleanup(func() { _ = k.Delete(name) })

	if _, err := k.Get(name); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Get before Set: %v, want ErrSecretNotFound", err)
	}
	for _, v := range [][]byte{[]byte("first secret"), []byte("second, longer secret")} {
		if err := k.Set(name, v); err != nil {
			t.Fatalf("Set: %v", err)
		}
		got, err := k.Get(name)
		if err != nil || !bytes.Equal(got, v) {
			t.Fatalf("Get = %q, %v; want %q", got, err, v)
		}
	}
	if err := k.Set(name, nil); !errors.Is(err, errEmptySecret) {
		t.Fatalf("Set(empty) = %v, want errEmptySecret", err)
	}
	if err := k.Delete(name); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := k.Delete(name); err != nil {
		t.Fatalf("Delete of what is gone: %v, want nil", err)
	}
	if _, err := k.Get(name); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Get after Delete: %v, want ErrSecretNotFound", err)
	}
	if d := k.Describe(); d.Mechanism != MechanismKeychain || d.Fallback {
		t.Fatalf("Describe = %+v", d)
	}
}
