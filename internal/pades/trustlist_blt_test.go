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
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
)

// The Pošta card's shape (D-274, D-281), built with stand-ins: the
// token carries the signer and the root, not the intermediate between
// them, and the signer's AIA names that intermediate only over LDAP.
// Its revocation is HTTP. The national Trusted List carries the
// intermediate (read in the bundled seed, D-466), and production hands
// the list's CA certificates to SignDocument as TrustStore
// (interactive.go).
//
// So B-LT should be reached with the intermediate taken from the trust
// store as an anchor, nothing fetched for it, and no LDAP. The control
// is the same signature without the trust store, which must not reach
// B-LT: that is what makes the trust store, and not something else in
// the fixture, the reason.
type postaShape struct {
	root, intermediate *x509.Certificate
	session            *chainedSession
	tsa                *tsa.Client

	mu        sync.Mutex
	aiaAsked  []string
	ocspAsked map[string]int
}

func newPostaShape(t *testing.T) *postaShape {
	t.Helper()
	p := &postaShape{ocspAsked: map[string]int{}}
	root, rootKey := newTestCA(t)
	p.root = root

	interKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p.intermediate = issue(t, &x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: "stand-in Pošta Srbije CA 1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}, &interKey.PublicKey, root, rootKey)

	ocspSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		req, err := ocsp.ParseRequest(buf.Bytes())
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		p.mu.Lock()
		p.ocspAsked[req.SerialNumber.String()]++
		p.mu.Unlock()
		der, err := ocsp.CreateResponse(p.intermediate, p.intermediate, ocsp.Response{
			Status:       ocsp.Good,
			SerialNumber: req.SerialNumber,
			ThisUpdate:   time.Now(),
			NextUpdate:   time.Now().Add(time.Hour),
		}, interKey)
		if err != nil {
			t.Errorf("ocsp.CreateResponse: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = w.Write(der)
	}))
	t.Cleanup(ocspSrv.Close)

	ldapAIA := []string{"ldap://ldap-ocsp.example.invalid/CN=stand-in%20CA%201?cACertificate;binary"}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	leaf := issue(t, &x509.Certificate{
		SerialNumber:          big.NewInt(11),
		Subject:               pkix.Name{CommonName: "stand-in Pošta signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageContentCommitment,
		OCSPServer:            []string{ocspSrv.URL},
		IssuingCertificateURL: ldapAIA,
	}, &leafKey.PublicKey, p.intermediate, interKey)
	// The token's own chain is the root alone, as on the card.
	p.session = &chainedSession{cert: leaf, key: leafKey, caDER: root.Raw}

	tsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tsaCert := issue(t, &x509.Certificate{
		SerialNumber:          big.NewInt(12),
		Subject:               pkix.Name{CommonName: "stand-in Pošta Srbije TSA 1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping},
		OCSPServer:            []string{ocspSrv.URL},
		IssuingCertificateURL: ldapAIA,
	}, &tsaKey.PublicKey, p.intermediate, interKey)
	// Two seconds back, for the reason newBLTFixture gives.
	genTime := time.Now().Add(-2 * time.Second)
	ts := fakeTSAServerWithCerts(t, func([]byte) time.Time { return genTime }, [][]byte{tsaCert.Raw})
	t.Cleanup(ts.Close)
	p.tsa = tsa.NewClient(ts.URL, tsa.Auth{})
	return p
}

func issue(t *testing.T, tmpl *x509.Certificate, pub *rsa.PublicKey, parent *x509.Certificate, parentKey *rsa.PrivateKey) *x509.Certificate {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, parentKey)
	if err != nil {
		t.Fatalf("CreateCertificate(%s): %v", tmpl.Subject.CommonName, err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (p *postaShape) sign(t *testing.T, trustStore []*x509.Certificate) (*Result, error) {
	t.Helper()
	return SignDocument(context.Background(), buildMinimalPDF(t), p.session, Options{
		RequestedLevel: LevelBLT,
		TSA:            p.tsa,
		TrustStore:     trustStore,
		AIAFetch: func(_ context.Context, url string) ([]byte, error) {
			p.mu.Lock()
			p.aiaAsked = append(p.aiaAsked, url)
			p.mu.Unlock()
			return nil, errors.New("this test fetches nothing by AIA")
		},
	})
}

func TestThePostaShapeReachesBLTThroughTheTrustedListWithoutLDAP(t *testing.T) {
	p := newPostaShape(t)
	result, err := p.sign(t, []*x509.Certificate{p.intermediate})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}
	if result.AchievedLevel != LevelBLT {
		t.Fatalf("AchievedLevel = %s, want B-LT with the intermediate in the trust store (notes: %v)", result.AchievedLevel, result.Notes)
	}
	if len(p.aiaAsked) != 0 {
		t.Errorf("AIA was asked for %v; the trust store should have answered first", p.aiaAsked)
	}
	if n := p.ocspAsked[p.intermediate.SerialNumber.String()]; n != 0 {
		t.Errorf("the intermediate's revocation was asked %d times; an anchor needs none", n)
	}
	if got := len(dssArray(t, result.Bytes, "OCSPs")); got != 2 {
		t.Errorf("/DSS carries %d OCSP responses, want 2: the signer's and the timestamp authority's", got)
	}
}

func TestThePostaShapeWithoutTheTrustedListDoesNotReachBLT(t *testing.T) {
	p := newPostaShape(t)
	result, err := p.sign(t, nil)
	if err == nil && result.AchievedLevel == LevelBLT {
		t.Fatalf("B-LT without the trust store: the fixture does not depend on it, so the test above proves nothing (notes: %v)", result.Notes)
	}
	// Since C26 (D-469) a B-LT that falls short is refused rather than
	// kept at B-T, so the control's answer is that refusal.
	var coded *errs.Error
	if !errors.As(err, &coded) || coded.Code != errs.CodeLevelNotReached {
		t.Fatalf("without the trust store: %v; want LEVEL_NOT_REACHED", err)
	}
	t.Logf("without the trust store: %v", err)
}
