package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// post sends body to the harness's real HTTP server and returns the
// status and the decoded response. Everything here goes over a real
// socket through the real handler, because the thing being checked is
// what a client actually receives.
func (h *harness) post(path string, body any, headers map[string]string) (int, map[string]any, []byte) {
	h.t.Helper()
	var raw []byte
	switch v := body.(type) {
	case nil:
	case string:
		raw = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			h.t.Fatalf("marshalling the request body: %v", err)
		}
		raw = b
	}

	req, err := http.NewRequest(http.MethodPost, h.http.URL+path, bytes.NewReader(raw))
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
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("reading the response: %v", err)
	}
	var decoded map[string]any
	if len(out) > 0 {
		if err := json.Unmarshal(out, &decoded); err != nil {
			h.t.Fatalf("the response is not JSON: %v\n%s", err, out)
		}
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		h.t.Fatalf("the response content type is %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		h.t.Fatalf("the response carries Access-Control-Allow-Origin: %q", got)
	}
	return resp.StatusCode, decoded, out
}

func TestPairOverHTTPEndToEnd(t *testing.T) {
	h := newHarness(t)

	status, body, _ := h.post("/v2/pair/request", map[string]any{
		"applicationName": "My ERP",
		"origin":          "https://erp.example.com",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("pair request returned %d: %v", status, body)
	}
	id, _ := body["requestId"].(string)
	if id == "" {
		t.Fatalf("no request identifier in %v", body)
	}
	if secs, _ := body["expiresInSeconds"].(float64); int(secs) != int(PairingCodeTTL.Seconds()) {
		t.Fatalf("expiresInSeconds is %v, want %v", body["expiresInSeconds"], PairingCodeTTL.Seconds())
	}

	// The code is nowhere in the response — not under any key, and not
	// hidden inside the identifier.
	win := h.ui.last()
	if win == nil {
		t.Fatal("no pairing window was shown")
	}
	if _, _, raw := h.post("/v2/pair/request", map[string]any{
		"applicationName": "probe", "origin": "https://probe.example.com",
	}, nil); bytes.Contains(raw, []byte(win.prompt.Code)) {
		t.Fatalf("a response carried the code: %s", raw)
	}
	if strings.Contains(id, win.prompt.Code) {
		t.Fatal("the request identifier contains the code")
	}

	status, body, out := h.post("/v2/pair/confirm", map[string]any{
		"requestId": id,
		"code":      win.prompt.Code,
		"origin":    "https://erp.example.com",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("pair confirm returned %d: %v", status, body)
	}
	appID, _ := body["appId"].(string)
	encoded, _ := body["deviceSecret"].(string)
	if appID == "" || encoded == "" {
		t.Fatalf("confirm returned %s", out)
	}
	secret, err := DecodeDeviceSecret(encoded)
	if err != nil || len(secret) != DeviceSecretLength {
		t.Fatalf("the device secret is not %d base64-encoded bytes: %v (%q)", DeviceSecretLength, err, encoded)
	}
	if got, _ := body["applicationName"].(string); got != "My ERP" {
		t.Fatalf("confirm reported the name as %q", got)
	}

	// And the secret it issued is the one that actually authenticates.
	r := newHTTPTestRequest(h, "POST", "/v2/sign", []byte(`{}`), appID, secret)
	if _, apiErr := h.auth.Authenticate(r.request(), r.sentBody); apiErr != nil {
		t.Fatalf("the issued secret does not authenticate: %v", apiErr)
	}
}

// Every response body is exactly {"code": ..., "details": ...} and
// carries no human-readable message, in any language (SPEC §7).
func TestErrorBodiesAreCodesAndNothingElse(t *testing.T) {
	h := newHarness(t)

	status, body, raw := h.post("/v2/pair/request", map[string]any{
		"applicationName": "My ERP", "origin": "",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("status is %d, want 400", status)
	}
	if body["code"] != string(errs.CodeRequestInvalid) {
		t.Fatalf("code is %v, want %s", body["code"], errs.CodeRequestInvalid)
	}
	for key := range body {
		if key != "code" && key != "details" {
			t.Fatalf("the error body carries %q; only code and details may cross this boundary:\n%s", key, raw)
		}
	}
	// Nothing in the body is prose. The one nested value is a field
	// name — a structured fact, which is what Details is for.
	if got := body["details"].(map[string]any)["field"]; got != "origin" {
		t.Fatalf("details are %v, want the offending field named", body["details"])
	}
}

func TestTheProtocolIsNotReachableFromABrowser(t *testing.T) {
	h := newHarness(t)

	// A preflight is refused outright, so a browser can never get as
	// far as sending the real request. Together with the JSON content
	// type every endpoint requires, that is what keeps a device secret
	// out of a page (F7 §2.3).
	req, err := http.NewRequest(http.MethodOptions, h.http.URL+"/v2/pair/request", nil)
	if err != nil {
		t.Fatalf("building the preflight: %v", err)
	}
	req.Header.Set("Origin", "https://erp.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", HeaderSignature)
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the preflight: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		t.Fatalf("the preflight was answered with %d", resp.StatusCode)
	}
	for _, header := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Credentials",
	} {
		if got := resp.Header.Get(header); got != "" {
			t.Fatalf("the preflight response carries %s: %q", header, got)
		}
	}
}

// A form post from a page is a "simple request" and needs no preflight
// — so the content type is the second half of the same defence.
func TestARequestThatIsNotJSONIsRefused(t *testing.T) {
	h := newHarness(t)

	for _, ct := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data", ""} {
		status, body, _ := h.post("/v2/pair/request", `{"applicationName":"X","origin":"https://x"}`,
			map[string]string{"Content-Type": ct})
		if status != http.StatusBadRequest || body["code"] != string(errs.CodeRequestInvalid) {
			t.Fatalf("content type %q was answered %d %v", ct, status, body)
		}
	}
	if h.ui.count() != 0 {
		t.Fatal("a request that was refused still opened a pairing window")
	}
}

func TestOnlyPOSTIsAccepted(t *testing.T) {
	h := newHarness(t)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead} {
		req, err := http.NewRequest(method, h.http.URL+"/v2/pair/request", nil)
		if err != nil {
			t.Fatalf("building a %s: %v", method, err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := h.http.Client().Do(req)
		if err != nil {
			t.Fatalf("sending a %s: %v", method, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s was answered %d, want 400", method, resp.StatusCode)
		}
	}
}

func TestAnOversizedPairingBodyIsRefused(t *testing.T) {
	h := newHarness(t)

	huge := `{"applicationName":"` + strings.Repeat("a", maxPairingBody) + `","origin":"https://x"}`
	status, body, _ := h.post("/v2/pair/request", huge, nil)
	if status != http.StatusBadRequest || body["code"] != string(errs.CodeRequestInvalid) {
		t.Fatalf("an oversized body was answered %d %v", status, body)
	}
}

// A browser's Origin header cannot be forged by the page, so where one
// exists it is authoritative and the declared origin must agree.
func TestTheOriginHeaderAndTheDeclaredOriginMustAgree(t *testing.T) {
	h := newHarness(t)

	status, body, _ := h.post("/v2/pair/request", map[string]any{
		"applicationName": "My ERP", "origin": "https://erp.example.com",
	}, map[string]string{"Origin": "https://evil.example.com"})
	if status != http.StatusBadRequest || body["code"] != string(errs.CodeRequestInvalid) {
		t.Fatalf("a mismatched Origin header was answered %d %v", status, body)
	}
	if h.ui.count() != 0 {
		t.Fatal("a mismatched Origin header still opened a pairing window")
	}
}

func TestPairingStatusesMatchTheirCodes(t *testing.T) {
	h := newHarness(t)

	// One live request, so the second is PAIRING_IN_PROGRESS.
	status, body, _ := h.post("/v2/pair/request", map[string]any{
		"applicationName": "First", "origin": "https://first.example.com",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("the first request returned %d %v", status, body)
	}
	id, _ := body["requestId"].(string)
	win := h.ui.last()

	status, body, _ = h.post("/v2/pair/request", map[string]any{
		"applicationName": "Second", "origin": "https://second.example.com",
	}, nil)
	if status != http.StatusConflict || body["code"] != string(errs.CodePairingInProgress) {
		t.Fatalf("a second pairing request was answered %d %v", status, body)
	}

	status, body, _ = h.post("/v2/pair/confirm", map[string]any{
		"requestId": id, "code": wrongCode(win.prompt.Code), "origin": "https://first.example.com",
	}, nil)
	if status != http.StatusUnauthorized || body["code"] != string(errs.CodePairingCodeIncorrect) {
		t.Fatalf("a wrong code was answered %d %v", status, body)
	}

	status, body, _ = h.post("/v2/pair/confirm", map[string]any{
		"requestId": id, "code": win.prompt.Code, "origin": "https://elsewhere.example.com",
	}, nil)
	if status != http.StatusForbidden || body["code"] != string(errs.CodePairingOriginMismatch) {
		t.Fatalf("confirm from another origin was answered %d %v", status, body)
	}

	status, body, _ = h.post("/v2/pair/confirm", map[string]any{
		"requestId": "0123456789abcdef0123456789abcdef", "code": "000000", "origin": "https://first.example.com",
	}, nil)
	if status != http.StatusGone || body["code"] != string(errs.CodePairingExpired) {
		t.Fatalf("an unknown request identifier was answered %d %v", status, body)
	}
}

func TestNothingIsCacheable(t *testing.T) {
	h := newHarness(t)

	resp, err := h.http.Client().Post(h.http.URL+"/v2/pair/request", "application/json",
		strings.NewReader(`{"applicationName":"My ERP","origin":"https://erp.example.com"}`))
	if err != nil {
		t.Fatalf("posting: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control is %q, want no-store", got)
	}
}
