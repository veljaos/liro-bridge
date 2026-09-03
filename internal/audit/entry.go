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
	return buf
}

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
