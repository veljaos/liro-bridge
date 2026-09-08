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
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

// maxPairingBody bounds a pairing request or confirmation body. Both
// carry a handful of short strings; anything larger is not one of them.
const maxPairingBody = 8 << 10

// Options is everything a Server needs. Every field but Now is
// required for the endpoints that use it; a Server built without a
// Signer still serves pairing and health, which is what a test that is
// about pairing wants and nothing more.
type Options struct {
	Pairings *Pairings
	Flow     *PairingFlow
	Auth     *Authenticator
	Jobs     *jobs.Registry

	// Signer runs an accepted job: it shows the agent's own consent
	// window, waits for a person, opens the card and signs. It is an
	// interface, and this package never imports internal/ui, because
	// SPEC §4.2 rule 4 keeps the three front doors independent —
	// cmd/liro-bridge is the one place that knows a consent window is
	// drawn by WebView2.
	Signer Signer

	// Certificates enumerates what this machine can sign with, for
	// GET /v2/certificates. It is an interface for the same reason
	// Signer is: internal/api must not import internal/cli.
	Certificates CertificateSource

	// AgentVersion is what /v2/health reports and what bridge.json
	// carries.
	AgentVersion string

	// CertificateListingEnabled reports whether GET /v2/certificates is
	// available. A function rather than a bool for the same reason
	// DocumentSigningEnabled is one: the setting can change while the
	// agent is running, and a listener that had to be restarted to
	// notice would be a setting that silently did nothing.
	CertificateListingEnabled func() bool

	// DocumentSigningEnabled reports whether POST /v2/sign/pdf is
	// available (F7 §6: optional at install time, and disableable in
	// Settings). It is a function rather than a bool because the
	// setting can change while the agent is running, and a listener
	// that had to be restarted to notice would be a setting that
	// silently did nothing until the next reboot.
	DocumentSigningEnabled func() bool

	// Now may be nil, in which case time.Now is used.
	Now func() time.Time
}

// Server routes the protocol's endpoints. It holds no listener: the
// agent creates one and hands it Handler(), so that what binds a socket
// stays in one place and this type stays testable with httptest.
type Server struct {
	pairings *Pairings
	flow     *PairingFlow
	auth     *Authenticator
	jobs     *jobs.Registry
	signer   Signer

	certificates CertificateSource

	agentVersion       string
	documentSigning    func() bool
	certificateListing func() bool
	now                func() time.Time
}

// NewServer returns a Server over opts.
func NewServer(opts Options) *Server {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	documentSigning := opts.DocumentSigningEnabled
	if documentSigning == nil {
		documentSigning = func() bool { return true }
	}
	certificateListing := opts.CertificateListingEnabled
	if certificateListing == nil {
		certificateListing = func() bool { return true }
	}
	return &Server{
		pairings:           opts.Pairings,
		flow:               opts.Flow,
		auth:               opts.Auth,
		jobs:               opts.Jobs,
		signer:             opts.Signer,
		certificates:       opts.Certificates,
		agentVersion:       opts.AgentVersion,
		documentSigning:    documentSigning,
		certificateListing: certificateListing,
		now:                now,
	}
}

// Handler returns the protocol's routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/health", s.handleHealth)
	mux.HandleFunc("/v2/pair/request", s.handlePairRequest)
	mux.HandleFunc("/v2/pair/confirm", s.handlePairConfirm)
	mux.HandleFunc("/v2/certificates", s.handleCertificates)
	mux.HandleFunc("/v2/echo", s.handleEcho)
	mux.HandleFunc("/v2/sign", s.handleSignDigests)
	mux.HandleFunc("/v2/sign/pdf", s.handleSignDocuments)
	mux.HandleFunc("/v2/jobs/{id}/events", s.handleJobEvents)
	mux.HandleFunc("/v2/jobs/{id}/result", s.handleJobResult)
	return withProtocolRules(mux)
}

// withProtocolRules applies the rules that hold for every endpoint,
// before any of them sees a request.
//
//   - No CORS headers, ever, and no answer to a preflight. Together
//     with the four X-Liro-* headers every authenticated endpoint
//     requires, and the JSON content type every endpoint with a body
//     requires, this is what keeps a browser page from reaching the
//     agent at all: a request carrying those headers is never a
//     "simple request", so a browser must preflight it, and the
//     preflight is refused. F7 §2.3 says the device secret must live on
//     a server and never in a browser; this is that rule enforced
//     rather than documented.
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
//
// The default is 422 rather than 500, and that is the load-bearing
// line. Almost every code in this protocol means "understood, and it
// did not happen": the card was not there, the person said no, the
// document was not a PDF. Answering any of those 500 tells an
// integrator the agent is broken, and tells their monitoring the same
// — which is what the first real-client run reported, a job the person
// simply did not answer coming back as a server error. 500 is for
// INTERNAL and nothing else, which is what INTERNAL means (SPEC §7:
// "anything unclassified — always accompanied by a local log entry").
func statusFor(code errs.Code) int {
	switch code {
	case errs.CodeRequestInvalid:
		return http.StatusBadRequest
	case errs.CodeNotPaired, errs.CodeAuthFailed, errs.CodePairingCodeIncorrect:
		return http.StatusUnauthorized
	case errs.CodePairingDenied, errs.CodePairingOriginMismatch, errs.CodeDocumentSigningDisabled,
		errs.CodeCertificateListingDisabled, errs.CodeConsentDenied, errs.CodeConsentTimeout:
		// The person did not authorise it — by saying no, or by not
		// being there. Forbidden is what that is; there is nothing
		// wrong with the request and nothing wrong with the agent.
		return http.StatusForbidden
	case errs.CodePairingExpired:
		return http.StatusGone
	case errs.CodeJobNotFound:
		return http.StatusNotFound
	case errs.CodePairingInProgress, errs.CodeJobInProgress:
		return http.StatusConflict
	case errs.CodeRateLimited:
		return http.StatusTooManyRequests
	case errs.CodeVersionTooOld:
		return http.StatusUpgradeRequired
	case errs.CodeInternal:
		return http.StatusInternalServerError
	default:
		// Understood, and it did not happen: the card, the PIN, the
		// certificate, the document, the timestamp authority. A new
		// code lands here unless it is one of the cases above, which is
		// the right default — a condition nobody has classified yet is
		// far more likely to be one of these than to be this agent
		// failing.
		return http.StatusUnprocessableEntity
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

// requireJSONContentType refuses a request that does not declare a
// JSON body.
//
// It applies to every request that has a body, and to no request that
// does not — see handleHealth for the measurement behind that: .NET's
// HttpClient puts Content-Type on the content, so a GET with no body
// cannot carry one at all, and requiring it made every request from
// what will be F9's own SDK REQUEST_INVALID.
func requireJSONContentType(r *http.Request) *errs.Error {
	ct := r.Header.Get("Content-Type")
	if media, _, _ := strings.Cut(ct, ";"); strings.TrimSpace(media) != "application/json" {
		return errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("content type is not application/json"),
			map[string]any{"expectedContentType": "application/json"})
	}
	return nil
}

// tooLarge is the answer to a body over an endpoint's limit, naming the
// limit so a caller can act on it without reading the documentation.
func tooLarge(maxBytes int64) *errs.Error {
	return errs.WithDetails(errs.CodeRequestInvalid,
		errors.New("request body is too large"), map[string]any{"maxBytes": maxBytes})
}

// readBody reads at most maxBytes of r's body and requires it to be
// declared as JSON. It does not parse it.
//
// The raw bytes are what an authenticated endpoint's signature covers,
// hashed before any parsing (F7 §3) — a decoder that re-serialised
// what it read would be signing something else. Parsing happens after
// authentication, so a caller that has not authenticated learns
// nothing about how this agent reads a request.
func readBody(r *http.Request, maxBytes int64) ([]byte, *errs.Error) {
	if e := requireJSONContentType(r); e != nil {
		return nil, e
	}
	// The declared length is checked before a single byte is read, so a
	// caller cannot make the agent hold half a gigabyte in memory on
	// the way to being told the request was too large (F7 §6: "enforce
	// them before reading the body into memory"). The reader below is
	// the backstop for a request that declares no length at all.
	if r.ContentLength > maxBytes {
		return nil, tooLarge(maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return nil, errs.New(errs.CodeRequestInvalid, err)
	}
	if int64(len(body)) > maxBytes {
		return nil, tooLarge(maxBytes)
	}
	return body, nil
}

// decodeJSON parses a body that has already been read.
//
// Unknown fields are ignored rather than refused. The protocol carries
// a minimum client version precisely because agent and SDK versions
// drift apart on a real machine; an agent that refused a field a newer
// SDK added would turn every such drift into a hard failure, for no
// gain — a misspelled field name still surfaces, as the required field
// it was meant to be going missing.
func decodeJSON(body []byte, v any) *errs.Error {
	if err := json.Unmarshal(body, v); err != nil {
		return errs.New(errs.CodeRequestInvalid, err)
	}
	return nil
}

// readJSONBody reads and decodes in one step, for the two endpoints
// that have nothing to authenticate first: pairing has no secret yet.
func readJSONBody(r *http.Request, maxBytes int64, v any) ([]byte, *errs.Error) {
	body, e := readBody(r, maxBytes)
	if e != nil {
		return nil, e
	}
	if v != nil {
		if e := decodeJSON(body, v); e != nil {
			return body, e
		}
	}
	return body, nil
}
