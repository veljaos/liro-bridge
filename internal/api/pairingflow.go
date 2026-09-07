package api

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
)

// Pairing limits and lifetimes (F7 §2.1, §10).
const (
	// PairingCodeTTL is how long a six-digit code is good for.
	PairingCodeTTL = 5 * time.Minute

	// MaxPairingAttempts is how many wrong codes void a request. The
	// code is six digits, so five guesses out of a million is not a
	// number an attacker works with; the limit is there so that a
	// window left open for four minutes cannot quietly become an
	// oracle.
	MaxPairingAttempts = 5

	// PairingRequestsPerMinute is the per-origin ceiling on how often
	// an application may ask to pair (F7 §10). Pairing opens a window
	// in front of a person; an unpaired caller in a loop must not be
	// able to do that without end.
	PairingRequestsPerMinute = 3

	// pairingRateWindow is the window PairingRequestsPerMinute counts
	// over.
	pairingRateWindow = time.Minute

	// MaxOriginLength bounds the origin an application declares. 255 is
	// generous for a URL and small enough that the value is still one a
	// person can read in a window.
	MaxOriginLength = 255
)

// PairingPrompt is what the agent's pairing window shows: the
// application's declared name and origin, both already checked, and the
// six-digit code the person is to read out.
type PairingPrompt struct {
	RequestID string
	Code      string
	Name      string
	Origin    string
	ExpiresAt time.Time
}

// PairingWindow is one open pairing window, as the flow drives it.
type PairingWindow interface {
	// Denied is closed when the person refuses the pairing — the Deny
	// button, the title bar's close box, or Escape.
	Denied() <-chan struct{}

	// Confirmed tells the window the application supplied the right
	// code, so it can say so rather than simply vanishing at the moment
	// the person was reading it.
	Confirmed()

	// Close closes the window. Idempotent.
	Close()
}

// PairingUI opens the agent's own pairing window.
//
// It is an interface, and internal/api never imports internal/ui,
// because SPEC §4.2 rule 4 keeps the three front doors independent:
// cmd/liro-bridge is the one place that knows a pairing prompt is drawn
// by WebView2. Tests supply a fake and exercise the whole flow with no
// window at all.
type PairingUI interface {
	ShowPairing(PairingPrompt) (PairingWindow, error)
}

// PairingResult is what a successful confirm returns to the
// application. The device secret appears here and nowhere else, ever.
type PairingResult struct {
	Pairing      Pairing
	DeviceSecret []byte
}

// pendingPairing is the one pairing request that may be open at a time.
//
// Its window is set after the request has already claimed the flow's
// single slot — showing a window takes long enough that two requests
// arriving together would otherwise both get past the "one at a time"
// check and open two — so the window and the finished flag live behind
// this type's own mutex rather than the flow's.
type pendingPairing struct {
	id        string
	code      string
	name      string // sanitised; this is what is bound
	origin    string // validated; bound and compared verbatim
	expiresAt time.Time
	attempts  int
	stop      chan struct{} // closed once the request is finished

	mu       sync.Mutex
	window   PairingWindow
	finished bool
}

// setWindow attaches the window that was opened for p. A request that
// finished while the window was still being created — a shutdown, at
// worst — closes it immediately rather than leaving it on screen with
// nothing behind it.
func (p *pendingPairing) setWindow(w PairingWindow) {
	p.mu.Lock()
	if p.finished {
		p.mu.Unlock()
		w.Close()
		return
	}
	p.window = w
	p.mu.Unlock()
}

func (p *pendingPairing) currentWindow() PairingWindow {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.window
}

// finish ends p exactly once. paired says whether the application
// supplied the right code: the window is told so, and shows it, rather
// than vanishing at the moment the person was reading it.
func (p *pendingPairing) finish(paired bool) {
	p.mu.Lock()
	already := p.finished
	p.finished = true
	w := p.window
	p.mu.Unlock()

	if already {
		return
	}
	close(p.stop)
	if w == nil {
		return
	}
	if paired {
		w.Confirmed()
		return
	}
	w.Close()
}

// PairingFlow implements F7 §2: request, show a code, confirm.
type PairingFlow struct {
	mu       sync.Mutex
	pairings *Pairings
	ui       PairingUI
	now      func() time.Time
	after    func(time.Duration) <-chan time.Time

	pending *pendingPairing

	// denied remembers, until its original expiry, which request
	// identifiers the person refused, so that an application calling
	// confirm is told it was refused rather than that its request
	// expired. Those two need different reactions: one is worth
	// retrying and the other is an answer.
	denied map[string]time.Time

	// recent counts pairing requests per origin over pairingRateWindow.
	// Entries are pruned on every use, so it is bounded by the number
	// of distinct origins that asked in the last minute rather than by
	// the life of the process.
	recent map[string][]time.Time
}

// NewPairingFlow returns a flow over pairings, showing prompts through
// ui. now and after may be nil, in which case time.Now and time.After
// are used; tests supply their own so nothing waits five real minutes.
func NewPairingFlow(pairings *Pairings, ui PairingUI, now func() time.Time, after func(time.Duration) <-chan time.Time) *PairingFlow {
	if now == nil {
		now = time.Now
	}
	if after == nil {
		after = time.After
	}
	return &PairingFlow{
		pairings: pairings,
		ui:       ui,
		now:      now,
		after:    after,
		denied:   make(map[string]time.Time),
		recent:   make(map[string][]time.Time),
	}
}

// Request begins a pairing: it checks what the application declared,
// generates a six-digit code, and opens the agent's own window showing
// it. It returns the request identifier and how long the code is good
// for.
//
// The code is deliberately not returned. If it were, an application
// could pair itself without a human ever seeing the window, and the
// whole mechanism — a person reading a code off the agent's screen and
// giving it to the application — would prove nothing (F7 §2.1).
func (f *PairingFlow) Request(applicationName, origin string) (requestID string, expiresIn time.Duration, apiErr *errs.Error) {
	name, origin, apiErr := validatePairingRequest(applicationName, origin)
	if apiErr != nil {
		return "", 0, apiErr
	}

	f.mu.Lock()
	now := f.now()
	expired := f.sweepLocked(now)

	// The rate limit is charged before the "one at a time" check, and
	// charged for refused requests too. A caller in a loop is what §10
	// is about, and a limiter that only counts the requests it allowed
	// does not limit a loop at all.
	retryAfter, limited := f.rateLimitedLocked(origin, now)
	if limited {
		f.mu.Unlock()
		finishExpired(expired)
		return "", 0, errs.WithDetails(errs.CodeRateLimited, errors.New("too many pairing requests"),
			map[string]any{"retryAfterSeconds": retryAfterSeconds(retryAfter)})
	}
	f.recent[origin] = append(f.recent[origin], now)

	if f.pending != nil {
		f.mu.Unlock()
		finishExpired(expired)
		return "", 0, errs.New(errs.CodePairingInProgress, errors.New("a pairing window is already open"))
	}

	id, err := randomHex(16)
	if err != nil {
		f.mu.Unlock()
		finishExpired(expired)
		return "", 0, errs.New(errs.CodeInternal, err)
	}
	code, err := newPairingCode()
	if err != nil {
		f.mu.Unlock()
		finishExpired(expired)
		return "", 0, errs.New(errs.CodeInternal, err)
	}
	p := &pendingPairing{
		id:        id,
		code:      code,
		name:      name,
		origin:    origin,
		expiresAt: now.Add(PairingCodeTTL),
		stop:      make(chan struct{}),
	}
	f.pending = p
	f.mu.Unlock()
	finishExpired(expired)

	win, err := f.ui.ShowPairing(PairingPrompt{
		RequestID: p.id,
		Code:      p.code,
		Name:      p.name,
		Origin:    p.origin,
		ExpiresAt: p.expiresAt,
	})
	if err != nil {
		f.mu.Lock()
		if f.pending == p {
			f.pending = nil
		}
		f.mu.Unlock()
		p.finish(false)
		slog.Error("api: could not open the pairing window", "error", err)
		return "", 0, errs.New(errs.CodeInternal, err)
	}
	p.setWindow(win)

	go f.watch(p)

	return p.id, PairingCodeTTL, nil
}

// watch ends the pairing when the person refuses it or when the code
// expires, whichever comes first.
func (f *PairingFlow) watch(p *pendingPairing) {
	var denied <-chan struct{}
	if w := p.currentWindow(); w != nil {
		denied = w.Denied()
	}
	select {
	case <-denied:
		f.forget(p, true)
		p.finish(false)
		slog.Info("api: the person refused a pairing request")
	case <-f.after(PairingCodeTTL):
		f.forget(p, false)
		p.finish(false)
		slog.Info("api: a pairing request expired before it was confirmed")
	case <-p.stop:
		// Confirmed, voided by wrong codes, or reaped by a sweep.
	}
}

// forget clears p from the flow's single slot, and records a refusal
// when there was one.
func (f *PairingFlow) forget(p *pendingPairing, refused bool) {
	f.mu.Lock()
	if f.pending == p {
		f.pending = nil
	}
	if refused {
		f.denied[p.id] = p.expiresAt
	}
	f.mu.Unlock()
}

// Confirm completes a pairing. The device secret it returns is issued
// exactly once: a second call for the same identifier is answered
// PAIRING_EXPIRED, because the request is spent the moment the right
// code arrives.
//
// origin must be the origin the request was made from. It is the
// binding SPEC §6.2 describes, checked at the moment the secret would
// be issued.
func (f *PairingFlow) Confirm(requestID, code, origin string) (PairingResult, *errs.Error) {
	f.mu.Lock()
	now := f.now()
	expired := f.sweepLocked(now)

	if _, ok := f.denied[requestID]; ok && requestID != "" {
		f.mu.Unlock()
		finishExpired(expired)
		return PairingResult{}, errs.New(errs.CodePairingDenied, errors.New("the person refused this pairing"))
	}

	p := f.pending
	if p == nil || requestID == "" || p.id != requestID {
		f.mu.Unlock()
		finishExpired(expired)
		// Unknown, superseded, already spent or expired — one answer,
		// so that guessing identifiers reveals nothing about which ones
		// exist.
		return PairingResult{}, errs.New(errs.CodePairingExpired, errors.New("no such live pairing request"))
	}
	if p.origin != origin {
		f.mu.Unlock()
		finishExpired(expired)
		// Not fatal to the request: an integrator that declared two
		// different origins can fix the second call without starting
		// over, and whoever got this answer still does not have the
		// code.
		return PairingResult{}, errs.New(errs.CodePairingOriginMismatch,
			errors.New("confirm came from a different origin than request"))
	}

	if subtle.ConstantTimeCompare([]byte(p.code), []byte(code)) != 1 {
		p.attempts++
		remaining := MaxPairingAttempts - p.attempts
		if remaining > 0 {
			f.mu.Unlock()
			finishExpired(expired)
			return PairingResult{}, errs.WithDetails(errs.CodePairingCodeIncorrect,
				errors.New("wrong pairing code"), map[string]any{"attemptsRemaining": remaining})
		}
		f.pending = nil
		f.denied[p.id] = p.expiresAt
		f.mu.Unlock()
		finishExpired(expired)
		p.finish(false)
		slog.Info("api: a pairing request was voided after too many wrong codes")
		return PairingResult{}, errs.New(errs.CodePairingExpired, errors.New("too many wrong codes"))
	}

	// Right code. The request is spent before the secret is issued, so
	// a second call cannot reach this point even if what follows fails.
	f.pending = nil
	f.mu.Unlock()
	finishExpired(expired)

	pairing, secret, err := f.pairings.Add(p.name, p.origin)
	if err != nil {
		p.finish(false)
		slog.Error("api: could not store a new pairing", "error", err)
		return PairingResult{}, errs.New(errs.CodeInternal, err)
	}

	p.finish(true)
	slog.Info("api: an application was paired", "appId", pairing.AppID)
	return PairingResult{Pairing: pairing, DeviceSecret: secret}, nil
}

// Cancel ends whatever pairing request is open, if any, as though the
// person had refused it. It is what the agent calls when it is shutting
// down with a window still up.
func (f *PairingFlow) Cancel() {
	f.mu.Lock()
	p := f.pending
	f.pending = nil
	if p != nil {
		f.denied[p.id] = p.expiresAt
	}
	f.mu.Unlock()
	if p != nil {
		p.finish(false)
	}
}

// sweepLocked drops refusals and rate-limit entries that have aged out,
// and takes the pending request out of the slot if its code has
// expired. Held under f.mu; on every use, so neither map grows with the
// life of the process.
//
// It returns the expired request rather than closing its window,
// because closing a window is not something to do while holding a lock
// every request path needs. The caller passes it to finishExpired.
func (f *PairingFlow) sweepLocked(now time.Time) *pendingPairing {
	for id, forgetAt := range f.denied {
		if !forgetAt.After(now) {
			delete(f.denied, id)
		}
	}
	for origin, times := range f.recent {
		kept := times[:0]
		for _, t := range times {
			if now.Sub(t) < pairingRateWindow {
				kept = append(kept, t)
			}
		}
		if len(kept) == 0 {
			delete(f.recent, origin)
			continue
		}
		f.recent[origin] = kept
	}
	if f.pending != nil && !now.Before(f.pending.expiresAt) {
		p := f.pending
		f.pending = nil
		return p
	}
	return nil
}

// finishExpired closes an expired request's window, if there was one.
// Separate from sweepLocked so it runs outside the flow's lock.
func finishExpired(p *pendingPairing) {
	if p != nil {
		p.finish(false)
	}
}

// rateLimitedLocked reports whether origin has already used its
// allowance, and how long until it has one again. Held under f.mu.
func (f *PairingFlow) rateLimitedLocked(origin string, now time.Time) (time.Duration, bool) {
	times := f.recent[origin]
	if len(times) < PairingRequestsPerMinute {
		return 0, false
	}
	oldest := times[len(times)-PairingRequestsPerMinute]
	return pairingRateWindow - now.Sub(oldest), true
}

// retryAfterSeconds renders a wait for the Details of a RATE_LIMITED
// error, rounded up so that a caller obeying it is never refused a
// second time for being a fraction of a second early.
func retryAfterSeconds(d time.Duration) int {
	if d <= 0 {
		return 1
	}
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 1 {
		return 1
	}
	return secs
}

// newPairingCode returns six uniformly distributed decimal digits,
// leading zeros included — "042317" is a code, and rendering it as
// "42317" would be a code nobody can type.
func newPairingCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("api: generating a pairing code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// validatePairingRequest checks and normalises what an application
// declared about itself.
//
// The name is sanitised the way every other piece of untrusted display
// text in this project is (SPEC §6.6, F5 §5.3): control characters and
// Unicode direction overrides stripped, then truncated with the middle
// elided. The sanitised form is what gets bound, so that the name shown
// at pairing and the name shown above every later signature are
// byte-identical.
//
// The origin is not sanitised — it is refused. SPEC §6.2 and F7 §2.1
// require it to be shown verbatim, with no prettifying and no stripping
// of the scheme, because a person who sees http:// where they expected
// https:// must be able to notice. A value altered on its way to the
// screen is not verbatim, so the only honest options are to display
// exactly what arrived or to decline it — and an origin with a control
// character in it is not an origin.
func validatePairingRequest(applicationName, origin string) (string, string, *errs.Error) {
	name := strings.TrimSpace(consent.TruncateMiddle(consent.SanitizeDisplayText(applicationName), consent.MaxDisplayLength))
	if name == "" {
		return "", "", errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("applicationName is empty"), map[string]any{"field": "applicationName"})
	}

	if origin == "" || len(origin) > MaxOriginLength {
		return "", "", errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("origin is empty or too long"), map[string]any{"field": "origin"})
	}
	for _, r := range origin {
		if unicode.IsControl(r) || unicode.IsSpace(r) || isDirectionOverride(r) {
			return "", "", errs.WithDetails(errs.CodeRequestInvalid,
				errors.New("origin contains a control, whitespace or direction-override character"),
				map[string]any{"field": "origin"})
		}
	}
	return name, origin, nil
}

// isDirectionOverride mirrors internal/consent's own rule for the eight
// Unicode bidirectional-control characters SPEC §6.6 names. It is
// repeated rather than exported from there because the two do different
// things with the answer: consent strips them from text it will
// display, and this refuses a value that contains one.
func isDirectionOverride(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}
