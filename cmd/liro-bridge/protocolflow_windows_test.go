//go:build windows

package main

// A request that arrived over the protocol, driven through the same
// window machinery a person's own batch goes through (F7 §5, §6, §9).
//
// Everything here runs without a card and without a WebView2 window:
// the flow's decisions — which steps there are, which certificates are
// offered, what each document's outcome is, what the audit log records,
// what happens when nobody answers — are plain Go, exactly as F5 §10
// required of the consent screen's own data. The window itself has its
// own tests, and the card has the soft token.

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// ---- a window that records rather than draws ------------------------

// recordingWindow is a ui.Window that keeps what it was posted. It is
// not a stand-in for the real window — the real one has its own tests,
// which drive a real WebView2 instance — it is what lets the flow's own
// decisions be exercised without one.
type recordingWindow struct {
	mu       sync.Mutex
	payloads []map[string]any
	closed   bool
}

func (w *recordingWindow) PostJSON(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.payloads = append(w.payloads, decoded)
	return nil
}

func (w *recordingWindow) Eval(string) (string, error) { return `"{}"`, nil }
func (w *recordingWindow) Navigate(string) error       { return nil }
func (w *recordingWindow) Resize(int, int) error       { return nil }
func (w *recordingWindow) Handle() uintptr             { return 0 }
func (w *recordingWindow) Close() error                { w.mu.Lock(); w.closed = true; w.mu.Unlock(); return nil }

// posted returns every payload of a given type.
func (w *recordingWindow) posted(kind string) []map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []map[string]any
	for _, p := range w.payloads {
		if p["type"] == kind {
			out = append(out, p)
		}
	}
	return out
}

// ---- a batch to drive ----------------------------------------------

// protocolWindowFor builds a mainWindow in the state runProtocolFlow
// leaves it in, without opening anything.
func protocolWindowFor(t *testing.T, req api.SignRequest) *mainWindow {
	t.Helper()
	cfg := config.Default()
	m := newMainWindow(cfg, "sr-Latn")
	m.documentsSupplied = true
	m.force = true
	m.hashesOnly = req.Kind == api.SignDigests
	m.suppliedStamp = req.Stamp
	m.applySuppliedStamp()
	m.remote = &remoteBatch{
		req:           req,
		job:           testJob(t, len(req.Digests)),
		outcomes:      make([]api.SignOutcome, len(req.Digests)),
		stopCountdown: make(chan struct{}),
		expired:       make(chan struct{}),
	}
	for i, digest := range req.Digests {
		label := protocolLabel(req, i)
		m.inputs = append(m.inputs, interactiveInput{digest: digest, label: label})
		m.remoteItems = append(m.remoteItems, jobs.Item{DisplayName: label, State: jobs.StateWaiting})
	}
	m.win = &recordingWindow{}
	return m
}

func testJob(t *testing.T, total int) *jobs.Job {
	t.Helper()
	job, err := jobs.NewRegistry(nil).Submit("app", total, "fp")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	return job
}

// digestRequest is a hash-path request over n documents.
func digestRequest(n int, thumbprint string) api.SignRequest {
	req := api.SignRequest{
		Application: "Knjigovodstvo doo",
		Kind:        api.SignDigests,
		Thumbprint:  thumbprint,
	}
	for i := 0; i < n; i++ {
		sum := sha256.Sum256([]byte{byte(i)})
		req.Digests = append(req.Digests, sum[:])
		req.Labels = append(req.Labels, "ugovor-"+itoaTest(i)+".pdf")
	}
	return req
}

// documentRequest is a whole-document request over n copies of the
// committed blank fixture.
func documentRequest(t *testing.T, n int) api.SignRequest {
	t.Helper()
	blank, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Skipf("the blank fixture is not available: %v", err)
	}
	req := api.SignRequest{Application: "Knjigovodstvo doo", Kind: api.SignDocuments}
	for i := 0; i < n; i++ {
		req.Documents = append(req.Documents, api.Document{
			Name:    "ugovor-" + itoaTest(i) + ".pdf",
			Content: blank,
		})
		req.Digests = append(req.Digests, consent.DigestOf(blank))
		req.Labels = append(req.Labels, "ugovor-"+itoaTest(i)+".pdf")
	}
	return req
}

// ---- the flow's shape ------------------------------------------------

// TestAHashRequestNeverAsksAboutAStamp is F7 §5's own shape: on the
// hash path the caller built the PDF and the CMS itself, so there is no
// page for this agent to draw on and nothing to ask about.
func TestAHashRequestNeverAsksAboutAStamp(t *testing.T) {
	m := protocolWindowFor(t, digestRequest(2, "AABB"))
	steps := m.stepsFor()
	if len(steps) != 1 || steps[0] != stepCertificate {
		t.Fatalf("steps = %v, want the approval and nothing else", steps)
	}
	if m.asksHowToSign() {
		t.Fatal("a hash-only batch asks how to sign")
	}
	if m.headerFor(stepCertificate) != nil {
		t.Fatal("a one-step run carries a step header")
	}
}

// TestASuppliedStampMeansTheApprovalAndNothingElse is F7 §6's
// one-window, one-click case: an answer the caller gave is an answer
// the person is not asked again.
func TestASuppliedStampMeansTheApprovalAndNothingElse(t *testing.T) {
	req := documentRequest(t, 1)
	req.Stamp = &consent.StampChoice{Visible: true, Position: consent.StampPositionTopLeft}
	m := protocolWindowFor(t, req)

	steps := m.stepsFor()
	if len(steps) != 1 || steps[0] != stepCertificate {
		t.Fatalf("steps = %v, want the approval and nothing else", steps)
	}
	if m.cfg.StampPosition != consent.StampPositionTopLeft || !m.cfg.VisibleStamp {
		t.Fatalf("the supplied stamp did not reach the configuration: %+v", m.cfg.StampPosition)
	}
}

// TestADocumentRequestWithNoStampStillAsks is the other half: a caller
// with no opinion leaves the question with the person, exactly as a
// local batch does.
func TestADocumentRequestWithNoStampStillAsks(t *testing.T) {
	m := protocolWindowFor(t, documentRequest(t, 1))
	steps := m.stepsFor()
	if len(steps) != 2 || steps[1] != stepMethod {
		t.Fatalf("steps = %v, want the approval and the method", steps)
	}
}

// TestTheConsentScreenNamesTheApplicationBoundAtPairing is SPEC §6.6
// and F7 §9: the name above a signature is the one a person approved,
// never one supplied in the request.
func TestTheConsentScreenNamesTheApplicationBoundAtPairing(t *testing.T) {
	m := protocolWindowFor(t, digestRequest(1, "AABB"))
	if got := m.applicationName(); got != "Knjigovodstvo doo" {
		t.Fatalf("the consent screen names %q", got)
	}

	local := newMainWindow(config.Default(), "sr-Latn")
	if got := local.applicationName(); got != consent.ApplicationLocal {
		t.Fatalf("a local batch names %q, want %q", got, consent.ApplicationLocal)
	}
}

// TestTheLabelsTheCallerSentAreWhatTheWindowShows covers the other
// half of what the consent screen says about a protocol batch: how many
// documents, and which.
func TestTheLabelsTheCallerSentAreWhatTheWindowShows(t *testing.T) {
	m := protocolWindowFor(t, digestRequest(2, "AABB"))
	names := m.fileNames()
	if len(names) != 2 || names[0] != "ugovor-0.pdf" || names[1] != "ugovor-1.pdf" {
		t.Fatalf("the window shows %v", names)
	}
	if m.documentCount() != 2 {
		t.Fatalf("the window counts %d documents", m.documentCount())
	}

	// A caller that sent no labels still gets rows a person can count,
	// rather than two blank lines.
	req := digestRequest(2, "AABB")
	req.Labels = nil
	unlabelled := protocolWindowFor(t, req)
	for i, name := range unlabelled.fileNames() {
		if name == "" {
			t.Fatalf("document %d has no name on the consent screen", i)
		}
	}
}

// ---- which certificate ----------------------------------------------

func TestARequestedCertificateIsTheOnlyOneOffered(t *testing.T) {
	certs := []classify.Info{
		{Thumbprint: "AAAA", Subject: classify.Subject{DisplayName: "Ana"}},
		{Thumbprint: "BBBB", Subject: classify.Subject{DisplayName: "Bojan"}},
	}
	offered, err := certificatesOfferedFor(certs, "BBBB")
	if err != nil {
		t.Fatalf("certificatesOfferedFor: %v", err)
	}
	if len(offered) != 1 || offered[0].Thumbprint != "BBBB" {
		t.Fatalf("offered %v, want only the one the caller named", offered)
	}
}

func TestWithNoCertificateNamedThePersonChoosesAsTheyDoLocally(t *testing.T) {
	certs := []classify.Info{{Thumbprint: "AAAA"}, {Thumbprint: "BBBB"}}
	offered, err := certificatesOfferedFor(certs, "")
	if err != nil {
		t.Fatalf("certificatesOfferedFor: %v", err)
	}
	if len(offered) != 2 {
		t.Fatalf("offered %d certificates, want every one a person would see", len(offered))
	}
}

// TestARequestForACertificateThatIsNotHereOpensNoWindow: asking a
// person to approve a batch nothing on this machine can sign is not a
// question worth putting on a screen.
func TestARequestForACertificateThatIsNotHereOpensNoWindow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		certs []classify.Info
		want  string
	}{
		{"the named one is absent", []classify.Info{{Thumbprint: "AAAA"}}, "CCCC"},
		{"there are none at all", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := certificatesOfferedFor(tc.certs, tc.want)
			if err == nil {
				t.Fatal("a request for a certificate that is not here was accepted")
			}
			if got := codeOfInteractive(err); got != errs.CodeCertNotFound {
				t.Fatalf("code is %q, want CERT_NOT_FOUND", got)
			}
		})
	}
}

// ---- signing ---------------------------------------------------------

// TestAHashBatchIsSignedAndHandedBack drives the real run — the same
// runBatch a person's own batch goes through — over a protocol request,
// and checks the signatures against the signer's own public key.
//
// Nothing is written anywhere: the whole point of the hash path is that
// the agent never possesses a document (SPEC §4.3), and the output is a
// response rather than a file.
func TestAHashBatchIsSignedAndHandedBack(t *testing.T) {
	req := digestRequest(3, "AABB")
	m := protocolWindowFor(t, req)
	session := newStampSession(t)
	m.auditStore = tempAuditStore(t)

	m.runBatch(context.Background(), consentDecision{
		approved:   true,
		thumbprint: "AABB",
		session:    session,
		level:      pades.LevelBB,
		allowBB:    true,
		cfg:        m.cfg,
	})

	result := m.remote.result()
	if result.Code != "" {
		t.Fatalf("the batch reported %q", result.Code)
	}
	if len(result.Outcomes) != 3 {
		t.Fatalf("%d outcomes, want one per digest sent", len(result.Outcomes))
	}
	cert, err := x509.ParseCertificate(session.Certificate().DER)
	if err != nil {
		t.Fatalf("parsing the signer certificate: %v", err)
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("the signer's key is %T", cert.PublicKey)
	}
	for i, o := range result.Outcomes {
		if o.Code != "" {
			t.Fatalf("outcome %d failed with %q", i, o.Code)
		}
		if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, req.Digests[i], o.Signature); err != nil {
			t.Fatalf("signature %d does not verify against the digest that was sent: %v", i, err)
		}
	}
}

// TestADocumentBatchIsSignedInMemory is F7 §6's path: the document
// arrives, is signed, and goes back — and nothing lands on disk.
func TestADocumentBatchIsSignedInMemory(t *testing.T) {
	req := documentRequest(t, 2)
	m := protocolWindowFor(t, req)
	m.auditStore = tempAuditStore(t)
	// A folder the run could write into if it were going to, so that
	// "nothing was written" is a measurement rather than an assumption.
	outDir := t.TempDir()
	m.cfg.OutputFolder = outDir

	m.runBatch(context.Background(), consentDecision{
		approved:   true,
		thumbprint: "AABB",
		session:    newStampSession(t),
		level:      pades.LevelBB,
		allowBB:    true,
		cfg:        m.cfg,
	})

	result := m.remote.result()
	if result.Code != "" {
		t.Fatalf("the batch reported %q", result.Code)
	}
	for i, o := range result.Outcomes {
		if o.Code != "" {
			t.Fatalf("document %d failed with %q", i, o.Code)
		}
		if !strings.HasPrefix(string(o.Document), "%PDF") {
			t.Fatalf("document %d does not come back as a PDF", i)
		}
		if len(o.Document) <= len(req.Documents[i].Content) {
			t.Fatalf("document %d came back no larger than it went in", i)
		}
		if o.AchievedLevel != string(pades.LevelBB) {
			t.Fatalf("document %d reports level %q, want the one it reached", i, o.AchievedLevel)
		}
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a protocol batch wrote %d files; its output is a response", len(entries))
	}
}

// TestEveryDocumentTheRunDidNotReachGetsACode is what stops a caller
// being handed an empty entry where a signature should be. A card
// removed at document two ends the batch (F6 §3, F2 §5.3), and the
// documents after it were never attempted — which is an answer, not a
// blank.
func TestEveryDocumentTheRunDidNotReachGetsACode(t *testing.T) {
	m := protocolWindowFor(t, digestRequest(4, "AABB"))
	m.auditStore = tempAuditStore(t)
	session := &failingSession{inner: newStampSession(t), failAt: 1, code: errs.CodeCardNotPresent}

	m.runBatch(context.Background(), consentDecision{
		approved: true, thumbprint: "AABB", session: session,
		level: pades.LevelBB, allowBB: true, cfg: m.cfg,
	})

	result := m.remote.result()
	if len(result.Outcomes) != 4 {
		t.Fatalf("%d outcomes, want one per digest sent", len(result.Outcomes))
	}
	if len(result.Outcomes[0].Signature) == 0 {
		t.Fatal("the first document, which was signed, has no signature")
	}
	for i := 1; i < 4; i++ {
		if result.Outcomes[i].Code == "" {
			t.Fatalf("document %d came back with neither a signature nor a reason", i)
		}
	}
	if result.Outcomes[1].Code != errs.CodeCardNotPresent {
		t.Fatalf("the document that failed reports %q", result.Outcomes[1].Code)
	}
	if result.Outcomes[3].Code != errs.CodeCardNotPresent {
		t.Fatalf("a document the run never reached reports %q, want the reason the batch ended",
			result.Outcomes[3].Code)
	}
}

// ---- what the audit log records --------------------------------------

func TestTheAuditEntryRecordsWhichPathWasUsed(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  api.SignRequest
		want audit.Channel
	}{
		{"hashes", digestRequest(1, "AABB"), audit.ChannelAPIDigests},
		{"documents", documentRequest(t, 1), audit.ChannelAPIDocuments},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			m := protocolWindowFor(t, tc.req)
			m.auditStore = auditStoreIn(t, dir)
			m.selected = "AABB"
			m.deny()

			entries := auditEntriesIn(t, dir)
			if len(entries) != 1 {
				t.Fatalf("%d audit entries, want one", len(entries))
			}
			if entries[0].Channel != tc.want {
				t.Fatalf("the entry records channel %q, want %q", entries[0].Channel, tc.want)
			}
			if entries[0].Application != "Knjigovodstvo doo" {
				t.Fatalf("the entry records application %q", entries[0].Application)
			}
			if entries[0].Outcome != audit.OutcomeDenied {
				t.Fatalf("the entry records outcome %q", entries[0].Outcome)
			}
		})
	}
}

func TestALocalBatchStillRecordsNoChannelAtAll(t *testing.T) {
	dir := t.TempDir()
	m := newMainWindow(config.Default(), "sr-Latn")
	m.auditStore = auditStoreIn(t, dir)
	m.step = stepCertificate
	m.deny()

	entries := auditEntriesIn(t, dir)
	if len(entries) != 1 {
		t.Fatalf("%d audit entries, want one", len(entries))
	}
	if entries[0].Channel != audit.ChannelLocal {
		t.Fatalf("a local batch records channel %q", entries[0].Channel)
	}
	if entries[0].Application != consent.ApplicationLocal {
		t.Fatalf("a local batch records application %q", entries[0].Application)
	}
}

func TestARefusalIsRecordedOnceHoweverManyWaysOutItTook(t *testing.T) {
	dir := t.TempDir()
	m := protocolWindowFor(t, digestRequest(1, "AABB"))
	m.auditStore = auditStoreIn(t, dir)
	m.deny()
	m.deny()
	m.finishRemote()

	if entries := auditEntriesIn(t, dir); len(entries) != 1 {
		t.Fatalf("%d audit entries, want one refusal", len(entries))
	}
}

// ---- the consent deadline -------------------------------------------

// TestNobodyAnsweringIsAConsentTimeout is F7 §7.4: the window expires,
// the caller is told CONSENT_TIMEOUT rather than that it was refused,
// and it may submit again.
func TestNobodyAnsweringIsAConsentTimeout(t *testing.T) {
	dir := t.TempDir()
	m := protocolWindowFor(t, digestRequest(1, "AABB"))
	m.auditStore = auditStoreIn(t, dir)
	// The deadline rather than the constant, so this waits a second
	// instead of two minutes and still exercises the same code.
	m.remote.deadline = time.Now().Add(200 * time.Millisecond)

	go m.watchConsentDeadline()
	select {
	case <-m.remote.expired:
	case <-time.After(30 * time.Second):
		t.Fatal("the consent deadline never fired")
	}

	m.consentExpired()
	m.finishRemote()
	if got := m.remote.result().Code; got != errs.CodeConsentTimeout {
		t.Fatalf("the caller is told %q, want CONSENT_TIMEOUT", got)
	}
	// A refusal nobody made is still a refusal in the log: SPEC §6.7
	// wants what did not happen as much as what did.
	if entries := auditEntriesIn(t, dir); len(entries) != 1 {
		t.Fatalf("%d audit entries, want the expiry recorded", len(entries))
	}
}

// TestTheCallerAndTheWindowAreBothToldHowLongIsLeft is the countdown's
// two audiences: the person sees a number in the last thirty seconds,
// and every awaiting_consent event carries the remaining time so an ERP
// can show its own.
func TestTheCallerAndTheWindowAreBothToldHowLongIsLeft(t *testing.T) {
	m := protocolWindowFor(t, digestRequest(1, "AABB"))
	win := m.win.(*recordingWindow)
	m.remote.deadline = time.Now().Add(2 * time.Second)

	seen := make(chan jobs.Update, 8)
	go func() {
		_ = m.remote.job.Follow(context.Background(), func(u jobs.Update) error {
			select {
			case seen <- u:
			default:
			}
			return nil
		})
	}()
	go m.watchConsentDeadline()
	defer m.remote.endCountdown()

	deadline := time.After(30 * time.Second)
	for {
		var u jobs.Update
		select {
		case u = <-seen:
		case <-deadline:
			t.Fatal("no awaiting_consent update carried the remaining time")
		}
		if u.State != jobs.JobAwaitingConsent || u.RemainingConsent <= 0 {
			continue
		}
		if u.RemainingConsent > ConsentTimeout {
			t.Fatalf("the caller was told %s remained, which is longer than the whole window", u.RemainingConsent)
		}
		break
	}

	// And the person: a number on the screen, in their own language,
	// resolved in Go — the page owns no clock.
	countdownDeadline := time.After(30 * time.Second)
	for {
		if posts := win.posted("countdown"); len(posts) > 0 {
			text, _ := posts[0]["text"].(string)
			if text == "" {
				t.Fatal("the countdown carries no words")
			}
			if !strings.Contains(text, "s") {
				t.Fatalf("the countdown reads %q", text)
			}
			break
		}
		select {
		case <-countdownDeadline:
			t.Fatal("the window was never told how long was left")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// TestApprovingStopsTheClock: once the person has answered, a
// countdown that kept running would be counting down to nothing.
func TestApprovingStopsTheClock(t *testing.T) {
	m := protocolWindowFor(t, digestRequest(1, "AABB"))
	m.remote.deadline = time.Now().Add(150 * time.Millisecond)

	done := make(chan struct{})
	go func() { m.watchConsentDeadline(); close(done) }()
	m.remote.endCountdown()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the countdown kept running after the person answered")
	}
	select {
	case <-m.remote.expired:
		t.Fatal("a batch whose person answered was reported as expired")
	default:
	}
}

// ---- helpers ---------------------------------------------------------

// failingSession is a session that signs the first failAt digests and
// then fails with code, which is what a card removed mid-batch looks
// like from above.
type failingSession struct {
	inner  keysource.Session
	failAt int
	code   errs.Code
	n      int
}

func (s *failingSession) SignDigest(ctx context.Context, alg keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	if s.n >= s.failAt {
		return nil, errs.New(s.code, nil)
	}
	s.n++
	return s.inner.SignDigest(ctx, alg, digest)
}

func (s *failingSession) Certificate() keysource.Certificate { return s.inner.Certificate() }
func (s *failingSession) Chain() [][]byte                    { return s.inner.Chain() }
func (s *failingSession) Close() error                       { return s.inner.Close() }

func tempAuditStore(t *testing.T) func() (*audit.Store, error) {
	t.Helper()
	return auditStoreIn(t, t.TempDir())
}

func auditStoreIn(t *testing.T, dir string) func() (*audit.Store, error) {
	t.Helper()
	return func() (*audit.Store, error) { return audit.NewStore(dir) }
}

func auditEntriesIn(t *testing.T, dir string) []audit.Entry {
	t.Helper()
	store, err := audit.NewStore(dir)
	if err != nil {
		t.Fatalf("opening the audit store: %v", err)
	}
	entries, err := store.All()
	if err != nil {
		t.Fatalf("reading the audit log: %v", err)
	}
	return entries
}

var _ ui.Window = (*recordingWindow)(nil)

// TestTheReportOfAProtocolBatchDoesNotClaimAnOutputFolder: a protocol
// batch writes nothing anywhere — its output is the answer to the
// program that asked — so the report must not send the person looking
// for files that do not exist.
func TestTheReportOfAProtocolBatchDoesNotClaimAnOutputFolder(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			m := protocolWindowFor(t, digestRequest(1, "AABB"))
			m.c = i18n.Load(locale)
			win := m.win.(*recordingWindow)

			m.postReport(jobs.Report{Succeeded: 1})

			reports := win.posted("report")
			if len(reports) != 1 {
				t.Fatalf("%d report payloads, want one", len(reports))
			}
			outputText, _ := reports[0]["outputText"].(string)
			if outputText != m.c.T("main.report_output_returned") {
				t.Fatalf("the report says the output went to %q", outputText)
			}
			if outputText == m.c.T("main.report_output_various") {
				t.Fatal("the report claims each document's own folder")
			}
			if reports[0]["canOpenOutput"] != false {
				t.Fatal("the report offers to open an output folder that does not exist")
			}
			// Signing more means going back to a document list, which a
			// protocol batch does not have.
			if reports[0]["canSignMore"] != false {
				t.Fatal("the report offers to sign more documents")
			}
		})
	}
}

// TestTheProgressScreenOfAProtocolBatchShowsItsOwnDocuments is the
// defect the `unused` linter found: the queue screen rendered
// m.queue.Items(), which for a protocol batch is empty, so the person
// would have watched a hundred documents sign with nothing on screen.
func TestTheProgressScreenOfAProtocolBatchShowsItsOwnDocuments(t *testing.T) {
	m := protocolWindowFor(t, digestRequest(3, "AABB"))
	win := m.win.(*recordingWindow)

	m.postPreparingCard()

	queues := win.posted("queue")
	if len(queues) != 1 {
		t.Fatalf("%d queue payloads, want one", len(queues))
	}
	files, _ := queues[0]["files"].([]any)
	if len(files) != 3 {
		t.Fatalf("the progress screen shows %d documents, want the three the caller sent", len(files))
	}
	first, _ := files[0].(map[string]any)
	if first["name"] != "ugovor-0.pdf" {
		t.Fatalf("the first row is named %v", first["name"])
	}
}
