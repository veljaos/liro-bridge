package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

// submitResponse is the 202 a signing request is answered with (F7
// §7.1). A hundred documents takes about 46 seconds after the PIN, and
// no HTTP request should be held open that long — so submission answers
// in milliseconds and says where to watch and where to collect.
type submitResponse struct {
	JobID            string `json:"jobId"`
	BatchFingerprint string `json:"batchFingerprint"`
	Total            int    `json:"total"`
	EventsURL        string `json:"eventsUrl"`
	ResultURL        string `json:"resultUrl"`
}

// eventBody is one server-sent event's JSON (F7 §7.2).
//
// Every field is a number or a stable string; there is no prose in it
// and there never will be, because a caller shows its own words in its
// own language (SPEC §7).
type eventBody struct {
	State     string `json:"state"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Failed    int    `json:"failed"`

	// ETAMs is present only once a first signature has actually been
	// measured — SPEC §12.9's rule that the estimate comes from
	// measurement and never from a constant means there is nothing
	// honest to put here before then.
	ETAMs *int64 `json:"etaMs,omitempty"`

	// ConsentRemainingMs is present only while a person is being asked,
	// so an ERP can show its own countdown rather than guessing when
	// the window will expire (F7 §7.4).
	ConsentRemainingMs *int64 `json:"consentRemainingMs,omitempty"`

	// Code is present only on the failed state.
	Code string `json:"code,omitempty"`
}

// digestsResultBody is what a finished /v2/sign job hands back.
type digestsResultBody struct {
	Signatures []*string     `json:"signatures"`
	Failures   []failureBody `json:"failures,omitempty"`
	Counts     resultCounts  `json:"counts"`
}

// documentsResultBody is what a finished /v2/sign/pdf job hands back.
type documentsResultBody struct {
	Documents []signedDocumentBody `json:"documents"`
	Failures  []failureBody        `json:"failures,omitempty"`
	Counts    resultCounts         `json:"counts"`
}

type signedDocumentBody struct {
	Name string `json:"name"`
	// Content is absent for a document that failed; its entry is in
	// Failures instead, with the index it had in the request.
	Content       string `json:"content,omitempty"`
	AchievedLevel string `json:"achievedLevel,omitempty"`
}

// failureBody names one document that did not sign, by the position it
// had in the request, so a caller can line failures up against what it
// sent without matching on a name it may have sent twice.
type failureBody struct {
	Index int    `json:"index"`
	Code  string `json:"code"`
}

type resultCounts struct {
	Total     int `json:"total"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
}

// handleSignDigests is POST /v2/sign (F7 §5): the caller built the PDF
// and the CMS itself and sends only hashes, so the agent never
// possesses the document.
func (s *Server) handleSignDigests(w http.ResponseWriter, r *http.Request) {
	if e := requirePOST(r); e != nil {
		fail(w, e)
		return
	}
	raw, e := readBody(r, maxSignBody)
	if e != nil {
		fail(w, e)
		return
	}
	// Authenticated before the body is parsed, not after: a caller that
	// has not authenticated is told nothing about how this agent reads
	// a request, and the hash the signature covers is over the bytes as
	// received either way (F7 §3).
	pairing, e := s.auth.Authenticate(r, raw)
	if e != nil {
		fail(w, e)
		return
	}
	var body signDigestsBody
	if e := decodeJSON(raw, &body); e != nil {
		fail(w, e)
		return
	}
	req, e := validateDigests(body, pairing.Name)
	if e != nil {
		fail(w, e)
		return
	}
	s.startJob(w, pairing, req)
}

// handleSignDocuments is POST /v2/sign/pdf (F7 §6): the caller cannot
// build CMS, so the agent receives the document, signs it and returns
// it.
func (s *Server) handleSignDocuments(w http.ResponseWriter, r *http.Request) {
	if e := requirePOST(r); e != nil {
		fail(w, e)
		return
	}
	raw, e := readBody(r, maxSignPDFBody)
	if e != nil {
		fail(w, e)
		return
	}
	pairing, e := s.auth.Authenticate(r, raw)
	if e != nil {
		fail(w, e)
		return
	}
	// Checked after authentication, deliberately: whether this machine
	// offers the whole-document path is a fact about how it is set up,
	// and an unpaired caller has no business learning it.
	if !s.documentSigning() {
		fail(w, errs.New(errs.CodeDocumentSigningDisabled,
			errors.New("the whole-document signing path is switched off")))
		return
	}
	var body signDocumentsBody
	if e := decodeJSON(raw, &body); e != nil {
		fail(w, e)
		return
	}
	req, e := validateDocuments(body, pairing.Name)
	if e != nil {
		fail(w, e)
		return
	}
	s.startJob(w, pairing, req)
}

// startJob accepts a validated request, answers 202, and runs the job
// on its own goroutine.
func (s *Server) startJob(w http.ResponseWriter, pairing Pairing, req SignRequest) {
	if s.jobs == nil || s.signer == nil {
		// A build with no signer wired serves pairing and health and
		// nothing else. Saying so as INTERNAL is the truth: it is this
		// agent's own misconfiguration, not the caller's request.
		fail(w, errs.New(errs.CodeInternal, errors.New("no signer is wired")))
		return
	}

	fingerprint := consent.Fingerprint(req.Digests)
	job, err := s.jobs.Submit(pairing.AppID, len(req.Digests), fingerprint)
	if err != nil {
		if errors.Is(err, jobs.ErrJobInProgress) {
			fail(w, errs.New(errs.CodeJobInProgress, err))
			return
		}
		slog.Error("api: could not accept a signing job", "error", err, "appId", pairing.AppID)
		fail(w, errs.New(errs.CodeInternal, err))
		return
	}

	slog.Info("api: a signing job was accepted",
		"jobId", job.ID, "appId", pairing.AppID, "kind", string(req.Kind), "documents", len(req.Digests))

	go s.runJob(job, req)

	writeJSON(w, http.StatusAccepted, submitResponse{
		JobID:            job.ID,
		BatchFingerprint: fingerprint,
		Total:            len(req.Digests),
		EventsURL:        jobEventsPath(job.ID),
		ResultURL:        jobResultPath(job.ID),
	})
}

func jobEventsPath(id string) string { return "/v2/jobs/" + id + "/events" }
func jobResultPath(id string) string { return "/v2/jobs/" + id + "/result" }

// runJob hands the request to the agent's own signing flow and records
// what came back.
//
// The context is deliberately not the request's: the request is over
// the moment the 202 is written, and a job that stopped because a
// caller hung up would be a job whose consent window vanished from in
// front of a person mid-decision.
func (s *Server) runJob(job *jobs.Job, req SignRequest) {
	defer func() {
		if p := recover(); p != nil {
			slog.Error("api: a signing job panicked", "jobId", job.ID, "panic", p)
			job.Fail(s.now(), errs.CodeInternal, jobs.Update{})
		}
	}()

	result, err := s.signer.Sign(context.Background(), req, job)
	if err != nil {
		code := codeOf(err)
		slog.Info("api: a signing job ended without a signature", "jobId", job.ID, "code", string(code))
		job.Fail(s.now(), code, jobs.Update{Failed: len(req.Digests)})
		return
	}
	if result.Code != "" {
		slog.Info("api: a signing job ended without a signature", "jobId", job.ID, "code", string(result.Code))
		job.Fail(s.now(), result.Code, jobs.Update{Failed: len(req.Digests)})
		return
	}

	body, marshalErr := marshalResult(req, result)
	if marshalErr != nil {
		slog.Error("api: could not render a job result", "jobId", job.ID, "error", marshalErr)
		job.Fail(s.now(), errs.CodeInternal, jobs.Update{})
		return
	}

	succeeded := result.Signed()
	if succeeded == 0 {
		// Every document failed. That is a failed job, not a completed
		// one with an empty result: a caller branching on the state
		// must not have to count outcomes to find out nothing was
		// signed.
		job.Fail(s.now(), firstFailureCode(result), jobs.Update{
			Completed: len(result.Outcomes),
			Failed:    len(result.Outcomes),
		})
		return
	}
	job.Complete(s.now(), body, jobs.Update{
		Completed: len(result.Outcomes),
		Failed:    len(result.Outcomes) - succeeded,
	})
	slog.Info("api: a signing job finished",
		"jobId", job.ID, "signed", succeeded, "failed", len(result.Outcomes)-succeeded)
}

// firstFailureCode is the code a wholly failed job reports: the first
// document's, since a batch that failed entirely almost always failed
// for one reason and the codes that end a batch (the card gone, the PIN
// blocked) are exactly that reason.
func firstFailureCode(result SignResult) errs.Code {
	for _, o := range result.Outcomes {
		if code := outcomeFailureCode(o); code != "" {
			return code
		}
	}
	return errs.CodeSignFailed
}

// marshalResult renders a finished job's outcomes as the body its
// endpoint returns.
func marshalResult(req SignRequest, result SignResult) ([]byte, error) {
	counts := resultCounts{
		Total:     len(result.Outcomes),
		Succeeded: result.Signed(),
	}
	counts.Failed = counts.Total - counts.Succeeded

	var failures []failureBody
	for i, o := range result.Outcomes {
		if code := outcomeFailureCode(o); code != "" {
			failures = append(failures, failureBody{Index: i, Code: string(code)})
		}
	}

	if req.Kind == SignDigests {
		signatures := make([]*string, 0, len(result.Outcomes))
		for _, o := range result.Outcomes {
			if outcomeFailureCode(o) != "" {
				// A null in the array rather than a shorter array: the
				// caller sent digest 47 and has to be able to find
				// signature 47, and a list that silently closes up over
				// a failure is how a signature ends up attached to the
				// wrong document.
				signatures = append(signatures, nil)
				continue
			}
			encoded := base64.StdEncoding.EncodeToString(o.Signature)
			signatures = append(signatures, &encoded)
		}
		return json.Marshal(digestsResultBody{Signatures: signatures, Failures: failures, Counts: counts})
	}

	docs := make([]signedDocumentBody, 0, len(result.Outcomes))
	for i, o := range result.Outcomes {
		entry := signedDocumentBody{Name: labelAt(req, i)}
		if outcomeFailureCode(o) == "" {
			entry.Content = base64.StdEncoding.EncodeToString(o.Document)
			entry.AchievedLevel = o.AchievedLevel
		}
		docs = append(docs, entry)
	}
	return json.Marshal(documentsResultBody{Documents: docs, Failures: failures, Counts: counts})
}

// outcomeFailureCode is why one document has no signature, or empty
// when it has one. A Signer gives every document it did not sign a
// code; an outcome carrying neither a code nor any bytes is a Signer
// that did not, and calling that SIGN_FAILED is the only honest answer
// left — it is certainly not a signature.
func outcomeFailureCode(o SignOutcome) errs.Code {
	if o.Code != "" {
		return o.Code
	}
	if len(o.Signature) == 0 && len(o.Document) == 0 {
		return errs.CodeSignFailed
	}
	return ""
}

func labelAt(req SignRequest, i int) string {
	if i < len(req.Labels) {
		return req.Labels[i]
	}
	return ""
}

// codeOf digs an errs.Code out of an error, or calls it INTERNAL.
func codeOf(err error) errs.Code {
	var e *errs.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return errs.CodeInternal
}

// handleJobEvents is GET /v2/jobs/{id}/events: server-sent events, one
// per state change, until the job finishes (F7 §7.2).
func (s *Server) handleJobEvents(w http.ResponseWriter, r *http.Request) {
	job, e := s.authenticatedJob(r)
	if e != nil {
		fail(w, e)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		// Unreachable with net/http's own server, and an error rather
		// than a silent buffered stream if it ever is not: a progress
		// stream that arrives all at once at the end is not progress.
		fail(w, errs.New(errs.CodeInternal, errors.New("the response cannot be streamed")))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	// Nothing between this agent and its caller is meant to buffer an
	// event stream, and the one thing that commonly does is told not to.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	err := job.Follow(r.Context(), func(u jobs.Update) error {
		data, err := json.Marshal(eventFor(u))
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Debug("api: an event stream ended early", "jobId", job.ID, "error", err)
	}
}

// eventFor renders one update as the event a caller reads.
func eventFor(u jobs.Update) eventBody {
	body := eventBody{
		State:     string(u.State),
		Completed: u.Completed,
		Total:     u.Total,
		Failed:    u.Failed,
		Code:      string(u.Code),
	}
	if u.ETAKnown {
		ms := u.ETA.Milliseconds()
		body.ETAMs = &ms
	}
	if u.State == jobs.JobAwaitingConsent && u.RemainingConsent > 0 {
		ms := u.RemainingConsent.Milliseconds()
		body.ConsentRemainingMs = &ms
	}
	return body
}

// handleJobResult is GET /v2/jobs/{id}/result: 202 with the current
// counts while the job is running, 200 with the signatures once it has
// finished — and 404 for every call after that, because the result is
// delivered once and the job is then forgotten (F7 §7.3).
func (s *Server) handleJobResult(w http.ResponseWriter, r *http.Request) {
	job, e := s.authenticatedJob(r)
	if e != nil {
		fail(w, e)
		return
	}

	state := job.Snapshot()
	if !state.State.Terminal() {
		writeJSON(w, http.StatusAccepted, eventFor(state))
		return
	}
	body, ok := job.TakeResult()
	if !ok {
		// Finished with nothing to give — the person refused, or every
		// document failed — or already collected. Either way there is
		// no result here and there never will be.
		s.jobs.Forget(job.ID)
		if state.Code != "" {
			fail(w, errs.New(state.Code, errors.New("the job produced no signatures")))
			return
		}
		fail(w, errs.New(errs.CodeJobNotFound, errors.New("the result has already been collected")))
		return
	}
	// Forgotten before the body is written, not after: a caller that
	// reads the response and immediately asks again must find nothing,
	// and a write that fails halfway does not entitle anyone to a
	// second copy of a qualified signature.
	s.jobs.Forget(job.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		slog.Warn("api: a job result could not be delivered", "jobId", job.ID, "error", err)
	}
}

// authenticatedJob is the two checks every job endpoint makes: the
// request authenticates, and the job belongs to the application that
// made it.
//
// A job that belongs to somebody else is answered exactly as one that
// does not exist. Telling a paired application that a job identifier it
// guessed is real, but not its own, is the same shape of hint F7 §3
// refuses to give about which authentication check failed.
func (s *Server) authenticatedJob(r *http.Request) (*jobs.Job, *errs.Error) {
	if e := requireGET(r); e != nil {
		return nil, e
	}
	// No content type is required, because there is no content — see
	// handleHealth for why asking for one on a GET is a rule real
	// clients cannot obey. What keeps these two out of a browser is
	// the four X-Liro-* headers they require: a request carrying those
	// is never a "simple request", so a page has to preflight it, and
	// the preflight is refused (withProtocolRules).
	//
	// The signature covers the hash of no bytes at all
	// (EmptyBodySHA256) — the one thing every integrator gets wrong
	// once, and why docs/PROTOCOL.md states the constant outright.
	pairing, e := s.auth.Authenticate(r, nil)
	if e != nil {
		return nil, e
	}
	if s.jobs == nil {
		return nil, errs.New(errs.CodeJobNotFound, errors.New("no job registry"))
	}
	job, ok := s.jobs.Get(r.PathValue("id"))
	if !ok || job.Owner != pairing.AppID {
		return nil, errs.New(errs.CodeJobNotFound, errors.New("no such job for this application"))
	}
	return job, nil
}

// jobSweepInterval is how often an idle agent discards results nobody
// collected. It is a fraction of the ten-minute deadline rather than
// equal to it, so a job is forgotten within a minute of its deadline
// rather than up to ten minutes after it.
const jobSweepInterval = time.Minute

// SweepJobs discards uncollected results on a timer until ctx is done.
// The registry also sweeps on every operation, so this is what keeps an
// agent nobody is talking to from holding a result for an afternoon.
func (s *Server) SweepJobs(ctx context.Context) {
	if s.jobs == nil {
		return
	}
	ticker := time.NewTicker(jobSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.jobs.Sweep()
		}
	}
}
