package dss

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

// buildMinimalPDF constructs, by hand with exact byte offsets, the
// smallest classic-xref PDF this test needs: a Catalog with no /Pages
// tree (Apply only ever touches the trailer's /Root dictionary).
func buildMinimalPDF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offset1 := buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 2\n")
	buf.WriteString("0000000000 65535 f \n")
	fmt.Fprintf(&buf, "%010d %05d n \n", offset1, 0)
	fmt.Fprintf(&buf, "trailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xrefStart)
	return buf.Bytes()
}

// buildTestChain returns a self-signed CA (used directly as "issuer" —
// good enough for OCSP/CRL construction, which only needs a signing
// key, not a full trust chain) and one end-entity certificate pointing
// its AIA/CRL DP extensions at ocspURL/crlURL.
func buildTestChain(t *testing.T, ocspURL, crlURL string) (ca *x509.Certificate, caKey *rsa.PrivateKey, leaf *x509.Certificate) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(CA): %v", err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("ParseCertificate(CA): %v", err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	var ocspURLs, crlURLs []string
	if ocspURL != "" {
		ocspURLs = []string{ocspURL}
	}
	if crlURL != "" {
		crlURLs = []string{crlURL}
	}
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Test Signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		OCSPServer:            ocspURLs,
		CRLDistributionPoints: crlURLs,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(leaf): %v", err)
	}
	leaf, err = x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("ParseCertificate(leaf): %v", err)
	}
	return ca, caKey, leaf
}

func ocspServer(t *testing.T, ca *x509.Certificate, caKey *rsa.PrivateKey, leaf *x509.Certificate) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respTemplate := ocsp.Response{
			Status:       ocsp.Good,
			SerialNumber: leaf.SerialNumber,
			ThisUpdate:   time.Now().Add(-time.Minute),
			NextUpdate:   time.Now().Add(time.Hour),
			Certificate:  ca,
		}
		der, err := ocsp.CreateResponse(ca, ca, respTemplate, caKey)
		if err != nil {
			t.Fatalf("ocsp.CreateResponse: %v", err)
		}
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(der)
	}))
}

func TestCollectRevocationPrefersOCSP(t *testing.T) {
	ca, caKey, leaf := buildTestChain(t, "", "")
	os := ocspServer(t, ca, caKey, leaf)
	defer os.Close()
	leaf.OCSPServer = []string{os.URL}

	entries := CollectRevocation(context.Background(), []*x509.Certificate{leaf, ca}, 0, nil)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (one per certificate, ca's left empty since it is the excluded root)", len(entries))
	}
	if len(entries[0].OCSPResponse) == 0 {
		t.Fatal("no OCSP response collected")
	}
	if len(entries[0].CRL) != 0 {
		t.Fatal("CRL collected even though OCSP succeeded")
	}
	if len(entries[1].OCSPResponse) != 0 || len(entries[1].CRL) != 0 {
		t.Fatal("evidence collected for the root, which has no issuer to check it against")
	}
}

func TestCollectRevocationFallsBackToCRL(t *testing.T) {
	ca, caKey, leaf := buildTestChain(t, "", "")
	cs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tmpl := &x509.RevocationList{
			Number:     big.NewInt(1),
			ThisUpdate: time.Now().Add(-time.Minute),
			NextUpdate: time.Now().Add(time.Hour),
		}
		der, err := x509.CreateRevocationList(rand.Reader, tmpl, ca, caKey)
		if err != nil {
			t.Fatalf("CreateRevocationList: %v", err)
		}
		_, _ = w.Write(der)
	}))
	defer cs.Close()
	leaf.CRLDistributionPoints = []string{cs.URL}

	entries := CollectRevocation(context.Background(), []*x509.Certificate{leaf, ca}, 0, nil)
	if len(entries) != 2 || len(entries[0].CRL) == 0 {
		t.Fatalf("entries = %#v, want a CRL for the one non-root certificate", entries)
	}
	if len(entries[0].OCSPResponse) != 0 {
		t.Fatal("no OCSP configured, but an OCSP response was recorded")
	}
}

func TestCollectRevocationNoEndpointsYieldsEmptyEntry(t *testing.T) {
	ca, _, leaf := buildTestChain(t, "", "")
	entries := CollectRevocation(context.Background(), []*x509.Certificate{leaf, ca}, 0, nil)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if len(entries[0].OCSPResponse) != 0 || len(entries[0].CRL) != 0 {
		t.Fatal("evidence collected despite no OCSP/CRL endpoints being configured")
	}
}

func buildPlaceholderDoc(t *testing.T) (*pdf.Document, []byte) {
	t.Helper()
	// A minimal synthetic PDF, reused from internal/pades/pdf's own
	// test fixtures via the package this test lives outside of: build
	// one inline instead, since fixtures_test.go's helpers are
	// unexported to that package's tests only.
	src := buildMinimalPDF(t)
	doc, err := pdf.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc, src
}

func TestApplyEmbedsDSSAndVRI(t *testing.T) {
	ca, caKey, leaf := buildTestChain(t, "", "")
	os := ocspServer(t, ca, caKey, leaf)
	defer os.Close()
	leaf.OCSPServer = []string{os.URL}

	doc, _ := buildPlaceholderDoc(t)
	entries := CollectRevocation(context.Background(), []*x509.Certificate{leaf, ca}, 0, nil)
	fakeCMS := []byte("fake cms bytes for VRI keying")

	result, err := Apply(doc, fakeCMS, []*x509.Certificate{leaf, ca}, entries)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !result.Complete {
		t.Fatal("Complete = false, want true (the one non-root certificate has OCSP evidence)")
	}

	doc2, err := pdf.Parse(result.Bytes)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	root, ok := doc2.ResolveDict(doc2.Trailer().Get(pdf.Name("Root")))
	if !ok {
		t.Fatal("/Root did not resolve")
	}
	dssDict, ok := doc2.ResolveDict(root.Get(pdf.Name("DSS")))
	if !ok {
		t.Fatal("/DSS did not resolve")
	}
	certs, ok := doc2.Resolve(dssDict.Get(pdf.Name("Certs"))).(pdf.Array)
	if !ok || len(certs) != 2 {
		t.Fatalf("/DSS/Certs = %#v, want 2 entries", dssDict.Get(pdf.Name("Certs")))
	}
	ocsps, ok := doc2.Resolve(dssDict.Get(pdf.Name("OCSPs"))).(pdf.Array)
	if !ok || len(ocsps) != 1 {
		t.Fatalf("/DSS/OCSPs = %#v, want 1 entry", dssDict.Get(pdf.Name("OCSPs")))
	}
	vri, ok := doc2.ResolveDict(dssDict.Get(pdf.Name("VRI")))
	if !ok {
		t.Fatal("/DSS/VRI did not resolve")
	}
	key := VRIKey(fakeCMS)
	if _, ok := vri[pdf.Name(key)]; !ok {
		t.Fatalf("/DSS/VRI has no entry for key %s", key)
	}
}

// TestCollectRevocationSkipsCRLLargerThanCap is Task 1b's core case: a
// CRL that is successfully downloaded and parses as valid is still not
// handed back for embedding once it exceeds maxArtefactSize — the
// discriminator is size, not validity. A small cap (rather than a real
// multi-megabyte CRL) keeps the test fast while exercising exactly the
// same comparison the real 30,136,214-byte MUP CRL tripped.
func TestCollectRevocationSkipsCRLLargerThanCap(t *testing.T) {
	ca, caKey, leaf := buildTestChain(t, "", "")
	cs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tmpl := &x509.RevocationList{
			Number:     big.NewInt(1),
			ThisUpdate: time.Now().Add(-time.Minute),
			NextUpdate: time.Now().Add(time.Hour),
		}
		der, err := x509.CreateRevocationList(rand.Reader, tmpl, ca, caKey)
		if err != nil {
			t.Fatalf("CreateRevocationList: %v", err)
		}
		_, _ = w.Write(der)
	}))
	defer cs.Close()
	leaf.CRLDistributionPoints = []string{cs.URL}

	// Fetch once with no cap to learn the real size of this test's CRL.
	uncapped := CollectRevocation(context.Background(), []*x509.Certificate{leaf, ca}, 0, nil)
	if len(uncapped[0].CRL) == 0 {
		t.Fatal("setup: expected a CRL to be embeddable with no cap")
	}
	realSize := int64(len(uncapped[0].CRL))

	entries := CollectRevocation(context.Background(), []*x509.Certificate{leaf, ca}, realSize-1, nil)
	if len(entries[0].CRL) != 0 {
		t.Fatal("CRL returned despite exceeding the cap")
	}
	if !entries[0].TooLarge {
		t.Fatal("TooLarge = false, want true (the CRL was fetched but exceeded the cap)")
	}
	if entries[0].SkippedBytes != realSize {
		t.Fatalf("SkippedBytes = %d, want %d (the CRL's actual size)", entries[0].SkippedBytes, realSize)
	}
}

// TestCollectRevocationDefaultCapIs5MB pins Task 1b's specific number so
// a future change to the default cannot drift silently.
func TestCollectRevocationDefaultCapIs5MB(t *testing.T) {
	if DefaultMaxArtefactSize != 5*1024*1024 {
		t.Fatalf("DefaultMaxArtefactSize = %d, want 5 MiB (Task 1b's measured default)", DefaultMaxArtefactSize)
	}
}

func TestApplyReportsTooLargeReason(t *testing.T) {
	ca, _, leaf := buildTestChain(t, "", "")
	doc, _ := buildPlaceholderDoc(t)
	entries := []Entry{
		{Certificate: leaf, TooLarge: true, SkippedBytes: 30136214},
		{Certificate: ca},
	}

	result, err := Apply(doc, []byte("cms"), []*x509.Certificate{leaf, ca}, entries)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Complete {
		t.Fatal("Complete = true, want false")
	}
	if !result.TooLarge {
		t.Fatal("TooLarge = false, want true (Task 1c needs to distinguish this from plain unavailability)")
	}
	if result.LargestSkippedBytes != 30136214 {
		t.Fatalf("LargestSkippedBytes = %d, want 30136214", result.LargestSkippedBytes)
	}
}

func TestApplyIncompleteWhenCollectionFails(t *testing.T) {
	ca, _, leaf := buildTestChain(t, "", "") // no OCSP/CRL configured
	doc, src := buildPlaceholderDoc(t)
	entries := CollectRevocation(context.Background(), []*x509.Certificate{leaf, ca}, 0, nil)

	result, err := Apply(doc, []byte("cms"), []*x509.Certificate{leaf, ca}, entries)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Complete {
		t.Fatal("Complete = true, want false (no evidence was collected for the signer certificate)")
	}
	if result.TooLarge {
		t.Fatal("TooLarge = true, want false (nothing was fetched at all here, so this is plain unavailability, not an oversized artefact)")
	}
	// D-079: no OCSP and no CRL for any certificate means nothing gets
	// embedded, so Apply must not write a revision at all — Bytes stays
	// doc's own original, unmodified bytes.
	if !bytes.Equal(result.Bytes, src) {
		t.Fatal("Bytes != doc's original bytes, want no revision written when there is no revocation evidence at all")
	}
}

// TestApplyWritesNoRevisionWhenAllEntriesAreTooLarge is D-079's other
// no-evidence case: every entry has OCSP/CRL evidence that was fetched
// but discarded for exceeding the size cap (TooLarge), not simply
// missing. A /DSS carrying only /Certs is exactly as unjustified in
// this case as in the plain-unavailability one, so Apply must still
// skip writing the revision.
func TestApplyWritesNoRevisionWhenAllEntriesAreTooLarge(t *testing.T) {
	ca, _, leaf := buildTestChain(t, "", "")
	doc, src := buildPlaceholderDoc(t)
	entries := []Entry{
		{Certificate: leaf, TooLarge: true, SkippedBytes: 30136214},
		{Certificate: ca},
	}

	result, err := Apply(doc, []byte("cms"), []*x509.Certificate{leaf, ca}, entries)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if result.Complete {
		t.Fatal("Complete = true, want false")
	}
	if !result.TooLarge {
		t.Fatal("TooLarge = false, want true")
	}
	if !bytes.Equal(result.Bytes, src) {
		t.Fatal("Bytes != doc's original bytes, want no revision written when the only evidence was too large to embed")
	}
}
