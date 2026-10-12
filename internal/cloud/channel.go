package cloud

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

// What the person is told a link came to. One per Handle.
type Outcome int

const (
	// OutcomeLinkUnusable: the link was malformed; nothing was fetched.
	OutcomeLinkUnusable Outcome = iota
	// OutcomeOriginRefused: the person did not allow the server.
	OutcomeOriginRefused
	// OutcomeUnreachable: the server could not be reached or would not
	// give the job.
	OutcomeUnreachable
	// OutcomeJobGone: the server says the job expired or was answered.
	OutcomeJobGone
	// OutcomeJobRefused: the job was fetched and could not be put in
	// front of the person; the server was told why.
	OutcomeJobRefused
	// OutcomeBusy: another request's window is up.
	OutcomeBusy
	// OutcomeAnswered: the person answered — signed, refused or let it
	// time out — and the server has the answer.
	OutcomeAnswered
	// OutcomeUndelivered: the person answered and the server would not
	// take the answer. Nothing is kept: the documents were the server's.
	OutcomeUndelivered
	// OutcomeDropped: the link never reached Handle — the agent already
	// had cloudLinkQueue links waiting, and this one was let go. Not
	// Handle's to return; the agent's queue says it (worklist 0.6,
	// D-473).
	OutcomeDropped
)

// Channel turns links into requests in front of the person. Every field
// is required except Now.
type Channel struct {
	Client  *Client
	Origins *Origins

	// AllowOrigin asks the person whether origin may send requests,
	// naming nothing but the origin (F14 §2). It blocks until they
	// answer; false is a refusal, a closed window included.
	AllowOrigin func(origin string) bool

	// Signer is the loopback protocol's own — the same consent window,
	// one-window slot, timeout, PIN dialog and audit entry (F14 §7).
	Signer api.Signer
	Jobs   *jobs.Registry

	// DocumentSigning is the person's setting for the whole-document
	// path, read at each job.
	DocumentSigning func() bool

	Now func() time.Time
}

func (c *Channel) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Handle follows one link to its end. It never returns an error: every
// way it ends is an Outcome, logged, for the caller to tell the person.
func (c *Channel) Handle(ctx context.Context, raw string) Outcome {
	link, err := ParseLink(raw)
	if err != nil {
		// The link itself is not logged: it may carry a token.
		slog.Warn("cloud: a link could not be used", "error", err)
		return OutcomeLinkUnusable
	}
	log := slog.With("origin", link.Origin, "job", Redacted(link.JobURL))

	if !c.Origins.Allowed(link.Origin) {
		if !c.AllowOrigin(link.Origin) {
			log.Info("cloud: the person did not allow this server")
			return OutcomeOriginRefused
		}
		if err := c.Origins.Allow(link.Origin, c.now()); err != nil {
			// Allowed for this request, said in the log; asked again
			// next time, which is the safe side of a list that will not
			// save.
			log.Warn("cloud: the server was allowed and could not be remembered", "error", err)
		} else {
			log.Info("cloud: the person allowed this server")
		}
	}

	body, err := c.Client.FetchJob(ctx, link.JobURL, api.MaxCloudJobBytes)
	if errors.Is(err, ErrJobGone) {
		log.Info("cloud: the server says the job is gone")
		return OutcomeJobGone
	}
	if err != nil {
		log.Warn("cloud: the job could not be fetched", "error", err)
		return OutcomeUnreachable
	}

	job, e := api.ParseCloudJob(body, link.Origin, c.DocumentSigning())
	var resultURL *url.URL
	sameOrigin := false
	if job.ResultURL != "" {
		resultURL, sameOrigin = SameOrigin(job.ResultURL, link.Origin)
	}
	if e != nil {
		log.Info("cloud: the job was refused before any window", "code", string(e.Code))
		if sameOrigin {
			c.deliverFailure(ctx, log, resultURL, e)
		}
		return OutcomeJobRefused
	}
	if !sameOrigin {
		// Nothing is sent anywhere: the one address there is to send to
		// is the one this rule forbids.
		log.Warn("cloud: the job's resultUrl is not on the server it came from; refused")
		return OutcomeJobRefused
	}

	req := job.Request
	tracked, err := c.Jobs.Submit("cloud:"+link.Origin, len(req.Digests), consent.Fingerprint(req.Digests))
	if errors.Is(err, jobs.ErrJobInProgress) {
		log.Info("cloud: a request from this server is already up")
		c.deliverFailure(ctx, log, resultURL, errs.New(errs.CodeJobInProgress, err))
		return OutcomeBusy
	}
	if err != nil {
		log.Error("cloud: the job could not be accepted", "error", err)
		c.deliverFailure(ctx, log, resultURL, errs.New(errs.CodeInternal, err))
		return OutcomeJobRefused
	}
	log.Info("cloud: a request was put in front of the person", "kind", string(req.Kind), "documents", len(req.Digests))

	// Not ctx: the person's window does not close because whatever asked
	// for it went away (the loopback protocol's runJob, for the same reason).
	result, err := c.Signer.Sign(context.Background(), req, tracked)
	if err != nil {
		code := errs.CodeInternal
		var coded *errs.Error
		if errors.As(err, &coded) {
			code = coded.Code
		}
		result = api.SignResult{Code: code}
	}
	out, err := api.MarshalCloudResult(req, result)
	if err != nil {
		log.Error("cloud: the result could not be rendered", "error", err)
		tracked.Fail(c.now(), errs.CodeInternal, jobs.Update{})
		return OutcomeUndelivered
	}
	if result.Code != "" {
		tracked.Fail(c.now(), result.Code, jobs.Update{Failed: len(req.Digests)})
	} else {
		tracked.Complete(c.now(), nil, jobs.Update{Completed: len(result.Outcomes), Failed: len(result.Outcomes) - result.Signed()})
	}
	c.Jobs.Forget(tracked.ID)

	if err := c.Client.PostResult(ctx, resultURL, out); err != nil {
		log.Warn("cloud: the server did not take the answer", "error", err, "code", string(result.Code), "signed", result.Signed())
		return OutcomeUndelivered
	}
	log.Info("cloud: the answer was delivered", "code", string(result.Code), "signed", result.Signed())
	return OutcomeAnswered
}

func (c *Channel) deliverFailure(ctx context.Context, log *slog.Logger, to *url.URL, e *errs.Error) {
	body, err := api.MarshalCloudFailure(e)
	if err != nil {
		return
	}
	if err := c.Client.PostResult(ctx, to, body); err != nil {
		log.Info("cloud: the server was not told why the job was refused", "error", err)
	}
}
