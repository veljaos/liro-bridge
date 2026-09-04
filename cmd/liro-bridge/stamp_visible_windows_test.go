//go:build windows

package main

// Task 1 (F5 fourth-real-run review), measured on the produced bytes:
// what the consent window now asks for has to come out the other end as
// a signature that can actually be seen. The report's own evidence was
//
//	field 2073 | T = Liro-Signature-1 | Rect = [0, 0, 0, 0] | no appearance stream
//
// so this test signs through exactly the path runSignInteractive uses
// (signInteractiveOne with interactiveStampOptions' own output) and
// asserts the opposite: a non-degenerate /Rect and an appearance
// stream. It needs no card and no window — it is the wiring, not the
// rendering, that this round changed, and the rendering already has its
// own tests in internal/pades.
import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
)

// stampSession is a signing session backed by a key generated here: the
// same shape as a card's session (SPEC §5.1), with the givenName and
// surname attributes the stamp builds its name line from (SPEC §11.7).
type stampSession struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func newStampSession(t *testing.T) *stampSession {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(0x4C49524F),
		Subject: pkix.Name{
			CommonName: "ВЕЉКО СТАНОЈЕВИЋ 011445479 Sign",
			ExtraNames: []pkix.AttributeTypeAndValue{
				{Type: asn1.ObjectIdentifier{2, 5, 4, 42}, Value: "ВЕЉКО"},
				{Type: asn1.ObjectIdentifier{2, 5, 4, 4}, Value: "СТАНОЈЕВИЋ"},
			},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &stampSession{cert: cert, key: key}
}

func (s *stampSession) SignDigest(_ context.Context, _ keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	return rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest)
}
func (s *stampSession) Certificate() keysource.Certificate {
	return keysource.Certificate{Thumbprint: "STAMP-TEST", DER: s.cert.Raw}
}
func (s *stampSession) Chain() [][]byte { return nil }
func (s *stampSession) Close() error    { return nil }

// rectPattern matches a signature widget's /Rect array as this project
// writes it, capturing the four numbers.
var rectPattern = regexp.MustCompile(`/Rect \[([-0-9. ]+)\]`)

func signThroughInteractivePath(t *testing.T, stamp consent.StampChoice) []byte {
	t.Helper()
	c := i18n.Load("sr-Latn")
	in, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	out := filepath.Join(t.TempDir(), "blank-potpisan.pdf")

	if _, err := signInteractiveOne(t.Context(), in, newStampSession(t), interactiveSignOptions{
		level:   pades.LevelBB,
		outPath: out,
		allowBB: true,
		stamp:   interactiveStampOptions(c, stamp),
	}); err != nil {
		t.Fatalf("signInteractiveOne: %v", err)
	}
	signed, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading the signed document: %v", err)
	}
	return signed
}

// TestInteractiveSignatureIsVisibleWhenTheStampIsAskedFor is the
// finding, inverted: with the checkbox on (its default), the signature
// widget has a real rectangle and an appearance stream.
func TestInteractiveSignatureIsVisibleWhenTheStampIsAskedFor(t *testing.T) {
	signed := signThroughInteractivePath(t, consent.DefaultStampChoice())

	matches := rectPattern.FindAllStringSubmatch(string(signed), -1)
	if len(matches) == 0 {
		t.Fatal("the signed document has no /Rect at all")
	}
	sawRealRect := false
	for _, m := range matches {
		if m[1] != "0 0 0 0" {
			sawRealRect = true
		}
	}
	if !sawRealRect {
		t.Errorf("every /Rect is [0 0 0 0] — the signature is invisible: %v", matches)
	}
	if !regexp.MustCompile(`/AP\b`).Match(signed) {
		t.Error("the signature widget has no /AP appearance stream — nothing is drawn")
	}
	// The stamp's own font subset is the other thing that must be there
	// for anything to be legible (SPEC §13.2).
	if !regexp.MustCompile(`LIROBR`).Match(signed) {
		t.Error("the embedded font subset is absent — a stamp with no glyphs")
	}
}

// TestInteractiveSignatureStaysInvisibleWhenTheStampIsOff is SPEC
// §13.4: unticking the box leaves the invisible-signature path exactly
// as it was, with the degenerate rectangle and no appearance.
func TestInteractiveSignatureStaysInvisibleWhenTheStampIsOff(t *testing.T) {
	signed := signThroughInteractivePath(t, consent.StampChoice{Visible: false})

	matches := rectPattern.FindAllStringSubmatch(string(signed), -1)
	if len(matches) == 0 {
		t.Fatal("the signed document has no /Rect at all")
	}
	for _, m := range matches {
		if m[1] != "0 0 0 0" {
			t.Errorf("/Rect [%s] with the stamp switched off — the invisible path must stay invisible", m[1])
		}
	}
	if regexp.MustCompile(`LIROBR`).Match(signed) {
		t.Error("the stamp's font subset was embedded even though no stamp was asked for")
	}
}

// TestInteractiveStampCornerReachesTheOutput proves the corner choice
// is not cosmetic: bottom-left and top-right put the widget in
// different places on the same page.
func TestInteractiveStampCornerReachesTheOutput(t *testing.T) {
	bottomLeft := rectPattern.FindAllStringSubmatch(string(signThroughInteractivePath(t,
		consent.StampChoice{Visible: true, Position: consent.StampPositionBottomLeft})), -1)
	topRight := rectPattern.FindAllStringSubmatch(string(signThroughInteractivePath(t,
		consent.StampChoice{Visible: true, Position: consent.StampPositionTopRight})), -1)

	if len(bottomLeft) == 0 || len(topRight) == 0 {
		t.Fatal("one of the two signed documents has no /Rect")
	}
	if bottomLeft[len(bottomLeft)-1][1] == topRight[len(topRight)-1][1] {
		t.Errorf("bottom-left and top-right produced the same rectangle %q — the corner is ignored",
			bottomLeft[len(bottomLeft)-1][1])
	}
}

// TestConfiguredBBSignsWithoutATimestampAndReportsBB is Task 3's
// outcome: with B-B configured, no timestamp authority is contacted
// (opts.tsaClient is nil, which is what runSignInteractive leaves it as
// for this level) and the achieved level reported back — and so into
// the audit entry — is B-B, accurately.
func TestConfiguredBBSignsWithoutATimestampAndReportsBB(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := populatedSettings() // SignatureLevel "b-b"
	in, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}

	result, err := signInteractiveOne(t.Context(), in, newStampSession(t), interactiveSignOptions{
		level:   interactiveLevel(cfg),
		outPath: filepath.Join(t.TempDir(), "blank-potpisan.pdf"),
		stamp:   interactiveStampOptions(c, consent.DefaultStampChoice()),
		// allowBB deliberately false: a configured B-B must not need
		// the "degrade on failure" escape hatch, because nothing is
		// attempted that could fail.
	})
	if err != nil {
		t.Fatalf("signing at a configured B-B failed: %v", err)
	}
	if result.AchievedLevel != pades.LevelBB {
		t.Errorf("achieved level = %q, want B-B", result.AchievedLevel)
	}
	for _, note := range result.Notes {
		t.Errorf("a deliberate B-B reported a degradation note: %q", note)
	}
}
