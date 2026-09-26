//go:build linux

package pkcs11

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOneLibraryBehindTwoNamesIsOneCandidate is D-360: SafeSign's module is
// reached as libaetpkss.so and libaetpkss.so.3, both symlinks to one file,
// and discovery loaded it twice. The layout here is SafeSign's, in a
// directory the test owns.
func TestOneLibraryBehindTwoNamesIsOneCandidate(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "libvendor.so.3.9.33.1")
	if err := os.WriteFile(real, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"libvendor.so", "libvendor.so.3"} {
		if err := os.Symlink("libvendor.so.3.9.33.1", filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	a := candidateKey(filepath.Join(dir, "libvendor.so"))
	b := candidateKey(filepath.Join(dir, "libvendor.so.3"))
	if a != b {
		t.Fatalf("two names for one file are two candidates: %q and %q", a, b)
	}

	other := filepath.Join(dir, "libother.so")
	if err := os.WriteFile(other, []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if candidateKey(other) == a {
		t.Fatal("two different files became one candidate; NetSeT's two builds would lose one")
	}

	missing := filepath.Join(dir, "typo.so")
	if candidateKey(missing) != strings.ToLower(filepath.Clean(missing)) {
		t.Errorf("a path that does not exist keys as %q, want its own name", candidateKey(missing))
	}
}
