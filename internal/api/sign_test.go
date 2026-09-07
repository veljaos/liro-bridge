package api

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/consent"
)

// b64digest is one valid SHA-256 digest, base64, as a caller sends it.
func b64digest(seed byte) string {
	sum := sha256.Sum256([]byte{seed})
	return base64.StdEncoding.EncodeToString(sum[:])
}

func TestSignDigestsRefusesEveryMalformedRequest(t *testing.T) {
	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{
			name:  "SHA-1 is never signed",
			body:  map[string]any{"certificateThumbprint": testThumbprint, "digestAlgorithm": "SHA1", "digests": []string{b64digest(1)}},
			field: "digestAlgorithm",
		},
		{
			name:  "an unknown algorithm",
			body:  map[string]any{"certificateThumbprint": testThumbprint, "digestAlgorithm": "MD5", "digests": []string{b64digest(1)}},
			field: "digestAlgorithm",
		},
		{
			name:  "no digests at all",
			body:  map[string]any{"certificateThumbprint": testThumbprint, "digestAlgorithm": "SHA256", "digests": []string{}},
			field: "digests",
		},
		{
			name: "a digest of the wrong length for the algorithm",
			body: map[string]any{"certificateThumbprint": testThumbprint, "digestAlgorithm": "SHA256",
				"digests": []string{base64.StdEncoding.EncodeToString([]byte("too short"))}},
			field: "digests",
		},
		{
			name: "a digest that is not base64",
			body: map[string]any{"certificateThumbprint": testThumbprint, "digestAlgorithm": "SHA256",
				"digests": []string{"not base64 at all !!"}},
			field: "digests",
		},
		{
			name: "more digests than the limit",
			body: map[string]any{"certificateThumbprint": testThumbprint, "digestAlgorithm": "SHA256",
				"digests": manyDigests(MaxDigests + 1)},
			field: "digests",
		},
		{
			name: "labels that do not line up with the digests",
			body: map[string]any{"certificateThumbprint": testThumbprint, "digestAlgorithm": "SHA256",
				"digests": []string{b64digest(1), b64digest(2)}, "labels": []string{"one.pdf"}},
			field: "labels",
		},
		{
			name:  "no certificate named",
			body:  map[string]any{"digestAlgorithm": "SHA256", "digests": []string{b64digest(1)}},
			field: "certificateThumbprint",
		},
		{
			name: "a thumbprint that is not hexadecimal",
			body: map[string]any{"certificateThumbprint": "not-a-thumbprint", "digestAlgorithm": "SHA256",
				"digests": []string{b64digest(1)}},
			field: "certificateThumbprint",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			c := h.client("My ERP", "https://erp.example.com")
			status, body := c.do(http.MethodPost, "/v2/sign", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status %d, want 400: %v", status, body)
			}
			if body["code"] != "REQUEST_INVALID" {
				t.Fatalf("code is %v, want REQUEST_INVALID", body["code"])
			}
			details, _ := body["details"].(map[string]any)
			if details["field"] != tc.field {
				t.Fatalf("details.field is %v, want %q", details["field"], tc.field)
			}
			if h.signer.count() != 0 {
				t.Fatal("a malformed request reached the signer")
			}
		})
	}
}

// TestNothingMalformedEverReachesAWindow is the property behind the
// table above, stated on its own: validation happens before a job is
// created, so a broken request never opens a window in front of a
// person and never occupies the application's one job slot.
func TestNothingMalformedEverReachesAWindow(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	status, _ := c.do(http.MethodPost, "/v2/sign",
		map[string]any{"certificateThumbprint": testThumbprint, "digestAlgorithm": "SHA1", "digests": []string{b64digest(1)}})
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", status)
	}
	if h.registry.Len() != 0 {
		t.Fatal("a malformed request created a job")
	}
	// And the application can still submit a good one straight away.
	if status, body := c.submitDigests(1); status != http.StatusAccepted {
		t.Fatalf("the next submission returned %d: %v", status, body)
	}
}

func TestSignDigestsAcceptsTheMaximumAndRefusesOneMore(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	status, body := c.do(http.MethodPost, "/v2/sign", map[string]any{
		"certificateThumbprint": testThumbprint,
		"digestAlgorithm":       "SHA256",
		"digests":               manyDigests(MaxDigests),
	})
	if status != http.StatusAccepted {
		t.Fatalf("%d digests were refused with %d: %v", MaxDigests, status, body)
	}
}

// TestLabelsAreSanitisedBeforeAnythingSeesThem is SPEC §6.6 for text
// that arrives from a program rather than from the file system: a
// direction override that would make "invoice\u202efdp.exe" render as
// "invoice exe.pdf" is gone before the window, the log or the audit
// entry could carry it.
func TestLabelsAreSanitisedBeforeAnythingSeesThem(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	hostile := "invoice\u202efdp.exe"
	long := strings.Repeat("d", consent.MaxDisplayLength*2) + ".pdf"
	status, body := c.do(http.MethodPost, "/v2/sign", map[string]any{
		"certificateThumbprint": testThumbprint,
		"digestAlgorithm":       "SHA256",
		"digests":               []string{b64digest(1), b64digest(2)},
		"labels":                []string{hostile, long},
	})
	if status != http.StatusAccepted {
		t.Fatalf("status %d: %v", status, body)
	}

	req, ok := h.signer.lastRequestAfter(t, 1)
	if !ok {
		t.Fatal("the signer was never called")
	}
	if strings.ContainsRune(req.Labels[0], '\u202e') {
		t.Errorf("a direction override survived into the label: %q", req.Labels[0])
	}
	if len([]rune(req.Labels[1])) > consent.MaxDisplayLength {
		t.Errorf("a label of %d characters survived, want at most %d",
			len([]rune(req.Labels[1])), consent.MaxDisplayLength)
	}
}

// TestTheApplicationNameComesFromPairing is SPEC §6.6 and F7 §2.2: an
// application cannot present itself as anything but the name a person
// approved, however it fills in its request.
func TestTheApplicationNameComesFromPairing(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	body := digestsRequest(1, testThumbprint)
	body["applicationName"] = "Liro"
	body["application"] = "Liro"
	if status, resp := c.do(http.MethodPost, "/v2/sign", body); status != http.StatusAccepted {
		t.Fatalf("status %d: %v", status, resp)
	}
	req, ok := h.signer.lastRequestAfter(t, 1)
	if !ok {
		t.Fatal("the signer was never called")
	}
	if req.Application != "Knjigovodstvo doo" {
		t.Fatalf("the flow was told the application is %q, want the name bound at pairing", req.Application)
	}
}

func TestSignDocumentsAcceptsAWholeDocumentAndReturnsIt(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	status, submit := c.do(http.MethodPost, "/v2/sign/pdf", documentsRequest(2))
	if status != http.StatusAccepted {
		t.Fatalf("status %d, want 202: %v", status, submit)
	}
	_, body := c.collect(submit["jobId"].(string))
	docs, ok := body["documents"].([]any)
	if !ok || len(docs) != 2 {
		t.Fatalf("documents is %v, want two entries", body["documents"])
	}
	first := docs[0].(map[string]any)
	if first["name"] != "ugovor-1.pdf" {
		t.Errorf("the first document is named %v", first["name"])
	}
	content, _ := first["content"].(string)
	if _, err := base64.StdEncoding.DecodeString(content); err != nil || content == "" {
		t.Errorf("the signed document is not base64: %q", content)
	}
	if first["achievedLevel"] != "B-T" {
		t.Errorf("achievedLevel is %v, want the level actually reached", first["achievedLevel"])
	}
}

// TestSignDocumentsBatchFingerprintIsOverTheDocumentsAsSent lets a
// caller check what was approved against what it sent, which is the
// whole reason SPEC §6.6 has a fingerprint at all.
func TestSignDocumentsBatchFingerprintIsOverTheDocumentsAsSent(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	body := documentsRequest(2)
	status, submit := c.do(http.MethodPost, "/v2/sign/pdf", body)
	if status != http.StatusAccepted {
		t.Fatalf("status %d: %v", status, submit)
	}

	var digests [][]byte
	for _, d := range body["documents"].([]map[string]any) {
		raw, err := base64.StdEncoding.DecodeString(d["content"].(string))
		if err != nil {
			t.Fatalf("decoding the test's own document: %v", err)
		}
		digests = append(digests, consent.DigestOf(raw))
	}
	if submit["batchFingerprint"] != consent.Fingerprint(digests) {
		t.Fatalf("batchFingerprint is %v, want %s", submit["batchFingerprint"], consent.Fingerprint(digests))
	}
}

func TestSignDocumentsRefusesEveryMalformedRequest(t *testing.T) {
	cases := []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"no documents", map[string]any{"documents": []any{}}, "documents"},
		{"content that is not base64", map[string]any{"documents": []map[string]any{{"name": "a.pdf", "content": "!!!"}}}, "documents"},
		{"an empty document", map[string]any{"documents": []map[string]any{{"name": "a.pdf", "content": ""}}}, "documents"},
		{"a level that is not a level", withLevel(documentsRequest(1), "b-xt"), "level"},
		{"a stamp with no answer in it", withStamp(documentsRequest(1), map[string]any{"position": "bottom-right"}), "stamp.visible"},
		{"a visible stamp with no corner", withStamp(documentsRequest(1), map[string]any{"visible": true}), "stamp.position"},
		{"a corner that is not one", withStamp(documentsRequest(1), map[string]any{"visible": true, "position": "middle"}), "stamp.position"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			c := h.client("My ERP", "https://erp.example.com")
			status, body := c.do(http.MethodPost, "/v2/sign/pdf", tc.body)
			if status != http.StatusBadRequest {
				t.Fatalf("status %d, want 400: %v", status, body)
			}
			details, _ := body["details"].(map[string]any)
			if details["field"] != tc.field {
				t.Fatalf("details.field is %v, want %q", details["field"], tc.field)
			}
		})
	}
}

func TestSignDocumentsRefusesTooManyDocuments(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	docs := make([]map[string]any, 0, MaxDocuments+1)
	for i := 0; i <= MaxDocuments; i++ {
		docs = append(docs, map[string]any{"name": "a.pdf", "content": base64.StdEncoding.EncodeToString([]byte("%PDF"))})
	}
	status, body := c.do(http.MethodPost, "/v2/sign/pdf", map[string]any{"documents": docs})
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %v", status, body)
	}
	details, _ := body["details"].(map[string]any)
	if got, ok := details["max"].(float64); !ok || int(got) != MaxDocuments {
		t.Fatalf("details.max is %v, want %d", details["max"], MaxDocuments)
	}
}

// TestAnOversizedRequestIsRefusedBeforeItIsRead is F7 §6's "enforce
// them before reading the body into memory": the declared length is
// what is checked, so a caller cannot make the agent hold half a
// gigabyte on the way to being told the request was too large.
func TestAnOversizedRequestIsRefusedBeforeItIsRead(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	oversized := []byte(strings.Repeat("x", int(maxSignBody)+1))
	req := c.request(http.MethodPost, "/v2/sign", oversized)
	if req.ContentLength <= maxSignBody {
		t.Fatalf("the test sent %d bytes, which is not over the %d-byte limit", req.ContentLength, maxSignBody)
	}
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, body, raw := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", status, raw)
	}
	details, _ := body["details"].(map[string]any)
	if got, ok := details["maxBytes"].(float64); !ok || int64(got) != maxSignBody {
		t.Fatalf("the refusal names the limit as %v, want %d", details["maxBytes"], maxSignBody)
	}
	if h.signer.count() != 0 {
		t.Fatal("an oversized request reached the signer")
	}
}

func TestSignDocumentsIsRefusedWhenItIsSwitchedOff(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")
	h.setDocumentSigning(false)

	status, body := c.do(http.MethodPost, "/v2/sign/pdf", documentsRequest(1))
	if status != http.StatusForbidden {
		t.Fatalf("status %d, want 403: %v", status, body)
	}
	if body["code"] != "DOCUMENT_SIGNING_DISABLED" {
		t.Fatalf("code is %v, want DOCUMENT_SIGNING_DISABLED", body["code"])
	}

	// The hash path is untouched: it is the one SPEC §4.3 calls the
	// most secure arrangement available, and switching off the other
	// one must not take it with it.
	if status, body := c.submitDigests(1); status != http.StatusAccepted {
		t.Fatalf("the hash path returned %d with documents switched off: %v", status, body)
	}

	// And it comes back without a restart, because the setting is read
	// each time rather than captured when the listener started. A
	// second application asks, so this cannot pass by being refused for
	// the other reason a submission can be refused.
	h.setDocumentSigning(true)
	other := h.client("Another ERP", "https://other.example.com")
	if status, body := other.do(http.MethodPost, "/v2/sign/pdf", documentsRequest(1)); status != http.StatusAccepted {
		t.Fatalf("status %d after being switched back on, want 202: %v", status, body)
	}
}

// TestASuppliedStampIsCarriedThroughUnchanged is what makes F7 §6's
// one-window, one-click case possible: an answer the caller gave is an
// answer the person is not asked again.
func TestASuppliedStampIsCarriedThroughUnchanged(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	body := withStamp(documentsRequest(1), map[string]any{"visible": true, "position": "top-left"})
	body["level"] = "b-t"
	if status, resp := c.do(http.MethodPost, "/v2/sign/pdf", body); status != http.StatusAccepted {
		t.Fatalf("status %d: %v", status, resp)
	}
	req, ok := h.signer.lastRequestAfter(t, 1)
	if !ok {
		t.Fatal("the signer was never called")
	}
	if req.Stamp == nil || !req.Stamp.Visible || req.Stamp.Position != "top-left" {
		t.Fatalf("the stamp reached the flow as %+v", req.Stamp)
	}
	if req.Level != "b-t" {
		t.Fatalf("the level reached the flow as %q", req.Level)
	}
}

func TestNoStampBlockMeansThePersonIsAsked(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")
	if status, resp := c.do(http.MethodPost, "/v2/sign/pdf", documentsRequest(1)); status != http.StatusAccepted {
		t.Fatalf("status %d: %v", status, resp)
	}
	req, ok := h.signer.lastRequestAfter(t, 1)
	if !ok {
		t.Fatal("the signer was never called")
	}
	if req.Stamp != nil {
		t.Fatalf("an absent stamp block became %+v rather than a question", req.Stamp)
	}
	if req.Level != "" {
		t.Fatalf("an absent level became %q rather than the agent's own setting", req.Level)
	}
}

// ---- helpers --------------------------------------------------------

func manyDigests(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, b64digest(byte(i%251)))
	}
	return out
}

func withLevel(body map[string]any, level string) map[string]any {
	body["level"] = level
	return body
}

func withStamp(body map[string]any, stamp map[string]any) map[string]any {
	body["stamp"] = stamp
	return body
}
