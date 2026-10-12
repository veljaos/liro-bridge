package cloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

func TestALinkIsReadStrictly(t *testing.T) {
	good := "liro-bridge://sign?job=" + url.QueryEscape("https://erp.example.com/liro/jobs/7f3?t=abc")
	l, err := ParseLink(good)
	if err != nil {
		t.Fatalf("ParseLink(%q): %v", good, err)
	}
	if l.Origin != "https://erp.example.com" || l.JobURL.Path != "/liro/jobs/7f3" {
		t.Fatalf("got %+v", l)
	}
	if got := Redacted(l.JobURL); strings.Contains(got, "abc") {
		t.Errorf("the redacted URL carries the token: %s", got)
	}

	bad := map[string]string{
		"another scheme":       "https://sign?job=https://erp.example.com/j",
		"another action":       "liro-bridge://pair?job=https://erp.example.com/j",
		"http job":             "liro-bridge://sign?job=" + url.QueryEscape("http://erp.example.com/j"),
		"no job":               "liro-bridge://sign",
		"two jobs":             "liro-bridge://sign?job=https://a.example/j&job=https://b.example/j",
		"an extra parameter":   "liro-bridge://sign?job=https://a.example/j&then=x",
		"user information":     "liro-bridge://sign?job=" + url.QueryEscape("https://me:pw@erp.example.com/j"),
		"a fragment":           "liro-bridge://sign?job=" + url.QueryEscape("https://erp.example.com/j#x"),
		"a path on the action": "liro-bridge://sign/elsewhere?job=https://erp.example.com/j",
		"too long":             "liro-bridge://sign?job=https://erp.example.com/" + strings.Repeat("a", MaxLinkLength),
		"no host":              "liro-bridge://sign?job=" + url.QueryEscape("https:///j"),
		"javascript":           "liro-bridge://sign?job=javascript:alert(1)",
	}
	for name, raw := range bad {
		if _, err := ParseLink(raw); !errors.Is(err, ErrLink) {
			t.Errorf("%s: %q was accepted (%v)", name, raw, err)
		}
	}
}

func TestOriginsAreCompared(t *testing.T) {
	cases := map[string]string{
		"https://ERP.example.com/x":      "https://erp.example.com",
		"https://erp.example.com:443/x":  "https://erp.example.com",
		"https://erp.example.com:8443/x": "https://erp.example.com:8443",
		"https://[::1]:8443/x":           "https://[::1]:8443",
	}
	for raw, want := range cases {
		u, _ := url.Parse(raw)
		if got := OriginOf(u); got != want {
			t.Errorf("OriginOf(%s) = %s, want %s", raw, got, want)
		}
	}
	if _, ok := SameOrigin("https://evil.example/r", "https://erp.example.com"); ok {
		t.Error("another host is the same origin")
	}
	if _, ok := SameOrigin("http://erp.example.com/r", "https://erp.example.com"); ok {
		t.Error("http is the same origin as https")
	}
	if _, ok := SameOrigin("https://erp.example.com/r?x=1", "https://erp.example.com"); !ok {
		t.Error("the same origin is not")
	}
}

func TestTheListOfServersIsTheOnDiskOne(t *testing.T) {
	o := NewOrigins(t.TempDir())
	if o.Allowed("https://a.example") {
		t.Fatal("an empty list allows something")
	}
	if err := o.Allow("https://a.example", time.Now()); err != nil {
		t.Fatal(err)
	}
	other := NewOrigins(filepathDir(o))
	if !other.Allowed("https://a.example") {
		t.Fatal("a second reader of the same directory does not see the allowed origin")
	}
	if err := other.Remove("https://a.example"); err != nil {
		t.Fatal(err)
	}
	if o.Allowed("https://a.example") {
		t.Fatal("a removed origin is still allowed by the first reader")
	}
}

func filepathDir(o *Origins) string { return filepath.Dir(o.path) }

// fakeSigner answers like the agent's consent flow would.
type fakeSigner struct {
	mu     sync.Mutex
	asked  []api.SignRequest
	answer func(api.SignRequest) api.SignResult
}

func (f *fakeSigner) Sign(_ context.Context, req api.SignRequest, _ *jobs.Job) (api.SignResult, error) {
	f.mu.Lock()
	f.asked = append(f.asked, req)
	f.mu.Unlock()
	return f.answer(req), nil
}

// server is a job server: GET /job serves body, POST /result records.
type server struct {
	*httptest.Server
	mu       sync.Mutex
	jobBody  func(base string) string
	posted   []map[string]any
	fetches  int
	jobCode  int
	redirect string
}

func newServer(t *testing.T, jobBody func(base string) string) *server {
	s := &server{jobBody: jobBody, jobCode: http.StatusOK}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/job":
			s.fetches++
			if s.redirect != "" {
				http.Redirect(w, r, s.redirect, http.StatusFound)
				return
			}
			if s.jobCode != http.StatusOK {
				w.WriteHeader(s.jobCode)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, s.jobBody(s.URL))
		case r.Method == http.MethodPost && r.URL.Path == "/result":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			s.posted = append(s.posted, m)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *server) link() string {
	return "liro-bridge://sign?job=" + url.QueryEscape(s.URL+"/job?t=secret-token")
}

const tinyPDF = "%PDF-1.4\n%%EOF\n"

func documentsJob(base string) string {
	return `{"protocol":"liro-cloud/1","kind":"documents","documents":[{"name":"Ugovor.pdf","content":"` +
		base64.StdEncoding.EncodeToString([]byte(tinyPDF)) + `"}],"level":"b-b","resultUrl":"` + base + `/result"}`
}

func channelFor(t *testing.T, s *server, allow bool, signer *fakeSigner) (*Channel, *int) {
	asked := 0
	c := &Channel{
		Client:          NewClient("test").withHTTPClient(s.Client()),
		Origins:         NewOrigins(t.TempDir()),
		AllowOrigin:     func(string) bool { asked++; return allow },
		Signer:          signer,
		Jobs:            jobs.NewRegistry(nil),
		DocumentSigning: func() bool { return true },
	}
	return c, &asked
}

func signedAll(req api.SignRequest) api.SignResult {
	out := make([]api.SignOutcome, len(req.Digests))
	for i := range out {
		out[i] = api.SignOutcome{Document: []byte("signed"), AchievedLevel: "B-B"}
	}
	return api.SignResult{Outcomes: out}
}

func TestALinkFromAServerThePersonAllowsEndsWithTheSignatureOnThatServer(t *testing.T) {
	s := newServer(t, documentsJob)
	signer := &fakeSigner{answer: signedAll}
	c, asked := channelFor(t, s, true, signer)

	if got := c.Handle(context.Background(), s.link()); got != OutcomeAnswered {
		t.Fatalf("outcome %d, want answered", got)
	}
	if *asked != 1 {
		t.Fatalf("the person was asked %d times about the server, want once", *asked)
	}
	if len(signer.asked) != 1 || signer.asked[0].Application != OriginOf(mustURL(s.URL)) {
		t.Fatalf("the consent window was asked %v; it must name the origin the agent connected to", signer.asked)
	}
	if len(s.posted) != 1 || s.posted[0]["protocol"] != api.CloudProtocol || s.posted[0]["batchFingerprint"] == nil {
		t.Fatalf("posted %v", s.posted)
	}
	docs, _ := s.posted[0]["documents"].([]any)
	if len(docs) != 1 {
		t.Fatalf("posted documents %v", s.posted[0]["documents"])
	}

	// The second link from the same server is not asked about again.
	if got := c.Handle(context.Background(), s.link()); got != OutcomeAnswered {
		t.Fatalf("second outcome %d", got)
	}
	if *asked != 1 {
		t.Fatalf("the person was asked again about a server they allowed")
	}
}

func mustURL(raw string) *url.URL { u, _ := url.Parse(raw); return u }

func TestAServerThePersonDoesNotAllowIsNeverFetched(t *testing.T) {
	s := newServer(t, documentsJob)
	signer := &fakeSigner{answer: signedAll}
	c, _ := channelFor(t, s, false, signer)
	if got := c.Handle(context.Background(), s.link()); got != OutcomeOriginRefused {
		t.Fatalf("outcome %d", got)
	}
	if s.fetches != 0 || len(signer.asked) != 0 || len(s.posted) != 0 {
		t.Fatalf("fetched %d, signed %d, posted %d after a refusal", s.fetches, len(signer.asked), len(s.posted))
	}
}

func TestAResultAddressOnAnotherServerSendsNothingAnywhere(t *testing.T) {
	other := newServer(t, documentsJob)
	s := newServer(t, func(string) string { return documentsJob(other.URL) })
	signer := &fakeSigner{answer: signedAll}
	c, _ := channelFor(t, s, true, signer)
	if got := c.Handle(context.Background(), s.link()); got != OutcomeJobRefused {
		t.Fatalf("outcome %d", got)
	}
	if len(signer.asked) != 0 || len(s.posted) != 0 || len(other.posted) != 0 {
		t.Fatalf("a job whose result goes elsewhere reached the window or was posted (%d, %d, %d)", len(signer.asked), len(s.posted), len(other.posted))
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	other := newServer(t, documentsJob)
	s := newServer(t, documentsJob)
	s.redirect = other.URL + "/job"
	c, _ := channelFor(t, s, true, &fakeSigner{answer: signedAll})
	if got := c.Handle(context.Background(), s.link()); got != OutcomeUnreachable {
		t.Fatalf("outcome %d", got)
	}
	if other.fetches != 0 {
		t.Fatal("the redirect was followed")
	}
}

func TestAJobTheLoopbackProtocolWouldRefuseIsRefusedAndTheServerIsTold(t *testing.T) {
	cases := map[string]func(string) string{
		"another protocol": func(b string) string {
			return strings.Replace(documentsJob(b), `"liro-cloud/1"`, `"liro-cloud/9"`, 1)
		},
		"content that is not base64": func(b string) string {
			return `{"protocol":"liro-cloud/1","kind":"documents","documents":[{"name":"x.pdf","content":"%%%"}],"resultUrl":"` + b + `/result"}`
		},
		"no documents": func(b string) string {
			return `{"protocol":"liro-cloud/1","kind":"documents","documents":[],"resultUrl":"` + b + `/result"}`
		},
		"an unknown kind": func(b string) string {
			return `{"protocol":"liro-cloud/1","kind":"everything","resultUrl":"` + b + `/result"}`
		},
	}
	for name, body := range cases {
		s := newServer(t, body)
		signer := &fakeSigner{answer: signedAll}
		c, _ := channelFor(t, s, true, signer)
		if got := c.Handle(context.Background(), s.link()); got != OutcomeJobRefused {
			t.Errorf("%s: outcome %d", name, got)
			continue
		}
		if len(signer.asked) != 0 {
			t.Errorf("%s: reached the consent window", name)
		}
		if len(s.posted) != 1 || s.posted[0]["code"] == nil {
			t.Errorf("%s: the server was told %v", name, s.posted)
		}
	}
}

func TestAPersonsRefusalIsTheAnswerTheServerGets(t *testing.T) {
	s := newServer(t, documentsJob)
	c, _ := channelFor(t, s, true, &fakeSigner{answer: func(api.SignRequest) api.SignResult {
		return api.SignResult{Code: errs.CodeConsentDenied}
	}})
	if got := c.Handle(context.Background(), s.link()); got != OutcomeAnswered {
		t.Fatalf("outcome %d", got)
	}
	if len(s.posted) != 1 || s.posted[0]["code"] != string(errs.CodeConsentDenied) {
		t.Fatalf("posted %v", s.posted)
	}
}

func TestAJobTheServerSaysIsGoneIsNotAFailure(t *testing.T) {
	s := newServer(t, documentsJob)
	s.jobCode = http.StatusGone
	c, _ := channelFor(t, s, true, &fakeSigner{answer: signedAll})
	if got := c.Handle(context.Background(), s.link()); got != OutcomeJobGone {
		t.Fatalf("outcome %d", got)
	}
}

func TestACertificateTheSystemDoesNotTrustIsRefused(t *testing.T) {
	s := newServer(t, documentsJob)
	c := &Channel{
		Client:          NewClient("test"), // the system's roots, not the test server's
		Origins:         NewOrigins(t.TempDir()),
		AllowOrigin:     func(string) bool { return true },
		Signer:          &fakeSigner{answer: signedAll},
		Jobs:            jobs.NewRegistry(nil),
		DocumentSigning: func() bool { return true },
	}
	if got := c.Handle(context.Background(), s.link()); got != OutcomeUnreachable {
		t.Fatalf("outcome %d: a self-signed server was trusted", got)
	}
}
