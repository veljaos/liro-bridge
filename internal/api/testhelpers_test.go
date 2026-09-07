package api

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// memSecretStore is an in-memory SecretStore. The real one encrypts
// with DPAPI and is exercised by internal/platform's own tests; nothing
// in this package's behaviour depends on which one it is given, which
// is the whole reason SecretStore is an interface here.
type memSecretStore struct {
	mu     sync.Mutex
	values map[string][]byte
	setErr error
}

func newMemSecretStore() *memSecretStore {
	return &memSecretStore{values: map[string][]byte{}}
}

func (m *memSecretStore) Get(name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.values[name]
	if !ok {
		return nil, platform.ErrSecretNotFound
	}
	return append([]byte(nil), v...), nil
}

func (m *memSecretStore) Set(name string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setErr != nil {
		return m.setErr
	}
	m.values[name] = append([]byte(nil), value...)
	return nil
}

func (m *memSecretStore) Delete(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, name)
	return nil
}

func (m *memSecretStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.values)
}

// fakeWindow stands in for the agent's pairing window.
type fakeWindow struct {
	mu        sync.Mutex
	denied    chan struct{}
	closed    bool
	confirmed bool

	prompt PairingPrompt
}

func newFakeWindow(p PairingPrompt) *fakeWindow {
	return &fakeWindow{denied: make(chan struct{}), prompt: p}
}

func (w *fakeWindow) Denied() <-chan struct{} { return w.denied }

func (w *fakeWindow) Confirmed() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.confirmed = true
}

func (w *fakeWindow) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
}

func (w *fakeWindow) deny() {
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case <-w.denied:
	default:
		close(w.denied)
	}
}

func (w *fakeWindow) state() (closed, confirmed bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closed, w.confirmed
}

// fakeUI records every prompt it was asked to show.
type fakeUI struct {
	mu      sync.Mutex
	windows []*fakeWindow
	err     error
}

func (u *fakeUI) ShowPairing(p PairingPrompt) (PairingWindow, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.err != nil {
		return nil, u.err
	}
	w := newFakeWindow(p)
	u.windows = append(u.windows, w)
	return w, nil
}

func (u *fakeUI) last() *fakeWindow {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.windows) == 0 {
		return nil
	}
	return u.windows[len(u.windows)-1]
}

func (u *fakeUI) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.windows)
}

// harness is one agent's worth of protocol state, wired to a fake
// window and a clock the test moves by hand.
type harness struct {
	t        *testing.T
	clock    *fakeClock
	secrets  *memSecretStore
	pairings *Pairings
	ui       *fakeUI
	flow     *PairingFlow
	nonces   *NonceCache
	auth     *Authenticator
	server   *Server
	http     *httptest.Server

	// expire is fed by the test to make a pairing request's watcher
	// believe its five minutes are up, without waiting five minutes.
	expire chan time.Time
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	clock := newFakeClock()
	secrets := newMemSecretStore()
	pairings, err := OpenPairings(filepath.Join(t.TempDir(), "pairings.json"), secrets, clock.Now)
	if err != nil {
		t.Fatalf("OpenPairings: %v", err)
	}
	h := &harness{
		t:        t,
		clock:    clock,
		secrets:  secrets,
		pairings: pairings,
		ui:       &fakeUI{},
		nonces:   NewNonceCache(clock.Now),
		expire:   make(chan time.Time, 1),
	}
	h.flow = NewPairingFlow(pairings, h.ui, clock.Now, func(time.Duration) <-chan time.Time { return h.expire })
	h.auth = NewAuthenticator(pairings, h.nonces, clock.Now)
	h.server = NewServer(pairings, h.flow, h.auth)
	h.http = httptest.NewServer(h.server.Handler())
	t.Cleanup(h.http.Close)
	return h
}

// pair runs a whole pairing — request, read the code off the window,
// confirm — and returns the paired application and its device secret.
func (h *harness) pair(name, origin string) (Pairing, []byte) {
	h.t.Helper()
	id, _, e := h.flow.Request(name, origin)
	if e != nil {
		h.t.Fatalf("pair request: %v", e)
	}
	win := h.ui.last()
	if win == nil {
		h.t.Fatal("no pairing window was shown")
	}
	result, e := h.flow.Confirm(id, win.prompt.Code, origin)
	if e != nil {
		h.t.Fatalf("pair confirm: %v", e)
	}
	return result.Pairing, result.DeviceSecret
}

// httptestRequest is one correctly signed request, plus everything a
// test needs to break exactly one thing about it and nothing else.
//
// body is what the signature covers; sentBody is what Authenticate is
// actually handed. They are the same until a test makes them differ,
// which is how "an empty body signed as if it had content" and "a body
// altered after signing" are expressed.
type httptestRequest struct {
	h        *harness
	req      *http.Request
	method   string
	path     string
	appID    string
	secret   []byte
	nonce    string
	body     []byte
	sentBody []byte
}

func newHTTPTestRequest(h *harness, method, path string, body []byte, appID string, secret []byte) *httptestRequest {
	return newHTTPTestRequestAt(h, method, path, body, appID, secret, h.clock.Now())
}

func newHTTPTestRequestAt(h *harness, method, path string, body []byte, appID string, secret []byte, at time.Time) *httptestRequest {
	h.t.Helper()
	r := &httptestRequest{
		h: h, method: method, path: path, appID: appID, secret: secret,
		body: body, sentBody: body,
	}
	r.req = httptest.NewRequest(method, path, bytes.NewReader(body))
	r.resign(formatUnix(at), newTestNonce(), body)
	return r
}

// resign rebuilds the four headers for a given timestamp, nonce and
// signed body.
func (r *httptestRequest) resign(timestamp, nonce string, signedBody []byte) {
	r.nonce = nonce
	r.body = signedBody
	canonical := CanonicalString(r.method, r.path, timestamp, nonce, signedBody)
	r.req.Header.Set(HeaderAppID, r.appID)
	r.req.Header.Set(HeaderTimestamp, timestamp)
	r.req.Header.Set(HeaderNonce, nonce)
	r.req.Header.Set(HeaderSignature, Sign(r.secret, canonical))
}

func (r *httptestRequest) request() *http.Request { return r.req }

// replay returns a second request carrying exactly the same headers —
// what a captured request looks like when it is sent again.
func (r *httptestRequest) replay() *http.Request {
	again := httptest.NewRequest(r.method, r.path, bytes.NewReader(r.sentBody))
	for k, v := range r.req.Header {
		again.Header[k] = append([]string(nil), v...)
	}
	return again
}

var testNonceCounter struct {
	mu sync.Mutex
	n  int
}

func newTestNonce() string {
	testNonceCounter.mu.Lock()
	defer testNonceCounter.mu.Unlock()
	testNonceCounter.n++
	return "test-nonce-" + itoa(testNonceCounter.n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func formatUnix(t time.Time) string { return itoa(int(t.Unix())) }

// errSetFailed is the failure a test injects to prove Add leaves
// nothing behind when the secret cannot be stored.
var errSetFailed = errors.New("secret store is unwritable")
