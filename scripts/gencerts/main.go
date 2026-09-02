// Command gencerts builds the synthetic test certificates in
// testdata/certs/ used by internal/trust/classify's tests.
//
// These are NOT extracted from real cards: SPEC §11's structural facts
// (multi-valued RDNs, per-issuer KeyUsage, the personal-data traps) are
// public knowledge about how Serbian CAs build certificates, but the
// certificates a real card issues belong to a real person and were never
// available in this environment. Regenerating "real" fixtures was not an
// option, so this program builds synthetic certificates that reproduce
// the documented structure byte-for-byte where it matters (OIDs,
// multi-valued RDNs, KeyUsage), with fabricated names and identifiers.
// See docs/decisions.md and testdata/certs/README.md.
//
// The three "real" issuer trust anchors (MUP Gradjani CA 4, Posta Srbije
// CA 1, Halcom BG CA FL e-signature) ARE the genuine CA certificates
// published in the Republic of Serbia's real Trusted List — this program
// reads them out of internal/trust/tsl/seed/TSL-RS.xml and uses their
// real Subject bytes as the Issuer of each synthetic end-entity
// certificate, so the "qualified against the bundled TSL" test exercises
// classify's actual issuer-matching logic against real trust-anchor
// bytes, not a fabricated one. The synthetic certificates are not
// actually signed by those CAs (a throwaway key is used instead) —
// harmless, because F1 does not verify chain signatures; that arrives in
// F3.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	oidSerialNumber     = asn1.ObjectIdentifier{2, 5, 4, 5}
	oidGivenName        = asn1.ObjectIdentifier{2, 5, 4, 42}
	oidSurname          = asn1.ObjectIdentifier{2, 5, 4, 4}
	oidEmailAddress     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 1}
	oidQCPnQSCD         = asn1.ObjectIdentifier{0, 4, 0, 194112, 1, 2}
	oidPolicyMUP        = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 33589, 1, 1, 0}
	oidPolicyPosta      = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 15672, 10, 142, 1, 0}
	oidPolicyHalcom     = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 5939, 10, 1, 6}
	oidQCStatementsExt  = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}
	oidQcCompliance     = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}
	oidQcSSCD           = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}
	oidQcPDS            = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 5}
	oidQcTypeESign      = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 1}
	oidPKIXQCSyntaxV2   = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 11, 2}
	oidSemanticsNatural = asn1.ObjectIdentifier{0, 4, 0, 194121, 1, 1}
)

type qcStatement struct {
	StatementID asn1.ObjectIdentifier
	Info        asn1.RawValue `asn1:"optional"`
}

// semanticsInformation mirrors RFC 3739's SemanticsInformation, embedded
// as the Info of a PKIXQCSyntax-v2 qcStatement.
type semanticsInformation struct {
	SemanticsIdentifier asn1.ObjectIdentifier `asn1:"optional"`
}

func marshalQCStatements(statements []qcStatement) pkix.Extension {
	val, err := asn1.Marshal(statements)
	must(err)
	return pkix.Extension{Id: oidQCStatementsExt, Critical: false, Value: val}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// --- Reading the real CA certificates out of the bundled seed TSL ---

type seedDoc struct {
	Providers []struct {
		Services []struct {
			Certs []string `xml:"ServiceInformation>ServiceDigitalIdentity>DigitalId>X509Certificate"`
		} `xml:"TSPServices>TSPService"`
	} `xml:"TrustServiceProviderList>TrustServiceProvider"`
}

func loadRealCA(seedPath, fingerprintPrefix string) *x509.Certificate {
	raw, err := os.ReadFile(seedPath)
	must(err)
	var doc seedDoc
	must(xml.Unmarshal(raw, &doc))

	for _, p := range doc.Providers {
		for _, s := range p.Services {
			for _, c := range s.Certs {
				der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(c), ""))
				if err != nil {
					continue
				}
				sum := sha256.Sum256(der)
				fp := fmt.Sprintf("%X", sum[:])
				if strings.HasPrefix(fp, fingerprintPrefix) {
					cert, err := x509.ParseCertificate(der)
					must(err)
					return cert
				}
			}
		}
	}
	panic("real CA certificate not found for fingerprint prefix " + fingerprintPrefix)
}

// --- Certificate construction ---

type certSpec struct {
	filename string

	commonName        string
	givenName         string
	surname           string
	nationalID        string // PNORS-... — must never survive into classify.Subject
	issuerAssignedRef string // CA:RS-... — must become IssuerAssignedID
	emailInSubject    string // Halcom-style trap: emailAddress inside the DN
	emailInSAN        string // MUP/Posta-style trap: rfc822Name in SAN

	keyUsage x509.KeyUsage
	policies []asn1.ObjectIdentifier
	qc       []qcStatement

	issuer *x509.Certificate // real CA cert (Subject bytes reused as Issuer), or nil for self-signed
	// sameSubjectAs, if non-nil, forces this certificate to reuse
	// exactly the same Subject bytes as another spec's certificate —
	// reproducing SPEC §11.5's "two certificates, identical subject".
	sameSubjectAs *certSpec

	notBefore, notAfter time.Time

	// generatedSubject is filled in after generation so a later spec can
	// reuse it via sameSubjectAs.
	generatedSubject pkix.Name
	generatedRawSubj []byte
}

func buildSubject(s *certSpec) pkix.Name {
	if s.sameSubjectAs != nil {
		return s.sameSubjectAs.generatedSubject
	}
	var extra []pkix.AttributeTypeAndValue
	if s.givenName != "" {
		extra = append(extra, pkix.AttributeTypeAndValue{Type: oidGivenName, Value: s.givenName})
	}
	if s.surname != "" {
		extra = append(extra, pkix.AttributeTypeAndValue{Type: oidSurname, Value: s.surname})
	}
	if s.nationalID != "" || s.issuerAssignedRef != "" {
		// Deliberately PNORS before CA:RS: a parser that naively takes
		// the *first* serialNumber attribute value would get the
		// personal identifier here, not the CA reference (F1 §5.3 Trap
		// 1). Correct code must inspect the prefix of every value.
		extra = append(extra,
			pkix.AttributeTypeAndValue{Type: oidSerialNumber, Value: "PNORS-" + s.nationalID},
			pkix.AttributeTypeAndValue{Type: oidSerialNumber, Value: "CA:RS-" + s.issuerAssignedRef},
		)
	}
	if s.emailInSubject != "" {
		extra = append(extra, pkix.AttributeTypeAndValue{Type: oidEmailAddress, Value: s.emailInSubject})
	}
	return pkix.Name{
		Country:    []string{"RS"},
		CommonName: s.commonName,
		ExtraNames: extra,
	}
}

func generate(spec *certSpec, signingKey *rsa.PrivateKey, caKey *rsa.PrivateKey) []byte {
	subject := buildSubject(spec)

	tmpl := &x509.Certificate{
		SerialNumber:          randSerial(),
		Subject:               subject,
		NotBefore:             spec.notBefore,
		NotAfter:              spec.notAfter,
		KeyUsage:              spec.keyUsage,
		BasicConstraintsValid: true,
		Policies:              toOIDs(spec.policies),
	}
	if spec.emailInSAN != "" {
		tmpl.EmailAddresses = []string{spec.emailInSAN}
	}
	if len(spec.qc) > 0 {
		tmpl.ExtraExtensions = append(tmpl.ExtraExtensions, marshalQCStatements(spec.qc))
	}

	var parent *x509.Certificate
	var parentKey *rsa.PrivateKey
	if spec.issuer != nil {
		// x509.CreateCertificate insists priv's public half matches
		// parent.PublicKey, which the real CA's actual key obviously
		// would not (we do not have the Ministry's private key, nor
		// would we want it). A fake parent carrying the REAL issuer's
		// exact RawSubject bytes, but caKey's public key, satisfies
		// that check while still producing the real Issuer DN — the
		// only thing classify's F1 §5.4 issuer-matching logic reads.
		parent = &x509.Certificate{RawSubject: spec.issuer.RawSubject, PublicKey: &caKey.PublicKey}
		parentKey = caKey
	} else {
		parent = tmpl // self-signed
		parentKey = signingKey
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &signingKey.PublicKey, parentKey)
	must(err)

	parsed, err := x509.ParseCertificate(der)
	must(err)
	spec.generatedSubject = subject
	spec.generatedRawSubj = parsed.RawSubject
	return der
}

func toOIDs(ids []asn1.ObjectIdentifier) []x509.OID {
	out := make([]x509.OID, 0, len(ids))
	for _, id := range ids {
		oid, err := x509.OIDFromASN1OID(id)
		must(err)
		out = append(out, oid)
	}
	return out
}

func randSerial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	must(err)
	return n
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gencerts <seed-tsl-path> <output-dir>")
		os.Exit(2)
	}
	seedPath, outDir := os.Args[1], os.Args[2]
	must(os.MkdirAll(outDir, 0o755))

	mupCA := loadRealCA(seedPath, "0E3EFEB4F77F9511")
	postaCA := loadRealCA(seedPath, "20EC0DB0BC171A06")
	halcomCA := loadRealCA(seedPath, "8D56989132BC43F0")

	// One throwaway keypair used for every synthetic certificate's own
	// key, and re-used (nonsensically, but harmlessly) as the "issuing"
	// key when a real CA's Subject is borrowed as Issuer — F1 does not
	// verify chain signatures, only issuer-name matching (§5.4).
	entityKey, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)

	validFrom := time.Date(2021, 9, 23, 0, 0, 0, 0, time.UTC)
	validTo := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	longExpired := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	longExpiredEnd := time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)

	mupSigning := &certSpec{
		filename:          "mup_signing.der",
		commonName:        "ВЕЉКО СТАНОЈЕВИЋ 011445479 Sign",
		givenName:         "Вељко",
		surname:           "Станојевић",
		nationalID:        "0114454790123",
		issuerAssignedRef: "011445479",
		emailInSAN:        "veljko.stanojevic@example.rs",
		keyUsage:          x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		policies:          []asn1.ObjectIdentifier{oidQCPnQSCD, oidPolicyMUP},
		qc: []qcStatement{
			{StatementID: oidQcCompliance},
			{StatementID: oidQcSSCD},
			{StatementID: oidQcTypeESign},
			{StatementID: oidPKIXQCSyntaxV2, Info: naturalPersonInfo()},
		},
		issuer:    mupCA,
		notBefore: validFrom,
		notAfter:  validTo,
	}

	postaSigning := &certSpec{
		filename:          "posta_signing.der",
		commonName:        "Redžvel Mešković 200094362",
		givenName:         "Redžvel",
		surname:           "Mešković",
		nationalID:        "2000943620456",
		issuerAssignedRef: "200094362",
		emailInSAN:        "redzvel.meskovic@example.rs",
		keyUsage:          x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		policies:          []asn1.ObjectIdentifier{oidQCPnQSCD, oidPolicyPosta},
		qc: []qcStatement{
			{StatementID: oidQcCompliance},
			{StatementID: oidQcSSCD},
			{StatementID: oidQcTypeESign},
		},
		issuer:    postaCA,
		notBefore: validFrom,
		notAfter:  validTo,
	}

	halcomSigning := &certSpec{
		filename:          "halcom_signing.der",
		commonName:        "Zoran Milovanović 246275",
		givenName:         "Zoran",
		surname:           "Milovanović",
		nationalID:        "2462750789012",
		issuerAssignedRef: "246275",
		emailInSubject:    "zoran.milovanovic@example.rs", // trap 3: Halcom puts it in the DN
		keyUsage:          x509.KeyUsageContentCommitment, // trap: NOT digitalSignature (SPEC §11.4)
		policies:          []asn1.ObjectIdentifier{oidQCPnQSCD, oidPolicyHalcom},
		qc: []qcStatement{
			{StatementID: oidQcCompliance},
			{StatementID: oidQcSSCD},
			{StatementID: oidQcPDS}, // Halcom-only statement
			{StatementID: oidQcTypeESign},
		},
		issuer:    halcomCA,
		notBefore: validFrom,
		notAfter:  validTo,
	}

	halcomAuth := &certSpec{
		filename:      "halcom_auth.der",
		sameSubjectAs: halcomSigning, // SPEC §11.5: identical subject, different KeyUsage
		keyUsage:      x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		issuer:        halcomCA,
		notBefore:     validFrom,
		notAfter:      validTo,
	}

	selfSigned := &certSpec{
		filename:   "selfsigned_unrelated.der",
		commonName: "{3F2504E0-4F89-11D3-9A0C-0305E82C3301}", // mimics the Windows-internal GUID-subject certificates noted in F1 §3.5
		keyUsage:   0,                                        // no KeyUsage bits set at all: classify.Purpose == PurposeUnknown, matching F1 §5.4's real-world observation
		issuer:     nil,                                      // self-signed: no TSL service will ever match this issuer
		notBefore:  validFrom,
		notAfter:   validTo,
	}

	expired := &certSpec{
		filename:          "expired_signing.der",
		commonName:        "Test Expired 000000000",
		givenName:         "Test",
		surname:           "Expired",
		nationalID:        "0000000000000",
		issuerAssignedRef: "000000000",
		keyUsage:          x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		policies:          []asn1.ObjectIdentifier{oidQCPnQSCD, oidPolicyMUP},
		issuer:            mupCA,
		notBefore:         longExpired,
		notAfter:          longExpiredEnd,
	}

	specs := []*certSpec{mupSigning, postaSigning, halcomSigning, halcomAuth, selfSigned, expired}
	for _, spec := range specs {
		der := generate(spec, entityKey, caKey)
		outPath := filepath.Join(outDir, spec.filename)
		must(os.WriteFile(outPath, der, 0o644))
		fmt.Printf("wrote %s (%d bytes)\n", outPath, len(der))
	}

	if !subjectBytesEqual(halcomSigning, halcomAuth) {
		panic("halcom_signing and halcom_auth do not share identical Subject bytes")
	}
	fmt.Println("verified: halcom_signing and halcom_auth share identical Subject bytes")
}

func subjectBytesEqual(a, b *certSpec) bool {
	if len(a.generatedRawSubj) != len(b.generatedRawSubj) {
		return false
	}
	for i := range a.generatedRawSubj {
		if a.generatedRawSubj[i] != b.generatedRawSubj[i] {
			return false
		}
	}
	return true
}

func naturalPersonInfo() asn1.RawValue {
	val, err := asn1.Marshal(semanticsInformation{SemanticsIdentifier: oidSemanticsNatural})
	must(err)
	var raw asn1.RawValue
	_, err = asn1.Unmarshal(val, &raw)
	must(err)
	return raw
}
