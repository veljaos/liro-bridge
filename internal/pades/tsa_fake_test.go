package pades

// A minimal, from-scratch RFC 3161 TimeStampResp builder for this
// package's own tests, deliberately independent of
// internal/pades/tsa's own (unexported) test helper of the same shape
// (tsa/client_test.go's buildTestResponse) — internal/pades/tsa has no
// exported way to fabricate a response with a controllable genTime, and
// adding one would be test-only production surface for a single call
// site. This lets TestSignDocument* tests in this package drive a real
// httptest TSA that grants a token with a genTime this test controls —
// needed for Task 3 (clock drift) and Task 1c (reaching B-T before a
// B-LT degradation can be observed) without depending on network access
// to a real TSA.

import (
	"encoding/asn1"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/pades/tsa"
)

// oidSHA256Test mirrors internal/pades/tsa's own oidSHA256 — duplicated
// rather than imported since it is unexported there.
var oidSHA256Test = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}

type fakeMessageImprint struct {
	HashAlgorithm asn1.RawValue
	HashedMessage []byte
}

// fakeTimeStampReq mirrors RFC 3161's TimeStampReq closely enough to
// decode a request this project's own tsa.Client produced (see
// internal/pades/tsa/request.go's timeStampReq), so the fake server
// below can echo back the right nonce and message imprint.
type fakeTimeStampReq struct {
	Version        int
	MessageImprint fakeMessageImprint
	ReqPolicy      asn1.ObjectIdentifier `asn1:"optional"`
	Nonce          *big.Int              `asn1:"optional"`
	CertReq        bool                  `asn1:"optional,default:false"`
}

func fakeDERSeq(parts ...[]byte) []byte {
	return append([]byte{0x30}, fakeAppendLength(fakeConcat(parts...))...)
}
func fakeDERSet(parts ...[]byte) []byte {
	return append([]byte{0x31}, fakeAppendLength(fakeConcat(parts...))...)
}
func fakeDEROctet(b []byte) []byte { return append([]byte{0x04}, fakeAppendLength(b)...) }
func fakeDERExplicit(tag byte, inner []byte) []byte {
	return append([]byte{0xA0 | tag}, fakeAppendLength(inner)...)
}

func fakeDERGeneralizedTime(when time.Time) []byte {
	s := when.UTC().Format("20060102150405Z")
	return append([]byte{0x18}, fakeAppendLength([]byte(s))...)
}

func fakeMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := asn1.Marshal(v)
	if err != nil {
		t.Fatalf("asn1.Marshal: %v", err)
	}
	return b
}

func fakeAppendLength(content []byte) []byte {
	n := len(content)
	if n < 0x80 {
		return append([]byte{byte(n)}, content...)
	}
	var lb []byte
	for n > 0 {
		lb = append([]byte{byte(n)}, lb...)
		n >>= 8
	}
	out := append([]byte{0x80 | byte(len(lb))}, lb...)
	return append(out, content...)
}

func fakeConcat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// buildFakeTSTResponse builds a well-formed (definite-length DER),
// PKIStatus-granted TimeStampResp over digest/nonce with the given
// genTime — enough for internal/pades/tsa.Client's own validation
// (nonce echo, message imprint, ±10 minute skew) to accept, without a
// real TSA's signature (unsignerInfos is empty; nothing here verifies
// the token's own CMS signature).
func buildFakeTSTResponse(t *testing.T, digest []byte, nonce *big.Int, genTime time.Time) []byte {
	t.Helper()
	messageImprintDER := fakeDERSeq(fakeDERSeq(fakeMarshal(t, oidSHA256Test), []byte{0x05, 0x00}), fakeDEROctet(digest))
	tstInfo := fakeDERSeq(fakeConcat(
		fakeMarshal(t, big.NewInt(1)),
		fakeMarshal(t, asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 55016, 1, 1, 0}),
		messageImprintDER,
		fakeMarshal(t, big.NewInt(42)),
		fakeDERGeneralizedTime(genTime),
		fakeMarshal(t, nonce),
	))
	eContent := fakeDERExplicit(0, fakeDEROctet(tstInfo))
	encapContentInfo := fakeDERSeq(fakeConcat(fakeMarshal(t, asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}), eContent))
	signedData := fakeDERSeq(fakeConcat(
		fakeMarshal(t, big.NewInt(1)),
		fakeDERSet(fakeDERSeq(fakeMarshal(t, oidSHA256Test), []byte{0x05, 0x00})),
		encapContentInfo,
		fakeDERSet(), // no signerInfos needed for this test
	))
	contentInfo := fakeDERSeq(fakeConcat(fakeMarshal(t, asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}), fakeDERExplicit(0, signedData)))
	status := fakeDERSeq(fakeMarshal(t, big.NewInt(0))) // PKIStatusInfo{status: granted}
	return fakeDERSeq(fakeConcat(status, contentInfo))
}

// fakeTSAServer starts an httptest server implementing enough of RFC
// 3161 for tsa.Client.Timestamp to succeed against it, granting a token
// whose genTime is genTimeFor(requestedDigest) — a function rather than
// a fixed value so a caller can, if it wants, make genTime depend on
// what was actually requested (this project's tests use it only as a
// fixed-value closure, but the shape costs nothing extra).
func fakeTSAServer(t *testing.T, genTimeFor func(digest []byte) time.Time) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("fakeTSAServer: reading request: %v", err)
		}
		var req fakeTimeStampReq
		if _, err := asn1.Unmarshal(body, &req); err != nil {
			t.Fatalf("fakeTSAServer: parsing TimeStampReq: %v", err)
		}
		resp := buildFakeTSTResponse(t, req.MessageImprint.HashedMessage, req.Nonce, genTimeFor(req.MessageImprint.HashedMessage))
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(resp)
	}))
}

// fakeTSAClient is a small convenience wrapper for tests that just need
// a working TSA client without caring about the server's lifetime
// (callers that do care should use fakeTSAServer directly and Close it).
func fakeTSAClientAt(t *testing.T, genTime time.Time) *tsa.Client {
	t.Helper()
	server := fakeTSAServer(t, func([]byte) time.Time { return genTime })
	t.Cleanup(server.Close)
	return tsa.NewClient(server.URL, tsa.Auth{})
}
