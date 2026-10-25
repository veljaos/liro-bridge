package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// F14 §8, [[D-461]]: the health answer lists every version, a request
// naming one this agent does not speak is refused with the versions it
// does, and a request naming none — every client before this — is
// version 2 and served as it always was.
func TestHealthListsEveryProtocolVersionThisAgentSpeaks(t *testing.T) {
	srv := NewServer(Options{AgentVersion: "test"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v2/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health: %d %s", rec.Code, rec.Body)
	}
	var body struct {
		ProtocolVersion  int   `json:"protocolVersion"`
		ProtocolVersions []int `json:"protocolVersions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.ProtocolVersions) == 0 || body.ProtocolVersions[len(body.ProtocolVersions)-1] != body.ProtocolVersion {
		t.Fatalf("protocolVersions %v does not end with protocolVersion %d", body.ProtocolVersions, body.ProtocolVersion)
	}
}

func TestARequestForAProtocolVersionThisAgentDoesNotSpeakIsRefusedWithTheOnesItDoes(t *testing.T) {
	srv := NewServer(Options{AgentVersion: "test"})
	for _, h := range []string{"3", "1", "two", "-2"} {
		req := httptest.NewRequest(http.MethodGet, "/v2/health", nil)
		req.Header.Set(ProtocolHeader, h)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUpgradeRequired {
			t.Errorf("%s: %q answered %d, want 426", ProtocolHeader, h, rec.Code)
			continue
		}
		var body struct {
			Code    string `json:"code"`
			Details struct {
				ProtocolVersions []int `json:"protocolVersions"`
			} `json:"details"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != "PROTOCOL_UNSUPPORTED" || len(body.Details.ProtocolVersions) == 0 {
			t.Errorf("%q: answered %s", h, rec.Body)
		}
	}
	for _, h := range []string{"", "2", " 2 "} {
		req := httptest.NewRequest(http.MethodGet, "/v2/health", nil)
		if h != "" {
			req.Header.Set(ProtocolHeader, h)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: %q answered %d, want 200", ProtocolHeader, h, rec.Code)
		}
	}
}
