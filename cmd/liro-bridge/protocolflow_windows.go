//go:build windows

package main

// A request that arrived over the protocol, signed in the agent's own
// window (F7 §5, §6, §9).
//
// The whole point of this file is how little is in it. A protocol
// request goes through the same window, the same certificate step, the
// same Approve button and the same audit log as a person dropping
// files; SPEC §6.5 is explicit that the card caches its PIN
// independently of which process is talking to it, so the human at the
// window is the only boundary that actually holds, and a caller cannot
// be given a cheaper one. What differs is only what the batch is made
// of and where its output goes: digests or documents in memory rather
// than files on disk, and signatures handed back to the program that
// asked rather than written beside an input.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
)

// ConsentTimeout is how long a person has to answer a request that
// arrived over the protocol (F7 §7.4).
//
// 120 seconds, and the reason is consistency rather than taste: F2 §4.1
// already established a 120-second approval-to-first-signature window
// (signing.ApprovalWindow), and inventing a second number for the same
// human decision would mean two answers to one question — the pattern
// this project has had to remove three times (D-108, D-124, D-138).
const ConsentTimeout = 120 * time.Second

// ConsentCountdownFrom is when the window starts showing a countdown
// (F7 §7.4). The last thirty seconds: long enough to be a warning
// rather than a surprise, short enough that a person answering
// normally never sees a clock at all.
const ConsentCountdownFrom = 30 * time.Second

// consentTick is how often the countdown is refreshed and the
// awaiting_consent event republished. One second, because that is the
// resolution the number on screen is shown at.
const consentTick = time.Second

// remoteBatch is a protocol request being signed in the agent's own
// window.
type remoteBatch struct {
	req api.SignRequest
	job *jobs.Job

	// outcomes has one entry per document, filled in as the run
	// proceeds and handed back to the caller when it ends.
	outcomes []api.SignOutcome

	// code is why the whole batch produced nothing — the person
	// refused, the window expired, the card was not there. Empty when
	// at least one document was attempted.
	code errs.Code

	// deadline is when the person's 120 seconds run out.
	deadline time.Time

	// stopCountdown ends the countdown goroutine. Closed once, by
	// startSigning: from the moment the card is being opened the person
	// has answered, and a clock that kept running would be counting
	// down to nothing.
	stopCountdown chan struct{}
	countdownOnce sync.Once

	// expired is closed when the deadline passes with no answer.
	expired chan struct{}
	expiry  sync.Once
}

// endCountdown stops the countdown. Safe to call more than once and
// from any goroutine.
func (b *remoteBatch) endCountdown() {
	b.countdownOnce.Do(func() { close(b.stopCountdown) })
}

// expire records that nobody answered in time.
func (b *remoteBatch) expire() {
	b.expiry.Do(func() { close(b.expired) })
}

// result is what the caller is told, once the window is done with.
func (b *remoteBatch) result() api.SignResult {
	return api.SignResult{Outcomes: b.outcomes, Code: b.code}
}

// runProtocolFlow shows the agent's own window for one accepted job and
// returns what came of it.
//
// It runs on its own goroutine, one at a time (protocolSigner
// serialises them), because the agent shows one consent window at a
// time and a second request arriving mid-decision must wait rather than
// stack a window on top of the one a person is reading.
func runProtocolFlow(ctx context.Context, cfg config.Config, locale string, req api.SignRequest, job *jobs.Job) api.SignResult {
	m := newProtocolWindow(cfg, locale, req, job)

	if !m.gatherCertificatesBeforeOpening(ctx) {
		return api.SignResult{Code: errs.CodeInternal}
	}
	certs, err := certificatesOfferedFor(m.certInfos, req.Thumbprint)
	if err != nil {
		// Nothing to offer means nothing to ask about. Opening a window
		// on an empty list would be asking a person to approve a batch
		// no certificate on this machine can sign.
		slog.Info("protocol: no certificate to offer for a request",
			"jobId", job.ID, "code", string(codeOfInteractive(err)))
		return api.SignResult{Code: codeOfInteractive(err)}
	}
	m.certInfos = certs

	m.open(ctx, nil, stepCertificate)
	return m.remote.result()
}

// newProtocolWindow builds the flow for one protocol request: what the
// batch is made of, and every way this run differs from a person's own.
//
// It is its own function so that those differences are stated in
// exactly one place. A test that rebuilt them beside this would be
// measuring its own copy rather than the product's — which is how a
// settings window came to be checked against the value it was handed
// instead of the one on disk (D-134).
func newProtocolWindow(cfg config.Config, locale string, req api.SignRequest, job *jobs.Job) *mainWindow {
	m := newMainWindow(cfg, locale)
	m.documentsSupplied = true
	// There are no output files at all on this path — nothing is
	// written beside an input, because there is no input on disk — so
	// the output-file question has nothing to ask about.
	m.force = true
	m.hashesOnly = req.Kind == api.SignDigests
	// A remembered placement never applies to a request that arrived
	// over the protocol, whether or not the caller answered the method
	// question itself.
	//
	// A placed position is an answer to "where on *this* document",
	// given by a person who was looking at the page when they gave it.
	// The documents in a protocol batch are not that document and
	// nobody has looked at them: they arrived over a socket, from a
	// program, possibly a hundred at a time. Inheriting the position put
	// a stamp on top of a document's existing signature the first time
	// this path was used with a real card — visible to the owner,
	// because he looked; an ERP sending a hundred documents has nobody
	// looking.
	//
	// So the position is dropped and the corner is what is left: the
	// person still chooses in the window, exactly as they do locally, or
	// the default corner applies. Nothing is written to disk — the
	// remembered position is the person's own and stays theirs.
	m.cfg = withoutRememberedPlacement(m.cfg)
	m.suppliedStamp = req.Stamp
	m.applySuppliedStamp()
	m.remote = &remoteBatch{
		req:           req,
		job:           job,
		outcomes:      make([]api.SignOutcome, len(req.Digests)),
		stopCountdown: make(chan struct{}),
		expired:       make(chan struct{}),
	}
	if req.Level != "" {
		// A caller that named a level gets that level; one that did not
		// gets the person's own standing answer, which is what the
		// configuration already holds.
		m.cfg.SignatureLevel = req.Level
	}

	m.inputs = make([]interactiveInput, 0, len(req.Digests))
	items := make([]jobs.Item, 0, len(req.Digests))
	for i, digest := range req.Digests {
		label := protocolLabel(req, i)
		m.inputs = append(m.inputs, interactiveInput{digest: digest, label: label})
		items = append(items, jobs.Item{DisplayName: label, State: jobs.StateWaiting})
	}
	m.remoteItems = items
	return m
}

// protocolLabel is what the window shows for document i. Already
// sanitised by internal/api; a caller that supplied no labels gets a
// count, because "document 3 of 100" is more use on a consent screen
// than a blank row.
func protocolLabel(req api.SignRequest, i int) string {
	if i < len(req.Labels) && req.Labels[i] != "" {
		return req.Labels[i]
	}
	return fmt.Sprintf("#%d", i+1)
}

// certificatesOfferedFor narrows the certificate list to what this
// request may be signed with.
//
// With no thumbprint named, that is every certificate a person is
// normally offered, and they choose exactly as they do locally. With
// one named, it is that certificate and nothing else — see
// api.SignRequest.Thumbprint for why the hash path requires it: the
// caller has already built a CMS around a particular signer
// certificate, so a signature made with a different key produces a
// document that verifies against nothing. The person still chooses the
// row and still presses Approve (SPEC §6.5, §18.15); what they cannot
// do is be led into producing an invalid signature.
func certificatesOfferedFor(certs []classify.Info, thumbprint string) ([]classify.Info, error) {
	if len(certs) == 0 {
		return nil, errs.New(errs.CodeCertNotFound, errors.New("no certificate is available"))
	}
	if thumbprint == "" {
		return certs, nil
	}
	for _, info := range certs {
		if info.Thumbprint == thumbprint {
			return []classify.Info{info}, nil
		}
	}
	return nil, errs.New(errs.CodeCertNotFound, errors.New("the requested certificate is not available"))
}

// watchConsentDeadline publishes how long the person has left, and
// ends the flow when they run out (F7 §7.4).
//
// Two audiences, one clock. The window shows a countdown in the last
// thirty seconds, so the person sees the request is about to expire
// rather than finding it gone; and every tick republishes
// awaiting_consent with the remaining time, so a caller can show its
// own state rather than guessing at ours.
func (m *mainWindow) watchConsentDeadline() {
	b := m.remote
	b.job.Publish(jobs.Update{
		State:            jobs.JobAwaitingConsent,
		RemainingConsent: time.Until(b.deadline),
	})

	ticker := time.NewTicker(consentTick)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCountdown:
			return
		case <-ticker.C:
		}

		remaining := time.Until(b.deadline)
		if remaining <= 0 {
			slog.Info("protocol: nobody answered the consent window in time", "jobId", b.job.ID)
			b.expire()
			return
		}
		b.job.Publish(jobs.Update{State: jobs.JobAwaitingConsent, RemainingConsent: remaining})
		if remaining <= ConsentCountdownFrom {
			m.postCountdown(remaining)
		}
	}
}

// postCountdown puts the remaining seconds on the window.
//
// A number and a sentence, both resolved in Go: the page renders what
// it is given and owns no clock of its own, which is the same rule
// every other screen in this project follows (D-120, D-121 — a page
// holds no state beyond what Go last told it).
func (m *mainWindow) postCountdown(remaining time.Duration) {
	seconds := int(remaining.Round(time.Second) / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	if err := m.win.PostJSON(map[string]any{
		"type":    "countdown",
		"seconds": seconds,
		"text":    fmt.Sprintf(m.c.T("consent.countdown"), seconds),
	}); err != nil {
		slog.Debug("protocol: posting the countdown failed", "error", err)
	}
}

// consentExpired ends the flow when nobody answered in time. The
// window closes, the audit log records a refusal, and the caller is
// told CONSENT_TIMEOUT and can resubmit (F7 §7.4).
func (m *mainWindow) consentExpired() {
	m.remote.code = errs.CodeConsentTimeout
	m.deny()
	// Nothing is published here. The last thing a caller should see
	// before the terminal state is the last real countdown, not an
	// awaiting_consent event saying nothing remains — which is what an
	// earlier version sent, and what made the event stream end on a
	// countdown with no number in it.
}

// signRemoteOne signs one document of a protocol batch.
//
// Two shapes, one rule. On the hash path the card signs the digest the
// caller computed and nothing else happens — the agent never had a
// document and never will. On the document path the PDF engine does
// exactly what it does for a local batch, and the result stays in
// memory: this batch has no output folder, because its output is a
// response.
func (m *mainWindow) signRemoteOne(ctx context.Context, index int, session keysource.Session, opts interactiveSignOptions) (jobs.Outcome, error) {
	req := m.remote.req
	if req.Kind == api.SignDigests {
		signature, err := session.SignDigest(ctx, keysource.DigestSHA256, req.Digests[index])
		if err != nil {
			return jobs.Outcome{}, err
		}
		m.remote.outcomes[index] = api.SignOutcome{Signature: signature}
		return jobs.Outcome{}, nil
	}

	result, err := pades.SignDocument(ctx, req.Documents[index].Content, session, pades.Options{
		RequestedLevel:    opts.level,
		OnTSAFailureAbort: !opts.allowBB,
		TSA:               opts.tsaClient,
		TrustStore:        opts.trustStore,
		Now:               time.Now(),
		Stamp:             opts.stamp,
		RevocationMemory:  opts.revocationMemory,
	})
	if err != nil {
		return jobs.Outcome{}, err
	}
	m.remote.outcomes[index] = api.SignOutcome{
		Document:      result.Bytes,
		AchievedLevel: string(result.AchievedLevel),
	}
	return jobs.Outcome{
		AchievedLevel: string(result.AchievedLevel),
		StampAdjusted: result.StampMoved || result.StampPageFellBack,
	}, nil
}

// publishProgress turns one moment of the run into what the caller
// watching the event stream sees (F7 §7.2).
//
// The states map straight across, because they are the same two
// measurements SPEC §12.9 defines: preparing_card is the first
// signature, which is card initialisation and was measured at about
// 4.9 s, and signing is everything after it. The ETA is the runner's
// own, computed from the measured first signature and the measured
// median of the rest — never a constant.
func (m *mainWindow) publishProgress(p jobs.Progress) {
	if m.remote == nil {
		return
	}
	state := jobs.JobSigning
	if p.Phase == jobs.PhasePreparingCard {
		state = jobs.JobPreparingCard
	}
	if p.Phase == jobs.PhaseFinished {
		// The run is over; what it produced is the caller's answer, and
		// internal/api is what turns it into a terminal state. Saying
		// "signing" once more here would be the last thing the stream
		// carried before it ended.
		return
	}
	m.remote.job.Publish(jobs.Update{
		State:     state,
		Completed: p.Current,
		Total:     p.Total,
		Failed:    p.Failed,
		ETA:       p.ETA,
		ETAKnown:  p.ETAKnown,
	})
}

// recordRemoteFailure notes that one document of a protocol batch did
// not sign, so the caller is told which and why rather than being
// handed a shorter list than it sent.
func (m *mainWindow) recordRemoteFailure(index int, code errs.Code) {
	if m.remote == nil || index < 0 || index >= len(m.remote.outcomes) {
		return
	}
	m.remote.outcomes[index] = api.SignOutcome{Code: code}
}

// settleRemoteOutcomes gives every document the caller sent an answer.
//
// A run can end with documents it never reached: the card was removed
// at fifty, the PIN is now blocked, or the person closed the window
// mid-batch. The runner marks those skipped, which is the truth on
// screen, and a caller that sent a hundred digests needs the same truth
// as a hundred entries — an outcome with neither a signature nor a code
// would be counted as a success with nothing in it, which is the one
// answer that would be a lie.
func (m *mainWindow) settleRemoteOutcomes(report jobs.Report) {
	if m.remote == nil {
		return
	}
	code := errs.CodeSignFailed
	switch {
	case report.Aborted && report.AbortCode != "":
		// The reason the batch ended is the reason these were not
		// signed: a removed card is what happened to every document
		// after the one that noticed it.
		code = report.AbortCode
	case report.Stopped:
		// The person stopped it, which from the caller's side is the
		// same answer as refusing it in the first place.
		code = errs.CodeConsentDenied
	}
	for i := range m.remote.outcomes {
		o := m.remote.outcomes[i]
		if o.Code != "" || len(o.Signature) > 0 || len(o.Document) > 0 {
			continue
		}
		m.remote.outcomes[i] = api.SignOutcome{Code: code}
	}
}

// auditChannel is which front door this batch came through, for the
// audit log (F7 §6).
func (m *mainWindow) auditChannel() audit.Channel {
	if m.remote == nil {
		return audit.ChannelLocal
	}
	if m.remote.req.Kind == api.SignDigests {
		return audit.ChannelAPIDigests
	}
	return audit.ChannelAPIDocuments
}

// protocolSigner is the agent's own signing flow, as internal/api sees
// it: an api.Signer that opens a window and waits for a person.
//
// It serialises: one consent window at a time. F7 §10 bounds a caller
// to one job, but two different applications can each have one, and a
// second window opening over the one somebody is reading would be the
// maze F6b spent a whole pass removing. A job that arrives while
// another is being decided stays in JobQueued — which is exactly what
// that state is for — and its own 120 seconds start when its window
// opens, not when it was accepted.
type protocolSigner struct {
	mu       sync.Mutex
	fallback config.Config
}

func newProtocolSigner(fallback config.Config) *protocolSigner {
	return &protocolSigner{fallback: fallback}
}

// Sign implements api.Signer.
func (s *protocolSigner) Sign(ctx context.Context, req api.SignRequest, job *jobs.Job) (api.SignResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// The configuration is read here rather than captured when the
	// listener started: the agent outlives every window and every save
	// those windows make, and a copy handed down from startup is how a
	// setting saved a moment ago comes back as the old one (D-134).
	cfg := currentConfig(s.fallback)
	result := runProtocolFlow(ctx, cfg, cfg.Locale, req, job)
	return result, nil
}

// newJobRegistry is the agent's own job registry. One per process,
// like the pairing store and for the same reason (D-182): a job's
// identity, its one-at-a-time slot and its result all have to be the
// same fact to whichever request asks about them.
func newJobRegistry() *jobs.Registry { return jobs.NewRegistry(nil) }
