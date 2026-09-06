package dss

import "sync"

// EndpointMemory remembers, for the lifetime of one batch, which
// revocation endpoints did not answer, so that a responder which timed
// out for document 1 is not asked again for document 100.
//
// Measured (FTEST J-10, and again here): an OCSP responder that accepts
// the request and never answers costs 20 s per document — ocspAttempts
// (2) attempts of ocspTimeout (10 s) — and that cost is paid inside
// every SignDocument, because revocation is collected per document.
// Ten documents against such a responder measured 3m20s; a hundred is
// about 33 minutes of timeouts for work that should take 46 seconds.
// D-076 measured MUP's real responder as dropping connections, which is
// exactly this case. A responder that is unreachable at the network
// level for one document is unreachable for the next one a fraction of
// a second later; asking again buys nothing and costs twenty seconds.
//
// Only failures are remembered. A *successful* response is deliberately
// not cached across documents: D-046's ordering rule stands — revocation
// is collected after the signature exists, so that the response
// postdates the signature — and a response fetched after document 1
// predates document 100's signature. Whether that matters for a
// qualified signature is a question for the owner, not something to
// settle by quietly reusing a response.
//
// The memory is scoped to one batch, never to the process: a new batch
// starts with no assumptions, because a responder that was down five
// minutes ago may be up now. Callers create one per batch and pass it
// through pades.Options.RevocationMemory.
//
// A nil *EndpointMemory is usable and remembers nothing: every endpoint
// is tried every time, which is exactly the behaviour of a caller that
// has not opted in (a single-document signature has nothing to
// remember for).
type EndpointMemory struct {
	mu   sync.Mutex
	dead map[string]struct{}
}

// NewEndpointMemory returns a memory for one batch.
func NewEndpointMemory() *EndpointMemory {
	return &EndpointMemory{dead: make(map[string]struct{})}
}

// silent reports whether url has already failed to answer during this
// batch. A nil memory never reports anything as silent.
func (m *EndpointMemory) silent(url string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.dead[url]
	return ok
}

// remember records that url produced no usable answer. A nil memory
// remembers nothing.
//
// "No usable answer" is deliberately narrower than "no evidence
// embedded": an endpoint that answered with something this package then
// declined to embed (an artefact over D-076's size cap) is not
// remembered here, because it did answer, and because Entry.TooLarge —
// which is what makes the resulting B-T honest rather than silent — is
// computed from an actual fetch. Skipping it on later documents would
// turn a specific, reportable reason into a generic one.
func (m *EndpointMemory) remember(url string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dead[url] = struct{}{}
}

// SilentEndpoints returns how many endpoints this memory has stopped
// asking. It exists for logging and for tests: nothing in the signing
// path branches on it.
func (m *EndpointMemory) SilentEndpoints() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.dead)
}
