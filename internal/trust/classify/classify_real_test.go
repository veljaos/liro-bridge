package classify

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// realCertsDir holds real certificates extracted from actual cards — never
// committed, because they carry personal data (SPEC §6.7/§11.6). See
// testdata/certs/local/README.md. These tests exist because a synthetic
// fixture (testdata/certs, used by classify_test.go) encodes the same
// understanding of a CA's quirks that the implementation does; a shared
// wrong assumption between the two still produces a passing test. Only a
// real certificate from the real issuer can contradict that assumption.
const realCertsDir = "../../../testdata/certs/local"

// realCertFiles lists the .der files placed in realCertsDir, skipping the
// calling test with a clear message when the directory does not exist or
// is empty of them — the condition that keeps `go test ./...` green with
// no real certificates on hand.
func realCertFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(realCertsDir)
	if os.IsNotExist(err) {
		t.Skipf("%s does not exist: no real certificates available for this test — see %s/README.md", realCertsDir, realCertsDir)
	}
	if err != nil {
		t.Fatalf("reading %s: %v", realCertsDir, err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".der") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		t.Skipf("%s contains no .der files: no real certificates available for this test — see %s/README.md", realCertsDir, realCertsDir)
	}
	return files
}

func loadRealCert(t *testing.T, filename string) *x509.Certificate {
	t.Helper()
	der, err := os.ReadFile(filepath.Join(realCertsDir, filename))
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing %s: %v", filename, err)
	}
	return cert
}

// requireRealCert skips the test unless realCertsDir contains exactly
// filename (the naming convention suggested by
// testdata/certs/local/README.md). Presence of the directory alone is not
// enough for the tests below that need a specific real certificate.
func requireRealCert(t *testing.T, filename string) *x509.Certificate {
	t.Helper()
	realCertFiles(t) // directory-level skip, with its own message, first
	if _, err := os.Stat(filepath.Join(realCertsDir, filename)); os.IsNotExist(err) {
		t.Skipf("%s not found in %s: skipping (see %s/README.md for the expected naming)", filename, realCertsDir, realCertsDir)
	}
	return loadRealCert(t, filename)
}

func TestRealHalcomSigningCertificateIsPurposeSigningDespiteNoDigitalSignature(t *testing.T) {
	cert := requireRealCert(t, "halcom_signing.der")
	if cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 {
		t.Skip("this real certificate has digitalSignature set — cannot exercise SPEC §11.4's exception with it")
	}
	info := Classify(cert, nil, true, true, referenceTime)
	if info.Purpose != PurposeSigning {
		t.Fatalf("Purpose = %v, want PurposeSigning (SPEC §11.4: Halcom omits digitalSignature)", info.Purpose)
	}
}

func TestRealHalcomSigningAndAuthPairShareSubjectDifferThumbprintAndPurpose(t *testing.T) {
	realCertFiles(t)
	signingCert := requireRealCert(t, "halcom_signing.der")
	authCert := requireRealCert(t, "halcom_auth.der")

	signing := Classify(signingCert, nil, true, true, referenceTime)
	auth := Classify(authCert, nil, true, true, referenceTime)

	if !reflect.DeepEqual(signing.Subject, auth.Subject) {
		t.Fatalf("Subject differs between the paired real Halcom certificates: %+v vs %+v", signing.Subject, auth.Subject)
	}
	if signing.Thumbprint == auth.Thumbprint {
		t.Fatal("the paired real Halcom certificates produced the same thumbprint")
	}
	if signing.Purpose == auth.Purpose {
		t.Fatal("the paired real Halcom certificates produced the same Purpose")
	}
}

func TestRealMUPDisplayNameExcludesTrailingNumberAndSignSuffix(t *testing.T) {
	cert := requireRealCert(t, "mup_signing.der")
	info := Classify(cert, nil, true, true, referenceTime)

	if strings.Contains(info.Subject.DisplayName, "Sign") {
		t.Fatalf("DisplayName = %q, must not contain the literal CN suffix %q", info.Subject.DisplayName, "Sign")
	}
	if strings.ContainsAny(info.Subject.DisplayName, "0123456789") {
		t.Fatalf("DisplayName = %q, must not contain the trailing CA number from CN", info.Subject.DisplayName)
	}
}

func TestRealCertificatesIssuerAssignedIDIsCARSNeverPNORS(t *testing.T) {
	for _, f := range realCertFiles(t) {
		cert := loadRealCert(t, f)
		info := Classify(cert, nil, true, true, referenceTime)
		if info.Subject.IssuerAssignedID == "" {
			t.Errorf("%s: IssuerAssignedID is empty, want the CA:RS- value", f)
		}
		if strings.Contains(info.Subject.IssuerAssignedID, "PNORS") {
			t.Errorf("%s: IssuerAssignedID = %q, leaked the PNORS value", f, info.Subject.IssuerAssignedID)
		}
	}
}

func TestRealCertificatesNoFieldContainsThirteenConsecutiveDigits(t *testing.T) {
	for _, f := range realCertFiles(t) {
		cert := loadRealCert(t, f)
		info := Classify(cert, nil, true, true, referenceTime)
		for name, v := range stringFields(info) {
			if thirteenDigits.MatchString(v) {
				t.Errorf("%s: field %s = %q contains 13 consecutive digits (a JMBG?)", f, name, v)
			}
		}
	}
}

func TestRealCertificatesNoFieldContainsAtSign(t *testing.T) {
	for _, f := range realCertFiles(t) {
		cert := loadRealCert(t, f)
		info := Classify(cert, nil, true, true, referenceTime)
		for name, v := range stringFields(info) {
			if strings.Contains(v, "@") {
				t.Errorf("%s: field %s = %q contains '@' (an email address?)", f, name, v)
			}
		}
	}
}
