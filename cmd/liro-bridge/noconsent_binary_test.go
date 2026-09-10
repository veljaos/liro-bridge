package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// noConsentSymbol is the signing path that asks nobody. A release
// binary must not contain it — SPEC §18.2 — and this proves it by
// inspecting the binary rather than by trusting the build tag, exactly
// as F2 §7.3 and SPEC §16.6 are already proved for the soft token
// itself (D-031).
const noConsentSymbol = "internal/cli.RunSignWithoutConsent"

// softTokenPackage is the other thing that must be absent, checked here
// too so this one test answers the whole question "is this binary
// release-shaped".
const softTokenPackage = "internal/keysource/softtoken"

// TestTheNoConsentPathIsAbsentFromAReleaseBinary builds the agent both
// ways and reads the symbol table of each.
//
// Both directions are checked, for the reason D-031 gives for the soft
// token: a check that only looks for absence passes for the wrong
// reason the moment the build tag itself breaks — a typo turning
// `//go:build softtoken` into an ordinary comment would make the
// symbols vanish from the tagged build too, and the "absent" half would
// stay green forever.
func TestTheNoConsentPathIsAbsentFromAReleaseBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two binaries")
	}
	dir := t.TempDir()

	release := build(t, dir, "release", nil)
	tagged := build(t, dir, "tagged", []string{"softtoken"})

	releaseSyms := symbols(t, release)
	taggedSyms := symbols(t, tagged)

	for _, want := range []string{noConsentSymbol, softTokenPackage} {
		if strings.Contains(releaseSyms, want) {
			t.Errorf("a release-shaped binary contains %q", want)
		}
		if !strings.Contains(taggedSyms, want) {
			t.Errorf("%q is missing even from a binary built WITH the softtoken tag — this check's own methodology is broken", want)
		}
	}
}

func build(t *testing.T, dir, name string, tags []string) string {
	t.Helper()
	out := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	args := []string{"build", "-o", out}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, "./cmd/liro-bridge")
	cmd := exec.Command("go", args...)
	cmd.Dir = filepath.Join("..", "..")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, b)
	}
	return out
}

func symbols(t *testing.T, binary string) string {
	t.Helper()
	b, err := exec.Command("go", "tool", "nm", binary).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool nm %s: %v\n%s", binary, err, b)
	}
	return string(b)
}
