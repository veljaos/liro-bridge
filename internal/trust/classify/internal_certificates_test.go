package classify

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// selfSignedWith builds a self-signed certificate with the given common
// name and key usage. Built here rather than added to
// scripts/gencerts's committed fixtures because that generator draws
// fresh keys and serials from crypto/rand and rewrites every .der it
// owns, so adding one file would churn five unrelated fixtures — and
// this certificate carries nothing (no personal data, no CA bytes)
// that would make it worth freezing on disk. Same reasoning D-038
// applied to the synthetic PDF fixtures.
func selfSignedWith(t *testing.T, commonName string, ku x509.KeyUsage) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              ku,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert
}

// windowsInternalAuthKeyUsage is what the certificate reported in F6 §0b
// actually carries. It is the whole reason D-023's purpose-keyed rule
// let it through: these bits make purposeFromKeyUsage answer
// "authentication", not "unknown".
const windowsInternalAuthKeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment

// TestReportedWindowsInternalCertificateIsHidden is the direct
// regression test for F6 §0b. The subject is the exact GUID from the
// owner's own `certs` output. Run against D-023's rule
// (Purpose == PurposeUnknown && not qualified) this fails: the
// certificate classifies as PurposeAuthentication and was shown.
func TestReportedWindowsInternalCertificateIsHidden(t *testing.T) {
	cert := selfSignedWith(t, "5a26d334-110e-4468-910f-34313774f081", windowsInternalAuthKeyUsage)
	info := Classify(cert, bundledList(t), false, false, referenceTime)

	if info.Purpose != PurposeAuthentication {
		t.Fatalf("fixture assumption broken: Purpose = %v, want PurposeAuthentication", info.Purpose)
	}
	if !info.SelfSigned {
		t.Fatal("SelfSigned = false for a certificate that is its own issuer")
	}
	if !info.IsWindowsInternal() {
		t.Fatal("a self-signed, GUID-subject, unqualified certificate is still listed by default")
	}
}

func TestWindowsInternalRuleAcrossCertificateShapes(t *testing.T) {
	list := bundledList(t)

	cases := []struct {
		name   string
		info   Info
		hidden bool
	}{
		{
			name:   "bare GUID, authentication key usage",
			info:   Classify(selfSignedWith(t, "5a26d334-110e-4468-910f-34313774f081", windowsInternalAuthKeyUsage), list, false, false, referenceTime),
			hidden: true,
		},
		{
			name:   "braced GUID, authentication key usage",
			info:   Classify(selfSignedWith(t, "{3F2504E0-4F89-11D3-9A0C-0305E82C3301}", windowsInternalAuthKeyUsage), list, false, false, referenceTime),
			hidden: true,
		},
		{
			// F1 §5.4's original shape, still hidden by the first limb.
			name:   "GUID subject, no key usage bits at all",
			info:   Classify(loadCert(t, "selfsigned_unrelated.der"), list, false, false, referenceTime),
			hidden: true,
		},
		{
			// D-023's transcript property: a real authentication
			// certificate is shown, disabled, never hidden.
			name:   "qualified authentication certificate on a card",
			info:   Classify(loadCert(t, "halcom_auth.der"), list, true, true, referenceTime),
			hidden: false,
		},
		{
			name:   "qualified signing certificate",
			info:   Classify(loadCert(t, "halcom_signing.der"), list, true, true, referenceTime),
			hidden: false,
		},
		{
			// Being self-signed is not on its own disqualifying: a
			// company's own internal certificate has a name a person
			// recognises, and hiding it would be this rule overreaching.
			name:   "self-signed but named, not a GUID",
			info:   Classify(selfSignedWith(t, "Acme Internal Authentication", windowsInternalAuthKeyUsage), list, false, false, referenceTime),
			hidden: false,
		},
		{
			name:   "expired real signing certificate",
			info:   Classify(loadCert(t, "expired_signing.der"), list, true, true, referenceTime),
			hidden: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.info.IsWindowsInternal(); got != tc.hidden {
				t.Fatalf("IsWindowsInternal() = %t, want %t", got, tc.hidden)
			}
		})
	}
}

// TestSoftTokenCertificateIsNeverHidden guards SPEC §16.6: a test key
// must be loudly visible wherever it appears. A soft-token certificate
// is self-signed and software-backed, so it would otherwise be one
// GUID-shaped common name away from being filtered out of every listing.
func TestSoftTokenCertificateIsNeverHidden(t *testing.T) {
	cert := selfSignedWith(t, "5a26d334-110e-4468-910f-34313774f081", windowsInternalAuthKeyUsage)
	info := Classify(cert, bundledList(t), false, false, referenceTime)
	info.IsTestKey = true

	if info.IsWindowsInternal() {
		t.Fatal("a soft-token certificate was filtered out of the default listing")
	}
}

func TestIsGUID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"5a26d334-110e-4468-910f-34313774f081", true},
		{"2414ebcc-b68a-461c-9a69-bca3af581969", true},
		{"{3F2504E0-4F89-11D3-9A0C-0305E82C3301}", true},
		{"5A26D334-110E-4468-910F-34313774F081", true},
		{"", false},
		{"MUP Gradjani CA 4", false},
		{"Zoran Milovanović 246275", false},
		// Right length and hyphen positions, wrong alphabet.
		{"5a26d334-110e-4468-910f-34313774f08z", false},
		// Hyphens in the wrong places.
		{"5a26d3341-10e-4468-910f-34313774f081", false},
		// A GUID with something appended is not a GUID: a real name
		// that merely contains one must stay visible.
		{"urn:uuid:5a26d334-110e-4468-910f-34313774f081", false},
		{"5a26d334-110e-4468-910f-34313774f081 Sign", false},
		{"{5a26d334-110e-4468-910f-34313774f081", false},
	}
	for _, tc := range cases {
		if got := isGUID(tc.in); got != tc.want {
			t.Errorf("isGUID(%q) = %t, want %t", tc.in, got, tc.want)
		}
	}
}
