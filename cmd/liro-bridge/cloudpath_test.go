package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/cloud"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// The cloud channel's agent side, run without a tray, a window or a card
// (worklist 0.4 and P8, D-473). Each test names what it does not show.

// ---- open-link's good path ----------------------------------------------

func swapOpenLinkWays(t *testing.T, running bool) *int {
	t.Helper()
	became := 0
	oldRunning, oldBecome := agentIsRunning, becomeAgent
	agentIsRunning = func() bool { return running }
	becomeAgent = func(config.Config, string) int { became++; return 0 }
	t.Cleanup(func() { agentIsRunning, becomeAgent = oldRunning, oldBecome })
	return &became
}

const goodLink = "liro-bridge://sign?job=https%3A%2F%2Ferp.example.com%2Fjobs%2F1"

// A good link with an agent running is left in the inbox for it, and this
// launch does not become a second agent. Not shown: a desktop running
// open-link for a clicked link, and the running agent's tray.
func TestAGoodLinkIsLeftForTheRunningAgent(t *testing.T) {
	withIsolatedHome(t)
	became := swapOpenLinkWays(t, true)
	if code := runOpenLink([]string{goodLink}, io.Discard, config.Default(), "test"); code != 0 {
		t.Fatalf("open-link with a good link returned %d", code)
	}
	if *became != 0 {
		t.Fatal("open-link became the agent while one was running")
	}
	var got []string
	offerInboxLinks(jobs.NewInbox(shellInboxDir()), func(l string) { got = append(got, l) })
	if len(got) != 1 || got[0] != goodLink {
		t.Fatalf("the agent's watcher found %q in the inbox, want the link", got)
	}
}

// With no agent running, this launch becomes it — after leaving the link
// where the agent it becomes will find it.
func TestAGoodLinkWithNoAgentRunningBecomesIt(t *testing.T) {
	withIsolatedHome(t)
	became := swapOpenLinkWays(t, false)
	if code := runOpenLink([]string{goodLink}, io.Discard, config.Default(), "test"); code != 0 {
		t.Fatalf("open-link returned %d", code)
	}
	if *became != 1 {
		t.Fatalf("open-link became the agent %d times, want once", *became)
	}
	links, err := jobs.NewInbox(shellInboxDir()).TakeLinks()
	if err != nil || len(links) != 1 {
		t.Fatalf("the inbox holds %v (%v), want the link", links, err)
	}
}

// ---- the origin question --------------------------------------------------

// answeringWindow is a window that the person answers the moment it is
// filled. Not shown: origin.html, its buttons, Escape, or any real window.
type answeringWindow struct {
	opts   ui.Options
	answer func(ui.Options)
}

func (w *answeringWindow) PostJSON(any) error          { go w.answer(w.opts); return nil }
func (w *answeringWindow) Eval(string) (string, error) { return "", nil }
func (w *answeringWindow) Navigate(string) error       { return nil }
func (w *answeringWindow) Resize(int, int) error       { return nil }
func (w *answeringWindow) Close() error                { return nil }
func (w *answeringWindow) Handle() uintptr             { return 0 }

func TestTheOriginQuestionIsAnsweredAsThePersonAnswers(t *testing.T) {
	withIsolatedHome(t)
	old, oldTimeout := newOriginWindow, originAskTimeout
	t.Cleanup(func() { newOriginWindow, originAskTimeout = old, oldTimeout })
	originAskTimeout = 300 * time.Millisecond

	for _, tc := range []struct {
		name   string
		answer func(ui.Options)
		want   bool
	}{
		{"allow", func(o ui.Options) { o.OnMessage(ui.Message{Type: ui.MessageTypeApprove}) }, true},
		{"deny", func(o ui.Options) { o.OnMessage(ui.Message{Type: ui.MessageTypeCancel}) }, false},
		{"window closed", func(o ui.Options) { o.OnClosed() }, false},
		{"nobody answers", func(ui.Options) {}, false},
	} {
		newOriginWindow = func(o ui.Options) (ui.Window, error) {
			return &answeringWindow{opts: o, answer: tc.answer}, nil
		}
		if got := askOrigin(config.Default(), "https://erp.example.com"); got != tc.want {
			t.Errorf("%s: askOrigin = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The question waits for a consent window to go, and none opens while
// it is up — the one-window slot (F14 §2). It takes the same lock
// protocolSigner.Sign holds until its window has gone.
func TestTheOriginQuestionWaitsForTheSlotAndHoldsIt(t *testing.T) {
	s := newProtocolSigner(config.Default())
	s.mu.Lock() // a consent window is up
	asked := make(chan struct{})
	go s.holdSlot(func() { close(asked) })
	select {
	case <-asked:
		t.Fatal("the question was asked while a consent window was up")
	case <-time.After(150 * time.Millisecond):
	}
	s.mu.Unlock()
	select {
	case <-asked:
	case <-time.After(5 * time.Second):
		t.Fatal("the question was never asked after the consent window went")
	}

	inQuestion, release := make(chan struct{}), make(chan struct{})
	go s.holdSlot(func() { close(inQuestion); <-release })
	<-inQuestion
	if s.mu.TryLock() {
		s.mu.Unlock()
		t.Fatal("a consent window could open while the question was up")
	}
	close(release)
}

// ---- the whole path, against a stand-in Liro Business (P8) ---------------

// standInLiroBusiness serves one job of digests per path and records what
// is posted back. It is written from F14's description of the protocol,
// not from the real server.
type standInLiroBusiness struct {
	*httptest.Server
	mu      sync.Mutex
	digests [][]byte
	thumb   string
	fetched int
	results chan map[string]any
}

func newStandInLiroBusiness(t *testing.T, thumbprint string, digests [][]byte) *standInLiroBusiness {
	b := &standInLiroBusiness{digests: digests, thumb: thumbprint, results: make(chan map[string]any, 4)}
	b.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && len(r.URL.Path) > len("/jobs/") && r.URL.Path[:len("/jobs/")] == "/jobs/":
			b.mu.Lock()
			b.fetched++
			b.mu.Unlock()
			encoded := make([]string, len(b.digests))
			labels := make([]string, len(b.digests))
			for i, d := range b.digests {
				encoded[i] = base64.StdEncoding.EncodeToString(d)
				labels[i] = fmt.Sprintf("Faktura %d", i+1)
			}
			job, _ := json.Marshal(map[string]any{
				"protocol":              api.CloudProtocol,
				"kind":                  "digests",
				"certificateThumbprint": b.thumb,
				"digestAlgorithm":       "SHA-256",
				"digests":               encoded,
				"labels":                labels,
				"resultUrl":             b.URL + "/results/" + r.URL.Path[len("/jobs/"):],
			})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(job)
		case r.Method == http.MethodPost && len(r.URL.Path) > len("/results/") && r.URL.Path[:len("/results/")] == "/results/":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			b.results <- m
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(b.Close)
	return b
}

func (b *standInLiroBusiness) link(job string) string {
	return "liro-bridge://sign?job=" + url.QueryEscape(b.URL+"/jobs/"+job+"?t=secret-token")
}

func (b *standInLiroBusiness) roots() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(b.Certificate())
	return pool
}

// standInSigner stands for the consent window, the PIN and the card: it
// signs every digest with a key made here. Not shown: the agent's own
// consent flow, a person approving, a card, or a PAdES signature.
type standInSigner struct {
	key   *rsa.PrivateKey
	asked atomic.Int32
}

func (s *standInSigner) Sign(_ context.Context, req api.SignRequest, _ *jobs.Job) (api.SignResult, error) {
	s.asked.Add(1)
	out := make([]api.SignOutcome, len(req.Digests))
	for i, d := range req.Digests {
		sig, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, d)
		if err != nil {
			return api.SignResult{}, err
		}
		out[i] = api.SignOutcome{Signature: sig}
	}
	return api.SignResult{Outcomes: out}, nil
}

func standInCertificate(t *testing.T) (*rsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "stand-in signer"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}, &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "stand-in signer"}}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return key, cert
}

func TestTheWholeCloudPathAgainstAStandInLiroBusiness(t *testing.T) {
	withIsolatedHome(t)
	swapOpenLinkWays(t, true) // an agent is running: open-link leaves the link

	key, cert := standInCertificate(t)
	thumb := sha1.Sum(cert.Raw)
	d1, d2 := sha256.Sum256([]byte("faktura 1")), sha256.Sum256([]byte("faktura 2"))
	biz := newStandInLiroBusiness(t, hex.EncodeToString(thumb[:]), [][]byte{d1[:], d2[:]})
	signer := &standInSigner{key: key}
	var asked atomic.Int32
	told := make(chan cloud.Outcome, 4)
	links := wireCloudLinks(cloudParts{
		client:          cloud.NewClient("test").TrustingRoots(biz.roots()),
		origins:         cloud.NewOrigins(t.TempDir()),
		ask:             func(string) bool { asked.Add(1); return true },
		slot:            newProtocolSigner(config.Default()).holdSlot,
		signer:          signer,
		registry:        jobs.NewRegistry(nil),
		documentSigning: func() bool { return true },
		tell:            func(o cloud.Outcome, _ string) { told <- o },
	})
	quit := make(chan struct{})
	defer close(quit)
	go links.run(quit)
	inbox := jobs.NewInbox(shellInboxDir())

	for n, job := range []string{"1", "2"} {
		if code := runOpenLink([]string{biz.link(job)}, io.Discard, config.Default(), "test"); code != 0 {
			t.Fatalf("open-link returned %d", code)
		}
		if n == 0 {
			// The control P8 named: with the link left and no watcher's
			// tick, nothing reaches the server. Were a result to arrive
			// here, this test would not be measuring the path.
			select {
			case m := <-biz.results:
				t.Fatalf("a result reached the server before the watcher took the link: %v", m)
			case <-time.After(300 * time.Millisecond):
			}
		}
		offerInboxLinks(inbox, links.offer) // the watcher's tick

		var result map[string]any
		select {
		case result = <-biz.results:
		case <-time.After(15 * time.Second):
			t.Fatalf("job %s: no result reached the stand-in server", job)
		}
		if o := <-told; o != cloud.OutcomeAnswered {
			t.Fatalf("job %s: the link came to outcome %d, want answered", job, o)
		}
		sigs, _ := result["signatures"].([]any)
		if len(sigs) != 2 {
			t.Fatalf("job %s: the server got %v, want two signatures", job, result)
		}
		for i, d := range [][]byte{d1[:], d2[:]} {
			raw, err := base64.StdEncoding.DecodeString(fmt.Sprint(sigs[i]))
			if err != nil {
				t.Fatalf("job %s: signature %d is not base64", job, i)
			}
			if err := rsa.VerifyPKCS1v15(cert.PublicKey.(*rsa.PublicKey), crypto.SHA256, d, raw); err != nil {
				t.Errorf("job %s: signature %d does not verify against the signer's certificate: %v", job, i, err)
			}
		}
	}
	if n := asked.Load(); n != 1 {
		t.Errorf("the person was asked about the server %d times over two jobs, want once", n)
	}
	if n := signer.asked.Load(); n != 2 {
		t.Errorf("the signer was asked %d times, want once per job", n)
	}
}

// A server the person does not allow is never fetched from, and nothing
// is posted to it.
func TestARefusedServerIsNeverFetchedFrom(t *testing.T) {
	withIsolatedHome(t)
	swapOpenLinkWays(t, true)
	key, cert := standInCertificate(t)
	thumb := sha1.Sum(cert.Raw)
	d := sha256.Sum256([]byte("faktura"))
	biz := newStandInLiroBusiness(t, hex.EncodeToString(thumb[:]), [][]byte{d[:]})
	told := make(chan cloud.Outcome, 2)
	links := wireCloudLinks(cloudParts{
		client:          cloud.NewClient("test").TrustingRoots(biz.roots()),
		origins:         cloud.NewOrigins(t.TempDir()),
		ask:             func(string) bool { return false },
		slot:            newProtocolSigner(config.Default()).holdSlot,
		signer:          &standInSigner{key: key},
		registry:        jobs.NewRegistry(nil),
		documentSigning: func() bool { return true },
		tell:            func(o cloud.Outcome, _ string) { told <- o },
	})
	quit := make(chan struct{})
	defer close(quit)
	go links.run(quit)

	if code := runOpenLink([]string{biz.link("1")}, io.Discard, config.Default(), "test"); code != 0 {
		t.Fatalf("open-link returned %d", code)
	}
	offerInboxLinks(jobs.NewInbox(shellInboxDir()), links.offer)
	select {
	case o := <-told:
		if o != cloud.OutcomeOriginRefused {
			t.Fatalf("outcome %d, want the person's refusal", o)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the refused link came to no outcome")
	}
	biz.mu.Lock()
	fetched := biz.fetched
	biz.mu.Unlock()
	if fetched != 0 {
		t.Fatalf("a server the person refused was fetched from %d times", fetched)
	}
	select {
	case m := <-biz.results:
		t.Fatalf("something was posted to a refused server: %v", m)
	default:
	}
}
