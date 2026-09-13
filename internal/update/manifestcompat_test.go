package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
)

// withExtraField is a manifest of exactly the shape signrelease writes,
// plus one field this build does not know — which is what every release
// published up to and including v0.9.0 looks like to this build, since
// they all carry "notesURL".
func withExtraField(t *testing.T) (manifest, sig []byte, pub ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := signedRelease(t, priv, testManifest("1.2.3"))

	// Insert it the way a real manifest carries it: a sibling of
	// "version", inside the same object, before "artefacts".
	const anchor = "\"version\": \"1.2.3\",\n"
	if !strings.Contains(string(b), anchor) {
		t.Fatalf("the manifest is not the shape this test assumes:\n%s", b)
	}
	withField := strings.Replace(string(b), anchor,
		anchor+"  \"notesURL\": \"https://example.invalid/releases/tag/v1.2.3\",\n", 1)

	s, err := Sign(priv, []byte(withField))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(withField), []byte(s), pub
}

// An agent must go on seeing releases after one of them describes
// itself with a field the agent does not know.
//
// This is not a preference. ParseManifest used to refuse such a
// manifest, which means the day a release gained a field, every agent
// already installed would have stopped seeing releases at all — and
// stopped silently, since a failed check only reaches the log. The
// failure would have arrived at the exact moment somebody was trying to
// ship a fix.
func TestAnAgentStillSeesAReleaseThatCarriesAFieldItDoesNotKnow(t *testing.T) {
	manifest, sig, pub := withExtraField(t)

	m, err := VerifyManifest(manifest, sig, []ed25519.PublicKey{pub})
	if err != nil {
		t.Fatalf("a signed manifest with one unknown field was refused: %v", err)
	}
	if m.Version != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", m.Version)
	}
	if len(m.Artefacts) != 1 {
		t.Fatalf("artefacts = %d, want 1 — the rest of the manifest was not read", len(m.Artefacts))
	}
	if m.Artefacts[0].Name != "liro-bridge-1.2.3-x64.msi" {
		t.Errorf("artefact name = %q", m.Artefacts[0].Name)
	}
}

// And the release pipeline must still refuse it, which is the half that
// makes the tolerance above safe: a field this build does not know, in
// a document this build just wrote, is a mistake in the tooling, and
// verifyrelease is the last moment it can be caught.
func TestTheReleasePipelineStillRefusesAManifestOfTheWrongShape(t *testing.T) {
	manifest, _, _ := withExtraField(t)

	if err := CheckManifestShape(manifest); err == nil {
		t.Fatal("CheckManifestShape accepted a manifest carrying an unknown field")
	} else if !strings.Contains(err.Error(), "notesURL") {
		t.Errorf("the error does not name the field: %v", err)
	}

	// The shape this build does write passes, or the check would refuse
	// every release and mean nothing.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ours, _ := signedRelease(t, priv, testManifest("1.2.3"))
	if err := CheckManifestShape(ours); err != nil {
		t.Fatalf("CheckManifestShape refused a manifest of this build's own shape: %v", err)
	}
}

// The manifest has no field for the release page, and must not grow one
// back: what a person is offered when the agent cannot install for them
// is a constant this binary was built with, never an address read out
// of a document fetched over the network. The signature says the
// manifest is ours; it does not make every address inside it somewhere
// to send somebody.
func TestTheManifestCarriesNoAddressToSendAPersonTo(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := signedRelease(t, priv, testManifest("1.2.3"))

	for _, field := range []string{"notesURL", "notesUrl", "releasePage", "url", "URL"} {
		if strings.Contains(string(b), "\""+field+"\"") {
			t.Errorf("the manifest this build writes carries %q:\n%s", field, b)
		}
	}
	if !strings.Contains(ReleasesPageURL, "github.com/veljaos/liro-bridge/releases") {
		t.Errorf("ReleasesPageURL is not where a person would be sent: %q", ReleasesPageURL)
	}
}
