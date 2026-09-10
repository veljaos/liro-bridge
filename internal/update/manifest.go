// Package update implements SPEC §15.2's update channel: the agent
// checks GitHub Releases once a day, verifies the release's signature
// against a public key it embeds, and asks the person before doing
// anything with what it found. It never installs by itself, and there
// is no flag, configuration key or environment variable that makes it.
//
// Nothing in this package knows what a window is. What it decides —
// whether a check is due, whether a manifest verifies, whether a
// version is newer, which artefact to fetch — is a set of pure
// functions plus one HTTP GET, so all of it is testable with no
// network and no UI (the same split internal/consent has from
// internal/ui, for the same reason: F5 §10).
package update

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Manifest is what a release publishes about itself, as the file
// release.json alongside the artefacts. It is small on purpose: the
// agent verifies it before reading anything else, so everything it
// carries is inside the signature.
//
// It is not the GitHub API's release object. Fetching it as a plain
// release asset (see LatestManifestURL) rather than through the API
// means one unauthenticated GET with no token, no rate limit and no
// JSON shape this project does not own — and, more to the point, no
// query string this project would have to explain under SPEC §6.8.
type Manifest struct {
	// Version is the released version, without a leading "v": the same
	// string `liro-bridge --version` prints for that build.
	Version string `json:"version"`

	// Released is when the release was built, in RFC 3339. Shown to the
	// person beside the version; nothing branches on it.
	Released time.Time `json:"released"`

	// NotesURL is the release page. It is what the person is offered
	// when the agent cannot install for them — an update they decline
	// still has to be findable.
	NotesURL string `json:"notesURL"`

	// Artefacts is every file the release published, with the digest
	// the agent checks a download against. A digest that is inside the
	// signed manifest is what makes downloading the MSI safe: the
	// manifest is verified first, so the digest is not something the
	// same server could change independently of it.
	Artefacts []Artefact `json:"artefacts"`
}

// Artefact is one published file.
type Artefact struct {
	// Name is the asset's file name on the release page, which is also
	// the last path segment of its download URL.
	Name string `json:"name"`

	// Kind is "msi" or "exe". The agent installs an "msi" and offers an
	// "exe" only as something to download by hand — an EXE has no
	// uninstall entry to update and no upgrade semantics of its own.
	Kind string `json:"kind"`

	// SHA256 is the lower-case hex digest of the file's bytes.
	SHA256 string `json:"sha256"`

	// Size is the file's length in bytes. Checked before the digest so
	// that a wrong file is refused without reading all of it.
	Size int64 `json:"size"`
}

// Artefact kinds. Two, because two are what SPEC §15 says a release
// publishes.
const (
	KindMSI = "msi"
	KindEXE = "exe"
)

// ErrManifestInvalid is returned by ParseManifest for a document that
// is not a manifest this agent can act on. It is deliberately one
// error rather than one per field: a caller can do exactly one thing
// about any of them, which is to leave the release alone and say so.
var ErrManifestInvalid = errors.New("update: the release manifest is not valid")

// maxManifestBytes bounds what ParseManifest will look at. A release
// manifest for two artefacts is a few hundred bytes; 64 KB is four
// orders of magnitude of headroom and still small enough that a server
// answering with something enormous is refused rather than read.
const maxManifestBytes = 64 * 1024

// ParseManifest decodes and checks a manifest's own internal
// consistency. It says nothing about whether the manifest is
// authentic — that is VerifyManifest's question, and a caller must ask
// it first (SPEC §15.2: verify the signature before doing anything
// with what was downloaded).
func ParseManifest(b []byte) (Manifest, error) {
	if len(b) > maxManifestBytes {
		return Manifest{}, fmt.Errorf("%w: %d bytes is larger than a release manifest can be", ErrManifestInvalid, len(b))
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}
	if _, err := ParseVersion(m.Version); err != nil {
		return Manifest{}, fmt.Errorf("%w: version %q: %v", ErrManifestInvalid, m.Version, err)
	}
	if len(m.Artefacts) == 0 {
		return Manifest{}, fmt.Errorf("%w: it lists no artefacts", ErrManifestInvalid)
	}
	seen := make(map[string]bool, len(m.Artefacts))
	for _, a := range m.Artefacts {
		switch {
		case a.Name == "":
			return Manifest{}, fmt.Errorf("%w: an artefact has no name", ErrManifestInvalid)
		case seen[a.Name]:
			return Manifest{}, fmt.Errorf("%w: %q is listed twice", ErrManifestInvalid, a.Name)
		case a.Kind != KindMSI && a.Kind != KindEXE:
			return Manifest{}, fmt.Errorf("%w: %q has kind %q", ErrManifestInvalid, a.Name, a.Kind)
		case a.Size <= 0:
			return Manifest{}, fmt.Errorf("%w: %q has size %d", ErrManifestInvalid, a.Name, a.Size)
		}
		if err := checkDigest(a.SHA256); err != nil {
			return Manifest{}, fmt.Errorf("%w: %q: %v", ErrManifestInvalid, a.Name, err)
		}
		seen[a.Name] = true
	}
	return m, nil
}

// checkDigest rejects anything that is not 64 lower-case hex
// characters. Lower-case specifically: this project writes digests one
// way, and a manifest whose digests were spelled in the other would
// compare unequal against a freshly computed one for a file that is
// perfectly correct — a failure whose cause is invisible in a log.
func checkDigest(s string) error {
	if len(s) != 2*sha256Len {
		return fmt.Errorf("sha256 %q is %d characters, want %d", s, len(s), 2*sha256Len)
	}
	if _, err := hex.DecodeString(s); err != nil {
		return fmt.Errorf("sha256 %q is not hexadecimal", s)
	}
	for _, r := range s {
		if r >= 'A' && r <= 'F' {
			return fmt.Errorf("sha256 %q is upper-case; digests are written in lower case", s)
		}
	}
	return nil
}

// sha256Len is the digest length in bytes.
const sha256Len = 32

// Artefact returns the artefact of the given kind, and whether the
// manifest carries one at all. A release with no MSI is a release this
// agent can tell somebody about and cannot install.
func (m Manifest) Artefact(kind string) (Artefact, bool) {
	for _, a := range m.Artefacts {
		if a.Kind == kind {
			return a, true
		}
	}
	return Artefact{}, false
}
