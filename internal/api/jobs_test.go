package api

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

const testThumbprint = "7758D4A1B2C3D4E5F60718293A4B5C6D7E8F9012"

// submitDigests sends a valid /v2/sign request and returns the
// submission response.
func (c *client) submitDigests(n int) (int, map[string]any) {
	c.h.t.Helper()
	return c.do(http.MethodPost, "/v2/sign", digestsRequest(n, testThumbprint))
}

// collect polls the result endpoint until the job is no longer running,
// and returns the last status and body.
//
// It polls rather than sleeping: the job runs on its own goroutine and
// what is being tested is what a caller sees, which is exactly a caller
// asking again.
func (c *client) collect(jobID string) (int, map[string]any) {
	c.h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		status, body := c.do(http.MethodGet, jobResultPath(jobID), nil)
		if status != http.StatusAccepted {
			return status, body
		}
		if time.Now().After(deadline) {
			c.h.t.Fatalf("the job was still running after 10s: %v", body)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestSubmittingDigestsAnswers202WithAJobAndAFingerprint(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	status, body := c.submitDigests(3)
	if status != http.StatusAccepted {
		t.Fatalf("status %d, want 202: %v", status, body)
	}
	for _, field := range []string{"jobId", "batchFingerprint", "total", "eventsUrl", "resultUrl"} {
		if _, ok := body[field]; !ok {
			t.Errorf("the 202 is missing %q: %v", field, body)
		}
	}
	if got := body["total"].(float64); int(got) != 3 {
		t.Errorf("total is %v, want 3", body["total"])
	}
	jobID, _ := body["jobId"].(string)
	if body["eventsUrl"] != jobEventsPath(jobID) || body["resultUrl"] != jobResultPath(jobID) {
		t.Errorf("the two URLs do not name the job: %v", body)
	}

	// The fingerprint is the value the consent window shows, computed
	// the same way, so a caller can compare what it sent against what
	// the person was approving (SPEC §6.6).
	req, ok := h.signer.lastRequestAfter(t, 1)
	if !ok {
		t.Fatal("the signer was never called")
	}
	if body["batchFingerprint"] != consentFingerprint(req.Digests) {
		t.Errorf("batchFingerprint is %v, want %s", body["batchFingerprint"], consentFingerprint(req.Digests))
	}
}

func TestTheResultIsDeliveredOnceAndThenIsGone(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	_, submit := c.submitDigests(2)
	jobID := submit["jobId"].(string)

	status, body := c.collect(jobID)
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200: %v", status, body)
	}
	signatures, ok := body["signatures"].([]any)
	if !ok || len(signatures) != 2 {
		t.Fatalf("signatures is %v, want two entries", body["signatures"])
	}
	for i, s := range signatures {
		if _, err := base64.StdEncoding.DecodeString(s.(string)); err != nil {
			t.Errorf("signature %d is not base64: %v", i, err)
		}
	}

	status, body = c.do(http.MethodGet, jobResultPath(jobID), nil)
	if status != http.StatusNotFound {
		t.Fatalf("the second collection returned %d, want 404: %v", status, body)
	}
	if body["code"] != "JOB_NOT_FOUND" {
		t.Fatalf("code is %v, want JOB_NOT_FOUND", body["code"])
	}
}

func TestTheResultAnswers202WhileTheJobIsStillRunning(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	release := make(chan struct{})
	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		job.Publish(jobs.Update{State: jobs.JobAwaitingConsent, RemainingConsent: 120 * time.Second})
		<-release
		return signEverything(req, job)
	})

	_, submit := c.submitDigests(1)
	jobID := submit["jobId"].(string)

	status, body := c.do(http.MethodGet, jobResultPath(jobID), nil)
	if status != http.StatusAccepted {
		t.Fatalf("status %d, want 202 while the job is running: %v", status, body)
	}
	if body["state"] != string(jobs.JobAwaitingConsent) {
		t.Fatalf("state is %v, want %q", body["state"], jobs.JobAwaitingConsent)
	}
	if _, ok := body["signatures"]; ok {
		t.Fatal("a running job's 202 carried signatures")
	}
	close(release)

	if status, body := c.collect(jobID); status != http.StatusOK {
		t.Fatalf("status %d, want 200: %v", status, body)
	}
}

func TestOneJobPerApplicationOverHTTP(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	release := make(chan struct{})
	defer close(release)
	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		job.Publish(jobs.Update{State: jobs.JobAwaitingConsent})
		<-release
		return signEverything(req, job)
	})

	if status, body := c.submitDigests(1); status != http.StatusAccepted {
		t.Fatalf("the first submission returned %d: %v", status, body)
	}
	waitFor(t, func() bool { return h.signer.count() == 1 })

	status, body := c.submitDigests(1)
	if status != http.StatusConflict {
		t.Fatalf("the second submission returned %d, want 409: %v", status, body)
	}
	if body["code"] != "JOB_IN_PROGRESS" {
		t.Fatalf("code is %v, want JOB_IN_PROGRESS", body["code"])
	}
}

// TestAnotherApplicationsJobIsNotFound is what stops one paired
// application following or collecting another's batch. Two applications
// on one machine are ordinary — an ERP and a portal — and a job
// identifier is not a secret.
func TestAnotherApplicationsJobIsNotFound(t *testing.T) {
	h := newHarness(t)
	mine := h.client("My ERP", "https://erp.example.com")
	theirs := h.client("Somebody else", "https://other.example.com")

	release := make(chan struct{})
	defer close(release)
	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		job.Publish(jobs.Update{State: jobs.JobAwaitingConsent})
		<-release
		return signEverything(req, job)
	})

	_, submit := mine.submitDigests(1)
	jobID := submit["jobId"].(string)

	status, body := theirs.do(http.MethodGet, jobResultPath(jobID), nil)
	if status != http.StatusNotFound {
		t.Fatalf("status %d, want 404: %v", status, body)
	}
	if body["code"] != "JOB_NOT_FOUND" {
		t.Fatalf("code is %v, want JOB_NOT_FOUND", body["code"])
	}
}

func TestJobEndpointsRequireAuthentication(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")
	_, submit := c.submitDigests(1)
	jobID := submit["jobId"].(string)

	for _, path := range []string{jobResultPath(jobID), jobEventsPath(jobID)} {
		req, err := http.NewRequest(http.MethodGet, h.http.URL+path, nil)
		if err != nil {
			t.Fatalf("building the request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := h.http.Client().Do(req)
		if err != nil {
			t.Fatalf("sending the request: %v", err)
		}
		status, body, raw := statusAndBody(h, resp)
		_ = resp.Body.Close()
		if status != http.StatusUnauthorized {
			t.Fatalf("%s answered %d unauthenticated, want 401\n%s", path, status, raw)
		}
		if body["code"] != "AUTH_FAILED" {
			t.Fatalf("%s answered %v, want AUTH_FAILED", path, body["code"])
		}
	}
}

// TestTheEventStreamReportsEveryStateAndEnds is F7 §7.2 end to end
// over a real socket: a stream that is connected throughout carries
// every state the run passes through, and closes when the job is done.
//
// The run and the reader hand off to each other rather than racing:
// the signer publishes a state and waits until the stream has actually
// reported it before publishing the next. That is what the test is
// about — a real run's states are seconds apart, and a test that
// simply published them all and then read is asserting how fast this
// machine is, which is not a property of this code (D-112).
//
// It is also the honest shape of the guarantee. A follower always sees
// the state as it stands now and always sees the terminal one; what it
// is not promised is every intermediate state it was too slow to read,
// because a hundred "signing" events a caller has fallen behind on are
// worth less than the one that says where the batch actually is.
func TestTheEventStreamReportsEveryStateAndEnds(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	seen := make(chan string, 64)
	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		job.Publish(jobs.Update{State: jobs.JobAwaitingConsent, RemainingConsent: 120 * time.Second})
		awaitState(t, seen, string(jobs.JobAwaitingConsent))
		job.Publish(jobs.Update{State: jobs.JobAwaitingPIN})
		awaitState(t, seen, string(jobs.JobAwaitingPIN))
		job.Publish(jobs.Update{State: jobs.JobPreparingCard})
		awaitState(t, seen, string(jobs.JobPreparingCard))
		out := make([]SignOutcome, 0, len(req.Digests))
		for i := range req.Digests {
			job.Publish(jobs.Update{State: jobs.JobSigning, Completed: i + 1})
			awaitState(t, seen, string(jobs.JobSigning))
			out = append(out, SignOutcome{Signature: []byte{byte(i), 0xAA}})
		}
		return SignResult{Outcomes: out}, nil
	})

	_, submit := c.submitDigests(2)
	jobID := submit["jobId"].(string)

	resp, err := h.http.Client().Do(c.request(http.MethodGet, jobEventsPath(jobID), nil))
	if err != nil {
		t.Fatalf("opening the event stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the event stream answered %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("the event stream's content type is %q", got)
	}

	var states []string
	scanEvents(t, resp, func(event map[string]any) {
		state, _ := event["state"].(string)
		if len(states) == 0 || states[len(states)-1] != state {
			states = append(states, state)
		}
		select {
		case seen <- state:
		default:
		}
	})

	for _, want := range []string{"awaiting_consent", "awaiting_pin", "preparing_card", "signing", "completed"} {
		if !containsState(states, want) {
			t.Errorf("the stream never reported %q; it reported %v", want, states)
		}
	}
	if len(states) == 0 || states[len(states)-1] != "completed" {
		t.Fatalf("the stream ended on %v, want completed last", states)
	}
}

// awaitState blocks until the reader reports it has seen want.
func awaitState(t *testing.T, seen <-chan string, want string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case got := <-seen:
			if got == want {
				return
			}
		case <-deadline:
			t.Errorf("the stream never reported %q", want)
			return
		}
	}
}

// TestTheEventStreamCanBeDisconnectedAndReconnected is F7 §12's own
// case. A caller that loses its stream mid-batch reconnects and finds
// the job where it actually is, not where it was when it disconnected.
func TestTheEventStreamCanBeDisconnectedAndReconnected(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	proceed := make(chan struct{})
	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		job.Publish(jobs.Update{State: jobs.JobAwaitingConsent, RemainingConsent: 90 * time.Second})
		<-proceed
		return signEverything(req, job)
	})

	_, submit := c.submitDigests(2)
	jobID := submit["jobId"].(string)

	// Open a stream, read one event, and hang up mid-batch.
	first, err := h.http.Client().Do(c.request(http.MethodGet, jobEventsPath(jobID), nil))
	if err != nil {
		t.Fatalf("opening the first stream: %v", err)
	}
	reader := bufio.NewReader(first.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatalf("reading the first event: %v", err)
	}
	_ = first.Body.Close()

	close(proceed)

	// Reconnect. The stream starts from the state as it stands now,
	// which is the whole reason every event carries the full picture
	// rather than a delta.
	second, err := h.http.Client().Do(c.request(http.MethodGet, jobEventsPath(jobID), nil))
	if err != nil {
		t.Fatalf("reconnecting: %v", err)
	}
	defer func() { _ = second.Body.Close() }()
	states := readEventStates(t, second)
	if len(states) == 0 || states[len(states)-1] != "completed" {
		t.Fatalf("the reconnected stream ended on %v, want completed last", states)
	}

	// The result is still there afterwards: following a job is not
	// collecting it.
	if status, body := c.collect(jobID); status != http.StatusOK {
		t.Fatalf("collecting after the stream ended returned %d: %v", status, body)
	}
}

// TestTheAwaitingConsentEventCarriesTheRemainingTime is F7 §7.2's
// countdown: the one state that has a clock attached carries it, and
// every other state leaves the field out.
//
// The run and the reader hand off, exactly as
// TestTheEventStreamReportsEveryStateAndEnds does and for the same
// reason: the job stays in awaiting_consent until the stream has
// reported it, and only then is released. Publishing the state and
// immediately releasing the run asserted that the reader was attached
// and scheduled before the job left the state — one goroutine
// outrunning another — which under -race it was not. The stream is
// right to coalesce: a follower lands on the newest state rather than
// working through a backlog (PROTOCOL.md §6.2, D-188), so a state the
// run passes straight through is a state a follower may never see.
// Holding the job there is what makes the state observable; a sleep
// would only make it likely.
func TestTheAwaitingConsentEventCarriesTheRemainingTime(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	seen := make(chan string, 64)
	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		job.Publish(jobs.Update{State: jobs.JobAwaitingConsent, RemainingConsent: 118 * time.Second})
		awaitState(t, seen, string(jobs.JobAwaitingConsent))
		return signEverything(req, job)
	})
	_, submit := c.submitDigests(1)
	jobID := submit["jobId"].(string)

	resp, err := h.http.Client().Do(c.request(http.MethodGet, jobEventsPath(jobID), nil))
	if err != nil {
		t.Fatalf("opening the event stream: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var events []map[string]any
	scanEvents(t, resp, func(event map[string]any) {
		events = append(events, event)
		state, _ := event["state"].(string)
		select {
		case seen <- state:
		default:
		}
	})

	found := false
	for _, e := range events {
		if e["state"] != string(jobs.JobAwaitingConsent) {
			continue
		}
		found = true
		ms, ok := e["consentRemainingMs"].(float64)
		if !ok || ms <= 0 {
			t.Fatalf("the awaiting_consent event carries consentRemainingMs %v", e["consentRemainingMs"])
		}
	}
	if !found {
		t.Fatal("the stream never reported awaiting_consent")
	}
	// Every other state leaves it out rather than reporting zero: a
	// countdown of nothing is not a countdown.
	for _, e := range events {
		if e["state"] == string(jobs.JobAwaitingConsent) {
			continue
		}
		if _, ok := e["consentRemainingMs"]; ok {
			t.Errorf("state %v carries consentRemainingMs", e["state"])
		}
	}
}

func TestAJobThatWasRefusedAnswersWithItsCodeAndNoResult(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		return SignResult{Code: errs.CodeConsentDenied}, nil
	})
	_, submit := c.submitDigests(1)
	jobID := submit["jobId"].(string)

	status, body := c.collect(jobID)
	if status != http.StatusForbidden {
		t.Fatalf("status %d, want 403 — a refusal is not a server error", status)
	}
	if body["code"] != "CONSENT_DENIED" {
		t.Fatalf("code is %v, want CONSENT_DENIED", body["code"])
	}
	if _, ok := body["signatures"]; ok {
		t.Fatal("a refused job handed back signatures")
	}
}

func TestAJobThatTimedOutAnswersConsentTimeout(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		return SignResult{}, errs.New(errs.CodeConsentTimeout, errors.New("nobody answered"))
	})
	_, submit := c.submitDigests(1)
	jobID := submit["jobId"].(string)

	_, body := c.collect(jobID)
	if body["code"] != "CONSENT_TIMEOUT" {
		t.Fatalf("code is %v, want CONSENT_TIMEOUT", body["code"])
	}
}

// TestOneFailedDocumentDoesNotFailTheBatch is SPEC §12.10 as a caller
// sees it: document 2 of 3 fails, the other two come back signed, and
// the failure is named by the position it had in the request.
func TestOneFailedDocumentDoesNotFailTheBatch(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		out := make([]SignOutcome, len(req.Digests))
		for i := range out {
			if i == 1 {
				out[i] = SignOutcome{Code: errs.CodeSignFailed}
				continue
			}
			out[i] = SignOutcome{Signature: []byte{byte(i)}}
		}
		return SignResult{Outcomes: out}, nil
	})

	_, submit := c.submitDigests(3)
	status, body := c.collect(submit["jobId"].(string))
	if status != http.StatusOK {
		t.Fatalf("status %d, want 200: %v", status, body)
	}
	signatures := body["signatures"].([]any)
	if len(signatures) != 3 {
		t.Fatalf("signatures has %d entries, want 3 — one per digest sent", len(signatures))
	}
	if signatures[1] != nil {
		t.Errorf("the failed document's signature is %v, want null", signatures[1])
	}
	if signatures[0] == nil || signatures[2] == nil {
		t.Errorf("a successful document's signature is missing: %v", signatures)
	}
	failures, ok := body["failures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("failures is %v, want one entry", body["failures"])
	}
	f := failures[0].(map[string]any)
	if int(f["index"].(float64)) != 1 || f["code"] != "SIGN_FAILED" {
		t.Fatalf("the failure is %v, want index 1 SIGN_FAILED", f)
	}
}

func TestABatchWhereEveryDocumentFailedIsAFailedJob(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
		out := make([]SignOutcome, len(req.Digests))
		for i := range out {
			out[i] = SignOutcome{Code: errs.CodeCardNotPresent}
		}
		return SignResult{Outcomes: out}, nil
	})
	_, submit := c.submitDigests(2)
	_, body := c.collect(submit["jobId"].(string))
	if body["code"] != "CARD_NOT_PRESENT" {
		t.Fatalf("code is %v, want CARD_NOT_PRESENT", body["code"])
	}
}

// ---- helpers --------------------------------------------------------

// lastRequestAfter waits until the signer has been called n times and
// returns the last request.
func (f *fakeSigner) lastRequestAfter(t *testing.T, n int) (SignRequest, bool) {
	t.Helper()
	waitFor(t, func() bool { return f.count() >= n })
	return f.lastRequest()
}

// readEvents reads a whole SSE stream to its end and returns every
// event's decoded JSON.
func readEvents(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	var out []map[string]any
	scanEvents(t, resp, func(event map[string]any) { out = append(out, event) })
	return out
}

func readEventStates(t *testing.T, resp *http.Response) []string {
	t.Helper()
	var states []string
	for _, e := range readEvents(t, resp) {
		s, _ := e["state"].(string)
		if len(states) == 0 || states[len(states)-1] != s {
			states = append(states, s)
		}
	}
	return states
}

func containsState(states []string, want string) bool {
	for _, s := range states {
		if s == want {
			return true
		}
	}
	return false
}

// scanEvents reads an SSE stream to its end, calling fn for each event
// as it arrives rather than collecting them first — which is what lets
// a test hand off to the run producing them.
func scanEvents(t *testing.T, resp *http.Response, fn func(map[string]any)) {
	t.Helper()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatalf("an event is not JSON: %v\n%s", err, line)
		}
		fn(event)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("reading the event stream: %v", err)
	}
}

// TestTheJobEndpointsWorkWithNoContentTypeAtAll pins what the
// real-client run had to correct: a GET has no body, and a great many
// HTTP clients — .NET's HttpClient among them, which is what F9's own
// SDK will be built on — cannot attach a Content-Type to a request
// with no content. Requiring one made every such request
// REQUEST_INVALID.
func TestTheJobEndpointsWorkWithNoContentTypeAtAll(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")
	_, submit := c.submitDigests(1)
	jobID := submit["jobId"].(string)

	req := c.request(http.MethodGet, jobResultPath(jobID), nil)
	req.Header.Del("Content-Type")
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, body, raw := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status == http.StatusBadRequest {
		t.Fatalf("a GET with no content type was refused: %s", raw)
	}
	if body["code"] == "REQUEST_INVALID" {
		t.Fatalf("a GET with no content type was refused: %s", raw)
	}
}

// TestAPageStillCannotReachTheJobEndpoints is why the above is safe:
// they need the four X-Liro-* headers, and a request carrying those is
// never a "simple request" — so a page has to preflight it, and the
// preflight is refused with no CORS header of any kind.
func TestAPageStillCannotReachTheJobEndpoints(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")
	_, submit := c.submitDigests(1)
	jobID := submit["jobId"].(string)

	req, err := http.NewRequest(http.MethodOptions, h.http.URL+jobResultPath(jobID), nil)
	if err != nil {
		t.Fatalf("building the preflight: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "x-liro-app-id,x-liro-signature")
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the preflight: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("the preflight was answered %d, want 403", resp.StatusCode)
	}
	for _, header := range []string{
		"Access-Control-Allow-Origin", "Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods", "Access-Control-Max-Age",
	} {
		if got := resp.Header.Get(header); got != "" {
			t.Fatalf("the preflight answer carries %s: %q", header, got)
		}
	}
}

// TestAnUnauthenticatedCallerLearnsNothingAboutTheRequestShape is the
// order the checks run in: the body is read (its hash is what the
// signature covers) but not parsed until the request has
// authenticated. A caller that cannot authenticate is told
// AUTH_FAILED, not that its JSON was malformed — which would be a
// second thing to work from, in a protocol whose refusals deliberately
// say nothing (F7 §3).
func TestAnUnauthenticatedCallerLearnsNothingAboutTheRequestShape(t *testing.T) {
	h := newHarness(t)
	c := h.client("My ERP", "https://erp.example.com")

	// Nonsense that is not even JSON, signed with the wrong key.
	req := c.request(http.MethodPost, "/v2/sign", []byte("this is not json"))
	req.Header.Set(HeaderSignature, strings.Repeat("0", 64))
	resp, err := h.http.Client().Do(req)
	if err != nil {
		t.Fatalf("sending the request: %v", err)
	}
	status, body, raw := statusAndBody(h, resp)
	_ = resp.Body.Close()
	if status != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401: %s", status, raw)
	}
	if body["code"] != "AUTH_FAILED" {
		t.Fatalf("code is %v, want AUTH_FAILED", body["code"])
	}
	if _, ok := body["details"]; ok {
		t.Fatalf("the refusal carries details: %s", raw)
	}
}

// TestAJobThatFailedIsNotAServerError is what the first real-client run
// found: a window nobody answered came back as HTTP 500. It is not the
// agent that failed.
func TestAJobThatFailedIsNotAServerError(t *testing.T) {
	for _, tc := range []struct {
		name string
		code errs.Code
		want int
	}{
		{"the person did not answer", errs.CodeConsentTimeout, http.StatusForbidden},
		{"the person refused", errs.CodeConsentDenied, http.StatusForbidden},
		{"the card was not there", errs.CodeCardNotPresent, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			c := h.client("My ERP", "https://erp.example.com")
			code := tc.code
			h.signer.answer(func(req SignRequest, job *jobs.Job) (SignResult, error) {
				return SignResult{Code: code}, nil
			})
			_, submit := c.submitDigests(1)
			status, body := c.collect(submit["jobId"].(string))
			if status != tc.want {
				t.Fatalf("status %d, want %d: %v", status, tc.want, body)
			}
			if body["code"] != string(tc.code) {
				t.Fatalf("code is %v, want %s", body["code"], tc.code)
			}
		})
	}
}
