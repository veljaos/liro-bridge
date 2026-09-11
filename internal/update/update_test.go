package update

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A manifest and a signature built the way scripts/signrelease builds
// them, so every test below goes through the same bytes a release
// actually publishes rather than through a struct literal.
func signedRelease(t *testing.T, priv ed25519.PrivateKey, m Manifest) (manifest, sig []byte) {
	t.Helper()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("encoding the manifest: %v", err)
	}
	b = append(b, '\n')
	s, err := Sign(priv, b)
	if err != nil {
		t.Fatalf("signing: %v", err)
	}
	return b, []byte(s)
}

func testManifest(version string, artefacts ...Artefact) Manifest {
	if len(artefacts) == 0 {
		artefacts = []Artefact{{
			Name:   "liro-bridge-" + version + "-x64.msi",
			Kind:   KindMSI,
			SHA256: strings.Repeat("ab", 32),
			Size:   4_000_000,
		}}
	}
	return Manifest{
		Version:   version,
		Released:  time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		NotesURL:  "https://github.com/veljaos/liro-bridge/releases/tag/v" + version,
		Artefacts: artefacts,
	}
}

// The whole gate, in the order SPEC §15.2 requires it: a release
// signed by a key this build embeds verifies, and one signed by any
// other key does not — however well-formed everything else about it
// is.
func TestOnlyAKeyThisBuildEmbedsCanSignARelease(t *testing.T) {
	trustedPub, trustedPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, strangerPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trusted := []ed25519.PublicKey{trustedPub}

	manifest, sig := signedRelease(t, trustedPriv, testManifest("1.2.0"))
	m, err := VerifyManifest(manifest, sig, trusted)
	if err != nil {
		t.Fatalf("a release signed by a trusted key was refused: %v", err)
	}
	if m.Version != "1.2.0" {
		t.Errorf("verified manifest version = %q, want 1.2.0", m.Version)
	}

	_, strangerSig := signedRelease(t, strangerPriv, testManifest("1.2.0"))
	if _, err := VerifyManifest(manifest, strangerSig, trusted); err == nil {
		t.Fatal("a release signed by a key this build does not embed was accepted")
	}
}

// A signature over one manifest must not verify over another. This is
// the property that stops a stale, correctly-signed manifest being
// replayed in front of a newer one — and the reason the signature is
// over the manifest's exact bytes rather than over any field of it.
func TestASignatureDoesNotCarryToADifferentManifest(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trusted := []ed25519.PublicKey{pub}

	_, sig := signedRelease(t, priv, testManifest("1.2.0"))
	other, _ := signedRelease(t, priv, testManifest("9.9.9"))

	if _, err := VerifyManifest(other, sig, trusted); err == nil {
		t.Fatal("a signature over version 1.2.0 verified over a manifest claiming 9.9.9")
	}
}

// One byte changed anywhere in the manifest breaks it. Checked
// exhaustively over a real manifest's own bytes rather than at one
// hand-picked offset, since a signature that only covers part of what
// it appears to cover is exactly the defect this is about.
func TestEveryByteOfTheManifestIsCovered(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trusted := []ed25519.PublicKey{pub}
	manifest, sig := signedRelease(t, priv, testManifest("1.2.0"))

	for i := range manifest {
		tampered := append([]byte(nil), manifest...)
		tampered[i] ^= 0x01
		if _, err := VerifyManifest(tampered, sig, trusted); err == nil {
			t.Fatalf("flipping a bit at offset %d left the signature verifying", i)
		}
	}
}

// A build with no embedded key says so in its own words rather than
// reporting the release as untrustworthy: "this build cannot check
// signatures" and "this release is not signed properly" are different
// facts, and only one of them is about the release.
func TestABuildWithNoTrustedKeysSaysSoRatherThanBlamingTheRelease(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, sig := signedRelease(t, priv, testManifest("1.2.0"))
	_, err = VerifyManifest(manifest, sig, nil)
	if err == nil {
		t.Fatal("a build with no trusted keys accepted a release")
	}
	if err != ErrNoTrustedKeys {
		t.Errorf("err = %v, want ErrNoTrustedKeys", err)
	}
}

// This build's own embedded keys are real keys, and there are two of
// them — the second being the whole of this project's answer to a lost
// or compromised primary (F10 §4.1).
func TestThisBuildEmbedsTwoUsableReleaseKeys(t *testing.T) {
	keys := TrustedKeys()
	if len(keys) != 2 {
		t.Fatalf("this build embeds %d release keys, want 2 (a primary and a spare)", len(keys))
	}
	if keys[0].Equal(keys[1]) {
		t.Fatal("the spare release key is the same key as the primary; a spare that is the primary is not a spare")
	}
	for i, k := range keys {
		if len(k) != ed25519.PublicKeySize {
			t.Errorf("embedded key %d is %d bytes, want %d", i, len(k), ed25519.PublicKeySize)
		}
	}
}

// Two generations of key material were discarded before the pair this
// build embeds, and their public halves survive in this repository's
// own history and in terminal output (D-238). Neither was ever trusted
// by a released build, and the way to keep that true is to check it
// rather than to rely on nobody reverting a file.
//
// The values below are public halves, so naming them costs nothing —
// which is the whole reason this check can exist at all. Only the
// first generation's are known: it is the pair this file carried
// before the owner's own was pasted in, and it is recoverable from any
// clone. The second generation produced single keys whose public
// halves never reached this repository, so there is nothing here to
// compare against — recorded so that a reader does not take the two
// values below for the complete list.
func TestNoDiscardedReleaseKeyIsTrustedByThisBuild(t *testing.T) {
	// The first generation: an agent-generated pair, discarded because
	// its provenance rested on a claim rather than on the owner having
	// made it. See D-238.
	discarded := map[string]string{
		"first-generation primary": "DfVtjTl9lKSjZQB07a4dB6OcRIQPAuNGk0R9VDspvCk=",
		"first-generation spare":   "BNiw9ruQH5nNAJdNyzeOygTZCytQxVstE3g0gP1z5BE=",
	}
	for role, b64 := range discarded {
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			t.Fatalf("the %s constant in this test is not base64: %v", role, err)
		}
		for i, k := range TrustedKeys() {
			if k.Equal(ed25519.PublicKey(raw)) {
				t.Errorf("embedded key %d is the %s, which was discarded and must never be trusted", i, role)
			}
		}
	}
}

// The private halves are not in this repository. Checked by searching
// the tree rather than by asserting the intention, because the
// intention is what is already written down and the search is what
// would notice somebody pasting one in.
func TestNoReleasePrivateKeyIsInThisRepository(t *testing.T) {
	root := repositoryRoot(t)
	// An Ed25519 private key's base64 is 88 characters ending "==". The
	// public halves are 44 and end "=", so this cannot match them.
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".json", ".yml", ".yaml", ".md", ".txt", ".wxs", ".wxl", ".ps1":
		default:
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil || len(b) > 4<<20 {
			return nil
		}
		if looksLikeEd25519PrivateKey(string(b)) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Errorf("what looks like an Ed25519 private key is committed in: %s", strings.Join(found, ", "))
	}
}

// looksLikeEd25519PrivateKey looks for a base64 run that decodes to 64
// bytes whose second half is the public key *derived from* the first
// half. That derivation is what an Ed25519 private key is, and what 64
// arbitrary bytes are not.
//
// The obvious version of this check — priv.Public() compared against
// raw[32:] — is a tautology: crypto/ed25519 stores the public half in
// the last 32 bytes and Public() returns exactly those, so it matches
// any 64 bytes at all. Written that way first, it reported a private
// key in two files that contain nothing of the sort (an RSA test
// fixture and npm's lockfile), which is how it was caught. A check
// that fires on everything is a check that says nothing.
func looksLikeEd25519PrivateKey(s string) bool {
	const encoded = 88 // base64 of 64 bytes
	for i := 0; i+encoded <= len(s); i++ {
		chunk := s[i : i+encoded]
		if !strings.HasSuffix(chunk, "==") {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(chunk)
		if err != nil || len(raw) != ed25519.PrivateKeySize {
			continue
		}
		derived, ok := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize]).Public().(ed25519.PublicKey)
		if ok && derived.Equal(ed25519.PublicKey(raw[ed25519.SeedSize:])) {
			return true
		}
	}
	return false
}

// And the check itself is capable of firing: a real, freshly generated
// private key is found by it. Without this, the test above passes just
// as happily against a scan that matches nothing — which is the shape
// D-161 records, a test asserting the right property against a fixture
// that cannot fail.
func TestThePrivateKeyScanWouldActuallyFire(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if !looksLikeEd25519PrivateKey("something before " + base64.StdEncoding.EncodeToString(priv) + " and after") {
		t.Fatal("the scan did not find a real Ed25519 private key")
	}
	if looksLikeEd25519PrivateKey(base64.StdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize))) {
		t.Fatal("the scan reports 64 zero bytes as a private key")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}

// Semver precedence, including the two rules that are easy to get
// backwards: a pre-release sorts before the release it precedes, and
// build metadata is not part of the comparison at all.
func TestVersionPrecedence(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.99.99", 1},
		{"1.0.0", "1.0.0-rc.1", 1},
		{"1.0.0-rc.1", "1.0.0-rc.2", -1},
		{"1.0.0-rc.2", "1.0.0-rc.10", -1}, // numeric, not lexical
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-beta", -1},
		{"1.0.0+build.9", "1.0.0+build.1", 0}, // metadata ignored
		{"v1.2.3", "1.2.3", 0},                // a tag and a manifest agree
	}
	for _, c := range cases {
		a, err := ParseVersion(c.a)
		if err != nil {
			t.Fatalf("ParseVersion(%q): %v", c.a, err)
		}
		b, err := ParseVersion(c.b)
		if err != nil {
			t.Fatalf("ParseVersion(%q): %v", c.b, err)
		}
		if got := a.Compare(b); got != c.want {
			t.Errorf("%s vs %s = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := b.Compare(a); got != -c.want {
			t.Errorf("%s vs %s = %d, want %d (the comparison is not symmetric)", c.b, c.a, got, -c.want)
		}
	}
}

func TestWhatIsNotAVersion(t *testing.T) {
	for _, s := range []string{"", "dev", "1", "1.2", "1.2.3.4", "1.2.x", "01.2.3", "1.2.3-", "-1.2.3"} {
		if _, err := ParseVersion(s); err == nil {
			t.Errorf("ParseVersion(%q) accepted it", s)
		}
	}
}

// An unstamped build is never offered an update. `dev` is what
// cmd/liro-bridge's version defaults to, and there is no order between
// it and any release — offering a developer an "update" over their own
// build is worse than saying nothing.
func TestADevelopmentBuildIsNeverToldAnUpdateIsAvailable(t *testing.T) {
	srv, _ := releaseServer(t, "9.9.9")
	defer srv.Close()

	res, err := checkerFor(srv).Check(context.Background(), "dev")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Available {
		t.Error("a dev build was told a release is available")
	}
	if res.Comparable {
		t.Error("a dev build reported its version as comparable")
	}
	if res.Manifest.Version != "9.9.9" {
		t.Errorf("the manifest was not carried back: %q", res.Manifest.Version)
	}
}

func TestANewerReleaseIsAvailableAndAnOlderOneIsNot(t *testing.T) {
	srv, _ := releaseServer(t, "1.5.0")
	defer srv.Close()

	for _, c := range []struct {
		current string
		want    bool
	}{{"1.4.9", true}, {"1.5.0", false}, {"1.5.1", false}, {"2.0.0", false}} {
		res, err := checkerFor(srv).Check(context.Background(), c.current)
		if err != nil {
			t.Fatalf("Check(%s): %v", c.current, err)
		}
		if res.Available != c.want {
			t.Errorf("running %s against a 1.5.0 release: Available = %v, want %v", c.current, res.Available, c.want)
		}
	}
}

// The check refuses a release whose signature does not verify, and
// refuses it before it has parsed a single field of the manifest — so
// nothing downstream ever sees an unverified version number, a
// download URL or a digest.
func TestAnUnverifiableReleaseIsRefusedRatherThanReported(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, sig := signedRelease(t, otherPriv, testManifest("9.9.9"))
	srv := serve(t, manifest, sig, nil)
	defer srv.Close()

	c := NewChecker(srv.Client()).withURLs(srv.URL+"/release.json", srv.URL+"/release.json.sig")
	c.trusted = []ed25519.PublicKey{pub}

	res, err := c.Check(context.Background(), "1.0.0")
	if err == nil {
		t.Fatal("a release signed by an untrusted key was reported rather than refused")
	}
	if res.Available || res.Manifest.Version != "" {
		t.Errorf("an unverified manifest reached the caller: %+v", res)
	}
}

// A download is checked against the digest inside the verified
// manifest, so the file is covered by the same signature the manifest
// is. Wrong bytes are refused even though the server served them
// happily.
func TestADownloadIsCheckedAgainstTheSignedDigest(t *testing.T) {
	body := []byte("this is not really an installer, but it is exactly these bytes")
	sum := sha256.Sum256(body)
	art := Artefact{Name: "liro-bridge-1.5.0-x64.msi", Kind: KindMSI, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m := testManifest("1.5.0", art)
	manifest, sig := signedRelease(t, priv, m)

	served := body
	srv := serve(t, manifest, sig, func() []byte { return served })
	defer srv.Close()
	c := NewChecker(srv.Client()).withURLs(srv.URL+"/release.json", srv.URL+"/release.json.sig")
	c.trusted = []ed25519.PublicKey{pub}
	c.downloadBase = srv.URL + "/"

	got, err := c.Download(context.Background(), m, art)
	if err != nil {
		t.Fatalf("Download of the right bytes: %v", err)
	}
	if string(got) != string(body) {
		t.Error("Download returned different bytes from the ones served")
	}

	// One byte different, same length, same everything else.
	served = append([]byte(nil), body...)
	served[0] ^= 0x01
	if _, err := c.Download(context.Background(), m, art); err == nil {
		t.Fatal("a download whose digest does not match the signed manifest was accepted")
	}
}

// Once a day, and a check that failed still counts as having happened
// — a machine with no internet must not retry every minute (SPEC
// §6.8: offline is normal, not an error).
func TestWhenACheckIsDue(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		state   State
		enabled bool
		want    bool
	}{
		{"never checked", State{}, true, true},
		{"checked a minute ago", State{LastCheck: now.Add(-time.Minute)}, true, false},
		{"checked 23 hours ago", State{LastCheck: now.Add(-23 * time.Hour)}, true, false},
		{"checked 24 hours ago", State{LastCheck: now.Add(-24 * time.Hour)}, true, true},
		{"switched off, never checked", State{}, false, false},
		{"switched off, long overdue", State{LastCheck: now.Add(-100 * time.Hour)}, false, false},
		{"a clock that moved backwards", State{LastCheck: now.Add(time.Hour)}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Due(c.state, c.enabled, now); got != c.want {
				t.Errorf("Due = %v, want %v", got, c.want)
			}
		})
	}
}

// Switching the check off is the whole of it: there is no path through
// Due that reaches the network with the setting off, whatever the
// clock says.
func TestTheSettingIsTheWholeSwitch(t *testing.T) {
	for _, last := range []time.Time{{}, time.Now().Add(-365 * 24 * time.Hour), time.Now()} {
		if Due(State{LastCheck: last}, false, time.Now()) {
			t.Fatalf("a check was due with the setting off (last check %v)", last)
		}
	}
}

func TestStateRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "update-state.json")

	if got := LoadState(path); !got.LastCheck.IsZero() {
		t.Errorf("a missing state file did not read as the zero value: %+v", got)
	}

	want := State{
		LastCheck:        time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC),
		LastSeenVersion:  "1.5.0",
		DismissedVersion: "1.5.0",
	}
	if err := SaveState(path, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got := LoadState(path)
	if !got.LastCheck.Equal(want.LastCheck) || got.LastSeenVersion != want.LastSeenVersion || got.DismissedVersion != want.DismissedVersion {
		t.Errorf("state round trip: got %+v, want %+v", got, want)
	}

	// A corrupt file is one extra check, not a refusal to start.
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadState(path); !got.LastCheck.IsZero() {
		t.Errorf("a corrupt state file did not read as the zero value: %+v", got)
	}
}

// What one check actually sends. F10 §5 requires this to be stated;
// this is the statement, in a form that fails if it stops being true.
func TestACheckSendsNothingAboutTheUserOrTheMachine(t *testing.T) {
	var seen []*http.Request
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, sig := signedRelease(t, priv, testManifest("1.5.0"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(r.Context()))
		switch r.URL.Path {
		case "/release.json":
			_, _ = w.Write(manifest)
		case "/release.json.sig":
			_, _ = w.Write(sig)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewChecker(srv.Client()).withURLs(srv.URL+"/release.json", srv.URL+"/release.json.sig")
	c.trusted = []ed25519.PublicKey{pub}
	if _, err := c.Check(context.Background(), "1.0.0"); err != nil {
		t.Fatalf("Check: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("one check made %d requests, want 2 (the manifest and its signature)", len(seen))
	}
	for _, r := range seen {
		if r.Method != http.MethodGet {
			t.Errorf("%s %s: a check only ever GETs", r.Method, r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Errorf("%s carries a query string: %q", r.URL.Path, r.URL.RawQuery)
		}
		if len(r.Cookies()) != 0 {
			t.Errorf("%s carries cookies", r.URL.Path)
		}
		if ua := r.Header.Get("User-Agent"); ua != checkUserAgent {
			t.Errorf("User-Agent = %q, want the fixed %q with no version in it", ua, checkUserAgent)
		}
		// Whatever else is on the request must not be about this
		// machine or this build. Header names are checked rather than
		// values because a value that happens to be empty today is not
		// a promise about tomorrow.
		for name := range r.Header {
			switch strings.ToLower(name) {
			case "user-agent", "accept", "accept-encoding":
			default:
				t.Errorf("%s carries an unexpected header %q", r.URL.Path, name)
			}
		}
	}
}

// Helpers.

func releaseServer(t *testing.T, version string) (*httptest.Server, ed25519.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, sig := signedRelease(t, priv, testManifest(version))
	srv := serve(t, manifest, sig, nil)
	t.Cleanup(func() { testTrusted[srv.URL] = nil })
	testTrusted[srv.URL] = []ed25519.PublicKey{pub}
	return srv, pub
}

// testTrusted lets checkerFor hand a server's own key to the checker
// that talks to it, so a test does not have to repeat five lines of
// wiring for the common case.
var testTrusted = map[string][]ed25519.PublicKey{}

func checkerFor(srv *httptest.Server) *Checker {
	c := NewChecker(srv.Client()).withURLs(srv.URL+"/release.json", srv.URL+"/release.json.sig")
	c.trusted = testTrusted[srv.URL]
	c.downloadBase = srv.URL + "/"
	return c
}

func serve(t *testing.T, manifest, sig []byte, artefact func() []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/release.json":
			_, _ = w.Write(manifest)
		case r.URL.Path == "/release.json.sig":
			_, _ = w.Write(sig)
		case artefact != nil && strings.HasSuffix(r.URL.Path, ".msi"):
			_, _ = w.Write(artefact())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}
