package api

import (
	"net/http"
	"strings"
	"testing"
)

// get sends an unauthenticated GET to the harness's real server.
func (h *harness) get(path string, headers map[string]string) (int, map[string]any, []byte) {
	h.t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.http.URL+path, nil)
	if err != nil {
		h.t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := h.http.Client().Do(req)
	if err != nil {
		h.t.Fatalf("sending the request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return statusAndBody(h, resp)
}

func TestHealthNeedsNoAuthenticationAndSaysThreeThings(t *testing.T) {
	h := newHarness(t)
	status, body, raw := h.get("/v2/health", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200\n%s", status, raw)
	}
	if body["agentVersion"] != testAgentVersion {
		t.Errorf("agentVersion is %v, want %q", body["agentVersion"], testAgentVersion)
	}
	if got, ok := body["protocolVersion"].(float64); !ok || int(got) != ProtocolVersion {
		t.Errorf("protocolVersion is %v, want %d", body["protocolVersion"], ProtocolVersion)
	}
	if body["minimumClientVersion"] != MinimumClientVersion {
		t.Errorf("minimumClientVersion is %v, want %q", body["minimumClientVersion"], MinimumClientVersion)
	}
	if len(body) != 3 {
		t.Fatalf("/v2/health returned %d fields, want exactly 3: %v", len(body), body)
	}
}

// TestHealthRevealsNothingElse is F7 §4.3's own sentence as a test. The
// endpoint takes no authentication, so anything it says is said to
// anybody who can open a socket to this machine — which on a terminal
// server is every other person signed in to it.
func TestHealthRevealsNothingElse(t *testing.T) {
	h := newHarness(t)
	h.pair("Knjigovodstvo doo", "https://erp.example.com")

	_, _, raw := h.get("/v2/health", nil)
	lowered := strings.ToLower(string(raw))
	for _, forbidden := range []string{
		"knjigovodstvo", "erp.example.com", "certificate", "thumbprint",
		"pairing", "appid", "user", "job",
	} {
		if strings.Contains(lowered, forbidden) {
			t.Errorf("/v2/health mentions %q:\n%s", forbidden, raw)
		}
	}
}

// TestHealthAnswersAGETWithNoContentTypeAtAll is the shape a real HTTP
// client actually sends.
//
// It is a test because the opposite was built first and a real client
// refused it: .NET's HttpClient puts Content-Type on the content, so a
// GET with no body cannot carry one, and every request from what will
// be F9's own SDK came back REQUEST_INVALID. What keeps a browser out
// is the preflight refusal and the absence of any CORS header, not a
// content type a GET has no way to send.
func TestHealthAnswersAGETWithNoContentTypeAtAll(t *testing.T) {
	h := newHarness(t)
	status, body, raw := h.get("/v2/health", map[string]string{"Content-Type": ""})
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", status, raw)
	}
	if body["agentVersion"] != testAgentVersion {
		t.Fatalf("agentVersion is %v", body["agentVersion"])
	}
}

// TestAPageCannotReadWhatHealthSays is why the above is safe. A page
// can make this request; it cannot read the answer, because no
// Access-Control-Allow-Origin header is ever sent — statusAndBody
// fails the test if one appears.
func TestAPageCannotReadWhatHealthSays(t *testing.T) {
	h := newHarness(t)
	req, err := http.NewRequest(http.MethodGet, h.http.URL+"/v2/health", nil)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, _, raw := statusAndBody(h, resp); len(raw) == 0 {
		t.Fatal("the response is empty")
	}
	for _, header := range []string{
		"Access-Control-Allow-Origin", "Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods", "Access-Control-Expose-Headers",
	} {
		if got := resp.Header.Get(header); got != "" {
			t.Fatalf("the response carries %s: %q", header, got)
		}
	}
}

func TestHealthRefusesAnythingButGET(t *testing.T) {
	h := newHarness(t)
	status, body, raw := h.post("/v2/health", map[string]any{}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400\n%s", status, raw)
	}
	if body["code"] != "REQUEST_INVALID" {
		t.Fatalf("code is %v, want REQUEST_INVALID", body["code"])
	}
}
