package dss

import (
	"context"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// deadEndpoint returns a URL that accepts nothing: a listener opened and
// closed again, so every connection to it fails immediately. That is the
// same *outcome* as MUP's real responder (which drops the connection and
// costs ocspTimeout per attempt) without spending twenty seconds per
// document in a unit test — what these tests measure is how many times
// the endpoint is contacted, which is the behaviour the memory changes.
// The twenty seconds themselves are ocspTimeout's, pinned by D-045/D-046
// and unchanged here.
func deadEndpoint(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	url := "http://" + l.Addr().String() + "/ocsp"
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return url
}

// countingServer answers every request with handler and counts them.
func countingServer(t *testing.T, count *int64, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(count, 1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestASilentOCSPResponderIsAskedOnceForTheWholeBatch is J-10's own
// case, in the form a test can hold: an endpoint that does not answer is
// asked for the first document and never again. Against the previous
// code every document paid the full attempt count, which on a real
// responder that accepts and hangs is 20 s each — 3m20s measured for
// ten documents.
func TestASilentOCSPResponderIsAskedOnceForTheWholeBatch(t *testing.T) {
	url := deadEndpoint(t)
	ca, _, leaf := buildTestChain(t, url, "")
	certs := []*x509.Certificate{leaf, ca}

	mem := NewEndpointMemory()
	for i := 0; i < 10; i++ {
		entries := CollectRevocation(context.Background(), certs, 0, mem)
		if len(entries[0].OCSPResponse) != 0 {
			t.Fatalf("document %d: unexpected OCSP response from a dead endpoint", i+1)
		}
	}
	if got := mem.SilentEndpoints(); got != 1 {
		t.Fatalf("memory holds %d silent endpoints, want 1", got)
	}
}

// TestASilentOCSPResponderIsContactedOnlyTwice counts the actual
// connections, which is the assertion that fails against the old code:
// ten documents used to make twenty attempts, one batch now makes the
// two ocspAttempts of the first document and stops.
func TestASilentOCSPResponderIsContactedOnlyTwice(t *testing.T) {
	var attempts int64
	// A listener that accepts a connection and closes it without a
	// response: an attempt that reaches the endpoint and produces no
	// answer, which is countable and immediate.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(&attempts, 1)
			_ = conn.Close()
		}
	}()

	url := "http://" + l.Addr().String() + "/ocsp"
	ca, _, leaf := buildTestChain(t, url, "")
	certs := []*x509.Certificate{leaf, ca}

	mem := NewEndpointMemory()
	for i := 0; i < 10; i++ {
		CollectRevocation(context.Background(), certs, 0, mem)
	}
	if got := atomic.LoadInt64(&attempts); got != int64(ocspAttempts) {
		t.Fatalf("a silent responder was contacted %d times across ten documents, want %d (the first document's attempts, and no more)", got, ocspAttempts)
	}
}

// TestWithoutAMemoryEveryDocumentAsksAgain is the other direction: nil
// means nothing is remembered, which is what a single-document signature
// wants and what every caller did before this existed.
func TestWithoutAMemoryEveryDocumentAsksAgain(t *testing.T) {
	url := deadEndpoint(t)
	ca, _, leaf := buildTestChain(t, url, "")
	certs := []*x509.Certificate{leaf, ca}

	var mem *EndpointMemory // deliberately nil
	for i := 0; i < 3; i++ {
		CollectRevocation(context.Background(), certs, 0, mem)
	}
	if got := mem.SilentEndpoints(); got != 0 {
		t.Fatalf("a nil memory reported %d silent endpoints, want 0", got)
	}
}

// TestANewBatchAsksTheEndpointAgain pins the scope: the memory belongs
// to one batch, not to the process. A responder that was down five
// minutes ago may be up now, and the next batch must find that out.
func TestANewBatchAsksTheEndpointAgain(t *testing.T) {
	var count int64
	// A responder that refuses at the HTTP level: it answers (so it is
	// not "silent"), which lets this test separate "asked again" from
	// "remembered" without waiting on a timeout.
	srv := countingServer(t, &count, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	ca, _, leaf := buildTestChain(t, srv.URL, "")
	certs := []*x509.Certificate{leaf, ca}

	first := NewEndpointMemory()
	CollectRevocation(context.Background(), certs, 0, first)
	afterFirstBatch := atomic.LoadInt64(&count)

	second := NewEndpointMemory()
	CollectRevocation(context.Background(), certs, 0, second)
	if atomic.LoadInt64(&count) <= afterFirstBatch {
		t.Fatal("a new batch did not contact the endpoint again")
	}
}

// TestASuccessfulResponseIsNotCachedAcrossDocuments is D-046's rule,
// which this change deliberately does not touch: revocation is collected
// after the signature exists so the response postdates it, and a
// response fetched after document 1 predates document 100's signature.
// Only failures are remembered.
func TestASuccessfulResponseIsNotCachedAcrossDocuments(t *testing.T) {
	ca, caKey, leaf := buildTestChain(t, "", "")
	inner := ocspServer(t, ca, caKey, leaf)
	defer inner.Close()

	var count int64
	srv := countingServer(t, &count, func(w http.ResponseWriter, r *http.Request) {
		proxy, err := http.Post(inner.URL, "application/ocsp-request", r.Body)
		if err != nil {
			t.Errorf("proxying to the OCSP server: %v", err)
			return
		}
		defer func() { _ = proxy.Body.Close() }()
		w.Header().Set("Content-Type", "application/ocsp-response")
		_, _ = io.Copy(w, proxy.Body)
	})
	leaf.OCSPServer = []string{srv.URL}
	certs := []*x509.Certificate{leaf, ca}

	mem := NewEndpointMemory()
	for i := 0; i < 3; i++ {
		entries := CollectRevocation(context.Background(), certs, 0, mem)
		if len(entries[0].OCSPResponse) == 0 {
			t.Fatalf("document %d: no OCSP response", i+1)
		}
	}
	if got := atomic.LoadInt64(&count); got != 3 {
		t.Fatalf("a working responder was contacted %d times for three documents, want 3 — a successful response must not be reused across documents (D-046)", got)
	}
}

// TestAResponderThatAnswersBadlyIsStillAsked separates "did not answer"
// from "answered with something unusable". A responder that is plainly
// up is not the twenty-second cost this memory exists to remove, and
// silently giving up on it after one document would turn a transient
// server-side problem into a whole batch with no revocation evidence.
func TestAResponderThatAnswersBadlyIsStillAsked(t *testing.T) {
	var count int64
	srv := countingServer(t, &count, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("this is not an OCSP response"))
	})
	ca, _, leaf := buildTestChain(t, srv.URL, "")
	certs := []*x509.Certificate{leaf, ca}

	mem := NewEndpointMemory()
	for i := 0; i < 2; i++ {
		CollectRevocation(context.Background(), certs, 0, mem)
	}
	if got := atomic.LoadInt64(&count); got != int64(2*ocspAttempts) {
		t.Fatalf("a responder that answered was contacted %d times for two documents, want %d", got, 2*ocspAttempts)
	}
	if got := mem.SilentEndpoints(); got != 0 {
		t.Fatalf("a responder that answered was recorded as silent (%d endpoints)", got)
	}
}

// TestASilentCRLDistributionPointIsAskedOnceForTheWholeBatch covers the
// other endpoint kind. crlTimeout is 30 s, so the cost of asking a dead
// CRL host again is larger than for OCSP, not smaller.
func TestASilentCRLDistributionPointIsAskedOnceForTheWholeBatch(t *testing.T) {
	crlURL := deadEndpoint(t)
	ca, _, leaf := buildTestChain(t, "", crlURL)
	certs := []*x509.Certificate{leaf, ca}

	mem := NewEndpointMemory()
	for i := 0; i < 5; i++ {
		entries := CollectRevocation(context.Background(), certs, 0, mem)
		if len(entries[0].CRL) != 0 {
			t.Fatalf("document %d: unexpected CRL from a dead endpoint", i+1)
		}
	}
	if got := mem.SilentEndpoints(); got != 1 {
		t.Fatalf("memory holds %d silent endpoints after a dead CRL distribution point, want 1", got)
	}
}
