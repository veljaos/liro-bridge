//go:build windows

package main

// The signing flow: one window, several steps.
//
// Signing one document used to open six windows in sequence — the
// document list, the consent screen, the signing method, the placement
// picker, and back again. Each was defensible on its own and together
// they were a maze: every step took the foreground from the one before
// it, arrived somewhere else on the screen, and left nothing to press
// but a button that opened the next window.
//
// There is one window now. Its content changes:
//
//	1  Documents     drop, browse, list, remove
//	2  Certificate   the list, and the approval
//	3  Method        three cards
//	   -> Sign
//
// The steps are three pages this one window navigates between, not
// three windows. A navigation between two embedded pages costs a
// fraction of what a second WebView2 window costs (measured at a little
// over two seconds on the machine this was reported from), keeps the
// window's place on screen, and takes the foreground from nothing.
//
// The placement picker is the one thing that still opens separately,
// and only because it needs the room to show a page of the document at
// a size worth dragging on. It opens as the consequence of the first
// method rather than as a step of its own: choosing "sign, choosing
// where the signature goes" and pressing Sign opens it on the batch's
// first document. Closing it comes back to the method step and signs
// nothing.
//
// Nothing here weakens SPEC §6.5. The certificate step is still the
// agent's own window, the approval is still a deliberate press of a
// button nobody focused for the user, and a caller that supplies its
// own answers still sees that step and nothing else.

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// flowStep is one step of the signing flow. The order of the constants
// is the order of the steps.
type flowStep int

const (
	stepDocuments flowStep = iota
	stepCertificate
	stepMethod
)

// The three pages the one window shows. main.html carries the documents
// step and everything after the approval — the progress, the report,
// and the questions in between; consent.html is the certificate step;
// stamp.html is the method.
const (
	pageMain    = "/pages/main.html"
	pageConsent = "/pages/consent.html"
	pageStamp   = "/pages/stamp.html"
)

func (s flowStep) page() string {
	switch s {
	case stepCertificate:
		return pageConsent
	case stepMethod:
		return pageStamp
	default:
		return pageMain
	}
}

// The window's size at each step, measured rather than guessed, and the
// reason the window resizes at all: the document list and the
// certificate list need room, and three method cards do not. A window
// sized for its largest step is a window that is mostly empty for the
// other two.
//
// Width changes with height because the content does: a list of file
// names and a list of certificates are wide, and three button-sized
// targets are not. The window keeps its own centre across the change
// (ui.Window.Resize), so it stays where the person is looking.
const (
	// stepDocumentsWidth/Height: eight document rows without scrolling,
	// with the footer and the three actions still on screen under a
	// two-hundred-document list (D-106). The same size carries the
	// progress screen, the report, and the questions asked between the
	// approval and the first signature.
	stepDocumentsWidth  = 560
	stepDocumentsHeight = 690

	// stepCertificateWidth/Height: six certificate rows, which is the
	// bookkeeper's machine holding several clients' cards — SPEC §14.1
	// calls that normal rather than an edge case. The header and footer
	// take 216 points between them and six rows plus their gaps are 544.
	stepCertificateWidth  = 520
	stepCertificateHeight = 760

	// stepMethodWidth/Height: the three methods and whatever the chosen
	// one reveals under itself, the step header, and the actions.
	// Measured in sr-Cyrl, the longest of the three catalogues, with
	// the second method chosen — the fullest the screen gets, since the
	// four corners are taller than anything the other two reveal: the
	// form needs 208 points there and the chrome above and below it
	// 163, and the rest is about ten points of headroom so that a
	// one-word label change does not immediately put a scrollbar back.
	//
	// It was 345 while this screen was a window of its own, which is
	// what the step header costs: measured, and found the hard way —
	// the layout test passed at 345 because it was posting a payload
	// with no header in it while the real screen had one, and the third
	// option was cut off on screen.
	stepMethodWidth  = 440
	stepMethodHeight = 380
)

func (s flowStep) size() (width, height int) {
	switch s {
	case stepCertificate:
		return stepCertificateWidth, stepCertificateHeight
	case stepMethod:
		return stepMethodWidth, stepMethodHeight
	default:
		return stepDocumentsWidth, stepDocumentsHeight
	}
}

// jsStep is the step header: where you are, and the way back.
//
// Small dots rather than the words, because the step is context and not
// the subject of the screen. The words are still there for anyone who
// needs them — Label is the row's aria-label, so a screen reader says
// "Korak 2 od 3" while the eye sees three dots.
type jsStep struct {
	Index    int    `json:"index"`
	Total    int    `json:"total"`
	Back     bool   `json:"back"`
	BackText string `json:"backText"`
	Label    string `json:"label"`
}

// stepsFor is the flow this run actually has.
//
// It is computed rather than fixed because two things can shorten it: a
// caller that brings its own documents has no first step, and a caller
// that brings its own answer to the method question has no third —
// which is what makes "the person sees the approval and nothing else"
// one window and one click (D-124).
//
// Nothing lengthens it. Choosing to place the stamp by looking at the
// page used to add a fourth step, which asked nothing: it said where
// the last position was and then opened the picker. The picker is the
// consequence of that choice, not a step before it.
func (m *mainWindow) stepsFor() []flowStep {
	var out []flowStep
	if !m.documentsSupplied {
		out = append(out, stepDocuments)
	}
	out = append(out, stepCertificate)
	if m.asksHowToSign() {
		out = append(out, stepMethod)
	}
	return out
}

// asksHowToSign reports whether this run has a method question at all.
//
// Two things remove it. A caller that supplied its own answer has
// answered it (D-124), and a batch of bare digests has nothing to ask
// about: on the hash path the caller built the PDF and the CMS itself
// (SPEC §4.3), so there is no page for this agent to draw a stamp on
// and no document it could draw one into. Asking anyway would be a
// screen whose every answer does the same thing.
func (m *mainWindow) asksHowToSign() bool {
	return m.suppliedStamp == nil && !m.hashesOnly
}

// headerFor is the step header for one step, or nil when this run has
// only one step — a single dot is not a sequence, and a header saying
// "1 of 1" is noise on the one screen that must be nothing but the
// approval.
func (m *mainWindow) headerFor(step flowStep) *jsStep {
	steps := m.stepsFor()
	if len(steps) < 2 {
		return nil
	}
	for i, s := range steps {
		if s != step {
			continue
		}
		return &jsStep{
			Index:    i + 1,
			Total:    len(steps),
			Back:     i > 0,
			BackText: m.c.T("step.back"),
			Label:    fmt.Sprintf(m.c.T("step.counter"), i+1, len(steps)),
		}
	}
	return nil
}

// isLastStep reports whether step is the end of the flow — which is
// what decides whether its primary action says Next or Sign.
func (m *mainWindow) isLastStep(step flowStep) bool {
	steps := m.stepsFor()
	return len(steps) > 0 && steps[len(steps)-1] == step
}

// primaryLabelFor is what the primary action of step says: Sign on the
// last step, Next on every other.
func (m *mainWindow) primaryLabelFor(step flowStep) string {
	if m.isLastStep(step) {
		return m.c.T("stampwindow.sign")
	}
	return m.c.T("step.next")
}

// ---- moving between steps ------------------------------------------

// show puts the window on one step: navigating if the step lives on
// another page, resizing to what that step needs, and posting the
// step's own payload.
func (m *mainWindow) show(step flowStep) {
	if !m.gotoPage(step.page()) {
		return
	}
	m.step = step
	m.showingReport = false
	w, h := step.size()
	m.resize(w, h)

	switch step {
	case stepDocuments:
		m.postFiles()
	case stepCertificate:
		m.postCertificateStep()
	case stepMethod:
		m.postStampStep()
	}
}

// gotoPage navigates the window to page unless it is already there.
func (m *mainWindow) gotoPage(page string) bool {
	if m.page == page {
		return true
	}
	if err := m.win.Navigate(page); err != nil {
		slog.Error("signing flow: could not show the next step", "page", page, "error", err)
		return false
	}
	m.page = page
	if page == pageMain {
		// A navigation is a fresh document: every data-i18n label on it
		// is blank until the strings arrive, because that is what
		// carries them (bridge.js). The other two pages get theirs from
		// the step that posts them; this one has screens — the queue,
		// the report, the three questions — that do not, and they came
		// up with every button unlabelled until this.
		//
		// The strings and nothing else. This used to post the document
		// list's whole payload, which also *showed* the document list —
		// so navigating here on the way to the progress screen put step
		// 1 back on screen and left it there for as long as opening the
		// card took. A page arrives showing nothing until the step that
		// navigated to it says which screen it wants.
		if err := m.win.PostJSON(map[string]any{
			"type":    "strings",
			"strings": m.staticStrings(),
		}); err != nil {
			slog.Warn("signing flow: posting the page's strings failed", "error", err)
		}
	}
	return true
}

func (m *mainWindow) resize(width, height int) {
	if err := m.win.Resize(width, height); err != nil {
		slog.Warn("signing flow: resizing the window failed", "error", err)
	}
}

// firstStep is where this run begins, and where cancelling a step
// returns to.
func (m *mainWindow) firstStep() flowStep {
	steps := m.stepsFor()
	return steps[0]
}

// back moves one step earlier. Someone who picked the wrong certificate
// goes back one step, not out of the flow and into it again.
func (m *mainWindow) back() {
	steps := m.stepsFor()
	for i, s := range steps {
		if s == m.step && i > 0 {
			m.show(steps[i-1])
			return
		}
	}
}

// advance moves one step later, or starts signing when there is no
// later step.
func (m *mainWindow) advance(ctx context.Context) bool {
	steps := m.stepsFor()
	for i, s := range steps {
		if s != m.step {
			continue
		}
		if i+1 < len(steps) {
			m.show(steps[i+1])
			return false
		}
	}
	return m.startSigning(ctx)
}

// ---- the certificate step ------------------------------------------

// postCertificateStep enumerates the certificates and posts the
// approval screen. The enumeration happens here, on the way into the
// step, because it is the only slow thing in the flow and it must not
// be done twice for one batch.
func (m *mainWindow) postCertificateStep() {
	vm := consent.BuildViewModel(m.applicationName(), m.digests(), m.fileNames(), m.certInfos)
	payload := buildConsentInit(m.c, vm, m.certificateNotice())
	payload["step"] = m.headerFor(stepCertificate)
	if err := m.win.PostJSON(payload); err != nil {
		slog.Error("signing flow: could not post the certificate step", "error", err)
	}
}

// certificateNotice is the sentence the certificate step shows when
// there is nothing on it to choose. Empty when there is.
//
// The three whole-machine states get their own sentence, because they
// are the three a person can do something about and they are not the
// same thing: no reader is a cable, no card is a card, and a stopped
// Windows service is neither. A row-level reason — an expired
// certificate, one whose card is out — is already printed under the row
// it belongs to, so the summary line above stays general rather than
// repeating one row's reason as if it were the machine's.
func (m *mainWindow) certificateNotice() string {
	if m.listing != nil {
		// The enumeration has not answered yet. Without this the screen
		// would spend that second asserting that nothing here can sign,
		// which is a statement about a question nobody has answered.
		return m.c.T("consent.looking_for_certificates")
	}
	switch m.certReason {
	case "":
		return ""
	case errs.CodeCertNotFound:
		return m.c.T("consent.no_certificate_found")
	case errs.CodeCertNotUsable, errs.CodeCertExpired:
		return m.c.T("consent.no_usable_certificate")
	default:
		return m.c.T(i18n.CodeKey(m.certReason))
	}
}

// gatherCertificates fills in what the certificate step shows. It is
// called on the way out of the documents step (or once, at the start,
// for a run that has no documents step) rather than on every arrival at
// the certificate step: pressing Back from the method step must not
// re-enumerate a smart card.
func (m *mainWindow) gatherCertificates(ctx context.Context) bool {
	report, err := m.gather(ctx)
	if err != nil {
		slog.Error("signing flow: listing certificates failed", "error", err)
		m.fail(err)
		return false
	}
	m.applyListing(certificateListing{report: report})
	return true
}

// visibleCertificates is what a person is offered: every enumerated row
// less the ones no listing shows by default — a Windows-internal
// artefact, and a certificate that is not for signing at all
// (classify.Info.HiddenByDefault). One rule, in one place, for the
// `certs` command, the Certificates window and this flow.
func visibleCertificates(report cli.Report) []classify.Info {
	out := make([]classify.Info, 0, len(report.Certificates))
	for _, row := range report.Certificates {
		if row.Hidden() {
			continue
		}
		out = append(out, row.Info)
	}
	return out
}

// approveCertificate is what pressing Approve means: the human said yes
// to this batch with this certificate. It is the product's only real
// gate (SPEC §6.5) and it is unchanged by being a step — the button was
// not focused for the user, and nothing else on the screen can produce
// this.
func (m *mainWindow) approveCertificate(ctx context.Context) bool {
	if m.selected == "" {
		return false
	}
	return m.advance(ctx)
}

// ---- denials and failures ------------------------------------------

// deny records a refusal in the audit log. Every way out of the flow
// after the certificate step has been reached goes through it: SPEC
// §6.7 wants the refusals as much as the approvals.
func (m *mainWindow) deny() {
	if m.denied {
		// One refusal, however many ways out of the flow it took. A
		// person who presses Cancel and then closes the window has
		// refused once.
		return
	}
	m.denied = true
	open := m.auditStore
	if open == nil {
		open = newAuditStore
	}
	store, err := open()
	// A refusal can be the entry that opens a new chain just as a
	// signature can. It is carried the same way and shown the same way —
	// on the next screen that has somewhere to put it.
	entry := auditRecord{
		thumbprint:  m.selected,
		application: m.applicationName(),
		channel:     m.auditChannel(),
		documents:   m.documentCount(),
		outcome:     audit.OutcomeDenied,
	}
	if d := recordInteractiveAudit(store, err, entry); d != nil {
		m.auditNotice = auditChainNotice(m.c, store, d)
	}
}

// cancelFlow is what Cancel and Escape mean. Refusing is a decision and
// is recorded; stepping back is not a decision and is not.
//
// It returns true when the window should close, which is what
// cancelling means for a run that has no document list to return to.
func (m *mainWindow) cancelFlow() bool {
	if m.failed || m.showingReport {
		return true
	}
	if m.step != m.firstStep() {
		m.deny()
	}
	if m.documentsSupplied {
		return true
	}
	if m.step == stepDocuments {
		return true
	}
	m.selected = ""
	m.show(stepDocuments)
	return false
}

// fail shows the one screen for something that went wrong before a
// single document could be signed — the card, the timestamp authority,
// the certificate. The report screen is for a batch that ran; this is
// for one that never did.
// failRemote tells a protocol caller why its batch never started —
// the card, the certificate, the timestamp authority — with the same
// code the person is shown a sentence for.
//
// Separate from fail because fail is about a screen: it exists so that
// a caller is never left with CONSENT_DENIED for something the person
// never got the chance to deny.
func (m *mainWindow) failRemote(err error) {
	if m.remote == nil || m.remote.code != "" {
		return
	}
	m.remote.code = codeOfInteractive(err)
}

func (m *mainWindow) fail(err error) {
	m.failed = true
	m.exit = 1
	if !m.gotoPage(pageMain) {
		return
	}
	m.resize(stepDocumentsWidth, stepDocumentsHeight)
	if postErr := m.win.PostJSON(askFailedPayload(cli.ErrorMessage(err, m.c), err, m.c)); postErr != nil {
		slog.Warn("signing flow: posting the failure failed", "error", postErr)
	}
}

// ---- the method step -----------------------------------------------

// postStampStep posts the method screen: the three methods, the step
// header, and Sign.
//
// Sign, for all three of them. Choosing to place the stamp by looking
// at the page used to say Next instead, because it led to a step that
// asked nothing and then opened the picker; the picker opens from here
// now, so every method ends this screen the same way.
func (m *mainWindow) postStampStep() {
	payload := buildStampInit(m.c, m.cfg, stampRoleStep, m.method)
	payload["step"] = m.headerFor(stepMethod)
	payload["primaryLabel"] = m.primaryLabelFor(stepMethod)
	if err := m.win.PostJSON(payload); err != nil {
		slog.Error("signing flow: could not post the signing method", "error", err)
	}
}

// handleStampAction is one click on the method screen.
func (m *mainWindow) handleStampAction(ctx context.Context) bool {
	form, err := readStampSettings(m.win)
	if err != nil {
		slog.Warn("signing flow: reading the signing method failed", "error", err)
		return false
	}

	if form.Action == "back" {
		m.back()
		return false
	}
	if !form.Saved {
		return false
	}
	m.cfg = applyStampSettings(m.cfg, form)
	m.method = stampMethodOf(m.cfg)
	if form.Method == stampMethodPlaced {
		// A method chosen but not yet acted on: the configuration only
		// says "placed" once something has actually been placed, and
		// the picker is what places it.
		m.method = stampMethodPlaced
	}
	m.saveConfig()

	if m.method == stampMethodPlaced {
		return m.signAtAChosenPosition(ctx)
	}
	return m.advance(ctx)
}

// placementOutcome is what an answer from the picker means for the
// flow: whether there is now a position to sign with, which method the
// method screen should show if there is not, and what — if anything —
// it has to say about why.
type placementOutcome struct {
	cfg    config.Config
	sign   bool
	method string
	note   string
}

// placementResult is the picker's three endings, as a function of what
// it answered and nothing else.
//
// It is a function of its own so that all three can be exercised
// without a placement window: a position chosen, the window cancelled,
// and — the one that has to be right — a document the renderer cannot
// draw at all. That last is F6b §5's rule, a preview this project
// cannot produce must not become a document this project will not
// sign, and it is unreachable from the screen otherwise.
func placementResult(cfg config.Config, place func(config.Config) (config.Config, bool, string)) placementOutcome {
	next, placed, note := place(cfg)
	if note != "" {
		// The remembered position is left exactly as it was on disk;
		// the corners are what is being offered instead, which is not
		// the same thing as what the configuration holds.
		return placementOutcome{cfg: next, method: stampMethodCorners, note: note}
	}
	if !placed {
		// Closing the picker signs nothing and changes nothing. The
		// method screen is still behind it, still showing the method
		// that was chosen, and the person can choose again or press
		// Sign to open the picker a second time.
		return placementOutcome{cfg: next, method: stampMethodPlaced}
	}
	return placementOutcome{cfg: next, sign: true, method: stampMethodPlaced}
}

// signAtAChosenPosition is what confirming the first method comes to:
// the placement picker opens on the batch's first document, with the
// certificate just chosen, at the remembered position if there is one.
// Using a position from it signs the batch; closing it signs nothing.
//
// There is no screen between the choice and the picker. There was one,
// and it asked nothing — it named the remembered position and opened
// the picker, which is where that position is shown anyway, as
// something that can be acted on rather than read.
func (m *mainWindow) signAtAChosenPosition(ctx context.Context) bool {
	doc := stampWindowDoc{
		path: firstInputPath(m.inputs),
		cert: certificateFor(m.certs, m.selected),
	}
	if doc.path == "" {
		// A protocol batch's documents arrived over a socket and are
		// nowhere on disk, so there is no page to open the picker on.
		// placeStamp's no-path branch opens a file chooser, which here
		// would place the stamp by looking at some *other* document —
		// the same error the remembered position makes, one step
		// further on. The corners are what is left, and the screen says
		// so, exactly as it does for a document the renderer cannot
		// draw (F6b §5).
		m.method = stampMethodCorners
		m.postStampStep()
		postWindowStatus(m.win, m.c.T("place.unavailable"), ui.IntentWarning)
		return false
	}
	out := placementResult(m.cfg, func(in config.Config) (config.Config, bool, string) {
		return placeStamp(in, m.c, m.locale, doc, m.win.Handle())
	})
	m.cfg = out.cfg
	m.method = out.method

	if out.sign {
		m.saveConfig()
		return m.startSigning(ctx)
	}
	if out.note != "" {
		// A document that could not be drawn: the corners are what is
		// left, and the screen says why rather than appearing to have
		// ignored the click.
		m.postStampStep()
		postWindowStatus(m.win, out.note, ui.IntentWarning)
	}
	return false
}

// ---- helpers the flow shares with the batch ------------------------

func (m *mainWindow) digests() [][]byte {
	out := make([][]byte, 0, len(m.inputs))
	for _, in := range m.inputs {
		out = append(out, in.digest)
	}
	return out
}

func (m *mainWindow) fileNames() []string {
	out := make([]string, 0, len(m.inputs))
	for _, in := range m.inputs {
		if in.label != "" {
			out = append(out, in.label)
			continue
		}
		out = append(out, filepath.Base(in.path))
	}
	return out
}

// applicationName is who is asking, as the consent window shows it.
//
// For a request that arrived over the protocol it is the name bound at
// pairing and never one supplied in the request — SPEC §6.6's reason,
// unchanged: otherwise an application pairs as "Test" and presents
// itself as "Liro". For anything else it is "local".
func (m *mainWindow) applicationName() string {
	if m.remote != nil {
		return m.remote.req.Application
	}
	return consent.ApplicationLocal
}

// withoutRememberedPlacement drops the position a person placed by
// looking at a page, and leaves everything else about the stamp alone —
// whether there is one, which corner, which page, the reference line.
//
// It is a value returned rather than a field cleared because the
// configuration it is applied to is this run's own copy: what is on
// disk is the person's standing answer and is not touched (D-103).
func withoutRememberedPlacement(cfg config.Config) config.Config {
	if cfg.StampPosition == config.StampPositionCustom {
		cfg.StampPosition = config.DefaultStampPosition
	}
	cfg.StampPlacedPage, cfg.StampX, cfg.StampY = 0, 0, 0
	return cfg
}

// applySuppliedStamp folds a caller's supplied answer into the
// configuration, exactly as the method step would have. Nothing is
// saved to disk: an answer given by a caller is that caller's, not the
// person's standing preference (D-103).
func (m *mainWindow) applySuppliedStamp() {
	if m.suppliedStamp == nil {
		return
	}
	choice := m.suppliedStamp.Normalised()
	m.cfg.VisibleStamp = choice.Visible
	m.cfg.StampPosition = choice.Position
}
