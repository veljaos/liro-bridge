package consent

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/veljaos/liro-bridge/internal/trust/classify"
)

// ApplicationLocal is the ApplicationName used when a batch is
// triggered directly from the CLI rather than by a paired application
// (F5 §5.1's "application asking", audit.Entry's own "as bound at
// pairing, or 'local'"). F7 supplies real pairing; until then, every
// consent request this project can actually produce is local.
const ApplicationLocal = "local"

// ViewModel is everything the consent window's page renders, computed
// once per batch in Go and handed to the page as JSON (F5 §10: "The
// consent screen's data... is computed in Go and testable without a
// window"). Layout priority (F5 §5.1) is a property of the page's HTML/CSS,
// not of this struct's field order — but the doc comments below record
// which tier each field belongs to, since that grouping is the whole
// point of the screen.
type ViewModel struct {
	// Prominent: read in under two seconds (F5 §5.1).
	DocumentCount int

	// Secondary: present but quieter.
	ApplicationName string

	// Certificates offers every candidate row (F5 §5.2); the page is
	// responsible for prompting a deliberate choice — SPEC §11.5/§18.15
	// forbid a remembered default across sessions, so this ViewModel
	// never carries a pre-selected thumbprint.
	Certificates []CertificateOption

	// Details, collapsed by default (F5 §5.1).
	Fingerprint string // hex-encoded SHA-256 over the concatenated digests

	// FingerprintShort is what the window actually renders:
	// ShortFingerprint(Fingerprint). Fingerprint itself is still carried
	// in full, because the Copy action puts the whole 64 characters on
	// the clipboard — which is the only form that is any use for the
	// comparison SPEC §6.6 says the fingerprint exists for.
	FingerprintShort string

	Files         []string
	FilesOverflow int
}

// FingerprintPrefixLength is how much of the 64-character hex
// fingerprint the consent window shows before eliding the rest (Task 2,
// F5 second-real-run review). Sixteen hex characters is 64 bits — far
// beyond what anyone compares by eye, and the full value is one click
// away on the clipboard; the whole string rendered as one unbroken
// token overflowed the Details card and pushed the window's layout
// apart, which is the defect this exists to prevent.
const FingerprintPrefixLength = 16

// ShortFingerprint returns the first FingerprintPrefixLength characters
// of fingerprint followed by truncationMark, or fingerprint unchanged
// when it is already that short. The mark is the same three ASCII
// periods every other truncation in this project uses (D-057).
func ShortFingerprint(fingerprint string) string {
	if len(fingerprint) <= FingerprintPrefixLength {
		return fingerprint
	}
	return fingerprint[:FingerprintPrefixLength] + truncationMark
}

// BuildViewModel assembles a ViewModel from a batch's raw inputs. Every
// file name passes through CapFileNames' sanitisation pipeline before
// this function returns — nothing downstream ever sees an
// un-sanitised name (F5 §5.3).
func BuildViewModel(applicationName string, digests [][]byte, fileNames []string, certs []classify.Info) ViewModel {
	files, overflow := CapFileNames(fileNames)
	full := hex.EncodeToString(fingerprint(digests))
	return ViewModel{
		DocumentCount:    len(digests),
		ApplicationName:  applicationName,
		Certificates:     BuildCertificateOptions(certs),
		Fingerprint:      full,
		FingerprintShort: ShortFingerprint(full),
		Files:            files,
		FilesOverflow:    overflow,
	}
}

// fingerprint implements SPEC §6.6's batch fingerprint: SHA-256 over
// the concatenation of every digest, in order — the same computation
// internal/signing.NewBatch already performs (F2 §5.2) for the batch it
// actually signs. This package recomputes it independently from raw
// digests rather than importing internal/signing, since a ViewModel may
// be built before a Batch exists (the consent screen is shown, and a
// decision made, before signing begins) and duplicating one small pure
// function is simpler than restructuring either package's dependencies
// around a shared helper for it.
func fingerprint(digests [][]byte) []byte {
	h := sha256.New()
	for _, d := range digests {
		h.Write(d)
	}
	return h.Sum(nil)
}
