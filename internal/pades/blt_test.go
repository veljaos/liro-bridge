package pades

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/pades/pdf"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
)

// bltFixture is a CA, a signer and a timestamp authority it issued, and
// one OCSP responder answering for both — what B-LT needs since
// [[D-462]]: evidence for the signer's chain and for the timestamp's.
type bltFixture struct {
	ca      *x509.Certificate
	caKey   *rsa.PrivateKey
	session *chainedSession
	tsaCert *x509.Certificate
	tsa     *tsa.Client

	mu      sync.Mutex
	revoked map[string]bool // serial → revoked
	asked   map[string]int  // serial → requests
}

func newBLTFixture(t *testing.T) *bltFixture {
	t.Helper()
	f := &bltFixture{revoked: map[string]bool{}, asked: map[string]int{}}
	f.ca, f.caKey = newTestCA(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		req, err := ocsp.ParseRequest(buf.Bytes())
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.asked[req.SerialNumber.String()]++
		revoked := f.revoked[req.SerialNumber.String()]
		f.mu.Unlock()
		tmpl := ocsp.Response{
			Status:       ocsp.Good,
			SerialNumber: req.SerialNumber,
			ThisUpdate:   time.Now(),
			NextUpdate:   time.Now().Add(time.Hour),
			Certificate:  f.ca,
		}
		if revoked {
			tmpl.Status = ocsp.Revoked
			tmpl.RevokedAt = time.Now().Add(-time.Hour)
			tmpl.RevocationReason = ocsp.KeyCompromise
		}
		der, err := ocsp.CreateResponse(f.ca, f.ca, tmpl, f.caKey)
		if err != nil {
			t.Errorf("ocsp.CreateResponse: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(der)
	}))
	t.Cleanup(srv.Close)

	f.session = newChainedSession(t, f.ca, f.caKey, []string{srv.URL}, nil)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "pades.SignDocument test TSA"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		OCSPServer:   []string{srv.URL},
	}, f.ca, &key.PublicKey, f.caKey)
	if err != nil {
		t.Fatal(err)
	}
	if f.tsaCert, err = x509.ParseCertificate(der); err != nil {
		t.Fatal(err)
	}
	// Two seconds back: an OCSP thisUpdate in the same second as the
	// genTime encodes as the same instant, which is not after it.
	genTime := time.Now().Add(-2 * time.Second)
	ts := fakeTSAServerWithCerts(t, func([]byte) time.Time { return genTime }, [][]byte{f.tsaCert.Raw})
	t.Cleanup(ts.Close)
	f.tsa = tsa.NewClient(ts.URL, tsa.Auth{})
	return f
}

func (f *bltFixture) revoke(c *x509.Certificate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revoked[c.SerialNumber.String()] = true
}

func (f *bltFixture) sign(t *testing.T) (*Result, error) {
	t.Helper()
	return SignDocument(context.Background(), buildMinimalPDF(t), f.session, Options{
		RequestedLevel: LevelBLT,
		TSA:            f.tsa,
	})
}

func dssArray(t *testing.T, signed []byte, key string) pdf.Array {
	t.Helper()
	doc, err := pdf.Parse(signed)
	if err != nil {
		t.Fatalf("re-parse signed output: %v", err)
	}
	root, _ := doc.ResolveDict(doc.Trailer().Get(pdf.Name("Root")))
	d, ok := doc.ResolveDict(root.Get(pdf.Name("DSS")))
	if !ok {
		return nil
	}
	arr, _ := doc.Resolve(d.Get(pdf.Name(key))).(pdf.Array)
	return arr
}

// B-LT is reached when the signer's chain and the timestamp's both have
// evidence: two OCSP responses, one per certificate, and the TSA's
// certificate among the embedded ones.
func TestSignDocumentBLTAchievedWithOCSPEvidence(t *testing.T) {
	f := newBLTFixture(t)
	result, err := f.sign(t)
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBLT {
		t.Fatalf("AchievedLevel = %s, want B-LT (notes: %v)", result.AchievedLevel, result.Notes)
	}
	if got := len(dssArray(t, result.Bytes, "OCSPs")); got != 2 {
		t.Errorf("/DSS/OCSPs has %d entries, want 2: the signer's and the timestamp authority's", got)
	}
	if got := len(dssArray(t, result.Bytes, "Certs")); got != 3 {
		t.Errorf("/DSS/Certs has %d entries, want 3: signer, CA, timestamp authority", got)
	}
	r := verifyResult(t, result.Bytes)
	if len(r.Errors) > 0 || !r.SignatureOK || !r.HasTimestamp || !r.TimestampOK {
		t.Fatalf("independent verification failed: %+v errors=%v", r, r.Errors)
	}
}

// The signer's certificate revoked: nothing is saved, and the document
// fails with CERT_REVOKED — the code's first producer (D4).
func TestARevokedSignerCertificateSavesNothing(t *testing.T) {
	f := newBLTFixture(t)
	f.revoke(f.session.cert)
	result, err := f.sign(t)
	var coded *errs.Error
	if !errors.As(err, &coded) || coded.Code != errs.CodeCertRevoked {
		level := Level("no result")
		if result != nil {
			level = result.AchievedLevel
		}
		t.Fatalf("SignDocument gave %s, %v; want CERT_REVOKED", level, err)
	}
	if result != nil {
		t.Fatalf("a result came back for a revoked signer: level %s", result.AchievedLevel)
	}
}

// The timestamp authority's certificate revoked: the signature is the
// signer's and stands at B-T, and B-LT is not claimed.
func TestARevokedTimestampAuthorityKeepsBTAndSaysWhy(t *testing.T) {
	f := newBLTFixture(t)
	f.revoke(f.tsaCert)
	result, err := f.sign(t)
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBT {
		t.Fatalf("AchievedLevel = %s, want B-T", result.AchievedLevel)
	}
	if !strings.Contains(strings.Join(result.Notes, " "), "revoked") {
		t.Errorf("the notes do not say why: %v", result.Notes)
	}
	if dssArray(t, result.Bytes, "OCSPs") != nil {
		t.Errorf("a /DSS was written for a timestamp whose authority is revoked")
	}
}

// A timestamp that does not carry its authority's certificate cannot
// have its chain checked, so B-LT is not reached.
func TestATimestampWithoutItsAuthorityCertificateStopsAtBT(t *testing.T) {
	f := newBLTFixture(t)
	result, err := SignDocument(context.Background(), buildMinimalPDF(t), f.session, Options{
		RequestedLevel: LevelBLT,
		TSA:            fakeTSAClientAt(t, time.Now().Add(-2*time.Second)),
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBT {
		t.Fatalf("AchievedLevel = %s, want B-T", result.AchievedLevel)
	}
}
