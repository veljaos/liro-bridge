package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// MaxClockSkew is the window within which a request timestamp is
// accepted (SPEC §6.3, F7 §3).
//
// Chosen at 60 s: long enough for an unsynchronised desktop clock —
// which is a real machine on a real network, not a hypothetical — and
// short enough that a captured request is not replayable for long. The
// nonce cache (NonceTTL) is what closes the window entirely; the skew
// bound is what keeps that cache small.
const MaxClockSkew = 60 * time.Second

// authReason names which check refused a request. It never leaves this
// process: F7 §3 is explicit that a rejection says 401 and a code and
// never which check failed, because that is a hint to whoever is
// probing — a caller told "signature mismatch" has learned that its
// timestamp and nonce were fine.
//
// It exists because the agent's own log is the one place where knowing
// is useful and costs nothing, and because an integrator who cannot
// work out why their requests are refused can be told to look at it.
type authReason string

const (
	reasonMissingHeader  authReason = "missing_header"
	reasonNonceTooLong   authReason = "nonce_too_long"
	reasonBadTimestamp   authReason = "unparseable_timestamp"
	reasonClockSkew      authReason = "clock_skew"
	reasonSignature      authReason = "signature_mismatch"
	reasonNonceReused    authReason = "nonce_reused"
	reasonNonceCacheFull authReason = "nonce_cache_full"
	reasonOriginMismatch authReason = "origin_mismatch"
)

// Authenticator verifies the four headers on every protected request
// (F7 §3).
type Authenticator struct {
	pairings *Pairings
	nonces   *NonceCache
	now      func() time.Time
}

// NewAuthenticator returns an Authenticator over pairings. now may be
// nil, in which case time.Now is used.
func NewAuthenticator(pairings *Pairings, nonces *NonceCache, now func() time.Time) *Authenticator {
	if now == nil {
		now = time.Now
	}
	return &Authenticator{pairings: pairings, nonces: nonces, now: now}
}

// Authenticate verifies r against body — the raw bytes already read
// from r.Body, hashed before any parsing (F7 §3) — and returns the
// pairing the request was made under.
//
// The order of the checks is not arbitrary. The nonce is spent last,
// after the signature has verified, because consuming it first lets an
// unauthenticated caller fill the replay cache with fabricated values
// and lock a real application out of its own nonces. Verify, then
// spend.
//
// Every refusal is the same two codes and no details:
//
//   - NOT_PAIRED when there is no live pairing for the identifier —
//     the application was never paired, or its pairing has been
//     revoked. This is the one distinction worth making, because it is
//     the only one a caller can act on: re-pair.
//   - AUTH_FAILED for everything else: a missing header, an
//     unparseable or stale timestamp, a reused nonce, a wrong
//     signature, a mismatched origin. Which of them it was goes to the
//     agent's own log and no further.
func (a *Authenticator) Authenticate(r *http.Request, body []byte) (Pairing, *errs.Error) {
	appID := r.Header.Get(HeaderAppID)
	timestamp := r.Header.Get(HeaderTimestamp)
	nonce := r.Header.Get(HeaderNonce)
	signature := r.Header.Get(HeaderSignature)

	if appID == "" || timestamp == "" || nonce == "" || signature == "" {
		return a.refuse(errs.CodeAuthFailed, reasonMissingHeader, appID)
	}
	if len(nonce) > MaxNonceLength {
		return a.refuse(errs.CodeAuthFailed, reasonNonceTooLong, appID)
	}

	pairing, ok := a.pairings.Get(appID)
	if !ok {
		return a.refuse(errs.CodeNotPaired, "", appID)
	}
	secret, ok := a.pairings.Secret(appID)
	if !ok {
		// Metadata without a secret is not a pairing anything can be
		// authenticated against, and saying "not paired" is the truth.
		return a.refuse(errs.CodeNotPaired, "", appID)
	}

	// The origin binding (SPEC §6.2). A browser cannot lie about this
	// header, so where it is present it is authoritative; a server-side
	// caller — which is where F7 §2.3 says the secret must live —
	// sends none, and its binding is the secret itself.
	if origin := r.Header.Get("Origin"); origin != "" && origin != pairing.Origin {
		return a.refuse(errs.CodeAuthFailed, reasonOriginMismatch, appID)
	}

	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return a.refuse(errs.CodeAuthFailed, reasonBadTimestamp, appID)
	}
	now := a.now()
	drift := now.Sub(time.Unix(seconds, 0))
	if drift < 0 {
		drift = -drift
	}
	if drift > MaxClockSkew {
		return a.refuse(errs.CodeAuthFailed, reasonClockSkew, appID)
	}

	canonical := CanonicalString(r.Method, requestPath(r), timestamp, nonce, body)
	if !SignatureMatches(secret, canonical, signature) {
		return a.refuse(errs.CodeAuthFailed, reasonSignature, appID)
	}

	// Verified. Only now is the nonce spent.
	fresh, full := a.nonces.Use(appID, nonce)
	if full {
		return Pairing{}, errs.WithDetails(errs.CodeRateLimited, errors.New("nonce cache is full"),
			map[string]any{"retryAfterSeconds": int(NonceTTL.Seconds())})
	}
	if !fresh {
		return a.refuse(errs.CodeAuthFailed, reasonNonceReused, appID)
	}

	a.pairings.Touch(appID, now)
	return pairing, nil
}

// refuse logs which check failed and returns the uniform answer.
func (a *Authenticator) refuse(code errs.Code, reason authReason, appID string) (Pairing, *errs.Error) {
	if reason != "" {
		slog.Info("api: refused a request", "code", string(code), "reason", string(reason), "appId", appID)
	} else {
		slog.Info("api: refused a request", "code", string(code), "appId", appID)
	}
	return Pairing{}, errs.New(code, errors.New(string(code)))
}

// requestPath is the PATH line of the canonical string: the request
// path without the query string, as it appeared in the request line.
//
// EscapedPath rather than Path: Path is the percent-decoded form, and a
// client signs what it sent. For every path this protocol defines the
// two are identical — they are ASCII and contain nothing that needs
// escaping — but a client that percent-encodes something anyway should
// still be able to authenticate.
func requestPath(r *http.Request) string {
	if r.URL == nil {
		return "/"
	}
	return r.URL.EscapedPath()
}
