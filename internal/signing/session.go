// Package signing implements the orchestration layer above
// internal/keysource (F2 §4/§5): session lifetime, batching, PIN-policy
// detection and timing. It has no idea what a document, a PDF or a CMS
// structure is — it signs pre-computed digests, nothing else.
package signing

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// Explicit lifetime numbers from F2 §4.1 — not suggestions.
const (
	// IdleTimeout closes a session after this much inactivity: long
	// enough for a user to answer a phone call mid-batch, short enough
	// that a walked-away-from machine does not hold an authenticated
	// card open.
	IdleTimeout = 90 * time.Second

	// MaxLifetime is an absolute ceiling regardless of activity. A batch
	// of 1000 documents at ~0.41s each is under 7 minutes, so this
	// never truncates legitimate work.
	MaxLifetime = 30 * time.Minute

	// ApprovalWindow is how long after a session opens the first
	// signature must begin. The user has already approved; if signing
	// has not begun within this window, something is wrong.
	ApprovalWindow = 120 * time.Second
)

// ErrSessionExpired is returned when a session is used after one of the
// F2 §4.1 lifetime limits has been exceeded. The caller must open a new
// session (and, above this package, get a new approval and PIN).
var ErrSessionExpired = errors.New("signing: session has expired")

// Session wraps a keysource.Session with the lifetime rules from F2
// §4.1. Not safe for concurrent use (SPEC §8.5) — exactly like the
// keysource.Session it wraps. One Session is owned by one batch at a
// time (F2 §4.2); Manager enforces that above this type.
type Session struct {
	inner        keysource.Session
	openedAt     time.Time
	lastActivity time.Time
	signedOnce   bool
	closed       bool
	now          func() time.Time
}

func newSession(inner keysource.Session, now func() time.Time) *Session {
	t := now()
	return &Session{inner: inner, openedAt: t, lastActivity: t, now: now}
}

// WrapSession applies the F2 §4.1 lifetime rules to an
// already-opened keysource.Session, using the real wall clock. Callers
// that already have a keysource.Session in hand — the CLI's
// sign-digest command opens exactly one directly, without going
// through a Manager — use this instead of Manager.Open.
func WrapSession(inner keysource.Session) *Session {
	return newSession(inner, time.Now)
}

// checkLive returns ErrSessionExpired (wrapped with which limit fired)
// if any F2 §4.1 lifetime rule has been exceeded. It does not mutate or
// close the session — callers decide what to do with the result.
func (s *Session) checkLive() error {
	if s.closed {
		return ErrSessionExpired
	}
	now := s.now()
	if d := now.Sub(s.openedAt); d > MaxLifetime {
		return fmt.Errorf("%w: maximum lifetime %s exceeded (open for %s)", ErrSessionExpired, MaxLifetime, d)
	}
	if d := now.Sub(s.lastActivity); d > IdleTimeout {
		return fmt.Errorf("%w: idle timeout %s exceeded (idle for %s)", ErrSessionExpired, IdleTimeout, d)
	}
	if !s.signedOnce {
		if d := now.Sub(s.openedAt); d > ApprovalWindow {
			return fmt.Errorf("%w: no signature began within the %s approval window (waited %s)", ErrSessionExpired, ApprovalWindow, d)
		}
	}
	return nil
}

// SignDigest signs one digest, enforcing the session's lifetime rules
// first. A session that has expired is closed (freeing the underlying
// key handle) before the error is returned, so the caller's next
// attempt fails fast rather than retrying a dead session.
func (s *Session) SignDigest(ctx context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if err := s.checkLive(); err != nil {
		_ = s.Close()
		return nil, err
	}
	sig, err := s.inner.SignDigest(ctx, alg, digest)
	s.lastActivity = s.now()
	s.signedOnce = true
	return sig, err
}

// Certificate returns the signer certificate.
func (s *Session) Certificate() keysource.Certificate { return s.inner.Certificate() }

// Chain returns the issuing chain, if the underlying source supplies one.
func (s *Session) Chain() [][]byte { return s.inner.Chain() }

// Close closes the underlying keysource.Session. Idempotent.
func (s *Session) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.inner.Close()
}

// Manager owns open sessions, keyed by certificate thumbprint, and
// enforces that each is used by one batch at a time (F2 §4.2). Multiple
// sessions may exist simultaneously for different certificates; the
// registry itself is guarded by a mutex, but a Session obtained from it
// is not safe for concurrent use — sequential signing against one card
// is deliberate (F2 §4.2): the card is a single serial device, so
// concurrent requests would only queue in the driver anyway, and
// concurrent access to smart card APIs is a known source of
// driver-level failures.
type Manager struct {
	mu       sync.Mutex
	sessions map[keysource.Thumbprint]*Session
	now      func() time.Time
}

// NewManager returns an empty session registry.
func NewManager() *Manager {
	return &Manager{sessions: make(map[keysource.Thumbprint]*Session), now: time.Now}
}

// Open returns a live session for thumbprint, reusing one already in
// the registry if it has not expired, or opening a new one via source
// otherwise. The caller must not call Open again for the same
// thumbprint concurrently with using the returned Session — a Session
// is owned by one batch at a time.
func (m *Manager) Open(ctx context.Context, source keysource.Source, thumbprint keysource.Thumbprint) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.sessions[thumbprint]; ok {
		if existing.checkLive() == nil {
			return existing, nil
		}
		_ = existing.Close()
		delete(m.sessions, thumbprint)
	}

	inner, err := source.Open(ctx, thumbprint)
	if err != nil {
		return nil, err
	}
	sess := newSession(inner, m.now)
	m.sessions[thumbprint] = sess
	return sess, nil
}

// Close closes and forgets the session for thumbprint, if one is open.
func (m *Manager) Close(thumbprint keysource.Thumbprint) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[thumbprint]
	if !ok {
		return nil
	}
	delete(m.sessions, thumbprint)
	return sess.Close()
}

// Len reports how many sessions are currently registered, expired or
// not — used only by tests to observe registry bookkeeping.
func (m *Manager) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}
