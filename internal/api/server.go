// Package api implements the local HTTP protocol other programs use to
// ask the agent to sign (SPEC §4.3, §6, F7).
//
// Three properties shape everything in here.
//
// It listens on loopback and nowhere else, and it is not a web API: it
// sends no CORS headers, refuses preflight outright, and requires a
// JSON content type on every call — so a page in a browser cannot reach
// it even from the same machine. That is the enforcement of F7 §2.3's
// rule that a device secret belongs on an integrator's server and never
// in a browser, where it would be a secret every visitor has.
//
// Every request is authenticated with an HMAC over a canonical string
// (canonical.go), and no request reaches the signing engine without a
// person pressing Approve in the agent's own window. SPEC §6.5 is
// explicit about why: the card caches its PIN independently of which
// process is talking to it, so the human is the only boundary that
// actually holds. A request that arrived over HTTP is not more trusted
// than a person dropping files, and gets the same window.
//
// Errors that cross this boundary are codes from internal/errs and
// nothing else — no human-readable message, in any language (SPEC §7).
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// maxPairingBody bounds a pairing request or confirmation body. Both
// carry a handful of short strings; anything larger is not one of them.
const maxPairingBody = 8 << 10

// Server routes the protocol's endpoints. It holds no listener: the
// agent creates one and hands it Handler(), so that what binds a socket
// stays in one place and this type stays testable with httptest.
type Server struct {
	pairings *Pairings
	flow     *PairingFlow
	auth     *Authenticator
}

// NewServer returns a Server over the given pairing store, pairing flow
// and authenticator.
func NewServer(pairings *Pairings, flow *PairingFlow, auth *Authenticator) *Server {
	return &Server{pairings: pairings, flow: flow, auth: auth}
}

// Handler returns the protocol's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/pair/request", s.handlePairRequest)
	mux.HandleFunc("/v2/pair/confirm", s.handlePairConfirm)
	return withProtocolRules(mux)
}

// withProtocolRules applies the rules that hold for every endpoint,
// before any of them sees a request.
//
//   - No CORS headers, ever, and no answer to a preflight. Together
//     with the JSON content type every endpoint requires, this is what
//     keeps a browser page from reaching the agent at all: a request
//     carrying X-Liro-* headers and application/json is never a "simple
//     request", so a browser must preflight it, and the preflight is
//     refused. F7 §2.3 says the device secret must live on a server and
//     never in a browser; this is that rule enforced rather than
//     documented.
//   - Nothing is cacheable. Responses carry job state and, once, a
//     device secret.
func withProtocolRules(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			writeError(w, http.StatusForbidden,
				errs.New(errs.CodeRequestInvalid, errors.New("preflight is not answered")))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON sends v with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		// Nothing this package serialises can fail to marshal; if one
		// ever does, the caller still gets a code rather than a
		// half-written body.
		slog.Error("api: could not marshal a response", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"INTERNAL"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeError sends an errs.Error as the protocol's error body:
//
//	{"code": "CARD_NOT_PRESENT", "details": {"reader": "..."}}
//
// errs.Error marshals to exactly that and nothing else — its own
// English description is on a method deliberately excluded from
// serialisation (SPEC §7, D-002).
func writeError(w http.ResponseWriter, status int, e *errs.Error) {
	writeJSON(w, status, e)
}

// statusFor maps a code to the HTTP status that carries it.
//
// The status is a second, coarser statement of the same fact, for
// proxies and for a caller's own error handling; the code is what a
// caller branches on. Where the two could disagree the code wins,
// which is why this table is small and dull.
func statusFor(code errs.Code) int {
	switch code {
	case errs.CodeRequestInvalid:
		return http.StatusBadRequest
	case errs.CodeNotPaired, errs.CodeAuthFailed, errs.CodePairingCodeIncorrect:
		return http.StatusUnauthorized
	case errs.CodePairingDenied, errs.CodePairingOriginMismatch:
		return http.StatusForbidden
	case errs.CodePairingExpired:
		return http.StatusGone
	case errs.CodePairingInProgress:
		return http.StatusConflict
	case errs.CodeRateLimited:
		return http.StatusTooManyRequests
	case errs.CodeVersionTooOld:
		return http.StatusUpgradeRequired
	default:
		return http.StatusInternalServerError
	}
}

// fail writes e with the status its code maps to.
func fail(w http.ResponseWriter, e *errs.Error) {
	writeError(w, statusFor(e.Code), e)
}

// requirePOST refuses anything but a POST.
func requirePOST(r *http.Request) *errs.Error {
	if r.Method != http.MethodPost {
		return errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("wrong method"), map[string]any{"expectedMethod": http.MethodPost})
	}
	return nil
}

// readJSONBody reads at most maxBytes of r's body, requires it to be
// declared as JSON, and decodes it into v.
//
// It returns the raw bytes as well, because an authenticated endpoint
// signs the body exactly as received, before any parsing (F7 §3) — a
// decoder that re-serialised what it read would be signing something
// else.
func readJSONBody(r *http.Request, maxBytes int64, v any) ([]byte, *errs.Error) {
	ct := r.Header.Get("Content-Type")
	if media, _, _ := strings.Cut(ct, ";"); strings.TrimSpace(media) != "application/json" {
		return nil, errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("content type is not application/json"),
			map[string]any{"expectedContentType": "application/json"})
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return nil, errs.New(errs.CodeRequestInvalid, err)
	}
	if int64(len(body)) > maxBytes {
		return nil, errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("request body is too large"), map[string]any{"maxBytes": maxBytes})
	}
	if v != nil {
		// Unknown fields are ignored rather than refused. The protocol
		// carries a minimum client version precisely because agent and
		// SDK versions drift apart on a real machine; an agent that
		// refuses a field a newer SDK added would turn every such drift
		// into a hard failure, for no gain — a misspelled field name
		// still surfaces, as the required field it was meant to be
		// going missing.
		if err := json.Unmarshal(body, v); err != nil {
			return body, errs.New(errs.CodeRequestInvalid, err)
		}
	}
	return body, nil
}
