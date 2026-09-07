package audit

// Store persists the audit chain to disk (F5 §8.4):
// %LOCALAPPDATA%\Liro\audit\, one file per month, rotated by size at
// 5 MB, never uploaded. The chain itself spans every file in the
// directory — a file boundary is purely a storage detail, not a break
// in what Verify checks, since tampering with an earlier month's file
// must still be detectable.
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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

// Store guards its own directory with a mutex: Append must read the
// last entry and write the new one as one atomic-from-this-process
// operation, or two concurrent batches finishing at once could both
// compute the same PrevHash and silently fork the chain.
type Store struct {
	dir string
	mu  sync.Mutex
}

// NewStore returns a Store rooted at dir, creating it if necessary.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
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
func (s *Store) Append(next Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	b, err := json.Marshal(toJSONEntry(completed))
	if err != nil {
		return Entry{}, err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return Entry{}, err
	}
	return completed, nil
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
