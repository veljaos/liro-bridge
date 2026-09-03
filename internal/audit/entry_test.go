package audit

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// TestCanonicalBytesIsStable pins the exact byte layout CanonicalBytes
// produces — F5 §8.2: "The canonical form must be stable across
// versions... and test that it does not change." The expected bytes
// are built here with encoding/binary directly (not by calling
// CanonicalBytes' own helpers), so this test does not just check the
// implementation against itself: a future refactor that reorders or
// re-encodes a field changes this comparison and fails loudly, rather
// than silently changing what every previously-computed Hash means.
func TestCanonicalBytesIsStable(t *testing.T) {
	ts := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	e := Entry{
		Sequence:      42,
		Timestamp:     ts,
		Thumbprint:    "AABB",
		Application:   "local",
		DocumentCount: 3,
		Outcome:       OutcomeApproved,
		FailureCode:   "",
		IsTestKey:     false,
		PrevHash:      []byte{0xAB, 0xCD},
	}

	var want []byte
	u64 := func(v uint64) {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], v)
		want = append(want, b[:]...)
	}
	str := func(s string) {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(s)))
		want = append(want, l[:]...)
		want = append(want, []byte(s)...)
	}
	bs := func(b []byte) {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(b)))
		want = append(want, l[:]...)
		want = append(want, b...)
	}

	u64(42)                      // Sequence
	u64(uint64(ts.Unix()))       // Timestamp
	str("AABB")                  // Thumbprint
	str("local")                 // Application
	u64(3)                       // DocumentCount
	str(string(OutcomeApproved)) // Outcome
	str("")                      // FailureCode
	want = append(want, 0)       // IsTestKey (false)
	bs([]byte{0xAB, 0xCD})       // PrevHash

	got := e.CanonicalBytes()
	if !bytes.Equal(got, want) {
		t.Fatalf("CanonicalBytes layout changed.\ngot:  % X\nwant: % X", got, want)
	}
}

// TestCanonicalBytesTimestampIsTruncatedToSeconds proves two Entry
// values differing only in sub-second precision hash identically —
// otherwise a round-trip through a storage format with second-level
// precision would silently break every Hash ever computed with the
// in-memory, nanosecond-precision value.
func TestCanonicalBytesTimestampIsTruncatedToSeconds(t *testing.T) {
	base := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	a := Entry{Timestamp: base}
	b := Entry{Timestamp: base.Add(999 * time.Millisecond)}
	if !bytes.Equal(a.CanonicalBytes(), b.CanonicalBytes()) {
		t.Fatal("CanonicalBytes differs for two timestamps within the same second")
	}
}

// TestComputeHashChangesWithEveryField proves every field actually
// participates in the hash — a field that silently dropped out of
// CanonicalBytes would make tampering with it undetectable, defeating
// the entire chain (F5 §8.2).
func TestComputeHashChangesWithEveryField(t *testing.T) {
	base := Entry{
		Sequence:      1,
		Timestamp:     time.Unix(1000, 0),
		Thumbprint:    "AAAA",
		Application:   "local",
		DocumentCount: 1,
		Outcome:       OutcomeApproved,
		PrevHash:      []byte{1, 2, 3},
	}
	baseHash := base.ComputeHash()

	variants := []Entry{
		func() Entry { e := base; e.Sequence = 2; return e }(),
		func() Entry { e := base; e.Timestamp = time.Unix(2000, 0); return e }(),
		func() Entry { e := base; e.Thumbprint = "BBBB"; return e }(),
		func() Entry { e := base; e.Application = "other-app"; return e }(),
		func() Entry { e := base; e.DocumentCount = 2; return e }(),
		func() Entry { e := base; e.Outcome = OutcomeDenied; return e }(),
		func() Entry { e := base; e.FailureCode = errs.CodeCardNotPresent; return e }(),
		func() Entry { e := base; e.IsTestKey = true; return e }(),
		func() Entry { e := base; e.PrevHash = []byte{9, 9, 9}; return e }(),
	}
	for i, v := range variants {
		if bytes.Equal(v.ComputeHash(), baseHash) {
			t.Errorf("variant %d: ComputeHash did not change when a field changed", i)
		}
	}
}
