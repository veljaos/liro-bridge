package tsl

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// errNoNetworkInThisTest is handed to fixedFetcher so a Refresh that is
// never meant to happen fails loudly rather than reaching the Ministry.
var errNoNetworkInThisTest = errors.New("this test must not fetch")

// sequenceElement finds the sequence number as it is literally written
// in the XML, without going through this package's own parser. The point
// of the test below is to compare independent readings, so it must not
// share Parse's view of the document.
var sequenceElement = regexp.MustCompile(`<TSLSequenceNumber>\s*([0-9]+)\s*</TSLSequenceNumber>`)

// TestSeedSequenceAgreesWithWhatTheCodeReports is F6 §0a's required
// check. Four independent readings of one number must agree:
//
//  1. the seedSequence constant, which a human read off the document
//  2. the <TSLSequenceNumber> element in the embedded bytes, by regexp
//  3. Parse's own Sequence field
//  4. Provenance.Sequence, which is the number `liro-bridge certs`
//     actually prints
//
// The reported symptom (F6 §0a) was a test log saying currentSequence=37
// while certs said 36. It turned out that 37 was a value
// TestRefreshRejectsRollback writes into a store by hand to make the
// real, validly signed sequence-36 list read as a rollback — a fabricated
// number in a passing test's log line, not a list. This test is what
// makes that distinction checkable rather than argued: if the embedded
// seed and the number the agent reports ever genuinely disagree, this
// fails, and it cannot be satisfied by a value that exists only inside a
// test's own fixture.
func TestSeedSequenceAgreesWithWhatTheCodeReports(t *testing.T) {
	m := sequenceElement.FindSubmatch(seedXML)
	if m == nil {
		t.Fatal("no <TSLSequenceNumber> element in the embedded seed")
	}
	inDocument, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("parsing <TSLSequenceNumber> %q: %v", m[1], err)
	}
	if inDocument != seedSequence {
		t.Errorf("the embedded seed says sequence %d; seedSequence says %d", inDocument, seedSequence)
	}

	parsed := mustParseSeed(t)
	if parsed.Sequence != seedSequence {
		t.Errorf("Parse reports sequence %d; seedSequence says %d", parsed.Sequence, seedSequence)
	}

	// The Store is what every surface that prints a sequence number goes
	// through, so it is the reading that has to match the document.
	// A temp directory with no cache in it forces the embedded seed.
	store, err := NewFileStore(filepath.Join(t.TempDir(), "tsl-cache.xml"), DefaultURL, fixedFetcher(nil, errNoNetworkInThisTest))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	list, prov, err := store.Current(context.Background())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if prov.Source != SourceEmbedded {
		t.Fatalf("Source = %v, want SourceEmbedded — this test must exercise the seed, not a cache", prov.Source)
	}
	if prov.Sequence != seedSequence {
		t.Errorf("Provenance.Sequence = %d; the embedded seed says %d", prov.Sequence, seedSequence)
	}
	if list.Sequence != seedSequence {
		t.Errorf("Current list Sequence = %d; the embedded seed says %d", list.Sequence, seedSequence)
	}
}

// TestSeedIsTheMinistrysOwnBytes pins the two properties that went wrong
// (D-107): the Ministry publishes the list with CRLF line endings and a
// leading BOM, the seed keeps the former and drops the latter, and
// `text=auto` normalising the CRLF away is what made the digest stop
// matching the published one. The digest check alone would catch that,
// but it would not say why, and the next person to see it fail deserves
// to be told which of the two it is.
func TestSeedIsTheMinistrysOwnBytes(t *testing.T) {
	if bytes.HasPrefix(seedXML, []byte{0xEF, 0xBB, 0xBF}) {
		t.Error("the embedded seed still carries the server's UTF-8 BOM; it is stripped before committing")
	}
	if bytes.Contains(seedXML, []byte("\n")) && !bytes.Contains(seedXML, []byte("\r\n")) {
		t.Error("the embedded seed's CRLF line endings have been normalised to LF; " +
			"check .gitattributes marks it -text (D-107)")
	}
}
