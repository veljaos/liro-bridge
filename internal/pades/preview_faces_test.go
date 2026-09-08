package pades

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/pades/render"
)

// TestStampPreviewDrawsBothEmbeddedFacesWithoutSubstituting is the
// end-to-end check that the stamp's second, bold face (D-209) is a font
// a reader can actually read, and not merely a stream of bytes of the
// right length in the right place.
//
// The project's own renderer is that reader. It substitutes a stand-in
// face for any font program it cannot use and says so in Notes
// (D-154) — and a substituted face still produces a picture, with
// letters in it, that looks broadly right at a glance. A picture is
// therefore not evidence; the absence of that note is. This is the same
// reasoning internal/pades/verify applies to signatures: the check has
// to be made by something other than the code that produced the thing.
//
// Both scripts the stamp has to draw are exercised, because the two
// faces are addressed by GID and a wrong table would show up as the
// wrong letters rather than as no letters: the MUP fixture's Cyrillic
// name and the Halcom fixture's Latin one, in a Latin interface, which
// is also the rule the name never follows (SPEC §9.3).
func TestStampPreviewDrawsBothEmbeddedFacesWithoutSubstituting(t *testing.T) {
	for _, name := range []string{"mup_signing.der", "halcom_signing.der"} {
		cert := fixtureCertificate(t, name)
		out, _, signer, err := stampPreviewDocument(cert, testSigningTime,
			&StampOptions{Label: "Elektronski potpisano"})
		if err != nil {
			t.Fatalf("%s: stampPreviewDocument: %v", name, err)
		}
		rdoc, err := render.Open(out)
		if err != nil {
			t.Fatalf("%s: render.Open: %v", name, err)
		}
		res, err := rdoc.RenderPage(1, 4)
		if err != nil {
			t.Fatalf("%s: RenderPage: %v", name, err)
		}
		for note, count := range res.Notes {
			if strings.Contains(note, "substituted") {
				t.Errorf("%s (%s): the renderer had to substitute a font %d times (%q) — an embedded face is not readable",
					name, signer, count, note)
			}
		}
	}
}

// fixtureCertificate loads one of the synthetic issuer certificates
// (testdata/certs/README.md): real issuer names and real DN *structure*,
// including the givenName/surname pair signerDisplayName reads, with
// fabricated personal data.
func fixtureCertificate(t *testing.T, name string) *x509.Certificate {
	t.Helper()
	der, err := os.ReadFile(filepath.Join("..", "..", "testdata", "certs", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	return cert
}
