package classify

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

const testdataDir = "../../../testdata/certs"

func loadCert(t *testing.T, filename string) *x509.Certificate {
	t.Helper()
	der, err := os.ReadFile(filepath.Join(testdataDir, filename))
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing %s: %v", filename, err)
	}
	return cert
}

// referenceTime is inside the synthetic certificates' validity window
// (2021-09-23 to 2026-09-24) and after the bundled TSL's issue date, so
// Classify's qualification and expiry checks both exercise their normal
// paths rather than an edge case.
var referenceTime = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func bundledList(t *testing.T) *tsl.List {
	t.Helper()
	raw, err := os.ReadFile("../tsl/seed/TSL-RS.xml")
	if err != nil {
		t.Fatalf("reading bundled seed: %v", err)
	}
	list, err := tsl.Parse(raw)
	if err != nil {
		t.Fatalf("parsing bundled seed: %v", err)
	}
	return list
}

func TestHalcomSigningCertificateIsPurposeSigningDespiteNoDigitalSignature(t *testing.T) {
	cert := loadCert(t, "halcom_signing.der")
	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		t.Fatal("test fixture assumption broken: halcom_signing.der has digitalSignature set")
	}
	info := Classify(cert, nil, true, true, referenceTime)
	if info.Purpose != PurposeSigning {
		t.Fatalf("Purpose = %v, want PurposeSigning (SPEC §11.4: Halcom omits digitalSignature)", info.Purpose)
	}
}

func TestHalcomAuthenticationCertificateIsPurposeAuthentication(t *testing.T) {
	cert := loadCert(t, "halcom_auth.der")
	info := Classify(cert, nil, true, true, referenceTime)
	if info.Purpose != PurposeAuthentication {
		t.Fatalf("Purpose = %v, want PurposeAuthentication", info.Purpose)
	}
}

// TestTwoHalcomCertificatesShareSubjectButDifferThumbprintAndPurpose is
// the direct test of SPEC §11.5: two certificates that are otherwise
// indistinguishable by subject must still be told apart.
func TestTwoHalcomCertificatesShareSubjectButDifferThumbprintAndPurpose(t *testing.T) {
	signing := Classify(loadCert(t, "halcom_signing.der"), nil, true, true, referenceTime)
	auth := Classify(loadCert(t, "halcom_auth.der"), nil, true, true, referenceTime)

	if !reflect.DeepEqual(signing.Subject, auth.Subject) {
		t.Fatalf("Subject differs between the two Halcom certificates: %+v vs %+v", signing.Subject, auth.Subject)
	}
	if signing.Thumbprint == auth.Thumbprint {
		t.Fatal("the two Halcom certificates produced the same thumbprint")
	}
	if signing.Purpose == auth.Purpose {
		t.Fatal("the two Halcom certificates produced the same Purpose")
	}
}

func TestMUPDisplayNameExcludesTrailingNumberAndSignSuffix(t *testing.T) {
	cert := loadCert(t, "mup_signing.der")
	info := Classify(cert, nil, true, true, referenceTime)

	if strings.Contains(info.Subject.DisplayName, "Sign") {
		t.Fatalf("DisplayName = %q, must not contain the literal CN suffix %q", info.Subject.DisplayName, "Sign")
	}
	if strings.ContainsAny(info.Subject.DisplayName, "0123456789") {
		t.Fatalf("DisplayName = %q, must not contain the CA number from CN", info.Subject.DisplayName)
	}
	want := "Вељко Станојевић"
	if info.Subject.DisplayName != want {
		t.Fatalf("DisplayName = %q, want %q", info.Subject.DisplayName, want)
	}
}

func TestIssuerAssignedIDIsCARSNeverPNORS(t *testing.T) {
	for _, f := range []string{"mup_signing.der", "posta_signing.der", "halcom_signing.der"} {
		cert := loadCert(t, f)
		info := Classify(cert, nil, true, true, referenceTime)
		if info.Subject.IssuerAssignedID == "" {
			t.Errorf("%s: IssuerAssignedID is empty, want the CA:RS- value", f)
		}
		if strings.Contains(info.Subject.IssuerAssignedID, "PNORS") {
			t.Errorf("%s: IssuerAssignedID = %q, leaked the PNORS value", f, info.Subject.IssuerAssignedID)
		}
	}
}

var thirteenDigits = regexp.MustCompile(`\d{13}`)

// TestNoFieldContainsThirteenConsecutiveDigits and
// TestNoFieldContainsAtSign are, per F1 §5.6, worth more than they look:
// cheap, unambiguous, and exactly the kind of regression a reviewer
// would not notice by eye (a JMBG or email address quietly surviving
// into a struct that gets displayed or logged).
func TestNoFieldContainsThirteenConsecutiveDigits(t *testing.T) {
	for _, f := range []string{"mup_signing.der", "posta_signing.der", "halcom_signing.der", "halcom_auth.der", "expired_signing.der"} {
		cert := loadCert(t, f)
		info := Classify(cert, nil, true, true, referenceTime)
		for name, v := range stringFields(info) {
			if thirteenDigits.MatchString(v) {
				t.Errorf("%s: field %s = %q contains 13 consecutive digits (a JMBG?)", f, name, v)
			}
		}
	}
}

func TestNoFieldContainsAtSign(t *testing.T) {
	for _, f := range []string{"mup_signing.der", "posta_signing.der", "halcom_signing.der", "halcom_auth.der", "expired_signing.der"} {
		cert := loadCert(t, f)
		info := Classify(cert, nil, true, true, referenceTime)
		for name, v := range stringFields(info) {
			if strings.Contains(v, "@") {
				t.Errorf("%s: field %s = %q contains '@' (an email address?)", f, name, v)
			}
		}
	}
}

// stringFields flattens every personal-data-bearing string field of Info
// (Subject's fields, plus IssuerCN) for the two blanket scrub tests
// above. Thumbprint is deliberately excluded: it is a SHA-1 hex digest,
// not personal data, and as effectively-random hex it can coincidentally
// contain 13 consecutive digits with no bearing on whether a JMBG leaked
// anywhere.
func stringFields(info Info) map[string]string {
	return map[string]string{
		"IssuerCN":                 info.IssuerCN,
		"Subject.DisplayName":      info.Subject.DisplayName,
		"Subject.GivenName":        info.Subject.GivenName,
		"Subject.Surname":          info.Subject.Surname,
		"Subject.CommonName":       info.Subject.CommonName,
		"Subject.Organisation":     info.Subject.Organisation,
		"Subject.TaxID":            info.Subject.TaxID,
		"Subject.CompanyID":        info.Subject.CompanyID,
		"Subject.IssuerAssignedID": info.Subject.IssuerAssignedID,
		"Subject.Country":          info.Subject.Country,
		"Subject.Locality":         info.Subject.Locality,
	}
}

func TestAllThreeIssuersAreQualifiedAgainstBundledTSL(t *testing.T) {
	list := bundledList(t)
	for _, f := range []string{"mup_signing.der", "posta_signing.der", "halcom_signing.der"} {
		cert := loadCert(t, f)
		info := Classify(cert, list, true, true, referenceTime)
		if info.Qualification != QualificationQualified {
			t.Errorf("%s: Qualification = %v, want QualificationQualified", f, info.Qualification)
		}
	}
}

func TestSelfSignedCertificateIsNotQualified(t *testing.T) {
	list := bundledList(t)
	cert := loadCert(t, "selfsigned_unrelated.der")
	info := Classify(cert, list, false, true, referenceTime)
	if info.Qualification != QualificationNotQualified {
		t.Fatalf("Qualification = %v, want QualificationNotQualified", info.Qualification)
	}
}

func TestNilTSLListYieldsUnknownQualification(t *testing.T) {
	cert := loadCert(t, "mup_signing.der")
	info := Classify(cert, nil, true, true, referenceTime)
	if info.Qualification != QualificationUnknown {
		t.Fatalf("Qualification = %v, want QualificationUnknown when no list is available", info.Qualification)
	}
}

func TestExpiredCertificateIsNotUsable(t *testing.T) {
	cert := loadCert(t, "expired_signing.der")
	info := Classify(cert, nil, true, true, referenceTime)
	if info.Usable {
		t.Fatal("Usable = true for an expired certificate")
	}
	if info.NotUsableReason != errs.CodeCertExpired {
		t.Fatalf("NotUsableReason = %q, want %q", info.NotUsableReason, errs.CodeCertExpired)
	}
}

func TestHardwareCertificateWithNoCardIsNotUsable(t *testing.T) {
	cert := loadCert(t, "mup_signing.der")
	info := Classify(cert, nil, true, false, referenceTime) // onHardware=true, hardwarePresent=false
	if info.Usable {
		t.Fatal("Usable = true with the card absent")
	}
	if info.NotUsableReason != errs.CodeCardNotPresent {
		t.Fatalf("NotUsableReason = %q, want %q", info.NotUsableReason, errs.CodeCardNotPresent)
	}
}

func TestAuthenticationCertificateIsNeverUsableForSigning(t *testing.T) {
	cert := loadCert(t, "halcom_auth.der")
	info := Classify(cert, nil, true, true, referenceTime)
	if info.Usable {
		t.Fatal("Usable = true for an authentication-purpose certificate")
	}
	if info.NotUsableReason != errs.CodeCertNotUsable {
		t.Fatalf("NotUsableReason = %q, want %q", info.NotUsableReason, errs.CodeCertNotUsable)
	}
}

func TestUsableSigningCertificateWithCardPresent(t *testing.T) {
	cert := loadCert(t, "mup_signing.der")
	info := Classify(cert, nil, true, true, referenceTime)
	if !info.Usable {
		t.Fatalf("Usable = false, want true (NotUsableReason=%q)", info.NotUsableReason)
	}
	if info.NotUsableReason != "" {
		t.Fatalf("NotUsableReason = %q, want empty", info.NotUsableReason)
	}
}

func TestQcStatementRecognition(t *testing.T) {
	mup := loadCert(t, "mup_signing.der")
	if !hasEIDASPolicyOID(mup) {
		t.Error("mup_signing.der: hasEIDASPolicyOID = false, want true")
	}
	if !hasQcSSCD(mup) {
		t.Error("mup_signing.der: hasQcSSCD = false, want true")
	}
	if !hasQcCompliance(mup) {
		t.Error("mup_signing.der: hasQcCompliance = false, want true")
	}
	if !isESign(mup) {
		t.Error("mup_signing.der: isESign = false, want true")
	}
	if !hasNaturalPersonSyntax(mup) {
		t.Error("mup_signing.der: hasNaturalPersonSyntax = false, want true")
	}
	if IsESeal(mup) {
		t.Error("mup_signing.der: IsESeal = true, want false (it asserts esign)")
	}
	if got := issuerPolicyLabel(mup); got != "MUP" {
		t.Errorf("issuerPolicyLabel(mup) = %q, want %q", got, "MUP")
	}

	halcom := loadCert(t, "halcom_signing.der")
	if !hasQcPDS(halcom) {
		t.Error("halcom_signing.der: hasQcPDS = false, want true (Halcom-only statement)")
	}
	if got := issuerPolicyLabel(halcom); got != "Halcom" {
		t.Errorf("issuerPolicyLabel(halcom) = %q, want %q", got, "Halcom")
	}

	posta := loadCert(t, "posta_signing.der")
	if hasQcPDS(posta) {
		t.Error("posta_signing.der: hasQcPDS = true, want false (QcPDS is Halcom-only per SPEC §11.2)")
	}
	if got := issuerPolicyLabel(posta); got != "Posta Srbije" {
		t.Errorf("issuerPolicyLabel(posta) = %q, want %q", got, "Posta Srbije")
	}
}
