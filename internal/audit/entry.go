// Package audit implements the agent's append-only, tamper-evident
// audit log (F5 §8, SPEC §6.7): every batch outcome is recorded with a
// hash chain, so removing or altering an entry breaks every entry after
// it. Never transmitted, never containing the national identity number,
// email addresses, file names, document contents or personal names —
// see D-08x for why personal names are excluded even though they
// appear on the consent screen.
package audit

import (
	"crypto/sha256"
	"encoding/binary"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// Outcome is what happened to a batch.
type Outcome string

const (
	OutcomeApproved Outcome = "approved"
	OutcomeDenied   Outcome = "denied"
	OutcomeFailed   Outcome = "failed"
	OutcomePartial  Outcome = "partial"
)

// Channel is which front door a batch arrived through. F7 §6 requires
// the whole-document path to be recorded distinctly from the hash path,
// and the reason is a real difference rather than bookkeeping: on the
// hash path the agent never possessed the document at all (SPEC §4.3),
// which is a different fact about a signature from the other two.
type Channel string

const (
	// ChannelLocal is a batch a person started at this machine — the
	// window or the command line. It is the empty string so that an
	// entry written before this field existed canonicalises to exactly
	// the bytes it always did, and every audit log already on disk
	// still verifies.
	ChannelLocal Channel = ""

	// ChannelAPIDigests is POST /v2/sign: a paired application sent
	// hashes and the agent never saw a document.
	ChannelAPIDigests Channel = "api-digests"

	// ChannelAPIDocuments is POST /v2/sign/pdf: a paired application
	// sent whole documents.
	ChannelAPIDocuments Channel = "api-documents"
)

// Entry is one audit log record (F5 §8.1). Its field set is a
// deliberate allow-list: nothing that is not here can ever be written,
// which is what makes SPEC §6.7's "never JMBG, email, file names,
// document contents, personal names" true by construction rather than
// by convention.
type Entry struct {
	Sequence uint64
	// Timestamp is truncated to whole seconds before hashing (see
	// CanonicalBytes) so the canonical form is stable across a
	// round-trip through any storage format with second-level
	// precision, and so two runs of a test using time.Now() do not
	// intermittently disagree on sub-second jitter that was never
	// meaningful to an audit record.
	Timestamp time.Time

	// Thumbprint is the signing certificate's SHA-1 thumbprint —
	// identifies the signer without storing their identity (F5 §8.3).
	Thumbprint string

	// Application is the requesting application's name as bound at
	// pairing, or "local" for a CLI-triggered batch (F5 §8.1,
	// consent.ApplicationLocal).
	Application string

	DocumentCount int
	Outcome       Outcome
	FailureCode   errs.Code
	IsTestKey     bool

	// AchievedLevel is the PAdES level the batch actually reached —
	// "b-b", "b-t" or "b-lt", lowest across the batch — or empty when no
	// signature was produced at all (a denied batch, a batch that failed
	// before signing). Task 1 (F5 second-real-run review): a signature
	// saved without a timestamp is B-B, and SPEC §12.8/§18.11 require
	// that to be visible rather than merely not claimed; the audit log
	// is where a user goes to find out what was actually produced weeks
	// later, so "no timestamp" has to be recorded there, not only shown
	// once in a window that has since closed. It is a level string, not
	// a boolean: recording only "was it downgraded" would lose the
	// distinction between B-T and B-LT, which is the same question asked
	// one step further up.
	AchievedLevel string

	// Channel is which front door this batch arrived through (F7 §6).
	// Empty — ChannelLocal — for a batch a person started here, which
	// is every entry written before the protocol existed.
	Channel Channel

	// Discontinuity is set on, and only on, the first entry of a chain
	// that exists because an earlier one could not be continued. It
	// names the file that broke, where, and why (see discontinuity.go).
	//
	// Nil on every other entry, which is what makes "tell the person
	// once, not on every subsequent signature" a property of the data
	// rather than a flag somebody has to remember to clear.
	// Backend is which key source produced the signature:
	// keysource.Source.Name(), so "windows-cng", "pkcs11" or "softtoken".
	// Empty means an entry written before this field existed, and is not
	// a fourth value — every entry any existing log holds is one.
	//
	// It is here because a machine can have the same certificate visible
	// through more than one backend (D-310 measured one card answering
	// identically through CNG and through a vendor's PKCS#11 module),
	// and D-311 decides which one signs. A year from now, "which one
	// actually did" is a question only this field can answer.
	Backend string

	// Module names the PKCS#11 module, and is empty for every other
	// backend. Two builds of one vendor's module five years apart
	// differ by twenty-seven times on one call (D-305, D-271) and are
	// identical in everything CK_INFO reports — same manufacturer, same
	// library description, and a libraryVersion of two bytes that reads
	// the same for both. **The path is the only thing that tells them
	// apart**, which is why this field holds one and not a version.
	//
	// # What it holds, and the price of §6.7
	//
	// For a module found at one of the installation paths this project
	// has measured off real machines, the **whole path**: those are
	// under Program Files or System32 and carry no personal name by
	// construction.
	//
	// For a module found at the path a person configured themselves, the
	// **file name only**. A configured path is a person's own
	// installation and can be anywhere, including under their user
	// profile, where it would carry their name — and SPEC §6.7 says this
	// log never contains personal names.
	//
	// **So the price is stated here rather than discovered later.**
	// Somebody investigating a signature years from now, made through a
	// module somebody had configured by hand, will want to know where
	// that module was and will find only what it was called. That is not
	// an oversight and it is not recoverable from the log: it is what
	// §6.7 costs, paid here, on the one field where the useful value and
	// the forbidden one are the same string.
	//
	// The distinction is pkcs11.Origin's, which the discovery code
	// already draws for its own reason — a configured path failing is a
	// person's instruction failing, where a known path being absent is
	// not a failure at all. Leaning on a distinction that already exists
	// is better than inventing one for this.
	Module string

	Discontinuity *Discontinuity

	// PrevHash is the previous entry's Hash — zero-length for the
	// first entry in the whole chain.
	PrevHash []byte
	Hash     []byte
}

// CanonicalBytes returns e's stable, versionless serialisation — the
// input to its Hash. Field order and encoding are fixed here,
// explicitly, and tested not to change (TestCanonicalBytesIsStable):
// length-prefixing every variable-length field means no field's
// content can ever be interpreted as spilling into the next one, which
// is what makes this safe to hash directly rather than needing a
// collision-resistant delimiter scheme.
func (e Entry) CanonicalBytes() []byte {
	var buf []byte
	buf = appendUint64(buf, e.Sequence)
	buf = appendUint64(buf, uint64(e.Timestamp.Truncate(time.Second).Unix()))
	buf = appendString(buf, e.Thumbprint)
	buf = appendString(buf, e.Application)
	buf = appendUint64(buf, uint64(e.DocumentCount))
	buf = appendString(buf, string(e.Outcome))
	buf = appendString(buf, string(e.FailureCode))
	buf = appendBool(buf, e.IsTestKey)
	buf = appendBytes(buf, e.PrevHash)
	// AchievedLevel is appended last, and only when it is set. That is
	// what lets it be added to an audit log that already has entries in
	// it: an entry written before this field existed has no level, and
	// canonicalises to exactly the bytes it always did, so its stored
	// hash still matches and the chain across the change still verifies.
	// The encoding stays unambiguous because the field is last and
	// length-prefixed like every other — "absent" and "empty" are the
	// same fact here (no signature was produced), never two different
	// ones that could collide.
	if e.AchievedLevel != "" {
		buf = appendString(buf, e.AchievedLevel)
	}
	// The discontinuity is hashed like everything else: it is content,
	// and content that must not be alterable without breaking the chain
	// it starts. It is appended last and behind a one-byte presence
	// marker, so an entry that has one cannot canonicalise to the same
	// bytes as one that does not.
	//
	// Unambiguous by construction: after PrevHash the buffer either ends
	// (no level, no discontinuity), or continues with a 4-byte
	// big-endian length whose first byte is 0 for any level string worth
	// the name, or continues with this marker, which is 1. Two entries
	// differing in these fields cannot produce the same bytes, and an
	// entry written before either field existed canonicalises to exactly
	// what it always did — which is what lets both be added to a log
	// that already has entries in it.
	// The channel is appended behind its own marker, after the
	// discontinuity's, for the same reason the discontinuity is behind
	// one: an entry that has a channel must not be able to
	// canonicalise to the same bytes as one that does not, and an entry
	// written before this field existed must canonicalise to exactly
	// what it always did. ChannelLocal is the empty string precisely so
	// that every local batch — which is every entry any existing log
	// holds — is unchanged by this field's arrival.
	//
	// Markers are numbered in the order the fields were added rather
	// than by any meaning, and each is emitted after the last, so the
	// next optional field is one more marker and nothing else.
	if e.Discontinuity != nil {
		buf = append(buf, discontinuityMarker)
		buf = appendUint64(buf, uint64(e.Discontinuity.PreviousChain))
		buf = appendString(buf, e.Discontinuity.PreviousFile)
		buf = appendUint64(buf, e.Discontinuity.LastSequence)
		buf = appendBool(buf, e.Discontinuity.HasLastSequence)
		buf = appendUint64(buf, uint64(e.Discontinuity.Line))
		buf = appendString(buf, string(e.Discontinuity.Reason))
	}
	if e.Channel != ChannelLocal {
		buf = append(buf, channelMarker)
		buf = appendString(buf, string(e.Channel))
	}
	// The backend and the module go behind their own marker, after the
	// channel's, for the reason each of the three above has its own: an
	// entry that names a backend must not be able to canonicalise to
	// the same bytes as one that does not, and an entry written before
	// this field existed must canonicalise to exactly what it always
	// did. Every entry in every log on disk today has neither, so every
	// one of them is unchanged by this field's arrival.
	//
	// Both are written whenever either is set, and both are
	// length-prefixed, so "pkcs11 with no module" and "no backend with
	// a module" are different bytes rather than an ambiguity nobody
	// thought about.
	if e.Backend != "" || e.Module != "" {
		buf = append(buf, backendMarker)
		buf = appendString(buf, e.Backend)
		buf = appendString(buf, e.Module)
	}
	return buf
}

// discontinuityMarker introduces the optional trailing Discontinuity
// fields in CanonicalBytes. 1 rather than 0 so it cannot be confused
// with the leading byte of AchievedLevel's own length prefix.
const discontinuityMarker byte = 1

// channelMarker introduces the optional trailing Channel field.
const channelMarker byte = 2

// backendMarker introduces the optional trailing Backend and Module
// fields. Markers are numbered in the order the fields were added
// rather than by any meaning, and each is emitted after the last, so
// the next optional field is one more marker and nothing else.
const backendMarker byte = 3

// ComputeHash returns SHA-256 of e.CanonicalBytes() — e's own Hash and
// PrevHash fields are irrelevant to the input except that PrevHash is
// itself part of CanonicalBytes, which is the chaining mechanism: e's
// hash depends on the previous entry's hash, so altering any earlier
// entry changes every hash after it (F5 §8.2).
func (e Entry) ComputeHash() []byte {
	sum := sha256.Sum256(e.CanonicalBytes())
	return sum[:]
}

func appendUint64(buf []byte, v uint64) []byte {
	var tmp [8]byte
	binary.BigEndian.PutUint64(tmp[:], v)
	return append(buf, tmp[:]...)
}

func appendBool(buf []byte, v bool) []byte {
	if v {
		return append(buf, 1)
	}
	return append(buf, 0)
}

// appendBytes length-prefixes b with a 4-byte big-endian length before
// its content — the general form appendString also uses, so a field
// boundary can never be confused with content inside it.
func appendBytes(buf []byte, b []byte) []byte {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], uint32(len(b)))
	buf = append(buf, tmp[:]...)
	return append(buf, b...)
}

func appendString(buf []byte, s string) []byte {
	return appendBytes(buf, []byte(s))
}
