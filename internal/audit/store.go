package audit

// Store persists the audit chain to disk (F5 §8.4):
// %LOCALAPPDATA%\Liro\audit\, one file per month, rotated by size at
// 5 MB, never uploaded. The chain itself spans every file in the
// directory — a file boundary is purely a storage detail, not a break
// in what Verify checks, since tampering with an earlier month's file
// must still be detectable.
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// MaxFileSize is F5 §8.4's rotation threshold.
const MaxFileSize = 5 * 1024 * 1024

// jsonEntry is Entry's on-disk JSON shape. Hash and PrevHash are
// stored hex-encoded — JSON has no native byte-string type, and hex
// keeps the file human-inspectable, which matters for an artefact
// SPEC's export requirement expects a person to be able to look at.
type jsonEntry struct {
	Sequence      uint64    `json:"sequence"`
	Timestamp     time.Time `json:"timestamp"`
	Thumbprint    string    `json:"thumbprint"`
	Application   string    `json:"application"`
	DocumentCount int       `json:"documentCount"`
	Outcome       Outcome   `json:"outcome"`
	FailureCode   string    `json:"failureCode,omitempty"`
	IsTestKey     bool      `json:"isTestKey"`
	AchievedLevel string    `json:"achievedLevel,omitempty"`

	// Channel is which front door the batch arrived through (F7 §6).
	// omitempty, so a local batch's line is byte-identical to what it
	// always was — the empty channel is what "local" is.
	Channel Channel `json:"channel,omitempty"`

	// Discontinuity is present only on a chain's first entry, and only
	// when that chain exists because an earlier one could not be
	// continued. omitempty keeps every other entry's line byte-identical
	// to what it always was.
	Discontinuity *jsonDiscontinuity `json:"discontinuity,omitempty"`

	PrevHash string `json:"prevHash"`
	Hash     string `json:"hash"`
}

// jsonDiscontinuity is Discontinuity's on-disk shape. Named fields
// rather than a free-text note, for the reason Entry's own field set is
// an allow-list: a note is where a file name eventually ends up.
type jsonDiscontinuity struct {
	PreviousChain   int         `json:"previousChain"`
	PreviousFile    string      `json:"previousFile"`
	LastSequence    uint64      `json:"lastSequence"`
	HasLastSequence bool        `json:"hasLastSequence"`
	Line            int         `json:"line"`
	Reason          BreakReason `json:"reason"`
}

func toJSONEntry(e Entry) jsonEntry {
	return jsonEntry{
		Sequence:      e.Sequence,
		Timestamp:     e.Timestamp.Truncate(time.Second).UTC(),
		Thumbprint:    e.Thumbprint,
		Application:   e.Application,
		DocumentCount: e.DocumentCount,
		Outcome:       e.Outcome,
		FailureCode:   string(e.FailureCode),
		IsTestKey:     e.IsTestKey,
		AchievedLevel: e.AchievedLevel,
		Channel:       e.Channel,
		Discontinuity: toJSONDiscontinuity(e.Discontinuity),
		PrevHash:      hexEncode(e.PrevHash),
		Hash:          hexEncode(e.Hash),
	}
}

func toJSONDiscontinuity(d *Discontinuity) *jsonDiscontinuity {
	if d == nil {
		return nil
	}
	return &jsonDiscontinuity{
		PreviousChain:   d.PreviousChain,
		PreviousFile:    d.PreviousFile,
		LastSequence:    d.LastSequence,
		HasLastSequence: d.HasLastSequence,
		Line:            d.Line,
		Reason:          d.Reason,
	}
}

func fromJSONDiscontinuity(d *jsonDiscontinuity) *Discontinuity {
	if d == nil {
		return nil
	}
	return &Discontinuity{
		PreviousChain:   d.PreviousChain,
		PreviousFile:    d.PreviousFile,
		LastSequence:    d.LastSequence,
		HasLastSequence: d.HasLastSequence,
		Line:            d.Line,
		Reason:          d.Reason,
	}
}

func fromJSONEntry(j jsonEntry) (Entry, error) {
	prevHash, err := hexDecode(j.PrevHash)
	if err != nil {
		return Entry{}, fmt.Errorf("audit: decoding prevHash: %w", err)
	}
	hash, err := hexDecode(j.Hash)
	if err != nil {
		return Entry{}, fmt.Errorf("audit: decoding hash: %w", err)
	}
	return Entry{
		Sequence:      j.Sequence,
		Timestamp:     j.Timestamp,
		Thumbprint:    j.Thumbprint,
		Application:   j.Application,
		DocumentCount: j.DocumentCount,
		Outcome:       j.Outcome,
		FailureCode:   errCode(j.FailureCode),
		IsTestKey:     j.IsTestKey,
		AchievedLevel: j.AchievedLevel,
		Channel:       j.Channel,
		Discontinuity: fromJSONDiscontinuity(j.Discontinuity),
		PrevHash:      prevHash,
		Hash:          hash,
	}, nil
}

// Store guards its own directory with two locks, and needs both.
//
// mu is this process's. Append must read the last entry and write the
// new one as one operation, or two concurrent batches finishing at once
// could both compute the same PrevHash and fork the chain.
//
// lock is every process's, and covers exactly the same span. The
// process mutex is not enough and was never claimed to be: it is per
// *Store value*, so two Stores over one directory — which is what a
// tray agent and a `sign` process are, and what F6's twenty Explorer
// invocations are — fork the chain just as readily as two processes do.
// Measured, both ways, in D-223: two sequence-0 entries, both with an
// empty PrevHash, and a log that reports itself tampered with from that
// line onwards for ever.
//
// Reads (All, Chains, Verify, LatestChainFile, Export) take mu and not
// lock. They are tolerant of a chain that cannot be read to the end by
// construction, so the worst a concurrent append can do to a reader is
// hide the line being written at that instant — while making the audit
// window wait on a signing batch, which is what taking the lock here
// would do, buys nothing for it.
type Store struct {
	dir string
	mu  sync.Mutex

	lock platform.DirLock

	// lockTimeout is how long Append waits for lock. A field rather
	// than the constant directly so a test can drive the expiry path
	// without waiting out the real one.
	lockTimeout time.Duration
}

// AppendLockTimeout bounds how long one Append waits for the audit
// directory's lock.
//
// Bounded, because signing must never be blocked indefinitely by a log
// — SPEC §6.7 settles that direction for the unreadable case and it is
// the same trade here. Ten seconds rather than one, because the cost of
// waiting is a report screen that appears late and the cost of giving
// up is a permanent extra chain in the audit log, which a person will
// see for the rest of the log's life. An Append is a read and one line
// written; a machine that cannot finish twenty of them in ten seconds
// has a problem this timeout is not the answer to.
const AppendLockTimeout = 10 * time.Second

// NewStore returns a Store rooted at dir, creating it if necessary.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir, lock: platform.NewDirLock(dir), lockTimeout: AppendLockTimeout}, nil
}

// currentFile returns the file Append should write to for chain, for
// entries timestamped at t: the highest-rotation file that chain has for
// t's month, or a new rotation-1 file if it has none yet, or the next
// rotation if the current one has reached MaxFileSize.
func (s *Store) currentFile(chain int, t time.Time) (string, error) {
	prefix := fmt.Sprintf("%04d-%02d-", t.Year(), t.Month())
	matches, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	sort.Strings(matches)

	rotation := 0
	var latest string
	for _, f := range matches {
		base := filepath.Base(f)
		if chainNumberOf(base) != chain || !strings.HasPrefix(base, prefix) {
			continue
		}
		latest = f
		rotation++
	}
	if latest == "" {
		return s.chainFileName(chain, t.Year(), int(t.Month()), 1), nil
	}
	info, err := os.Stat(latest)
	if err != nil {
		return "", err
	}
	if info.Size() >= MaxFileSize {
		return s.chainFileName(chain, t.Year(), int(t.Month()), rotation+1), nil
	}
	return latest, nil
}

// Append computes next's Sequence/PrevHash/Hash against the last entry
// of the store's current chain and writes it (F5 §8.1/§8.2). It never
// modifies an existing file's earlier lines — one line is opened,
// written and closed per call — and it never truncates, renames or
// deletes anything.
//
// When the current chain cannot be continued — its last entry cannot be
// read, because a line is not a whole entry or because a file cannot be
// read at all — the old file is left exactly as it is and a new chain is
// started beside it, whose first entry records the break: which file
// preceded it, at which sequence and line it stopped, and why. The
// returned Entry carries that record, and only that one does, so a
// caller that tells the person tells them once.
//
// Refusing to continue a chain nobody can read stays right: a hash chain
// continued by guessing is not a hash chain. What changes is that the
// refusal is now recorded and recoverable rather than permanent and
// silent — measured, one truncated last line used to make every
// subsequent Append fail forever (FTEST B-9).
//
// The directory's own lock is held across all of it — the read, the
// arithmetic and the write — because the read is half of what forks a
// chain (D-223). Two more causes reach the same recovery through it:
//
//   - The lock was granted *abandoned*, meaning the process that held it
//     died mid-append, and the entry this one would chain from is not
//     sound. That is the one moment when "the line parses" is not enough
//     to know the chain is whole.
//   - The lock could not be taken within lockTimeout at all, so the
//     current chain's last entry cannot be read safely. The entry goes
//     into a chain of its own, created exclusively, rather than onto one
//     another process may be extending at this instant.
func (s *Store) Append(next Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, err := s.lock.Lock(s.lockTimeout)
	if errors.Is(err, platform.ErrDirLockTimeout) {
		// Somebody else has held the log for longer than anybody should.
		// This process cannot read the current chain's last entry
		// safely, so it does not: it starts a chain of its own and says
		// why, which is D-166's own remedy applied to a different cause.
		slog.Warn("audit: the log's lock could not be taken; starting a new chain rather than refusing to record the batch",
			"waited", s.lockTimeout, "lock", s.lock.Name())
		return s.appendUnguarded(next)
	}
	if err != nil {
		return Entry{}, err
	}
	defer s.lock.Unlock()

	return s.appendLocked(next, state)
}

// appendLocked is Append with the directory's lock held.
func (s *Store) appendLocked(next Entry, state platform.DirLockState) (Entry, error) {
	chains, err := s.chains()
	if err != nil {
		return Entry{}, err
	}

	chain := 1
	var prev *Entry
	if len(chains) > 0 {
		current := chains[len(chains)-1]
		chain = current.Number
		switch {
		case current.Truncated():
			// This chain's tail is unreadable. Leave it exactly where it
			// is — it is evidence up to the point it broke — and start
			// the next one beside it.
			chain = current.Number + 1
			next.Discontinuity = discontinuityFrom(current)
		case state == platform.DirLockAbandoned && !lastEntrySound(current.Entries):
			// The lock was granted abandoned: the process that held it
			// died while holding it, which is precisely when the last
			// line on disk cannot be assumed whole. It parsed — the case
			// above is the one where it did not — so the question left
			// is whether it is *sound*, and it is not. Chaining from it
			// would extend something already broken.
			chain = current.Number + 1
			next.Discontinuity = unsoundDiscontinuityFrom(current)
			slog.Warn("audit: the previous writer died mid-append and the chain's last entry is not sound; starting a new chain",
				"chain", current.Number, "entries", len(current.Entries))
		case len(current.Entries) > 0:
			last := current.Entries[len(current.Entries)-1]
			prev = &last
		}
	}

	completed := AppendEntry(prev, next)

	path, err := s.currentFile(chain, completed.Timestamp)
	if err != nil {
		return Entry{}, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Entry{}, err
	}
	defer func() { _ = f.Close() }()

	line, err := entryLine(completed)
	if err != nil {
		return Entry{}, err
	}
	if _, err := f.Write(line); err != nil {
		return Entry{}, err
	}
	return completed, nil
}

// maxUnguardedChains bounds appendUnguarded's search for a chain number
// nobody else has just taken. It is not a retry count: each step is one
// exclusive create that failed because the file already exists, which
// means one other process reached the same conclusion at the same
// moment. Sixty-four of those at once is not a state this program can
// reason its way out of.
const maxUnguardedChains = 64

// appendUnguarded writes next into a chain of its own, without the
// directory's lock, because the lock could not be had.
//
// Safe without it precisely because it touches nothing anybody else is
// writing: the file is created with O_EXCL, so exactly one process can
// own it, and a process that loses that race takes the next number
// rather than sharing the file. The one line written into it is the
// chain's first, so there is no last entry to read and nothing to
// compute a PrevHash from — which is the read half of Append, the half
// that forks, and the half this path does not do.
func (s *Store) appendUnguarded(next Entry) (Entry, error) {
	chains, err := s.chains()
	if err != nil {
		return Entry{}, err
	}

	d := &Discontinuity{Reason: BreakUnguarded}
	first := 1
	if n := len(chains); n > 0 {
		last := chains[n-1]
		first = last.Number + 1
		// The chain named here is the one this entry would have
		// continued, which is not necessarily the number one below the
		// file this ends up in: if another process is starting a chain
		// at the same moment, this one moves up a number and the chain
		// it could not continue is still the same chain.
		d.PreviousChain = last.Number
		if k := len(last.Files); k > 0 {
			d.PreviousFile = last.Files[k-1]
		}
		if k := len(last.Entries); k > 0 {
			d.LastSequence = last.Entries[k-1].Sequence
			d.HasLastSequence = true
		}
	}
	next.Discontinuity = d

	completed := AppendEntry(nil, next)
	line, err := entryLine(completed)
	if err != nil {
		return Entry{}, err
	}

	t := completed.Timestamp
	for chain := first; chain < first+maxUnguardedChains; chain++ {
		path := s.chainFileName(chain, t.Year(), int(t.Month()), 1)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return Entry{}, err
		}
		defer func() { _ = f.Close() }()
		if _, err := f.Write(line); err != nil {
			return Entry{}, err
		}
		return completed, nil
	}
	return Entry{}, fmt.Errorf("audit: no free chain number after %d attempts starting at %d", maxUnguardedChains, first)
}

// entryLine is one entry's on-disk line, newline included.
func entryLine(e Entry) ([]byte, error) {
	b, err := json.Marshal(toJSONEntry(e))
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// lastEntrySound reports whether the entry a new one would chain from
// is whole: its own Hash recomputes from its own content, and its
// PrevHash is the hash of the entry before it (or empty, when it is the
// chain's first).
//
// Deliberately the last entry rather than the whole chain. What is
// being asked is not "has this log ever been tampered with" — Verify
// answers that, and the export and the audit window are where a person
// is told — but the narrower question Append has to answer before it
// writes: is the thing I am about to extend intact. An entry damaged
// three months ago does not stop a correct successor being computed
// today, and starting a new chain over it would hide the older damage
// behind a fresh one.
func lastEntrySound(entries []Entry) bool {
	n := len(entries)
	if n == 0 {
		return true
	}
	last := entries[n-1]
	if !bytes.Equal(last.Hash, last.ComputeHash()) {
		return false
	}
	if n == 1 {
		return len(last.PrevHash) == 0
	}
	return bytes.Equal(last.PrevHash, entries[n-2].Hash)
}

// unsoundDiscontinuityFrom is discontinuityFrom for a chain that read
// perfectly and whose last entry is not sound. There is no line number
// to give — nothing failed to read — so Line stays 0, which is already
// what it means when the position is not known.
func unsoundDiscontinuityFrom(broken Chain) *Discontinuity {
	d := &Discontinuity{PreviousChain: broken.Number, Reason: BreakUnsound}
	if n := len(broken.Files); n > 0 {
		d.PreviousFile = broken.Files[n-1]
	}
	if n := len(broken.Entries); n > 0 {
		d.LastSequence = broken.Entries[n-1].Sequence
		d.HasLastSequence = true
	}
	return d
}

// discontinuityFrom turns a chain that cannot be continued into the
// record its successor's first entry carries.
func discontinuityFrom(broken Chain) *Discontinuity {
	d := &Discontinuity{
		PreviousChain: broken.Number,
		PreviousFile:  broken.TruncatedFile,
		Line:          broken.TruncatedAtLine,
		Reason:        broken.TruncatedReason,
	}
	if n := len(broken.Entries); n > 0 {
		d.LastSequence = broken.Entries[n-1].Sequence
		d.HasLastSequence = true
	}
	return d
}

// LatestChainFile is the file the next Append would write into, as a
// base name. It is what a caller telling the person "the log continued
// in a new file" names.
func (s *Store) LatestChainFile() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chains, err := s.chains()
	if err != nil {
		return "", err
	}
	if len(chains) == 0 {
		return "", nil
	}
	last := chains[len(chains)-1]
	if n := len(last.Files); n > 0 {
		return last.Files[n-1], nil
	}
	return "", nil
}

// Dir is the directory this store lives in — where a caller telling the
// person where the log continued points them.
func (s *Store) Dir() string { return s.dir }

// All reads every entry in every chain, in order.
//
// Tolerant of a chain that cannot be read to the end: what survives is
// returned, and Chains/Verify are where the break itself is reported. A
// log whose last line was truncated still holds everything written
// before it, and refusing to show any of it — which is what this used to
// do — loses the evidence as surely as deleting it would.
func (s *Store) All() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chains, err := s.chains()
	if err != nil {
		return nil, err
	}
	var all []Entry
	for _, c := range chains {
		all = append(all, c.Entries...)
	}
	return all, nil
}

// Verify walks every chain in the store separately and reports each
// one's own result, plus the discontinuities between them.
//
// Separately, because they are separate chains: a new chain's first
// entry has no PrevHash by construction, so walking every entry in the
// store as one sequence would report the very discontinuity this
// mechanism records as though it were tampering.
func (s *Store) Verify() (StoreVerification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chains, err := s.chains()
	if err != nil {
		return StoreVerification{}, err
	}
	return VerifyChains(chains), nil
}
