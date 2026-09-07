package api

import (
	"sync"
	"time"
)

// NonceTTL is how long a spent nonce is remembered (F7 §3: "Nonce seen
// in the last five minutes"). It is deliberately the same length as the
// pairing code's own lifetime, and comfortably longer than the ±60 s
// timestamp window it works with: a request whose timestamp is inside
// the skew window is always inside the nonce window too, so there is no
// gap where a replay would be accepted because the nonce had been
// forgotten but the timestamp was still fresh.
const NonceTTL = 5 * time.Minute

// MaxNonceLength bounds a client-supplied nonce. A nonce is only
// required to be unique; nothing reads it, so nothing needs it to be
// long. The cap exists because every accepted nonce is held in memory
// for NonceTTL, and a caller sending megabyte nonces would be choosing
// how much of the agent's memory to occupy.
const MaxNonceLength = 128

// maxNonceEntries bounds the cache outright. Only a request whose
// signature has already verified reaches the cache (see Authenticate),
// so filling it takes a paired application sending on the order of
// three hundred requests a second, sustained, for five minutes — which
// is not a client this agent should keep serving. Reaching the cap is
// reported as RATE_LIMITED rather than by evicting entries: evicting
// the oldest would silently re-open exactly the replay window the cache
// exists to close.
const maxNonceEntries = 100_000

// NonceCache remembers which nonces have been spent, per application.
//
// Two properties matter and neither is incidental:
//
//   - It expires entries as it goes. Every use sweeps whatever has aged
//     out, under the same lock, so the cache is bounded by the request
//     rate over NonceTTL rather than growing for the life of the
//     process.
//   - A nonce is spent only after the signature has validated. That
//     ordering lives in Authenticate rather than here, but it is the
//     reason this type has one method that both checks and records:
//     splitting them would invite a caller to check early — which lets
//     an unauthenticated caller fill the cache with fabricated values
//     and lock a real application out of its own nonces.
//
// Nonces are scoped to the application that sent them. Two applications
// independently choosing the same random nonce is otherwise a rejected
// request for one of them, for no reason either could diagnose; and
// scoping cannot weaken replay protection, because a replay carries the
// original signature, which only verifies under the original
// application's secret and therefore only ever reaches its own scope.
type NonceCache struct {
	mu         sync.Mutex
	seen       map[nonceKey]time.Time // -> the moment the entry may be forgotten
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

type nonceKey struct {
	appID string
	nonce string
}

// NewNonceCache returns an empty cache with the standard TTL. now may
// be nil, in which case time.Now is used; tests supply their own.
func NewNonceCache(now func() time.Time) *NonceCache {
	return newNonceCacheWithLimit(now, maxNonceEntries)
}

// newNonceCacheWithLimit is NewNonceCache with the hard cap as a
// parameter, so that the test for what happens at the cap does not have
// to build a hundred thousand entries to reach it.
func newNonceCacheWithLimit(now func() time.Time, maxEntries int) *NonceCache {
	if now == nil {
		now = time.Now
	}
	return &NonceCache{
		seen:       make(map[nonceKey]time.Time),
		ttl:        NonceTTL,
		maxEntries: maxEntries,
		now:        now,
	}
}

// Use records nonce as spent for appID and reports whether it was
// fresh. A false return means the nonce has already been used inside
// the TTL — a replay.
//
// full is true when the cache is at its hard cap and the nonce was not
// recorded; the caller must refuse the request rather than treat it as
// fresh.
func (c *NonceCache) Use(appID, nonce string) (fresh bool, full bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.sweepLocked(now)

	key := nonceKey{appID: appID, nonce: nonce}
	if _, ok := c.seen[key]; ok {
		return false, false
	}
	if len(c.seen) >= c.maxEntries {
		return false, true
	}
	c.seen[key] = now.Add(c.ttl)
	return true, false
}

// sweepLocked drops every entry whose TTL has passed. Held under c.mu.
//
// It walks the whole map, which is what the previous implementation of
// this protocol did and is the right shape here: the map holds only the
// nonces of authenticated requests made in the last five minutes, so it
// is small in every case this agent is actually in — a hundred-document
// batch is a few hundred entries — and a sweep that happens on a
// schedule instead needs a goroutine, a shutdown path and a test for
// both. maxNonceEntries is what bounds the worst case: a full map is a
// walk of a hundred thousand entries, which is about a millisecond, and
// only a caller flooding the agent can put it there.
func (c *NonceCache) sweepLocked(now time.Time) {
	for key, expiry := range c.seen {
		if !expiry.After(now) {
			delete(c.seen, key)
		}
	}
}

// Len returns how many nonces are currently remembered. It exists for
// tests and for a log line, not for the protocol.
func (c *NonceCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}

// Forget drops every nonce belonging to appID. Called when a pairing is
// revoked: the secret those nonces were signed with is gone, so nothing
// can present them again, and holding them serves nothing.
func (c *NonceCache) Forget(appID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.seen {
		if key.appID == appID {
			delete(c.seen, key)
		}
	}
}
