package tsa

import (
	"context"
	"crypto/sha256"
	"encoding/asn1"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// buildTestResponse constructs a well-formed (definite-length DER)
// TimeStampResp granting a token over digest with the given nonce and
// genTime — enough for the client's own validation (nonce, imprint,
// skew) to exercise against, without needing a real TSA's signature
// (this file never verifies the token's own CMS signature — that is
// the independent verifier's job in F3 §8, not this client's).
func buildTestResponse(t *testing.T, digest []byte, nonce *big.Int, genTime time.Time) []byte {
	t.Helper()

	messageImprintDER := derSeq(t,
		derSeq(t, marshalASN1(t, oidSHA256), []byte{0x05, 0x00}),
		derOctet(digest),
	)
	tstInfoContent := concatAll(
		derInt(t, big.NewInt(1)),
		marshalASN1(t, asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55016, 1, 1, 0}),
		messageImprintDER,
		derInt(t, big.NewInt(42)),
		derGeneralizedTime(t, genTime),
		derInt(t, nonce),
	)
	tstInfo := derSeq(t, tstInfoContent)

	eContent := derExplicitTag(t, 0, derOctet(tstInfo))
	encapContentInfo := derSeq(t, concatAll(marshalASN1(t, asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}), eContent))
	signedData := derSeq(t, concatAll(
		derInt(t, big.NewInt(1)),
		derSet(t, derSeq(t, marshalASN1(t, oidSHA256), []byte{0x05, 0x00})),
		encapContentInfo,
		derSet(t), // no signerInfos needed for this test
	))
	contentInfo := derSeq(t, concatAll(
		marshalASN1(t, asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}),
		derExplicitTag(t, 0, signedData),
	))

	status := derSeq(t, derInt(t, big.NewInt(0))) // PKIStatusInfo{status: granted}
	resp := derSeq(t, concatAll(status, contentInfo))
	return resp
}

func marshalASN1(t *testing.T, v any) []byte {
	t.Helper()
	b, err := asn1.Marshal(v)
	if err != nil {
		t.Fatalf("asn1.Marshal: %v", err)
	}
	return b
}

func derInt(t *testing.T, v *big.Int) []byte { return marshalASN1(t, v) }

func derOctet(b []byte) []byte {
	return append([]byte{0x04}, appendLength(b)...)
}

func derSeq(t *testing.T, parts ...[]byte) []byte {
	t.Helper()
	content := concatAll(parts...)
	return append([]byte{0x30}, appendLength(content)...)
}

func derSet(t *testing.T, parts ...[]byte) []byte {
	t.Helper()
	content := concatAll(parts...)
	return append([]byte{0x31}, appendLength(content)...)
}

func derExplicitTag(t *testing.T, tag byte, inner []byte) []byte {
	t.Helper()
	return append([]byte{0xA0 | tag}, appendLength(inner)...)
}

func derGeneralizedTime(t *testing.T, when time.Time) []byte {
	t.Helper()
	s := when.UTC().Format("20060102150405Z")
	return append([]byte{0x18}, appendLength([]byte(s))...)
}

func appendLength(content []byte) []byte {
	n := len(content)
	var out []byte
	if n < 0x80 {
		out = append(out, byte(n))
	} else {
		var lb []byte
		for n > 0 {
			lb = append([]byte{byte(n)}, lb...)
			n >>= 8
		}
		out = append(out, 0x80|byte(len(lb)))
		out = append(out, lb...)
	}
	return append(out, content...)
}

func concatAll(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestClientTimestampSuccess(t *testing.T) {
	digest := sha256.Sum256([]byte("document bytes"))
	var capturedNonce atomic.Value

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != contentTypeQuery {
			t.Errorf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), contentTypeQuery)
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		var req timeStampReq
		if _, err := asn1.Unmarshal(body, &req); err != nil {
			t.Fatalf("server: unmarshalling request: %v", err)
		}
		capturedNonce.Store(req.Nonce)
		w.Header().Set("Content-Type", contentTypeReply)
		_, _ = w.Write(buildTestResponse(t, digest[:], req.Nonce, time.Now()))
	}))
	defer server.Close()

	c := NewClient(server.URL, Auth{})
	resp, err := c.Timestamp(context.Background(), digest[:])
	if err != nil {
		t.Fatalf("Timestamp: %v", err)
	}
	if resp.Nonce.Cmp(capturedNonce.Load().(*big.Int)) != 0 {
		t.Fatalf("returned nonce does not match what the server received")
	}
}

func TestClientTimestampRetriesOn5xxThenSucceeds(t *testing.T) {
	digest := sha256.Sum256([]byte("document bytes"))
	var calls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		var req timeStampReq
		_, _ = asn1.Unmarshal(body, &req)
		w.Header().Set("Content-Type", contentTypeReply)
		_, _ = w.Write(buildTestResponse(t, digest[:], req.Nonce, time.Now()))
	}))
	defer server.Close()

	start := time.Now()
	c := NewClient(server.URL, Auth{})
	_, err := c.Timestamp(context.Background(), digest[:])
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Timestamp: %v", err)
	}
	if calls != 3 {
		t.Fatalf("server received %d calls, want 3", calls)
	}
	// Backoff is 1s then 3s: two failures before success means at least
	// 1s+3s elapsed.
	if elapsed < 4*time.Second {
		t.Fatalf("elapsed %s, want at least the 1s+3s backoff", elapsed)
	}
}

func TestClientTimestampDoesNotRetryOn4xx(t *testing.T) {
	digest := sha256.Sum256([]byte("document bytes"))
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	c := NewClient(server.URL, Auth{})
	_, err := c.Timestamp(context.Background(), digest[:])
	if err == nil {
		t.Fatal("Timestamp succeeded, want an error")
	}
	if calls != 1 {
		t.Fatalf("server received %d calls, want exactly 1 (4xx must not be retried)", calls)
	}
}

func TestClientTimestampDoesNotRetryOnRejection(t *testing.T) {
	digest := sha256.Sum256([]byte("document bytes"))
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", contentTypeReply)
		status := derSeq(t, derInt(t, big.NewInt(int64(pkiStatusRejection))))
		_, _ = w.Write(derSeq(t, status))
	}))
	defer server.Close()

	c := NewClient(server.URL, Auth{})
	_, err := c.Timestamp(context.Background(), digest[:])
	if err == nil {
		t.Fatal("Timestamp succeeded, want a RejectionError")
	}
	var re *RejectionError
	if !errors.As(err, &re) {
		t.Fatalf("error = %v, want *RejectionError", err)
	}
	if calls != 1 {
		t.Fatalf("server received %d calls, want exactly 1 (an RFC 3161 rejection must not be retried)", calls)
	}
}

func TestClientTimestampFailsAfterThreeAttempts(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	digest := sha256.Sum256([]byte("x"))
	c := NewClient(server.URL, Auth{})
	_, err := c.Timestamp(context.Background(), digest[:])
	if err == nil {
		t.Fatal("Timestamp succeeded, want an error")
	}
	if calls != maxAttempts {
		t.Fatalf("server received %d calls, want %d", calls, maxAttempts)
	}
}

func TestClientTimestampRejectsGenTimeOutsideSkewWindow(t *testing.T) {
	digest := sha256.Sum256([]byte("document bytes"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		var req timeStampReq
		_, _ = asn1.Unmarshal(body, &req)
		w.Header().Set("Content-Type", contentTypeReply)
		_, _ = w.Write(buildTestResponse(t, digest[:], req.Nonce, time.Now().Add(-time.Hour)))
	}))
	defer server.Close()

	c := NewClient(server.URL, Auth{})
	_, err := c.Timestamp(context.Background(), digest[:])
	if err == nil {
		t.Fatal("Timestamp succeeded with genTime an hour in the past, want an error")
	}
}

func TestClientBasicAuthIsSent(t *testing.T) {
	digest := sha256.Sum256([]byte("document bytes"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "Test.Korisnik" || pass != "123456" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		var req timeStampReq
		_, _ = asn1.Unmarshal(body, &req)
		w.Header().Set("Content-Type", contentTypeReply)
		_, _ = w.Write(buildTestResponse(t, digest[:], req.Nonce, time.Now()))
	}))
	defer server.Close()

	c := NewClient(server.URL, Auth{BasicUsername: "Test.Korisnik", BasicPassword: "123456"})
	if _, err := c.Timestamp(context.Background(), digest[:]); err != nil {
		t.Fatalf("Timestamp with correct Basic auth: %v", err)
	}

	c2 := NewClient(server.URL, Auth{BasicUsername: "wrong", BasicPassword: "wrong"})
	if _, err := c2.Timestamp(context.Background(), digest[:]); err == nil {
		t.Fatal("Timestamp with wrong Basic auth succeeded, want an error")
	}
}
