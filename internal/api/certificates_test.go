package api

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// The hole this endpoint closes: /v2/sign requires a thumbprint and
// nothing told a caller where to get one.
func TestTheListingNamesTheCertificatesSignCanBeAskedFor(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	status, body := c.do(http.MethodGet, "/v2/certificates", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200: %v", status, body)
	}
	certs, ok := body["certificates"].([]any)
	if !ok || len(certs) != 2 {
		t.Fatalf("the listing is %v", body["certificates"])
	}

	first, _ := certs[0].(map[string]any)
	if first["thumbprint"] != "7758D4D4B8973EA619B3225185EDE740B3D1ECCE" {
		t.Errorf("thumbprint = %v", first["thumbprint"])
	}
	if first["displayName"] != "ВЕЉКО СТАНОЈЕВИЋ" {
		t.Errorf("displayName = %v", first["displayName"])
	}
	if first["issuer"] != "MUPGradjaniCA4" {
		t.Errorf("issuer = %v", first["issuer"])
	}
	if first["purpose"] != "signing" {
		t.Errorf("purpose = %v", first["purpose"])
	}
	if first["qualified"] != true || first["usable"] != true {
		t.Errorf("qualified/usable = %v/%v", first["qualified"], first["usable"])
	}
	if _, present := first["notUsableReason"]; present {
		t.Errorf("a usable certificate carries a reason it is not: %v", first["notUsableReason"])
	}

	// The other half of what an SDK needs to offer a choice: a
	// certificate that cannot sign right now is listed, and says why —
	// exactly as the agent's own window shows it (SPEC §11.5).
	second, _ := certs[1].(map[string]any)
	if second["usable"] != false {
		t.Errorf("the second certificate is reported usable")
	}
	if second["notUsableReason"] != string(errs.CodeCardNotPresent) {
		t.Errorf("notUsableReason = %v, want %s", second["notUsableReason"], errs.CodeCardNotPresent)
	}

	// The thumbprint the listing gives is the thumbprint /v2/sign
	// takes, which is the whole point of the endpoint: no case
	// conversion, no prefix, nothing to reformat.
	//
	// 202 means accepted, not done — the run is on its own goroutine —
	// so the job is followed to its end before the signer is read.
	// Reading it without that is a race, and one this test lost on a
	// loaded runner (D-252).
	status, submit := c.do(http.MethodPost, "/v2/sign", digestsRequest(1, first["thumbprint"].(string)))
	if status != http.StatusAccepted {
		t.Fatalf("signing with the thumbprint the listing gave answers %d", status)
	}
	c.awaitJob(submit["jobId"].(string))
	req, ok := h.signer.lastRequest()
	if !ok || req.Thumbprint != first["thumbprint"] {
		t.Fatalf("the signer was asked for %q", req.Thumbprint)
	}
}

// The rule that decides the shape of the response: nothing the
// classification layer strips, and no certificate.
//
// A Serbian qualified certificate carries the holder's national
// identity number in its Subject DN and their email in the DN or the
// SAN (SPEC §11.6). Both are scrubbed before anything is displayed, and
// this endpoint reports what that scrubbing produced. The DER carries
// both in full, so it is not here — which is the same reason the audit
// log has no field one could travel in (D-084).
func TestTheListingCarriesNoCertificateAndNoPersonalIdentifier(t *testing.T) {
	h := newHarness(t)
	h.certs.set([]Certificate{{
		Thumbprint:  "AABBCCDD",
		DisplayName: "Petar Petrović",
		Issuer:      "Posta Srbije CA 1",
		Purpose:     "signing",
		Qualified:   true,
		Usable:      true,
	}}, nil)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	resp, err := h.http.Client().Do(c.request(http.MethodGet, "/v2/certificates", nil))
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, body, raw := statusAndBody(h, resp)

	for _, forbidden := range []string{"pem", "der", "certificate", "publicKey", "subject", "email"} {
		for key := range body["certificates"].([]any)[0].(map[string]any) {
			if strings.EqualFold(key, forbidden) {
				t.Errorf("the listing carries a %q field", key)
			}
		}
	}
	if strings.Contains(string(raw), "BEGIN CERTIFICATE") {
		t.Error("the listing carries a PEM certificate")
	}
	// Not a proxy for the rule but the rule itself, on the one shape it
	// is easiest to leak by: thirteen consecutive digits is a JMBG
	// (SPEC §11.6), and an "@" is an email address.
	if strings.ContainsAny(string(raw), "@") {
		t.Errorf("the listing carries an email address:\n%s", raw)
	}
	if thirteenDigits(string(raw)) {
		t.Errorf("the listing carries thirteen consecutive digits:\n%s", raw)
	}
}

func thirteenDigits(s string) bool {
	run := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			run++
			if run >= 13 {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

// Authenticated like everything else, and no more forgiving about it.
func TestTheListingIsAuthenticatedLikeEveryOtherEndpoint(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	// Unsigned.
	resp, err := h.http.Client().Get(h.http.URL + "/v2/certificates")
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, body, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusUnauthorized || body["code"] != string(errs.CodeAuthFailed) {
		t.Fatalf("an unsigned listing answers %d %v", status, body["code"])
	}
	if h.certs.count() != 0 {
		t.Fatal("the store was enumerated for a request that did not authenticate")
	}

	// Signed, then tampered with.
	req := c.request(http.MethodGet, "/v2/certificates", nil)
	req.Header.Set(HeaderNonce, "a-different-nonce")
	resp, err = h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, body, _ = statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusUnauthorized || body["code"] != string(errs.CodeAuthFailed) {
		t.Fatalf("a tampered listing answers %d %v", status, body["code"])
	}

	// A revoked pairing is NOT_PAIRED here as anywhere else.
	if err := h.pairings.Revoke(c.pairing.AppID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	status, body = c.do(http.MethodGet, "/v2/certificates", nil)
	if status != http.StatusUnauthorized || body["code"] != string(errs.CodeNotPaired) {
		t.Fatalf("a revoked pairing answers %d %v", status, body["code"])
	}
}

// The switch: the person at the machine can decline to answer, and an
// application that asks is told so rather than told it is unpaired.
func TestTheListingCanBeSwitchedOff(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	h.setCertificateListing(false)
	status, body := c.do(http.MethodGet, "/v2/certificates", nil)
	if status != http.StatusForbidden {
		t.Fatalf("status %d, want 403", status)
	}
	if body["code"] != string(errs.CodeCertificateListingDisabled) {
		t.Fatalf("code = %v, want %s", body["code"], errs.CodeCertificateListingDisabled)
	}
	if h.certs.count() != 0 {
		t.Fatal("the store was enumerated for a listing that is switched off")
	}

	// Signing is unaffected: the switch is about who may be told what
	// is here, not about what may be signed.
	if status, _ := c.do(http.MethodPost, "/v2/sign", digestsRequest(1, "7758D4")); status != http.StatusAccepted {
		t.Fatalf("signing answers %d with the listing off", status)
	}

	// And it is read per request, not captured when the listener
	// started (D-134): turning it back on works without a restart.
	h.setCertificateListing(true)
	if status, _ := c.do(http.MethodGet, "/v2/certificates", nil); status != http.StatusOK {
		t.Fatalf("status %d after turning the listing back on", status)
	}
}

// A caller that has not authenticated must not learn whether the
// listing is switched off — that is a fact about how this machine is
// set up (D-195's reasoning, applied to the second switch).
func TestAnUnpairedCallerLearnsNothingAboutTheSwitch(t *testing.T) {
	h := newHarness(t)
	h.setCertificateListing(false)

	resp, err := h.http.Client().Get(h.http.URL + "/v2/certificates")
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, body, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if body["code"] != string(errs.CodeAuthFailed) {
		t.Fatalf("an unauthenticated caller is told %v (%d)", body["code"], status)
	}
}

// The listing is a GET with no body, so it does not declare a content
// type — the rule §2.5 records because .NET's HttpClient cannot break
// it (D-197).
func TestTheListingAnswersAGETWithNoContentTypeAtAll(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	req := c.request(http.MethodGet, "/v2/certificates", nil)
	req.Header.Del("Content-Type")
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, body, _ := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200: %v", status, body)
	}
}

// Anything but a GET is a request to fix.
func TestTheListingRefusesAnythingButAGET(t *testing.T) {
	h := newHarness(t)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	status, body := c.do(http.MethodPost, "/v2/certificates", map[string]any{})
	if status != http.StatusBadRequest || body["code"] != string(errs.CodeRequestInvalid) {
		t.Fatalf("POST answers %d %v", status, body["code"])
	}
}

// An enumeration that fails is this agent's own problem, not the
// caller's request — INTERNAL, and the agent logs it.
func TestAnEnumerationFailureIsInternal(t *testing.T) {
	h := newHarness(t)
	h.certs.set(nil, errors.New("the smart card service is not running"))
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	status, body := c.do(http.MethodGet, "/v2/certificates", nil)
	if status != http.StatusInternalServerError || body["code"] != string(errs.CodeInternal) {
		t.Fatalf("status %d code %v", status, body["code"])
	}
}

// An empty machine is an empty list, not an error: a person with no
// card in yet is not a fault, and an SDK showing "no certificates" is
// the right thing for it to show.
func TestAMachineWithNoCertificatesAnswersAnEmptyList(t *testing.T) {
	h := newHarness(t)
	h.certs.set(nil, nil)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	status, body := c.do(http.MethodGet, "/v2/certificates", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	certs, ok := body["certificates"].([]any)
	if !ok || len(certs) != 0 {
		t.Fatalf("certificates = %v, want an empty array", body["certificates"])
	}
}

// A soft-token certificate is marked as one wherever it appears
// (SPEC §16.6). A caller that could not tell would be the one place it
// was not.
func TestATestKeyIsMarkedInTheListing(t *testing.T) {
	h := newHarness(t)
	h.certs.set([]Certificate{{
		Thumbprint: "DEADBEEF", DisplayName: "Test Key", Issuer: "liro-softtoken",
		Purpose: "signing", Usable: true, IsTestKey: true,
	}}, nil)
	c := h.client("Knjigovodstvo doo", "https://erp.example.com")

	_, body := c.do(http.MethodGet, "/v2/certificates", nil)
	first := body["certificates"].([]any)[0].(map[string]any)
	if first["isTestKey"] != true {
		t.Fatalf("a soft-token certificate is not marked: %v", first)
	}
}

// The interface is what keeps internal/api out of internal/cli
// (SPEC §4.2 rule 4). This is the compile-time statement of it.
var _ CertificateSource = (*fakeCertificates)(nil)
