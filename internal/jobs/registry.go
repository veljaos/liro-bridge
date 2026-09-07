package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// A job is one batch a program asked the agent to sign, from the
// moment the request is accepted to the moment its result is collected
// (F7 §7). It lives here, beside the queue and the runner, because it
// is the same thing they are about — a batch being signed — seen by a
// caller that is not sitting at the window.
//
// Nothing in this file knows about HTTP. The registry hands out
// identifiers, remembers state, publishes changes to whoever is
// following, holds a result until it is collected exactly once, and
// forgets a job nobody came back for. internal/api is what turns that
// into 202s, an event stream and a 404.

// JobState is where a job has got to. Exactly F7 §7.2's seven names,
// in the order a job passes through them.
//
// It is JobState rather than State because this package already has a
// State: the one a single document is in inside a queue. Two different
// things, two different names — a run has documents in it, and a job
// has a run in it.
type JobState string

const (
	// JobQueued: the request has been accepted and nothing is on
	// screen yet. A job waits here while another job's consent window
	// is open, because the agent shows one at a time.
	JobQueued JobState = "queued"

	// JobAwaitingConsent: the agent's own window is up and a person
	// has not answered yet. Updates in this state carry
	// RemainingConsent, so a caller can show its own countdown rather
	// than guessing when the window will expire (F7 §7.4).
	JobAwaitingConsent JobState = "awaiting_consent"

	// JobAwaitingPIN: the person approved, and the agent is opening
	// the card — which is where the operating system's own PIN dialog
	// appears, if the card asks for one. The PIN never enters this
	// process (D-025); what this state says is that a dialog nobody
	// here controls may be in front of the person.
	JobAwaitingPIN JobState = "awaiting_pin"

	// JobPreparingCard: the first signature is in flight. SPEC §12.9
	// measured that at about 4.9 s on a MUP card, and it is card
	// initialisation rather than anything this agent is doing — a
	// caller showing a progress bar needs to know the period is
	// expected rather than stalled (F7 §7.2).
	JobPreparingCard JobState = "preparing_card"

	// JobSigning: every signature after the first.
	JobSigning JobState = "signing"

	// JobCompleted: the run finished and a result is waiting to be
	// collected. At least one document was signed.
	JobCompleted JobState = "completed"

	// JobFailed: nothing was signed. Code says why.
	JobFailed JobState = "failed"
)

// Terminal reports whether s is a state a job never leaves.
func (s JobState) Terminal() bool { return s == JobCompleted || s == JobFailed }

// Update is one moment of a job, as a follower sees it. It carries no
// strings a person reads: every word a caller shows is that caller's
// own, chosen from the code and the counts (SPEC §7).
type Update struct {
	State JobState

	// Completed, Total and Failed count documents.
	Completed int
	Total     int
	Failed    int

	// ETA is the estimated remaining time for the whole batch, and
	// ETAKnown says whether it means anything yet. It is computed from
	// the measured first signature and the measured median of the rest
	// (signing.EstimatedTotal), never from a constant — SPEC §12.9's
	// own rule, and the reason it stays honest when a card or a key
	// size changes.
	ETA      time.Duration
	ETAKnown bool

	// RemainingConsent is how long the person has left to answer, and
	// applies only to JobAwaitingConsent (F7 §7.4).
	RemainingConsent time.Duration

	// Code is why a failed job failed, and is empty in every other
	// state.
	Code errs.Code
}

// ErrJobInProgress is returned by Registry.Submit when the owner
// already has a job that has not finished (F7 §10: one job per
// application at a time).
var ErrJobInProgress = errors.New("jobs: this application already has a job running")

// DefaultJobTTL is how long a finished job's result waits to be
// collected before it is discarded (F7 §7.3). Ten minutes is long
// enough for a caller that polls slowly or was restarted mid-batch, and
// short enough that qualified signatures are not left lying in a
// process's memory for an afternoon.
const DefaultJobTTL = 10 * time.Minute

// Job is one batch, from acceptance to collection.
type Job struct {
	// ID is what the caller was given and uses to follow and collect.
	ID string

	// Owner is the paired application the job belongs to — what makes
	// "one job per application" checkable, and what a second submission
	// is refused against.
	Owner string

	// Total is how many documents the job covers.
	Total int

	// Fingerprint is SHA-256 over the concatenated digests, hex —
	// exactly what the consent window shows, so a technical user can
	// compare what was approved against what the application says it
	// sent (SPEC §6.6).
	Fingerprint string

	// CreatedAt is when the request was accepted.
	CreatedAt time.Time

	mu    sync.Mutex
	state Update
	// changed is closed and replaced on every publish. A follower reads
	// the state, then waits on the channel it saw beside it, so it
	// always ends up on the newest state and can never miss the
	// terminal one — which a buffered channel of updates can, the
	// moment a slow follower's buffer fills.
	changed chan struct{}

	result      []byte
	resultTaken bool
	finishedAt  time.Time
}

// Registry holds every live job.
type Registry struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	running map[string]string // owner -> the identifier of its unfinished job
	ttl     time.Duration
	now     func() time.Time
}

// NewRegistry returns an empty registry. now may be nil, in which case
// time.Now is used.
func NewRegistry(now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	return &Registry{
		jobs:    make(map[string]*Job),
		running: make(map[string]string),
		ttl:     DefaultJobTTL,
		now:     now,
	}
}

// WithTTL returns r with a different collection deadline. It exists so
// a test can reach the deadline without waiting ten minutes; the
// production value is DefaultJobTTL and nothing else sets it.
func (r *Registry) WithTTL(ttl time.Duration) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ttl = ttl
	return r
}

// Submit accepts a job for owner and returns it in JobQueued.
//
// It refuses with ErrJobInProgress when that owner already has a job
// that has not finished. The slot is freed when the job finishes, not
// when its result is collected: a caller that never collects would
// otherwise be locked out for the whole ten minutes, and "one job at a
// time" (F7 §10) is about what the agent is doing, not about what the
// caller has read.
func (r *Registry) Submit(owner string, total int, fingerprint string) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()

	// "Running" means unfinished, not "still known": a finished job
	// stays in the registry until its result is collected or its
	// deadline passes, and holding the owner's one slot for that whole
	// time would lock a caller out for ten minutes over a batch that is
	// already done.
	if id, ok := r.running[owner]; ok {
		if j, known := r.jobs[id]; known && !j.Snapshot().State.Terminal() {
			return nil, ErrJobInProgress
		}
		delete(r.running, owner)
	}

	id, err := randomID()
	if err != nil {
		return nil, err
	}
	j := &Job{
		ID:          id,
		Owner:       owner,
		Total:       total,
		Fingerprint: fingerprint,
		CreatedAt:   r.now(),
		state:       Update{State: JobQueued, Total: total},
		changed:     make(chan struct{}),
	}
	r.jobs[id] = j
	r.running[owner] = id
	return j, nil
}

// Get returns the job with this identifier.
func (r *Registry) Get(id string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
	j, ok := r.jobs[id]
	return j, ok
}

// Forget removes a job outright. The result endpoint calls it after
// handing the result over: F7 §7.3 is explicit that the result is
// delivered once and the job is then forgotten, and a second call
// answers 404 because there is nothing there rather than because a
// flag says so.
func (r *Registry) Forget(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return
	}
	delete(r.jobs, id)
	if r.running[j.Owner] == id {
		delete(r.running, j.Owner)
	}
}

// Len is how many jobs the registry currently holds. For tests and a
// log line, not for the protocol.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.jobs)
}

// Sweep discards finished jobs whose results nobody collected within
// the deadline. It runs on every registry operation as well, so a
// caller that never calls it still cannot make the registry grow
// without bound; this exists so an agent can also sweep on a timer and
// not leave a result sitting in memory until the next request happens
// to arrive.
func (r *Registry) Sweep() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sweepLocked()
}

// sweepLocked drops every finished job past its deadline. Held under
// r.mu.
//
// Only finished jobs are ever discarded. An unfinished one is bounded
// by its own consent timeout and by the run it is in; discarding it
// from underneath a signature in flight would leave a caller with no
// way to learn what happened to a batch that is still happening.
func (r *Registry) sweepLocked() {
	now := r.now()
	for id, j := range r.jobs {
		if !j.expired(now, r.ttl) {
			continue
		}
		delete(r.jobs, id)
		if r.running[j.Owner] == id {
			delete(r.running, j.Owner)
		}
	}
}

func (j *Job) expired(now time.Time, ttl time.Duration) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.finishedAt.IsZero() {
		return false
	}
	return now.Sub(j.finishedAt) >= ttl
}

// Snapshot is the job's current state.
func (j *Job) Snapshot() Update {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.state
}

// Publish records a new state and wakes every follower.
//
// A terminal state cannot be published over: once a job has completed
// or failed, that is what it did, and a late progress hook from a run
// that has already ended must not resurrect it.
func (j *Job) Publish(u Update) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state.State.Terminal() {
		return
	}
	if u.Total == 0 {
		u.Total = j.Total
	}
	j.state = u
	close(j.changed)
	j.changed = make(chan struct{})
}

// Complete finishes the job with a result to be collected once.
func (j *Job) Complete(now time.Time, result []byte, u Update) {
	u.State = JobCompleted
	j.finish(now, result, u)
}

// Fail finishes the job with nothing to collect. code is what the
// caller is told.
func (j *Job) Fail(now time.Time, code errs.Code, u Update) {
	u.State = JobFailed
	u.Code = code
	j.finish(now, nil, u)
}

func (j *Job) finish(now time.Time, result []byte, u Update) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.state.State.Terminal() {
		return
	}
	if u.Total == 0 {
		u.Total = j.Total
	}
	j.state = u
	j.result = result
	j.finishedAt = now
	close(j.changed)
	j.changed = make(chan struct{})
}

// TakeResult hands the result over exactly once. The second call
// reports false, whatever the first one did — and so does the first,
// for a job that finished with nothing to give: a refused batch and a
// collected one are both "there is no result here", and a caller must
// not be handed an empty success for either.
func (j *Job) TakeResult() ([]byte, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.state.State.Terminal() || j.resultTaken || j.result == nil {
		return nil, false
	}
	j.resultTaken = true
	out := j.result
	j.result = nil
	return out, true
}

// Follow calls fn with the job's state, starting with the state as it
// stands now and again on every change, and returns when the job has
// reached a terminal state (fn having been called with it) or ctx is
// done.
//
// fn returning an error ends the follow with that error — which is how
// an event stream stops when its client has gone away.
//
// What is guaranteed, exactly: a follower always sees the state as it
// stands when it starts, and always sees the terminal one. What it is
// not promised is every intermediate state it was too slow to read —
// a follower that falls behind lands on the newest state rather than
// working through a backlog. That is the right shape for progress: a
// hundred "signing" updates a caller has fallen behind on are worth
// less than the one that says where the batch actually is, and a
// caller reconnecting mid-batch wants where the job is now rather than
// where it was.
func (j *Job) Follow(ctx context.Context, fn func(Update) error) error {
	for {
		j.mu.Lock()
		u := j.state
		changed := j.changed
		j.mu.Unlock()

		if err := fn(u); err != nil {
			return err
		}
		if u.State.Terminal() {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// randomID is a job identifier: 16 random bytes, hex. It is not a
// secret — every request that uses it is already authenticated, and it
// travels in a URL — but it is unguessable anyway, so that a paired
// application cannot stumble onto another one's job by counting.
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("jobs: reading random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}
