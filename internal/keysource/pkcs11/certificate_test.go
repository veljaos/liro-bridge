package pkcs11

import (
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

func TestThumbprintIsUppercaseHexSHA1(t *testing.T) {
	// sha1 of no bytes, which is a published value and needs no fixture.
	if got, want := thumbprint(nil), "DA39A3EE5E6B4B0D3255BFEF95601890AFD80709"; got != want {
		t.Errorf("thumbprint(nil) = %q, want %q", got, want)
	}
	got := thumbprint([]byte("liro"))
	if len(got) != 40 {
		t.Errorf("thumbprint returned %d characters, want 40", len(got))
	}
	if got != strings.ToUpper(got) {
		t.Errorf("thumbprint returned %q, which is not uppercase — the whole agent "+
			"compares these as strings, and the CNG backend produces uppercase", got)
	}
}

func TestDedupeCollapsesOneCertificateSeenTwice(t *testing.T) {
	// One card, two slots of one module: the same certificate twice.
	found := []CertificateInfo{
		{Thumbprint: "AAAA", DER: []byte("first"), Label: "Sign", SlotID: 0},
		{Thumbprint: "BBBB", DER: []byte("twin"), Label: "Auth", SlotID: 0},
		{Thumbprint: "AAAA", DER: []byte("first"), Label: "Sign", SlotID: 1},
	}
	got := dedupe(found)
	if len(got) != 2 {
		t.Fatalf("got %d rows from 3 sightings of 2 certificates, want 2", len(got))
	}
	if got[0].Thumbprint != keysource.Thumbprint("AAAA") || got[1].Thumbprint != keysource.Thumbprint("BBBB") {
		t.Errorf("got %v; the first sighting of each certificate should win, in order", got)
	}
	if string(got[0].DER) != "first" {
		t.Errorf("the surviving row carries %q, want the DER of the first sighting", got[0].DER)
	}
}

func TestDedupeDropsARowWithNoThumbprint(t *testing.T) {
	got := dedupe([]CertificateInfo{{Thumbprint: "", DER: []byte("x")}, {Thumbprint: "AAAA", DER: []byte("y")}})
	if len(got) != 1 || got[0].Thumbprint != keysource.Thumbprint("AAAA") {
		t.Errorf("got %v; a row with no thumbprint cannot be deduplicated against "+
			"anything and must not reach the caller", got)
	}
}

func TestDedupeOfNothingIsNotNil(t *testing.T) {
	// The certificate step renders a list; a nil slice and an empty one read
	// the same in Go but not through every JSON boundary this crosses.
	if got := dedupe(nil); got == nil {
		t.Error("dedupe(nil) returned a nil slice, want an empty one")
	}
}

// TestOpenStopsAtTheWall pins the one thing this package deliberately does not
// do. If this ever starts passing by returning a session, the login step has
// been built and SPEC §6.5.1's eight clauses apply to it.
func TestOpenStopsAtTheWall(t *testing.T) {
	_, err := NewSource("whatever").Open(t.Context(), keysource.Thumbprint("AAAA"))
	if err == nil {
		t.Fatal("Open returned a session; the login step is not built (F11 §5)")
	}
}
