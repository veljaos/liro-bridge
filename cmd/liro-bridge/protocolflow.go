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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
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
