package api

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// quietLogger keeps net/http's own "panic serving" report out of the
// test output: these tests cause the panics on purpose.
func quietLogger() *log.Logger { return log.New(io.Discard, "", 0) }

// The one promise PROTOCOL.md §7 makes about every failure: "Every
// error body is {code, details?} ... and nothing else."
//
// It was not true. An SDK asked a stale agent for GET /v2/certificates
// — a route that agent was built before — and net/http's own ServeMux
// answered it: HTTP 404, Content-Type text/plain, body "404 page not
// found". No JSON, no code, nothing an integrator can branch on. The
// SDK could only report PROTOCOL_VIOLATION, which is true and useless.
//
// These tests are the audit that found it, kept: every way this package
// can answer a request must carry a code (D-264).

// TestAnUnknownPathAnswersWithACode covers the defect directly.
func TestAnUnknownPathAnswersWithACode(t *testing.T) {
	h := newHarness(t)

	// Deliberately unauthenticated: an integrator whose SDK is newer
	// than the agent has a perfectly good pairing and is still asking
	// for a route that is not there. The answer must not depend on
	// credentials it already has.
	for _, path := range []string{
		"/v2/certificates/",      // a trailing slash is a different path to ServeMux
		"/v2/something-invented", // a route from a future protocol
		"/v2/jobs/abc/nonsense",  // a wildcard route's unknown tail
		"/",                      // the root
		"/v2",                    // a prefix of a real route
	} {
		t.Run(path, func(t *testing.T) {
			resp, err := h.http.Client().Get(h.http.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)

			if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("Content-Type is %q, want JSON — a plain-text body is the defect itself", got)
			}
			var parsed struct {
				Code    string         `json:"code"`
				Details map[string]any `json:"details"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatalf("the body is not JSON (%q): %v", body, err)
			}
			if parsed.Code != string(errs.CodeEndpointNotFound) {
				t.Errorf("code is %q, want %s", parsed.Code, errs.CodeEndpointNotFound)
			}
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("status is %d, want 404", resp.StatusCode)
			}
			if parsed.Details["path"] != path {
				t.Errorf("details.path is %v, want %q", parsed.Details["path"], path)
			}
		})
	}
}

// TestTheUnknownPathAnswerCarriesNoQueryString: details are structured
// facts, and a query string is the part of a URL most likely to carry
// something a caller did not mean to have echoed back to it.
func TestTheUnknownPathAnswerCarriesNoQueryString(t *testing.T) {
	h := newHarness(t)

	resp, err := h.http.Client().Get(h.http.URL + "/v2/invented?token=sekret")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if strings.Contains(string(body), "sekret") {
		t.Errorf("the query string was echoed back: %s", body)
	}
}

// TestEveryRealRouteStillAnswersItsOwnCode: a catch-all registered at
// "/" matches everything, so the thing to prove is that it did not
// swallow the routes that already worked. Each of these must answer
// with its own code rather than ENDPOINT_NOT_FOUND.
func TestEveryRealRouteStillAnswersItsOwnCode(t *testing.T) {
	h := newHarness(t)

	// Unauthenticated calls to real routes: the code differs per route,
	// but none of them may be ENDPOINT_NOT_FOUND.
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v2/health"},
		{http.MethodPost, "/v2/pair/request"},
		{http.MethodPost, "/v2/pair/confirm"},
		{http.MethodGet, "/v2/certificates"},
		{http.MethodPost, "/v2/echo"},
		{http.MethodPost, "/v2/sign"},
		{http.MethodPost, "/v2/sign/pdf"},
		{http.MethodGet, "/v2/jobs/whatever/events"},
		{http.MethodGet, "/v2/jobs/whatever/result"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, h.http.URL+tc.path, strings.NewReader("{}"))
			if err != nil {
				t.Fatalf("building the request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := h.http.Client().Do(req)
			if err != nil {
				t.Fatalf("sending: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, _ := io.ReadAll(resp.Body)

			// Health answers 200 and is not an error at all.
			if resp.StatusCode == http.StatusOK {
				return
			}
			var parsed struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatalf("the body is not JSON (%q): %v", body, err)
			}
			if parsed.Code == "" {
				t.Fatalf("answered %d with no code: %q", resp.StatusCode, body)
			}
			if parsed.Code == string(errs.CodeEndpointNotFound) {
				t.Errorf("the catch-all swallowed a real route: %s %s", tc.method, tc.path)
			}
		})
	}
}

// panickingCertificates is a certificate source that panics, standing
// in for any handler bug at all.
type panickingCertificates struct{}

func (panickingCertificates) Certificates(context.Context) ([]Certificate, error) {
	panic("a handler bug")
}

// TestAPanickingHandlerAnswersINTERNAL is the audit's second finding.
//
// runJob already recovers its own panics and fails the job; the HTTP
// handlers did not, so a panic in one reached net/http, which closes
// the connection without writing anything at all. To a caller that is
// a network error rather than an answer — the same defect as the bare
// 404, one layer up.
func TestAPanickingHandlerAnswersINTERNAL(t *testing.T) {
	h := newHarness(t)
	h.server.certificates = panickingCertificates{}
	// net/http logs the recovered panic; keep it out of the test output.
	h.http.Config.ErrorLog = quietLogger()

	c := h.client("Panicking app", "local")
	status, body := c.do(http.MethodGet, "/v2/certificates", nil)

	if status != http.StatusInternalServerError {
		t.Fatalf("status is %d, want 500: %v", status, body)
	}
	if body["code"] != string(errs.CodeInternal) {
		t.Fatalf("code is %v, want INTERNAL", body["code"])
	}
}

// TestTheRecoveredHandlerDoesNotBreakStreaming: the panic guard wraps
// every ResponseWriter, and GET /v2/jobs/{id}/events refuses to stream
// at all unless its writer is an http.Flusher. A wrapper that dropped
// that interface would turn the progress stream into one buffered
// delivery at the end, which is not progress — and no existing test
// would have noticed, because the stream still arrives.
func TestTheRecoveredHandlerDoesNotBreakStreaming(t *testing.T) {
	var sawFlusher bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, sawFlusher = w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewServer(recoverPanics(inner))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/anything")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if !sawFlusher {
		t.Error("the wrapped writer is not an http.Flusher, so the event stream would buffer")
	}
}

// TestAPanicAfterTheResponseStartedIsNotDoubleWritten: once the status
// line is gone there is nothing truthful left to write, and trying
// would only produce net/http's "superfluous WriteHeader" warning.
func TestAPanicAfterTheResponseStartedIsNotDoubleWritten(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("after the headers")
	})

	srv := httptest.NewServer(recoverPanics(inner))
	srv.Config.ErrorLog = quietLogger()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/anything")
	if err != nil {
		// A connection torn down mid-body is a legitimate outcome here;
		// what must not happen is a second WriteHeader.
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status is %d, want the 200 that was already sent", resp.StatusCode)
	}
}

// TestEveryCodeHasAStatus keeps the new code inside the existing rule
// rather than beside it: statusFor must not fall through to a default
// for anything in errs.AllCodes.
func TestEndpointNotFoundIsA404(t *testing.T) {
	if got := statusFor(errs.CodeEndpointNotFound); got != http.StatusNotFound {
		t.Errorf("ENDPOINT_NOT_FOUND maps to %d, want 404", got)
	}
	if statusFor(errs.CodeEndpointNotFound) == statusFor(errs.CodeInternal) {
		t.Error("ENDPOINT_NOT_FOUND must not be a server error")
	}
}
