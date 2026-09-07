package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestZZDateProbe(t *testing.T) {
	h := newHarness(t)
	resp, err := h.http.Client().Post(h.http.URL+"/v2/pair/request", "application/json",
		strings.NewReader(`{"applicationName":"X","origin":"https://x.example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	t.Logf("200 path headers: %v", resp.Header)

	bad, err := h.http.Client().Post(h.http.URL+"/v2/pair/request", "text/plain", strings.NewReader(`x`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bad.Body.Close() }()
	t.Logf("400 path headers: %v", bad.Header)

	req, _ := http.NewRequest(http.MethodOptions, h.http.URL+"/v2/pair/request", nil)
	pre, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pre.Body.Close() }()
	t.Logf("403 preflight headers: %v", pre.Header)
}
