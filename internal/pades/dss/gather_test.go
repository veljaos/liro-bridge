package dss

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"

	"github.com/veljaos/liro-bridge/internal/pades/pdf"
)

type testCA struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func newCA(t *testing.T, name string, parent *testCA) *testCA {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &testCA{cert: c, key: key}
}

func (ca *testCA) issue(t *testing.T, serial int64, tmpl x509.Certificate) *x509.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl.SerialNumber = big.NewInt(serial)
	if tmpl.Subject.CommonName == "" {
		tmpl.Subject = pkix.Name{CommonName: "leaf"}
	}
	tmpl.NotBefore, tmpl.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (ca *testCA) crl(t *testing.T, signer *testCA, revoked ...*big.Int) []byte {
	t.Helper()
	var entries []x509.RevocationListEntry
	for _, s := range revoked {
		entries = append(entries, x509.RevocationListEntry{SerialNumber: s, RevocationTime: time.Now().Add(-time.Minute)})
	}
	if signer == nil {
		signer = ca
	}
	der, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number:                    big.NewInt(1),
		ThisUpdate:                time.Now(),
		NextUpdate:                time.Now().Add(time.Hour),
		RevokedCertificateEntries: entries,
	}, signer.cert, signer.key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func serve(t *testing.T, body func() []byte, hits *int32) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		_, _ = w.Write(body())
	}))
	t.Cleanup(s.Close)
	return s
}

// A CRL the issuer signed and that lists the certificate says revoked;
// the evidence is kept as the proof.
func TestACRLListingTheSerialSaysRevoked(t *testing.T) {
	ca := newCA(t, "CA", nil)
	var url string
	srv := serve(t, func() []byte { return ca.crl(t, nil, big.NewInt(7)) }, nil)
	url = srv.URL
	leaf := ca.issue(t, 7, x509.Certificate{CRLDistributionPoints: []string{url}})
	e := collectOne(context.Background(), leaf, ca.cert, time.Time{}, DefaultMaxArtefactSize, nil)
	if len(e.CRL) == 0 || !e.Revoked {
		t.Fatalf("CRL %d bytes, revoked %v; want the CRL and revoked", len(e.CRL), e.Revoked)
	}
}

// A CRL signed by anyone but the issuer is not evidence and is not
// embedded.
func TestACRLSignedBySomeoneElseIsNotUsed(t *testing.T) {
	ca := newCA(t, "CA", nil)
	other := newCA(t, "CA", nil) // the same name, another key
	srv := serve(t, func() []byte { return ca.crl(t, other) }, nil)
	leaf := ca.issue(t, 7, x509.Certificate{CRLDistributionPoints: []string{srv.URL}})
	if e := collectOne(context.Background(), leaf, ca.cert, time.Time{}, DefaultMaxArtefactSize, nil); len(e.CRL) != 0 {
		t.Fatalf("a CRL signed by another key was used (%d bytes)", len(e.CRL))
	}
}

// An OCSP response older than the timestamp is asked for once more; the
// second, fresh one is used.
func TestAStaleOCSPResponseIsAskedForOnceMore(t *testing.T) {
	ca := newCA(t, "CA", nil)
	var hits int32
	var leaf *x509.Certificate
	genTime := time.Now().Add(-10 * time.Second)
	srv := serve(t, func() []byte {
		this := time.Now()
		if atomic.LoadInt32(&hits) == 1 {
			this = genTime.Add(-time.Minute) // the first answer: cached, older than the signature
		}
		der, err := ocsp.CreateResponse(ca.cert, ca.cert, ocsp.Response{
			Status: ocsp.Good, SerialNumber: leaf.SerialNumber, ThisUpdate: this, NextUpdate: time.Now().Add(time.Hour),
		}, ca.key)
		if err != nil {
			t.Error(err)
		}
		return der
	}, &hits)
	leaf = ca.issue(t, 9, x509.Certificate{OCSPServer: []string{srv.URL}})
	e := collectOne(context.Background(), leaf, ca.cert, genTime, DefaultMaxArtefactSize, nil)
	if len(e.OCSPResponse) == 0 || e.Stale {
		t.Fatalf("response %d bytes, stale %v; want a fresh one", len(e.OCSPResponse), e.Stale)
	}
	if hits != 2 {
		t.Fatalf("the responder was asked %d times, want 2", hits)
	}
}

// An OCSP answer of Unknown proves nothing and is not embedded.
func TestAnUnknownOCSPAnswerIsNotEvidence(t *testing.T) {
	ca := newCA(t, "CA", nil)
	var leaf *x509.Certificate
	srv := serve(t, func() []byte {
		der, _ := ocsp.CreateResponse(ca.cert, ca.cert, ocsp.Response{
			Status: ocsp.Unknown, SerialNumber: leaf.SerialNumber, ThisUpdate: time.Now(), NextUpdate: time.Now().Add(time.Hour),
		}, ca.key)
		return der
	}, nil)
	leaf = ca.issue(t, 11, x509.Certificate{OCSPServer: []string{srv.URL}})
	if e := collectOne(context.Background(), leaf, ca.cert, time.Time{}, DefaultMaxArtefactSize, nil); e.found() {
		t.Fatal("an Unknown answer was taken as evidence")
	}
}

// An ldap:// address is never dialled, and is not remembered as silent:
// it was not asked.
func TestAnLDAPAddressIsNotDialled(t *testing.T) {
	ca := newCA(t, "CA", nil)
	leaf := ca.issue(t, 12, x509.Certificate{
		OCSPServer:            []string{"ldap://ldap-ocsp.example/cn=x"},
		CRLDistributionPoints: []string{"ldap://ldap-ocsp.example/cn=x?certificateRevocationList;binary"},
	})
	mem := NewEndpointMemory()
	if e := collectOne(context.Background(), leaf, ca.cert, time.Time{}, DefaultMaxArtefactSize, mem); e.found() {
		t.Fatal("evidence from an ldap address")
	}
	if mem.SilentEndpoints() != 0 {
		t.Fatalf("%d ldap addresses remembered as silent; none was asked", mem.SilentEndpoints())
	}
}

// The issuer comes from AIA when nothing at hand signed the certificate;
// the anchor in the trust store is embedded and asked nothing.
func TestGatherFindsIssuersAndStopsAtTheAnchor(t *testing.T) {
	root := newCA(t, "Root", nil)
	inter := newCA(t, "Intermediate", root)
	var interCRLHits, rootHits int32
	interCRL := serve(t, func() []byte { return inter.crl(t, nil) }, &interCRLHits)
	aia := serve(t, func() []byte { return inter.cert.Raw }, nil)
	leaf := inter.issue(t, 5, x509.Certificate{
		CRLDistributionPoints: []string{interCRL.URL},
		IssuingCertificateURL: []string{aia.URL},
	})
	m := Gather(context.Background(), GatherRequest{
		Signer:     leaf,
		TrustStore: []*x509.Certificate{inter.cert},
		FetchIssuer: func(ctx context.Context, url string) ([]byte, error) {
			atomic.AddInt32(&rootHits, 1)
			return nil, nil
		},
	})
	if !m.Complete {
		t.Fatalf("not complete; missing %d", len(m.Missing))
	}
	if len(m.CRLs) != 1 || len(m.Certs) != 2 {
		t.Fatalf("%d CRLs, %d certs; want 1 and 2 (the leaf and its anchor)", len(m.CRLs), len(m.Certs))
	}
	if rootHits != 0 {
		t.Fatalf("AIA was fetched %d times for an issuer the trust store had", rootHits)
	}

	// Without the trust store the intermediate comes from AIA, and is
	// then itself in need of evidence — none here, so incomplete.
	m = Gather(context.Background(), GatherRequest{
		Signer:      leaf,
		FetchIssuer: func(ctx context.Context, url string) ([]byte, error) { return inter.cert.Raw, nil },
	})
	if len(m.Certs) < 2 || !bytes.Equal(m.Certs[1].Raw, inter.cert.Raw) {
		t.Fatalf("the intermediate from AIA is not among the certificates")
	}
	if m.Complete {
		t.Fatal("complete, although the intermediate fetched from AIA has no evidence and no anchor above it was given")
	}
}

// A delegated responder's certificate is embedded and needs evidence of
// its own; with ocsp-nocheck it does not.
func TestGatherFollowsADelegatedResponder(t *testing.T) {
	for _, nocheck := range []bool{false, true} {
		root := newCA(t, "Root", nil)
		tmpl := x509.Certificate{
			Subject:     pkix.Name{CommonName: "Responder"},
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning},
		}
		if nocheck {
			tmpl.ExtraExtensions = []pkix.Extension{{Id: oidOCSPNoCheck, Value: []byte{0x05, 0x00}}}
		}
		rkey, _ := rsa.GenerateKey(rand.Reader, 2048)
		tmpl.SerialNumber = big.NewInt(40)
		tmpl.NotBefore, tmpl.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
		rder, err := x509.CreateCertificate(rand.Reader, &tmpl, root.cert, &rkey.PublicKey, root.key)
		if err != nil {
			t.Fatal(err)
		}
		responder, _ := x509.ParseCertificate(rder)
		var leaf *x509.Certificate
		srv := serve(t, func() []byte {
			der, err := ocsp.CreateResponse(root.cert, responder, ocsp.Response{
				Status: ocsp.Good, SerialNumber: leaf.SerialNumber, ThisUpdate: time.Now(), NextUpdate: time.Now().Add(time.Hour),
				Certificate: responder,
			}, rkey)
			if err != nil {
				t.Error(err)
			}
			return der
		}, nil)
		leaf = root.issue(t, 6, x509.Certificate{OCSPServer: []string{srv.URL}})
		m := Gather(context.Background(), GatherRequest{Signer: leaf, TrustStore: []*x509.Certificate{root.cert}})
		inCerts := false
		for _, c := range m.Certs {
			inCerts = inCerts || bytes.Equal(c.Raw, responder.Raw)
		}
		switch {
		case !nocheck && (!inCerts || m.Complete):
			t.Errorf("without nocheck: responder embedded %v, complete %v; want embedded and incomplete (it has no evidence of its own)", inCerts, m.Complete)
		case nocheck && (inCerts || !m.Complete):
			t.Errorf("with nocheck: responder embedded %v, complete %v; want not followed and complete", inCerts, m.Complete)
		}
	}
}

// A second signature's /DSS keeps the first one's: its arrays and its
// /VRI entry, and an artefact both share is written once.
func TestApplyMaterialKeepsTheDSSAlreadyThere(t *testing.T) {
	ca := newCA(t, "CA", nil)
	leaf := ca.issue(t, 3, x509.Certificate{})
	crl := ca.crl(t, nil)
	doc, err := pdf.Parse(buildMinimalPDF(t))
	if err != nil {
		t.Fatal(err)
	}
	first, err := ApplyMaterial(doc, []byte("cms one"), Material{Certs: []*x509.Certificate{leaf, ca.cert}, CRLs: [][]byte{crl}, Complete: true})
	if err != nil {
		t.Fatal(err)
	}
	doc2, err := pdf.Parse(first.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	other := ca.issue(t, 4, x509.Certificate{})
	second, err := ApplyMaterial(doc2, []byte("cms two"), Material{Certs: []*x509.Certificate{other, ca.cert}, CRLs: [][]byte{crl}, Complete: true})
	if err != nil {
		t.Fatal(err)
	}
	out, err := pdf.Parse(second.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := out.ResolveDict(out.Trailer().Get(pdf.Name("Root")))
	d, ok := out.ResolveDict(root.Get(pdf.Name("DSS")))
	if !ok {
		t.Fatal("no /DSS")
	}
	certs, _ := out.Resolve(d.Get(pdf.Name("Certs"))).(pdf.Array)
	crls, _ := out.Resolve(d.Get(pdf.Name("CRLs"))).(pdf.Array)
	vri, _ := out.ResolveDict(d.Get(pdf.Name("VRI")))
	if len(certs) != 3 {
		t.Errorf("/Certs has %d, want 3 (leaf, CA, the second leaf — the CA once)", len(certs))
	}
	if len(crls) != 1 {
		t.Errorf("/CRLs has %d, want the shared CRL once", len(crls))
	}
	if len(vri) != 2 {
		t.Errorf("/VRI has %d entries, want both signatures'", len(vri))
	}
}

// When a certificate gets no evidence, the log names it by thumbprint
// (never by subject, SPEC §18.3), with its role and where its evidence
// was asked for: card row 1.1 is read from the log (D-477). Before,
// only "OCSP/CRL unavailable for at least one certificate" reached
// anyone.
func TestTheLogNamesTheCertificateThatGotNoEvidence(t *testing.T) {
	var logBuf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))

	ca := newCA(t, "Anchor", nil)
	refused := httptest.NewServer(http.NotFoundHandler())
	refusedURL := refused.URL + "/ocsp"
	refused.Close()
	garbage := serve(t, func() []byte { return []byte("not a CRL") }, nil)
	leaf := ca.issue(t, 21, x509.Certificate{
		Subject:               pkix.Name{CommonName: "Holder Without Evidence"},
		OCSPServer:            []string{refusedURL},
		CRLDistributionPoints: []string{garbage.URL},
	})
	m := Gather(context.Background(), GatherRequest{Signer: leaf, TrustStore: []*x509.Certificate{ca.cert}})
	if m.Complete {
		t.Fatal("complete, although the leaf's responder is closed and its CRL is not one")
	}
	got := logBuf.String()
	for _, want := range []string{"no OCSP response or CRL could be had", "thumbprint=" + thumbprint(leaf), "role=signer", refusedURL, garbage.URL} {
		if !strings.Contains(got, want) {
			t.Errorf("the log does not say %q; it says:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Holder Without Evidence") {
		t.Errorf("the log names the signer by subject, a person's name (SPEC §18.3):\n%s", got)
	}

	// A CRL over the cap is named too, with its size.
	logBuf.Reset()
	big := serve(t, func() []byte { return ca.crl(t, nil) }, nil)
	leaf = ca.issue(t, 22, x509.Certificate{
		Subject:               pkix.Name{CommonName: "Holder With A Large CRL"},
		CRLDistributionPoints: []string{big.URL},
	})
	m = Gather(context.Background(), GatherRequest{Signer: leaf, TrustStore: []*x509.Certificate{ca.cert}, MaxArtefactSize: 16})
	if !m.TooLarge {
		t.Fatal("a CRL over a 16-byte cap was not reported too large")
	}
	got = logBuf.String()
	for _, want := range []string{"larger than the cap", "thumbprint=" + thumbprint(leaf), "role=signer", "cap=16"} {
		if !strings.Contains(got, want) {
			t.Errorf("the log does not say %q; it says:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Holder With A Large CRL") {
		t.Errorf("the log names the signer by subject, a person's name (SPEC §18.3):\n%s", got)
	}
}
