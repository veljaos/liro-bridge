package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
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
	registry *jobs.Registry
	signer   *fakeSigner
	server   *Server
	http     *httptest.Server

	certs *fakeCertificates

	mu                 sync.Mutex
	documentSigning    bool
	certificateListing bool

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
	h.registry = jobs.NewRegistry(clock.Now)
	h.signer = newFakeSigner()
	h.certs = newFakeCertificates()
	h.documentSigning = true
	h.certificateListing = true
	h.server = NewServer(Options{
		Pairings:     pairings,
		Flow:         h.flow,
		Auth:         h.auth,
		Jobs:         h.registry,
		Signer:       h.signer,
		Certificates: h.certs,
		AgentVersion: testAgentVersion,
		DocumentSigningEnabled: func() bool {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.documentSigning
		},
		CertificateListingEnabled: func() bool {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.certificateListing
		},
		Now: clock.Now,
	})
	h.http = httptest.NewServer(h.server.Handler())
	t.Cleanup(h.http.Close)
	return h
}

// testAgentVersion is what the harness's agent reports as its own
// version. A value that is obviously not a real release, so a test
// asserting on it cannot pass by accident against a hard-coded "dev".
const testAgentVersion = "9.9.9-test"

// setDocumentSigning turns the whole-document endpoint on or off while
// the agent is running, which is what the real setting does.
func (h *harness) setDocumentSigning(on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.documentSigning = on
}

// setCertificateListing turns the certificate listing on or off while
// the agent is running, which is what the real setting does.
func (h *harness) setCertificateListing(on bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.certificateListing = on
}

// fakeCertificates stands in for the machine's certificate store. It
// carries what internal/trust/classify would have produced — the shape
// the endpoint reports — because the enumeration itself is Windows and
// has its own tests.
type fakeCertificates struct {
	mu    sync.Mutex
	certs []Certificate
	err   error
	calls int
}

func newFakeCertificates() *fakeCertificates {
	return &fakeCertificates{certs: []Certificate{
		{
			Thumbprint:  "7758D4D4B8973EA619B3225185EDE740B3D1ECCE",
			DisplayName: "ВЕЉКО СТАНОЈЕВИЋ",
			Issuer:      "MUPGradjaniCA4",
			Purpose:     "signing",
			Qualified:   true,
			Usable:      true,
		},
		{
			Thumbprint:      "1234567890ABCDEF1234567890ABCDEF12345678",
			DisplayName:     "Zoran Milovanović",
			Issuer:          "Halcom CA PO 2",
			Purpose:         "signing",
			Qualified:       true,
			Usable:          false,
			NotUsableReason: errs.CodeCardNotPresent,
		},
	}}
}

func (f *fakeCertificates) Certificates(ctx context.Context) ([]Certificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return append([]Certificate(nil), f.certs...), nil
}

func (f *fakeCertificates) set(certs []Certificate, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.certs, f.err = certs, err
}

func (f *fakeCertificates) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeSigner stands in for the agent's own consent window and card. It
// records what it was asked, publishes whatever states the test told it
// to, and answers with whatever the test told it to answer.
//
// It exists so the whole protocol — submission, events, result,
// one-job-at-a-time, collect-once — is exercised with no window, no
// card and no PDF engine, on any platform.
type fakeSigner struct {
	mu sync.Mutex

	// requests records every SignRequest that reached the signer.
	requests []SignRequest

	// called is closed and replaced every time the signer is entered,
	// so a test can wait on the event rather than poll for the count.
	// It is the same close-and-replace broadcast jobs.Job uses, for
	// the same reason: nothing is buffered, so nothing can be missed,
	// and a waiter always lands on the state as it stands now.
	called chan struct{}

	// respond is what to do with a request. The default signs every
	// digest with a stand-in signature.
	respond func(req SignRequest, job *jobs.Job) (SignResult, error)
}

func newFakeSigner() *fakeSigner {
	return &fakeSigner{respond: signEverything, called: make(chan struct{})}
}

// signEverything is the default: publish the states a real run passes
// through, then hand back one stand-in signature per document.
func signEverything(req SignRequest, job *jobs.Job) (SignResult, error) {
	job.Publish(jobs.Update{State: jobs.JobAwaitingConsent, RemainingConsent: 90 * time.Second})
	job.Publish(jobs.Update{State: jobs.JobAwaitingPIN})
	job.Publish(jobs.Update{State: jobs.JobPreparingCard})
	out := make([]SignOutcome, 0, len(req.Digests))
	for i := range req.Digests {
		job.Publish(jobs.Update{State: jobs.JobSigning, Completed: i + 1})
		o := SignOutcome{Signature: []byte{byte(i), 0xAA}}
		if req.Kind == SignDocuments {
			o = SignOutcome{Document: []byte("%PDF-signed-" + string(rune('A'+i))), AchievedLevel: "B-T"}
		}
		out = append(out, o)
	}
	return SignResult{Outcomes: out}, nil
}

func (f *fakeSigner) Sign(ctx context.Context, req SignRequest, job *jobs.Job) (SignResult, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	respond := f.respond
	close(f.called)
	f.called = make(chan struct{})
	f.mu.Unlock()
	return respond(req, job)
}

// answer replaces what the signer does with the next request.
func (f *fakeSigner) answer(fn func(req SignRequest, job *jobs.Job) (SignResult, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.respond = fn
}

// lastRequest is the most recent SignRequest the signer was handed.
func (f *fakeSigner) lastRequest() (SignRequest, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		return SignRequest{}, false
	}
	return f.requests[len(f.requests)-1], true
}

func (f *fakeSigner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
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

// client is a paired application talking to the harness's agent over a
// real socket, signing every request the way docs/PROTOCOL.md tells an
// integrator to.
//
// It goes through the real handler over real HTTP rather than calling
// the handler directly, because what these tests are about is what a
// program on this machine actually receives — and because a streaming
// endpoint has no meaning at all without a socket to stream down.
type client struct {
	h       *harness
	pairing Pairing
	secret  []byte
}

// client pairs an application and returns it ready to make requests.
func (h *harness) client(name, origin string) *client {
	h.t.Helper()
	pairing, secret := h.pair(name, origin)
	return &client{h: h, pairing: pairing, secret: secret}
}

// request builds one signed request. It is separate from do so a test
// can alter exactly one header before sending it.
func (c *client) request(method, path string, body []byte) *http.Request {
	c.h.t.Helper()
	req, err := http.NewRequest(method, c.h.http.URL+path, bytes.NewReader(body))
	if err != nil {
		c.h.t.Fatalf("building the request: %v", err)
	}
	timestamp := formatUnix(c.h.clock.Now())
	nonce := newTestNonce()
	canonical := CanonicalString(method, path, timestamp, nonce, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderAppID, c.pairing.AppID)
	req.Header.Set(HeaderTimestamp, timestamp)
	req.Header.Set(HeaderNonce, nonce)
	req.Header.Set(HeaderSignature, Sign(c.secret, canonical))
	return req
}

// do sends a signed request and returns the status and the decoded
// JSON body.
func (c *client) do(method, path string, body any) (int, map[string]any) {
	c.h.t.Helper()
	raw := marshalForTest(c.h, body)
	resp, err := c.h.http.Client().Do(c.request(method, path, raw))
	if err != nil {
		c.h.t.Fatalf("sending the request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		c.h.t.Fatalf("reading the response: %v", err)
	}
	var decoded map[string]any
	if len(out) > 0 {
		if err := json.Unmarshal(out, &decoded); err != nil {
			c.h.t.Fatalf("the response is not JSON: %v\n%s", err, out)
		}
	}
	return resp.StatusCode, decoded
}

func marshalForTest(h *harness, body any) []byte {
	h.t.Helper()
	switch v := body.(type) {
	case nil:
		return nil
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			h.t.Fatalf("marshalling the request body: %v", err)
		}
		return b
	}
}

// digestsRequest is a valid /v2/sign body for n documents, with the
// digests a caller would have computed itself.
func digestsRequest(n int, thumbprint string) map[string]any {
	digests := make([]string, 0, n)
	labels := make([]string, 0, n)
	for i := 0; i < n; i++ {
		sum := sha256.Sum256([]byte{byte(i)})
		digests = append(digests, base64.StdEncoding.EncodeToString(sum[:]))
		labels = append(labels, "document-"+itoa(i+1)+".pdf")
	}
	return map[string]any{
		"certificateThumbprint": thumbprint,
		"digestAlgorithm":       "SHA256",
		"digests":               digests,
		"labels":                labels,
	}
}

// documentsRequest is a valid /v2/sign/pdf body for n documents.
func documentsRequest(n int) map[string]any {
	docs := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		docs = append(docs, map[string]any{
			"name":    "ugovor-" + itoa(i+1) + ".pdf",
			"content": base64.StdEncoding.EncodeToString([]byte("%PDF-1.7 document " + itoa(i))),
		})
	}
	return map[string]any{"documents": docs}
}

// statusAndBody reads a response into a status, a decoded body and the
// raw bytes — and checks, on the way, the two things every response in
// this protocol must have: a JSON content type and no CORS header.
func statusAndBody(h *harness, resp *http.Response) (int, map[string]any, []byte) {
	h.t.Helper()
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
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		h.t.Fatalf("the response carries Access-Control-Allow-Origin: %q", got)
	}
	return resp.StatusCode, decoded, out
}

// consentFingerprint is consent.Fingerprint, named here so a test can
// state the property ("the 202's fingerprint is the one the window
// shows") without the reader having to know which package computes it.
func consentFingerprint(digests [][]byte) string { return consent.Fingerprint(digests) }
