package audit

// A hash chain has a property that cuts both ways: because each entry
// carries the hash of the one before it, an entry cannot be removed or
// altered without breaking every entry after it — and a corrupt final
// line can never be appended to. A power cut mid-write, a full disk, a
// killed process: from that moment the chain's last entry cannot be
// read, the next PrevHash cannot be computed, and every signature after
// it goes unrecorded.
//
// Measured (FTEST B-9): a single unparseable line anywhere in the log
// makes every subsequent Append fail forever. Refusing to append onto a
// chain nobody can read is right — a hash chain continued by guessing is
// not a hash chain — but stopping there means the agent goes on signing
// and goes on not recording, which is exactly what SPEC §6.7 says the
// log is for.
//
// So the break is recorded rather than escaped: the old file is left
// exactly as it is (it is evidence up to the point it broke), a new
// chain is started beside it, and that new chain's first entry says what
// happened. That is what append-only logs do, and the reason is that the
// break is itself a fact worth recording rather than a state to get out
// of quietly.

// BreakReason says why a chain could not be continued. It is an
// enumerated value, never prose: Entry's field set is a deliberate
// allow-list (SPEC §6.7 — no file names, no personal names, no content),
// and a free-text field is the thing a future caller eventually puts one
// of those into.
type BreakReason string

const (
	// BreakUnparseable: the previous chain's last file was readable but
	// one of its lines is not a whole entry. A truncated last line is
	// exactly what a power cut or a killed process leaves behind.
	BreakUnparseable BreakReason = "unparseable"

	// BreakUnreachable: the previous chain could not be read at all — a
	// permissions change, a network drive that has gone away. The
	// alternative to starting a new chain here is refusing to sign, and
	// blocking a bookkeeper's afternoon over a log is worse than
	// recording that the log moved.
	BreakUnreachable BreakReason = "unreachable"

	// BreakUnsound: the previous chain read perfectly and its last
	// entry is not sound — its own hash does not recompute, or its
	// PrevHash does not match the entry before it. Appending to it
	// would chain from something already broken.
	//
	// It is only ever looked for after the lock over the audit
	// directory was granted abandoned, which is Windows saying the
	// process that held it died while holding it (D-223). That is the
	// one moment when "the file parses" is not enough to know the
	// chain is whole, and it is the only moment worth paying for the
	// check.
	BreakUnsound BreakReason = "unsound"

	// BreakUnguarded: the lock over the audit directory could not be
	// taken within the time allowed, so another process may be
	// appending to the current chain right now and this one cannot
	// safely read its last entry.
	//
	// Refusing to sign over a log is worse than recording that the log
	// moved — SPEC §6.7's own reasoning for the unreachable case — so
	// the entry goes into a chain of its own, created exclusively, and
	// says why it is there.
	BreakUnguarded BreakReason = "unguarded"
)

// Discontinuity is what a new chain's first entry says about why the
// chain exists at all. Its JSON tags are for the verification report
// the export writes beside the log — the entries' own on-disk shape is
// jsonDiscontinuity in store.go, kept separate for the same reason
// jsonEntry is. It is nil on every other entry, which is what
// makes "tell the person once" true by construction rather than by a
// flag somebody has to remember to clear.
//
// Everything in it is either this program's own generated file name or a
// number. PreviousFile is a base name of the form "2026-09-001.jsonl",
// produced by this package from a date and a rotation number: it is not
// a document's name and cannot become one, so SPEC §18.3's rule about
// file names is not in question here. The path it sits in is
// deliberately not recorded — the chain that broke is in the same
// directory as the chain that records it, and a stored absolute path
// would be wrong the moment a profile moved.
type Discontinuity struct {
	// PreviousChain is the chain number this one follows. 1 is the
	// original chain, which is the only one an audit directory written
	// before this existed can hold. 0 means there was no previous chain
	// at all — an empty directory whose lock could not be taken, which
	// is the one way a chain's *first* entry can carry a record of why
	// it is where it is.
	PreviousChain int `json:"previousChain"`

	// PreviousFile is the base name of the file that could not be
	// continued, or empty when there was no previous chain.
	PreviousFile string `json:"previousFile"`

	// LastSequence is the sequence number of the last entry that could
	// still be read from the previous chain, and HasLastSequence says
	// whether there was one at all — a chain whose very first line is
	// unreadable has none, and 0 is a real sequence number.
	LastSequence    uint64 `json:"lastSequence"`
	HasLastSequence bool   `json:"hasLastSequence"`

	// Line is the 1-based line of PreviousFile at which reading stopped,
	// or 0 when that is not known (the file could not be opened at all).
	Line int `json:"line"`

	// Reason is why, as far as it is known.
	Reason BreakReason `json:"reason"`
}
