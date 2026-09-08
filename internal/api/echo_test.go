package api

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// What the endpoint is for: an integrator prints their own canonical
// string beside the agent's and the difference is one line.
//
// The assertion is that the agent's answer is byte-for-byte what a
// correct client computes — not a paraphrase of it, and not something
// only this package can reproduce.
func TestEchoReturnsTheCanonicalStringTheAgentBuilt(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	body := []byte(`{"certificateThumbprint":"7758D4","digestAlgorithm":"SHA256"}`)
	req := c.request(http.MethodPost, "/v2/echo", body)
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, decoded, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusOK {
		t.Fatalf("status %d: %v", status, decoded)
	}

	// Built here the way an integrator would build it: from the four
	// header values the request carried and the bytes it sent.
	want := CanonicalString(http.MethodPost, "/v2/echo",
		req.Header.Get(HeaderTimestamp), req.Header.Get(HeaderNonce), body)
	if decoded["canonicalString"] != want {
		t.Fatalf("canonicalString =\n%q\nwant\n%q", decoded["canonicalString"], want)
	}

	sum := sha256.Sum256(body)
	if decoded["bodySha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("bodySha256 = %v, want %s", decoded["bodySha256"], hex.EncodeToString(sum[:]))
	}
	// The body hash is the last line of the canonical string, and the
	// two must be the same value — a caller comparing one and not the
	// other must not be able to be told two different things.
	if !strings.HasSuffix(decoded["canonicalString"].(string), decoded["bodySha256"].(string)) {
		t.Fatal("the canonical string does not end in the body hash it reports")
	}
	if decoded["canonicalString"].(string) != want {
		t.Fatal("the canonical string is not the one a client computes")
	}
	if strings.Count(decoded["canonicalString"].(string), "\n") != 4 {
		t.Fatalf("the canonical string has %d newlines, want 4",
			strings.Count(decoded["canonicalString"].(string), "\n"))
	}
}

// The reasoning the endpoint rests on, as an assertion: the signature
// is not returned, and nothing that would let one be derived is.
func TestEchoReturnsNothingTheCallerDidNotAlreadyHave(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	body := []byte(`{"hello":"world"}`)
	req := c.request(http.MethodPost, "/v2/echo", body)
	signature := req.Header.Get(HeaderSignature)
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	_, decoded, raw := statusAndBody(h, resp)
	_ = resp.Body.Close()

	if strings.Contains(string(raw), signature) {
		t.Errorf("the response carries the request's own signature:\n%s", raw)
	}
	if strings.Contains(string(raw), hex.EncodeToString(c.secret)) ||
		strings.Contains(string(raw), string(c.secret)) {
		t.Errorf("the response carries the device secret:\n%s", raw)
	}
	// Exactly two fields, so a future addition has to be a deliberate
	// one rather than something that arrived with a struct change.
	if len(decoded) != 2 {
		t.Fatalf("the response has %d fields: %v", len(decoded), decoded)
	}
	for _, want := range []string{"canonicalString", "bodySha256"} {
		if _, ok := decoded[want]; !ok {
			t.Errorf("the response has no %q", want)
		}
	}
}

// The other half of the reasoning: a request that reaches this handler
// has already authenticated, so it echoes nothing to anybody who could
// not already have computed it.
func TestEchoIsAuthenticatedLikeEveryOtherEndpoint(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")
	body := []byte(`{"hello":"world"}`)

	// Unsigned.
	resp, err := h.http.Client().Post(h.http.URL+"/v2/echo", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, decoded, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusUnauthorized || decoded["code"] != string(errs.CodeAuthFailed) {
		t.Fatalf("an unsigned echo answers %d %v", status, decoded["code"])
	}
	if _, present := decoded["canonicalString"]; present {
		t.Fatal("a refused request was told the canonical string anyway")
	}

	// Signed for one body and sent with another — which is exactly the
	// mistake this endpoint exists to make visible, and it still has to
	// be refused rather than diagnosed.
	req := c.request(http.MethodPost, "/v2/echo", body)
	req2, err := http.NewRequest(http.MethodPost, h.http.URL+"/v2/echo", strings.NewReader(`{"hello":"there"}`))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req2.Header = req.Header.Clone()
	resp, err = h.http.Client().Do(req2)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, decoded, _ = statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusUnauthorized || decoded["code"] != string(errs.CodeAuthFailed) {
		t.Fatalf("an altered body answers %d %v", status, decoded["code"])
	}
}

// It is a POST with a body, so it declares a content type like every
// other endpoint that has one — and refuses a body it cannot be told
// the type of.
func TestEchoRefusesAnythingButAPOSTAndAJSONContentType(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	status, body := c.do(http.MethodGet, "/v2/echo", nil)
	if status != http.StatusBadRequest || body["code"] != string(errs.CodeRequestInvalid) {
		t.Fatalf("GET answers %d %v", status, body["code"])
	}

	req := c.request(http.MethodPost, "/v2/echo", []byte(`{}`))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, decoded, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusBadRequest || decoded["code"] != string(errs.CodeRequestInvalid) {
		t.Fatalf("a plain-text body answers %d %v", status, decoded["code"])
	}
	if decoded["details"].(map[string]any)["expectedContentType"] != "application/json" {
		t.Fatalf("the refusal does not name the content type it wanted: %v", decoded["details"])
	}
}

// The body is hashed, never parsed. An integrator comparing the exact
// bytes they are about to sign must be able to send exactly those,
// whatever they are.
func TestEchoHashesTheBodyWithoutParsingIt(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	for _, body := range []string{"", "not json at all", `{"trailing":"newline"}` + "\n"} {
		req := c.request(http.MethodPost, "/v2/echo", []byte(body))
		resp, err := h.http.Client().Do(req)
		if err != nil {
			t.Fatalf("sending the request: %v", err)
		}
		status, decoded, _ := statusAndBody(h, resp)
		_ = resp.Body.Close()
		if status != http.StatusOK {
			t.Fatalf("body %q answers %d %v", body, status, decoded["code"])
		}
		sum := sha256.Sum256([]byte(body))
		if decoded["bodySha256"] != hex.EncodeToString(sum[:]) {
			t.Errorf("body %q hashes to %v, want %s", body, decoded["bodySha256"], hex.EncodeToString(sum[:]))
		}
	}
}

// An empty body still hashes to something, and the something is the
// constant §3.1 states outright — the one thing every integrator gets
// wrong once, which is why this endpoint is the place to show it.
func TestEchoOfAnEmptyBodyIsTheDocumentedConstant(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	req := c.request(http.MethodPost, "/v2/echo", nil)
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	_, decoded, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if decoded["bodySha256"] != EmptyBodySHA256 {
		t.Fatalf("an empty body hashes to %v, want %s", decoded["bodySha256"], EmptyBodySHA256)
	}
}

// A body over the limit is refused by size before it is read, like
// every other endpoint's.
func TestEchoRefusesABodyOverItsLimit(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	big := make([]byte, maxEchoBody+1)
	for i := range big {
		big[i] = 'a'
	}
	req := c.request(http.MethodPost, "/v2/echo", big)
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, decoded, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusBadRequest || decoded["code"] != string(errs.CodeRequestInvalid) {
		t.Fatalf("an oversized body answers %d %v", status, decoded["code"])
	}
	if decoded["details"].(map[string]any)["maxBytes"] != float64(maxEchoBody) {
		t.Fatalf("the refusal does not name the limit: %v", decoded["details"])
	}
}

// A nonce is spent here as it is anywhere else: an echo is an
// authenticated request and a replayed one is refused.
func TestAnEchoIsNotReplayable(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	body := []byte(`{"hello":"world"}`)
	req := c.request(http.MethodPost, "/v2/echo", body)
	headers := req.Header.Clone()
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, _, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusOK {
		t.Fatalf("the first echo answers %d", status)
	}

	again, err := http.NewRequest(http.MethodPost, h.http.URL+"/v2/echo", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	again.Header = headers
	resp, err = h.http.Client().Do(again)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, decoded, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusUnauthorized || decoded["code"] != string(errs.CodeAuthFailed) {
		t.Fatalf("a replayed echo answers %d %v", status, decoded["code"])
	}
}
