package api

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// F7 §12's list, in order: a wrong signature, a stale timestamp, a
// future timestamp, a reused nonce, an empty body signed as if it had
// content, a body altered after signing, a signature from another
// application's secret, a request to a path other than the one signed.
//
// Every one is refused, and — the property the test is really about —
// every one is refused with the same code and no details, so that
// nothing in the answer says which check failed. F7 §3: that is a hint
// to whoever is probing.
func TestEveryWayAuthenticationCanFailIsRefusedTheSameWay(t *testing.T) {
	h := newHarness(t)
	app, secret := h.pair("My ERP", "https://erp.example.com")
	other, otherSecret := h.pair("Another ERP", "https://other.example.com")
	_ = other

	body := []byte(`{"documents":[]}`)
	const path = "/v2/sign/pdf"

	tests := []struct {
		name   string
		mutate func(r *httptestRequest)
	}{
		{"a wrong signature", func(r *httptestRequest) {
			r.req.Header.Set(HeaderSignature, flipLastHexDigit(r.req.Header.Get(HeaderSignature)))
		}},
		{"a signature that is not hexadecimal", func(r *httptestRequest) {
			r.req.Header.Set(HeaderSignature, "not-a-signature")
		}},
		{"a stale timestamp", func(r *httptestRequest) {
			r.resign(formatUnix(h.clock.Now().Add(-MaxClockSkew-time.Second)), r.nonce, r.body)
		}},
		{"a future timestamp", func(r *httptestRequest) {
			r.resign(formatUnix(h.clock.Now().Add(MaxClockSkew+time.Second)), r.nonce, r.body)
		}},
		{"a timestamp that is not a number", func(r *httptestRequest) {
			r.resign("the-day-before-yesterday", r.nonce, r.body)
		}},
		{"an empty body signed as if it had content", func(r *httptestRequest) {
			// The signature covers body; the request delivers nothing.
			r.sentBody = nil
		}},
		{"a body altered after signing", func(r *httptestRequest) {
			r.sentBody = append(append([]byte(nil), r.body...), ' ')
		}},
		{"a signature from another application's secret", func(r *httptestRequest) {
			canonical := CanonicalString("POST", path, r.req.Header.Get(HeaderTimestamp), r.nonce, r.body)
			r.req.Header.Set(HeaderSignature, Sign(otherSecret, canonical))
		}},
		{"a request to a path other than the one signed", func(r *httptestRequest) {
			r.req.URL.Path = "/v2/sign"
		}},
		{"no App-Id header", func(r *httptestRequest) { r.req.Header.Del(HeaderAppID) }},
		{"no Timestamp header", func(r *httptestRequest) { r.req.Header.Del(HeaderTimestamp) }},
		{"no Nonce header", func(r *httptestRequest) { r.req.Header.Del(HeaderNonce) }},
		{"no Signature header", func(r *httptestRequest) { r.req.Header.Del(HeaderSignature) }},
		{"a nonce longer than the cap", func(r *httptestRequest) {
			r.req.Header.Set(HeaderNonce, string(bytes.Repeat([]byte("n"), MaxNonceLength+1)))
		}},
		{"an Origin header the pairing was not bound to", func(r *httptestRequest) {
			r.req.Header.Set("Origin", "https://evil.example.com")
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newHTTPTestRequest(h, "POST", path, body, app.AppID, secret)
			tc.mutate(r)
			_, apiErr := h.auth.Authenticate(r.request(), r.sentBody)
			if apiErr == nil {
				t.Fatal("the request was accepted")
			}
			if apiErr.Code != errs.CodeAuthFailed {
				t.Fatalf("code is %s, want %s — every one of these must be "+
					"indistinguishable from the outside", apiErr.Code, errs.CodeAuthFailed)
			}
			if len(apiErr.Details) != 0 {
				t.Fatalf("the refusal carried details %v; nothing in the answer "+
					"may say which check failed", apiErr.Details)
			}
		})
	}

	// A reused nonce is the one case that needs a first, accepted
	// request to set it up, so it is checked separately — with the same
	// assertion.
	t.Run("a reused nonce", func(t *testing.T) {
		r := newHTTPTestRequest(h, "POST", path, body, app.AppID, secret)
		if _, apiErr := h.auth.Authenticate(r.request(), r.sentBody); apiErr != nil {
			t.Fatalf("the first request was refused: %v", apiErr)
		}
		again := r.replay()
		_, apiErr := h.auth.Authenticate(again, r.sentBody)
		if apiErr == nil {
			t.Fatal("a replayed request was accepted")
		}
		if apiErr.Code != errs.CodeAuthFailed || len(apiErr.Details) != 0 {
			t.Fatalf("replay was refused as %s %v, want a bare %s",
				apiErr.Code, apiErr.Details, errs.CodeAuthFailed)
		}
	})
}

// An unknown or revoked application is the one distinction worth
// making, because it is the only one the caller can act on: re-pair.
func TestAnUnknownApplicationIsNotPaired(t *testing.T) {
	h := newHarness(t)
	_, secret := h.pair("My ERP", "https://erp.example.com")

	r := newHTTPTestRequest(h, "POST", "/v2/sign", []byte(`{}`), "0123456789abcdef0123456789abcdef", secret)
	_, apiErr := h.auth.Authenticate(r.request(), r.sentBody)
	if apiErr == nil || apiErr.Code != errs.CodeNotPaired {
		t.Fatalf("an unknown application was answered %v, want %s", apiErr, errs.CodeNotPaired)
	}
}

func TestARevokedPairingStopsWorkingImmediately(t *testing.T) {
	h := newHarness(t)
	app, secret := h.pair("My ERP", "https://erp.example.com")

	r := newHTTPTestRequest(h, "POST", "/v2/sign", []byte(`{}`), app.AppID, secret)
	if _, apiErr := h.auth.Authenticate(r.request(), r.sentBody); apiErr != nil {
		t.Fatalf("a live pairing was refused: %v", apiErr)
	}

	if err := h.pairings.Revoke(app.AppID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	next := newHTTPTestRequest(h, "POST", "/v2/sign", []byte(`{}`), app.AppID, secret)
	_, apiErr := h.auth.Authenticate(next.request(), next.sentBody)
	if apiErr == nil || apiErr.Code != errs.CodeNotPaired {
		t.Fatalf("a revoked pairing was answered %v, want %s", apiErr, errs.CodeNotPaired)
	}
}

// The ordering the whole replay defence rests on: a nonce is spent only
// after the signature has validated. Consuming it first would let an
// unauthenticated caller fill the cache with fabricated values and lock
// a real application out of its own nonces.
func TestAFailedSignatureDoesNotSpendTheNonce(t *testing.T) {
	h := newHarness(t)
	app, secret := h.pair("My ERP", "https://erp.example.com")

	body := []byte(`{}`)
	timestamp := formatUnix(h.clock.Now())
	nonce := "the-one-nonce"
	canonical := CanonicalString("POST", "/v2/sign", timestamp, nonce, body)

	// A caller with no secret at all, guessing, using a nonce the real
	// application is about to use.
	forged := httptest.NewRequest("POST", "/v2/sign", bytes.NewReader(body))
	forged.Header.Set(HeaderAppID, app.AppID)
	forged.Header.Set(HeaderTimestamp, timestamp)
	forged.Header.Set(HeaderNonce, nonce)
	forged.Header.Set(HeaderSignature, Sign([]byte("wrong wrong wrong wrong wrong 32"), canonical))
	if _, apiErr := h.auth.Authenticate(forged, body); apiErr == nil {
		t.Fatal("a forged signature was accepted")
	}
	if got := h.nonces.Len(); got != 0 {
		t.Fatalf("the nonce cache holds %d entries after a refused request; "+
			"an unauthenticated caller can fill it", got)
	}

	// The real application's own request, with that same nonce, must
	// still go through.
	genuine := httptest.NewRequest("POST", "/v2/sign", bytes.NewReader(body))
	genuine.Header.Set(HeaderAppID, app.AppID)
	genuine.Header.Set(HeaderTimestamp, timestamp)
	genuine.Header.Set(HeaderNonce, nonce)
	genuine.Header.Set(HeaderSignature, Sign(secret, canonical))
	if _, apiErr := h.auth.Authenticate(genuine, body); apiErr != nil {
		t.Fatalf("the real application was locked out of its own nonce: %v", apiErr)
	}
}

func TestTheSkewWindowIsExactlySixtySecondsEachWay(t *testing.T) {
	h := newHarness(t)
	app, secret := h.pair("My ERP", "https://erp.example.com")

	for _, offset := range []time.Duration{-MaxClockSkew, 0, MaxClockSkew} {
		r := newHTTPTestRequestAt(h, "POST", "/v2/sign", []byte(`{}`), app.AppID, secret,
			h.clock.Now().Add(offset))
		if _, apiErr := h.auth.Authenticate(r.request(), r.sentBody); apiErr != nil {
			t.Fatalf("a timestamp %v from now was refused: %v", offset, apiErr)
		}
	}
	for _, offset := range []time.Duration{-MaxClockSkew - time.Second, MaxClockSkew + time.Second} {
		r := newHTTPTestRequestAt(h, "POST", "/v2/sign", []byte(`{}`), app.AppID, secret,
			h.clock.Now().Add(offset))
		if _, apiErr := h.auth.Authenticate(r.request(), r.sentBody); apiErr == nil {
			t.Fatalf("a timestamp %v from now was accepted", offset)
		}
	}
}

// A server-side caller sends no Origin header at all — F7 §2.3 says the
// device secret belongs on a server, where there is no origin to speak
// of — and must not be refused for it.
func TestNoOriginHeaderIsFineForAServerSideCaller(t *testing.T) {
	h := newHarness(t)
	app, secret := h.pair("My ERP", "https://erp.example.com")

	r := newHTTPTestRequest(h, "POST", "/v2/sign", []byte(`{}`), app.AppID, secret)
	if got := r.req.Header.Get("Origin"); got != "" {
		t.Fatalf("the test request already carries an Origin header (%q)", got)
	}
	if _, apiErr := h.auth.Authenticate(r.request(), r.sentBody); apiErr != nil {
		t.Fatalf("a request with no Origin header was refused: %v", apiErr)
	}
}

func TestAuthenticationRecordsWhenTheApplicationWasLastUsed(t *testing.T) {
	h := newHarness(t)
	app, secret := h.pair("My ERP", "https://erp.example.com")

	before, _ := h.pairings.Get(app.AppID)
	if !before.LastUsedAt.IsZero() {
		t.Fatalf("a freshly paired application already has a last-used time: %v", before.LastUsedAt)
	}

	h.clock.Advance(time.Second)
	r := newHTTPTestRequest(h, "POST", "/v2/sign", []byte(`{}`), app.AppID, secret)
	if _, apiErr := h.auth.Authenticate(r.request(), r.sentBody); apiErr != nil {
		t.Fatalf("Authenticate: %v", apiErr)
	}
	after, _ := h.pairings.Get(app.AppID)
	if !after.LastUsedAt.Equal(h.clock.Now()) {
		t.Fatalf("last used is %v, want %v", after.LastUsedAt, h.clock.Now())
	}
}
