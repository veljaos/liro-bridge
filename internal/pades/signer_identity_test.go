package pades

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"strings"
	"testing"
	"time"
)

func buildTestCertificate(t *testing.T, subject pkix.Name) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      subject,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

// TestSignerDisplayNameUsesGivenNameAndSurname reproduces SPEC §11.7's
// three real, measured CN values directly, proving the name is built
// from givenName/surname and never from CN — which, for all three, would
// have leaked the CA's internal reference number and, for MUP, the
// literal English word "Sign".
func TestSignerDisplayNameUsesGivenNameAndSurname(t *testing.T) {
	tests := []struct {
		cn, given, surname, want string
	}{
		{"ВЕЉКО СТАНОЈЕВИЋ 011445479 Sign", "ВЕЉКО", "СТАНОЈЕВИЋ", "ВЕЉКО СТАНОЈЕВИЋ"},
		{"Zoran Milovanović 246275", "Zoran", "Milovanović", "Zoran Milovanović"},
		{"Redžvel Mešković 200094362", "Redžvel", "Mešković", "Redžvel Mešković"},
	}
	for _, tt := range tests {
		cert := buildTestCertificate(t, pkix.Name{
			CommonName: tt.cn,
			ExtraNames: []pkix.AttributeTypeAndValue{
				{Type: oidGivenNameStamp, Value: tt.given},
				{Type: oidSurnameStamp, Value: tt.surname},
			},
		})
		got := signerDisplayName(cert)
		if got != tt.want {
			t.Errorf("signerDisplayName(CN=%q) = %q, want %q", tt.cn, got, tt.want)
		}
		if strings.Contains(got, "Sign") || strings.ContainsAny(got, "0123456789") {
			t.Errorf("signerDisplayName(CN=%q) = %q leaked CA-number/suffix content from CN", tt.cn, got)
		}
	}
}

func TestSignerDisplayNameFallsBackToCommonNameWhenAttributesAbsent(t *testing.T) {
	cert := buildTestCertificate(t, pkix.Name{CommonName: "Some Legacy CN"})
	if got := signerDisplayName(cert); got != "Some Legacy CN" {
		t.Errorf("signerDisplayName = %q, want fallback to CN", got)
	}
}

// TestDocumentIDFromCertificateExtractsOnlyIDCRS is F4 §5.2's opt-in
// identity-document-number line: it must come from the IDCRS-prefixed
// value (SPEC §11.6 Trap 2, Halcom's non-resident ID-card number), never
// from anything else.
func TestDocumentIDFromCertificateExtractsOnlyIDCRS(t *testing.T) {
	cert := buildTestCertificate(t, pkix.Name{
		CommonName:   "Test",
		SerialNumber: "IDCRS-998877",
	})
	if got := documentIDFromCertificate(cert); got != "998877" {
		t.Errorf("documentIDFromCertificate = %q, want %q", got, "998877")
	}
}

// TestDocumentIDFromCertificateNeverReturnsPNORS is the security-relevant
// test F4 §5.2 and this project's own top-level engagement rules
// require: "the national identity number must be unreachable from this
// package." A certificate carrying only a PNORS value (the JMBG-style
// national ID, SPEC §11.6 Trap 2) — with no IDCRS present anywhere —
// must never have that value surface through documentIDFromCertificate,
// under any circumstance.
func TestDocumentIDFromCertificateNeverReturnsPNORS(t *testing.T) {
	cert := buildTestCertificate(t, pkix.Name{
		CommonName:   "Test",
		SerialNumber: "PNORS-0114454791234", // 13 digits, JMBG-shaped
	})
	got := documentIDFromCertificate(cert)
	if got != "" {
		t.Fatalf("documentIDFromCertificate returned %q for a PNORS-only certificate — the national identity number must be unreachable", got)
	}
}

// TestDocumentIDFromCertificateIgnoresPNORSEvenInsideSameMultiValuedRDN
// reproduces SPEC §11.6 Trap 1 directly: PNORS and CA:RS-... (and, in
// this synthetic case, IDCRS) can appear as two values of the same
// attribute type inside a single multi-valued RDN. A parser that
// returns "the" serialNumber value non-deterministically could return
// the PNORS value; this test proves documentIDFromCertificate always
// finds the IDCRS one instead of the PNORS one, regardless of RDN
// ordering.
func TestDocumentIDFromCertificateIgnoresPNORSEvenInsideSameMultiValuedRDN(t *testing.T) {
	oidSerialNumber := asn1.ObjectIdentifier{2, 5, 4, 5}
	cert := buildTestCertificate(t, pkix.Name{
		CommonName: "Test",
		ExtraNames: []pkix.AttributeTypeAndValue{
			// PNORS listed first, deliberately, so a naive "first match"
			// parser would return the national ID.
			{Type: oidSerialNumber, Value: "PNORS-0114454791234"},
			{Type: oidSerialNumber, Value: "IDCRS-998877"},
		},
	})
	got := documentIDFromCertificate(cert)
	if got != "998877" {
		t.Fatalf("documentIDFromCertificate = %q, want the IDCRS value %q (never the PNORS value)", got, "998877")
	}
}
