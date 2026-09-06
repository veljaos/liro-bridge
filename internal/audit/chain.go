package audit

import (
	"bytes"
	"time"
)

// AppendEntry fills in Sequence, PrevHash and Hash for a new entry
// following prev (nil for the very first entry in the whole chain) and
// returns the completed Entry. Every other field must already be set
// by the caller.
func AppendEntry(prev *Entry, next Entry) Entry {
	if prev == nil {
		next.Sequence = 0
		next.PrevHash = nil
	} else {
		next.Sequence = prev.Sequence + 1
		next.PrevHash = prev.Hash
	}
	next.Hash = next.ComputeHash()
	return next
}

// VerifyResult is the outcome of walking a chain.
type VerifyResult struct {
	// OK is true when every entry's Hash matches its own recomputed
	// CanonicalBytes hash and every entry's PrevHash matches the
	// previous entry's Hash.
	OK bool

	// BrokenAt is the index (into the slice passed to Verify) of the
	// first entry that fails either check, when OK is false. F5 §8.2
	// requires the break's position, not just whether one exists.
	BrokenAt int
}

// Verify walks entries in order and reports the first break (F5 §8.2:
// "Provide `audit verify` which walks the chain and reports the first
// break"). An empty chain is trivially OK.
//
// Two independent things are checked per entry, either of which can
// break the chain: its own Hash no longer matches its CanonicalBytes
// (the entry itself was altered), or its PrevHash no longer matches the
// previous entry's Hash (an entry was altered, inserted or removed
// somewhere earlier, which cascades — even if this entry's own Hash is
// internally self-consistent with its own now-different PrevHash, the
// PrevHash itself is wrong).
func Verify(entries []Entry) VerifyResult {
	var prevHash []byte
	for i, e := range entries {
		if !bytes.Equal(e.PrevHash, prevHash) {
			return VerifyResult{OK: false, BrokenAt: i}
		}
		if !bytes.Equal(e.Hash, e.ComputeHash()) {
			return VerifyResult{OK: false, BrokenAt: i}
		}
		prevHash = e.Hash
	}
	return VerifyResult{OK: true, BrokenAt: -1}
}

// ChainVerification is one chain's own result, plus what is known about
// how it began and — when it could not be read to the end — where it
// stopped.
type ChainVerification struct {
	// Chain is the chain's number: 1 for the original, rising by one for
	// each chain started after a break.
	Chain int `json:"chain"`

	// Files are the chain's files' base names, in chain order.
	Files []string `json:"files"`

	// EntryCount is how many entries this chain holds.
	EntryCount int `json:"entryCount"`

	// FirstAt and LastAt bracket the chain in time, so a report can say
	// when a break happened rather than only that one did. Zero for an
	// empty chain.
	FirstAt time.Time `json:"firstAt,omitzero"`
	LastAt  time.Time `json:"lastAt,omitzero"`

	// Result is this chain's own hash-chain walk.
	Result VerifyResult `json:"result"`

	// Discontinuity is what this chain's first entry says about why the
	// chain exists at all. Nil for the original chain.
	Discontinuity *Discontinuity `json:"discontinuity,omitempty"`

	// TruncatedFile, TruncatedAtLine and TruncatedReason say where
	// reading this chain stopped short, when it did.
	TruncatedFile   string      `json:"truncatedFile,omitempty"`
	TruncatedAtLine int         `json:"truncatedAtLine,omitempty"`
	TruncatedReason BreakReason `json:"truncatedReason,omitempty"`
}

// StoreVerification is every chain in a store, verified separately.
//
// Separately is the whole point: a chain that was started because an
// earlier one broke has no PrevHash on its first entry by construction,
// so walking every entry in the store as one sequence would report the
// discontinuity itself as tampering — which is the opposite of what a
// person needs to be told.
type StoreVerification struct {
	Chains []ChainVerification `json:"chains"`

	// OK is true when every chain walks cleanly and every one could be
	// read to its end. A discontinuity between two intact chains does
	// not make this false: the break is recorded, which is what
	// "intact" means for an append-only log that survived one.
	OK bool `json:"ok"`

	// BrokenAt is the position, counted across every chain's entries in
	// order — which is the order Export writes them, one per line — of
	// the first entry that fails its own chain's walk, or -1 when none
	// does. It exists so a message can point at a line in the exported
	// file a person actually has in front of them.
	BrokenAt int `json:"brokenAt"`

	// EntryCount is every chain's entries added together.
	EntryCount int `json:"entryCount"`
}

// Discontinuities returns every chain that began because an earlier one
// could not be continued, in order. It is what a report naming the
// breaks iterates over.
func (v StoreVerification) Discontinuities() []ChainVerification {
	var out []ChainVerification
	for _, c := range v.Chains {
		if c.Discontinuity != nil {
			out = append(out, c)
		}
	}
	return out
}

// VerifyChains walks each chain on its own and assembles the whole
// store's result.
func VerifyChains(chains []Chain) StoreVerification {
	out := StoreVerification{OK: true, BrokenAt: -1}
	offset := 0
	for _, c := range chains {
		result := Verify(c.Entries)
		cv := ChainVerification{
			Chain:           c.Number,
			Files:           c.Files,
			EntryCount:      len(c.Entries),
			Result:          result,
			Discontinuity:   c.Discontinuity,
			TruncatedFile:   c.TruncatedFile,
			TruncatedAtLine: c.TruncatedAtLine,
			TruncatedReason: c.TruncatedReason,
		}
		if n := len(c.Entries); n > 0 {
			cv.FirstAt = c.Entries[0].Timestamp
			cv.LastAt = c.Entries[n-1].Timestamp
		}
		if !result.OK {
			out.OK = false
			if out.BrokenAt < 0 {
				out.BrokenAt = offset + result.BrokenAt
			}
		}
		if c.Truncated() {
			// A chain that cannot be read to its end is not intact,
			// whatever the entries before the break say. The break is
			// recorded — the next chain's first entry names it — but the
			// file itself is damaged, and a verification that called
			// that "OK" would be answering a different question.
			out.OK = false
		}
		offset += len(c.Entries)
		out.EntryCount += len(c.Entries)
		out.Chains = append(out.Chains, cv)
	}
	return out
}
