package worker

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// discardFor is the per-module stderr sink a test does not care about.
func discardFor(string) io.Writer { return io.Discard }

// canned builds a Source over a child that answers whatever a test asks for,
// with a candidate attached the way Sources would attach one. The path is the
// canned one, which is what a Failure will name, so a test can tell which of
// several sources produced a row.
func canned(t *testing.T, a cannedAnswers, vendor string) Source {
	t.Helper()
	path := cannedPath(a)
	w := New(path, io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })
	return Source{
		w:         w,
		candidate: pkcs11.Candidate{Path: path, Vendor: vendor, Origin: pkcs11.OriginKnown},
	}
}

// TestADeadWorkerBecomesARowAndTheListingSurvivesIt is F11 §4 step 2, and the
// requirement F12 §2 ended by handing over: the supervisor turns a dead worker
// into ErrWorkerDied, and this turns that into a row.
//
// The shape being asserted is that a listing with a broken module in it is
// still a listing. An error where a list should be would mean one vendor's
// module misbehaving costs a person the certificates on a different card, which
// is the failure F11 §3 already refused for a module that would not load — this
// is the same refusal for one that loaded and then died.
func TestADeadWorkerBecomesARowAndTheListingSurvivesIt(t *testing.T) {
	good := canned(t, cannedAnswers{}, "Good Vendor")
	// DieOnRequest is one-based: this child ends instead of answering its first
	// request, every time it is respawned, which is what a module that kills its
	// host looks like from the parent's side (D-272, D-296).
	dead := canned(t, cannedAnswers{DieOnRequest: 1}, "Dying Vendor")
	alsoGood := canned(t, cannedAnswers{}, "Another Vendor")

	certs, failures := ListAll(context.Background(), []Source{good, dead, alsoGood})

	if len(failures) != 1 {
		t.Fatalf("got %d failures, want exactly 1 (the dead worker)", len(failures))
	}
	f := failures[0]
	if !errors.Is(f.Err, ErrWorkerDied) {
		t.Errorf("the failure carries %v, want an error that is ErrWorkerDied", f.Err)
	}
	if f.Candidate.Path != dead.candidate.Path {
		t.Errorf("the failure names module %q, want the one that died (%q)",
			f.Candidate.Path, dead.candidate.Path)
	}
	if f.Candidate.Vendor != "Dying Vendor" {
		t.Errorf("the failure names vendor %q, want %q", f.Candidate.Vendor, "Dying Vendor")
	}
	// Failure.Error() is what a log line or a report shows, and the module path
	// is the whole reason this type exists rather than a bare error.
	if !strings.Contains(f.Error(), dead.candidate.Path) {
		t.Errorf("Failure.Error() = %q, which does not name the module", f.Error())
	}

	if len(certs) != 2 {
		t.Fatalf("got %d certificates, want 2 — the two sources that answered", len(certs))
	}
}

// TestListAllDoesNotDeduplicate records a deliberate choice rather than an
// omission, and fails if somebody later makes it tidy.
//
// Both canned children answer with the same certificate, so a deduplicating
// ListAll would return one. It must return two: collapsing one card seen
// through two modules is F11 §4 step 3's job and has to happen across *all*
// backends including Windows CNG. A list deduplicated in one dimension and not
// the other is harder to reason about than one that is not deduplicated at all.
func TestListAllDoesNotDeduplicate(t *testing.T) {
	a := canned(t, cannedAnswers{}, "NetSeT 1.1.0.0")
	b := canned(t, cannedAnswers{}, "NetSeT 1.1.3.3")

	certs, failures := ListAll(context.Background(), []Source{a, b})
	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %v", failures)
	}
	if len(certs) != 2 {
		t.Fatalf("got %d certificates, want 2 — one card through two modules is two "+
			"sightings here, and collapsing them is step 3's job", len(certs))
	}
	if certs[0].Thumbprint != certs[1].Thumbprint {
		t.Fatalf("the two sightings have different thumbprints (%s, %s); this test is "+
			"not measuring what it claims to", certs[0].Thumbprint, certs[1].Thumbprint)
	}
}

// TestACancelledListingIsNotBlamedOnTheModules. A cancellation is the caller
// changing its mind, and recording a Failure against modules that were never
// asked would put this program's own decision into a report about somebody
// else's hardware.
func TestACancelledListingIsNotBlamedOnTheModules(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	certs, failures := ListAll(ctx, []Source{
		canned(t, cannedAnswers{}, "a"),
		canned(t, cannedAnswers{}, "b"),
	})
	if len(failures) != 0 {
		t.Errorf("a cancelled listing produced %d failures: %v", len(failures), failures)
	}
	if len(certs) != 0 {
		t.Errorf("a cancelled listing produced %d certificates, want none", len(certs))
	}
}

// TestAFailureFromASourceWithNoCandidateStillNamesTheModule. NewSource builds a
// Source with no candidate, and a row naming the path with an unknown origin is
// worth more than a row naming nothing.
func TestAFailureFromASourceWithNoCandidateStillNamesTheModule(t *testing.T) {
	path := cannedPath(cannedAnswers{})
	w := New(path, io.Discard)
	t.Cleanup(func() { _ = w.Close(context.Background()) })

	f := NewSource(w, nil).Failure(ErrWorkerDied)
	if f.Candidate.Path != path {
		t.Errorf("Failure names path %q, want %q", f.Candidate.Path, path)
	}
	if !errors.Is(f.Err, ErrWorkerDied) {
		t.Errorf("Failure carries %v", f.Err)
	}
}

// TestIsWorkerFailureTellsABrokenWorkerFromAnAnswerFromTheCard is the
// distinction callers above this package need and must not have to know the
// sentinels to make: a dead worker is something to log with a module path and
// carry past, where CKR_PIN_INCORRECT is an answer to show a person.
func TestIsWorkerFailureTellsABrokenWorkerFromAnAnswerFromTheCard(t *testing.T) {
	for _, err := range []error{ErrWorkerDied, ErrWorkerCannotStart, ErrWorkerAbandoned, ErrUnexpectedPINRequest} {
		if !IsWorkerFailure(err) {
			t.Errorf("IsWorkerFailure(%v) = false, want true", err)
		}
		// Wrapped, because that is how they arrive: the parent attaches the
		// module path on the way out.
		if !IsWorkerFailure(errors.Join(errors.New("pkcs11 worker: C:\\some.dll"), err)) {
			t.Errorf("IsWorkerFailure of a wrapped %v = false, want true", err)
		}
	}
	for _, err := range []error{
		errors.New("pkcs11 worker: C:\\some.dll: CKR_PIN_INCORRECT"),
		pkcs11.ErrNoPINEntry,
		context.Canceled,
	} {
		if IsWorkerFailure(err) {
			t.Errorf("IsWorkerFailure(%v) = true, want false — that is an answer, not a "+
				"broken worker", err)
		}
	}
}

// TestSourcesReportsAConfiguredPathThatIsNotThereAsAFailure is the half of
// Sources that can be measured without a module on the machine.
//
// A configured path is a person's own instruction, so its absence is a failure
// worth reporting — unlike a known path that is simply not installed, which is
// not a failure at all (F11 §3). This asserts that distinction survives the
// worker-backed route, because the whole point of Sources is that it answers
// the same question pkcs11.Sources does.
func TestSourcesReportsAConfiguredPathThatIsNotThereAsAFailure(t *testing.T) {
	const missing = `C:\this\module\does\not\exist\nowhere.dll`
	sources, failures := Sources(missing, nil, discardFor)
	t.Cleanup(func() { _ = CloseAll(context.Background(), sources) })

	var named bool
	for _, f := range failures {
		if f.Candidate.Path == missing {
			named = true
			if f.Candidate.Origin != pkcs11.OriginConfigured {
				t.Errorf("the configured path is reported with origin %v, want %v",
					f.Candidate.Origin, pkcs11.OriginConfigured)
			}
		}
	}
	if !named {
		t.Errorf("a configured module path that is not on disk produced no failure "+
			"naming it; failures were %v", failures)
	}
	for _, s := range sources {
		if s.ModulePath() == missing {
			t.Errorf("a module that is not on disk became a usable source")
		}
	}
}

// hasChild reports whether this worker currently has a child process. It reads
// the field under the mutex that owns it, which is available here because this
// is a test in the same package — a seam in the product for the same purpose
// would be one whose only user is a test (D-100).
func hasChild(w *Worker) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cmd != nil
}

// TestCloseAllClosesEveryWorkerAndNotJustTheFirst. These are child processes,
// and a loop that stopped early would leave them running for the life of the
// agent, holding a vendor module open against a card.
//
// Each worker is started first, deliberately: Worker.Close over a worker that
// never ran returns nil without doing anything, so a test that skipped this
// step would pass against a CloseAll that closed nothing at all.
func TestCloseAllClosesEveryWorkerAndNotJustTheFirst(t *testing.T) {
	sources := []Source{
		canned(t, cannedAnswers{}, "a"),
		canned(t, cannedAnswers{}, "b"),
		canned(t, cannedAnswers{}, "c"),
	}
	for i, s := range sources {
		if _, err := s.List(context.Background()); err != nil {
			t.Fatalf("source %d would not list: %v", i, err)
		}
		if !hasChild(s.w) {
			t.Fatalf("source %d has no child after a successful listing; this test "+
				"cannot measure what it claims to", i)
		}
	}

	if err := CloseAll(context.Background(), sources); err != nil {
		t.Errorf("CloseAll: %v", err)
	}
	for i, s := range sources {
		if hasChild(s.w) {
			t.Errorf("source %d still has a child process after CloseAll", i)
		}
	}
}

// TestEverySourceKeepsTheCandidateItWasBuiltFrom closes the gap a mutation
// found: nothing exercised Sources building a *working* source, because that
// needs a real module on the machine.
//
// The candidate is invisible until something goes wrong — Failure is the only
// thing that reads it — so this is the property that decides whether a worker
// dying a week from now produces a row naming the module and whose instruction
// it was, or a row naming nothing.
func TestEverySourceKeepsTheCandidateItWasBuiltFrom(t *testing.T) {
	want := []pkcs11.Candidate{
		{Path: `C:\Windows\System32\aetpkss1.dll`, Vendor: "A.E.T. Europe B.V.", Origin: pkcs11.OriginKnown},
		{Path: `C:\somewhere\else\personal64.dll`, Vendor: "Nexus", Origin: pkcs11.OriginConfigured},
	}
	got := sourcesFor(want, nil, discardFor)
	if len(got) != len(want) {
		t.Fatalf("got %d sources for %d candidates", len(got), len(want))
	}
	for i, s := range got {
		if s.candidate != want[i] {
			t.Errorf("source %d carries candidate %+v, want %+v", i, s.candidate, want[i])
		}
		if s.ModulePath() != want[i].Path {
			t.Errorf("source %d speaks to %q, want %q", i, s.ModulePath(), want[i].Path)
		}
		// The whole point: a failure from this source names the module and the
		// origin without asking anything, which is what makes it usable when the
		// thing that could have been asked is a process that has died.
		f := s.Failure(ErrWorkerDied)
		if f.Candidate != want[i] {
			t.Errorf("source %d's Failure names %+v, want %+v", i, f.Candidate, want[i])
		}
	}
}

// TestASourceFromDiscoveryCanActuallyCollectAPIN closes the second gap a
// mutation found.
//
// Nothing asserted that the PIN entry survives the trip through discovery, and
// dropping it there is invisible: every listing still works, every module still
// loads, and the only symptom is ErrNoPINEntry at the moment a person has
// chosen a document and approved a signature. That is the worst place in the
// program to discover a wiring mistake.
//
// The candidate's path is a canned one, so this needs no module on the machine
// and still runs the whole exchange through a real child over a real pipe.
func TestASourceFromDiscoveryCanActuallyCollectAPIN(t *testing.T) {
	asked := 0
	entry := func(dst []byte, _ pkcs11.PINRequest) (int, error) {
		asked++
		return copy(dst, askingCardPIN), nil
	}

	sources := sourcesFor([]pkcs11.Candidate{{
		Path:   cannedPath(askingCard(cannedAnswers{Label: "card"})),
		Vendor: "A vendor",
		Origin: pkcs11.OriginKnown,
	}}, entry, discardFor)
	t.Cleanup(func() { _ = CloseAll(context.Background(), sources) })

	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	sess, err := sources[0].Open(context.Background(), "ABCD")
	if err != nil {
		t.Fatalf("Open through a discovered source: %v", err)
	}
	t.Cleanup(func() { _ = sess.Close() })

	if asked != 1 {
		t.Fatalf("the PIN entry was called %d times, want exactly 1. A source built by "+
			"discovery that cannot collect a PIN fails only when somebody is waiting "+
			"to sign.", asked)
	}
}
