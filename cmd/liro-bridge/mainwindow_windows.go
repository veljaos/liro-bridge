//go:build windows

package main

// The signing window (F6 §1, §3, §4, §5): the one window a person
// uses. It owns the flow's state and its event loop; signflow_windows.go
// owns the steps themselves.
//
// Everything this file decides about a queue is decided in
// internal/jobs, which has no window and is tested without one. What is
// left here is wiring: turn a jobs.Queue into a payload, turn a click
// into a jobs call, and turn a jobs.Report into sentences.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/pades/tsa"
	"github.com/veljaos/liro-bridge/internal/signing"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// mainWindow is the one open signing window and everything it is
// driving.
type mainWindow struct {
	win      ui.Window
	messages chan ui.Message
	dropped  chan []string
	closed   chan struct{}

	c      *i18n.Catalogue
	locale string
	cfg    config.Config

	queue   jobs.Queue
	notices []jobs.Notice

	// runner is non-nil only while a batch is running; Stop reaches it
	// from the message loop while the run is on its own goroutine.
	runner *jobs.Runner
	// report is the last finished run, for the export and open-folder
	// actions.
	report *jobs.Report

	// ---- the flow (signflow_windows.go) ----

	// page is the page the window is currently showing, so a step that
	// shares a page with the one before it does not navigate.
	page string
	// step is where in the flow the window is.
	step flowStep
	// showingReport and failed say the window is on a screen that is
	// not a step, which is what makes Cancel there mean "close" rather
	// than "go back to the document list".
	showingReport bool
	failed        bool

	// documentsSupplied means the caller brought the documents, so this
	// run has no first step — `sign --interactive --in ...`, and every
	// request that arrives with its documents already named.
	documentsSupplied bool
	// suppliedStamp is the method question already answered by the
	// caller. With it set the flow is the certificate step and nothing
	// else: one window, one click (D-124).
	suppliedStamp *consent.StampChoice
	// force skips the output-file question, matching the command line's
	// --force.
	force bool

	// inputs is the batch as the flow sees it: one entry per document,
	// with the digest the batch fingerprint is built from.
	inputs []interactiveInput

	// certs is the last enumeration, and certInfos what the certificate
	// step offers from it. Gathered once per batch, on the way into
	// that step — pressing Back must not re-enumerate a smart card.
	certs     cli.Report
	certInfos []classify.Info
	// selected is the chosen certificate's thumbprint, empty until the
	// person picks one.
	selected string
	// method is the signing method the flow is working with, which is
	// what decides whether there is a position step after the method
	// step.
	method string

	// exit is the process exit code for a run started from the command
	// line. Zero unless something failed.
	exit int

	// auditStore opens the log this flow records its batch in. It is a
	// field rather than a direct call to newAuditStore so a test can run
	// the real batch loop -- which is the only way to test it -- without
	// appending to the developer's own audit log.
	//
	// That is not hypothetical: FTEST measured four fabricated entries
	// added to the owner's real log by every `go test ./...`, sixteen
	// over one session, in an append-only hash-chained file that is
	// meant to be the record of what was actually signed (SPEC 6.7).
	// They are indistinguishable, to anyone reading the log later, from
	// real signing sessions.
	auditStore func() (*audit.Store, error)
}

// runMainWindow opens the signing window and runs it until it is
// closed. initialPaths seeds the queue — the Explorer context menu and
// the command line both arrive that way; an empty slice opens the empty
// state.
func runMainWindow(ctx context.Context, cfg config.Config, locale string, initialPaths []string) int {
	return runMainWindowWatching(ctx, cfg, locale, initialPaths, nil)
}

// flowRequest is a run of the signing flow that does not begin at the
// document list: the command line's `sign --interactive`, and — when
// F7 brings it — a request from a paired application. What it carries
// is what the person is then not asked.
type flowRequest struct {
	// inputs are the documents, already read for their digests.
	inputs []interactiveInput
	// force skips the output-file question.
	force bool
	// stamp, when non-nil, answers the method question, so the person
	// sees the approval and nothing else.
	stamp *consent.StampChoice
}

// runSigningFlow opens the window on the certificate step, for a caller
// that brought its own documents. It returns the process exit code.
func runSigningFlow(ctx context.Context, cfg config.Config, locale string, req flowRequest) int {
	m := newMainWindow(cfg, locale)
	m.documentsSupplied = true
	m.suppliedStamp = req.stamp
	m.force = req.force
	m.inputs = req.inputs
	m.applySuppliedStamp()

	paths := make([]string, 0, len(req.inputs))
	for _, in := range req.inputs {
		paths = append(paths, in.path)
	}
	_, m.notices = m.queue.Add(paths)

	if !m.gatherCertificatesBeforeOpening(ctx) {
		return 1
	}
	return m.open(ctx, nil, stepCertificate)
}

// newMainWindow is the window's state before there is a window.
func newMainWindow(cfg config.Config, locale string) *mainWindow {
	return &mainWindow{
		messages:   make(chan ui.Message, 16),
		dropped:    make(chan []string, 16),
		closed:     make(chan struct{}),
		c:          i18n.Load(locale),
		locale:     locale,
		cfg:        cfg,
		method:     stampMethodOf(cfg),
		auditStore: newAuditStore,
	}
}

// gatherCertificatesBeforeOpening enumerates for a run that starts at
// the certificate step, where there is no earlier step to do it on the
// way out of. A failure here is fatal rather than a screen: there is no
// window yet to put one on.
func (m *mainWindow) gatherCertificatesBeforeOpening(ctx context.Context) bool {
	report, err := gatherInteractiveCertificates(ctx)
	if err != nil {
		slog.Error("signing flow: listing certificates failed", "error", err)
		return false
	}
	m.certs = report
	m.certInfos = visibleCertificates(report)
	return true
}

// runMainWindowWatching is runMainWindow with an inbox to keep draining
// while the window is open (F6 §2).
//
// Explorer starts one process per selected file and they do not all
// arrive before the first one has waited out its coalescing window —
// measured, twenty of them spread over 2390ms with a 1070ms stall in
// the middle. Rather than making everyone wait long enough for the
// worst machine, the window opens promptly and late arrivals join the
// list already on screen, exactly as a dropped file does. Nothing is
// stranded and nothing is signed that the person did not select.
func runMainWindowWatching(ctx context.Context, cfg config.Config, locale string, initialPaths []string, inbox *jobs.Inbox) int {
	m := newMainWindow(cfg, locale)
	if len(initialPaths) > 0 {
		_, notices := m.queue.Add(initialPaths)
		m.notices = notices
	}
	return m.open(ctx, inbox, stepDocuments)
}

// open creates the window on first and runs the flow until the window
// closes.
func (m *mainWindow) open(ctx context.Context, inbox *jobs.Inbox, first flowStep) int {
	width, height := first.size()
	win, err := ui.NewWindow(ui.Options{
		Title:  m.c.T("main.title"),
		Width:  width,
		Height: height,
		// A request that begins at the approval is a request the person
		// did not initiate, and SPEC §6.5 requires that window to come
		// to the front and stay there. A window the person opened
		// themselves is one they are already looking at, and making it
		// topmost would only put it over everything else they do with
		// it — the file chooser included.
		AlwaysOnTop: first == stepCertificate,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   first.page(),
		OnMessage:   func(msg ui.Message) { m.messages <- msg },
		// F6 §1: files dropped from Explorer. It hands the paths to
		// the loop and does nothing else — reading two hundred files'
		// sizes here would hold up every callback behind it, and the
		// loop is where the queue lives anyway.
		OnFilesDropped: func(paths []string) { m.dropped <- paths },
		OnClosed:       func() { close(m.closed) },
	})
	if err != nil {
		slog.Error("signing window: could not open", "error", err)
		return 1
	}
	m.win = win
	m.page = first.page()
	m.step = first
	defer func() { _ = win.Close() }()

	switch first {
	case stepCertificate:
		m.postCertificateStep()
	default:
		if err := win.PostJSON(m.filesPayload("init")); err != nil {
			slog.Error("signing window: could not post the initial state", "error", err)
			return 1
		}
	}

	if inbox != nil {
		stop := make(chan struct{})
		defer close(stop)
		go watchInbox(inbox, m.dropped, stop)
	}

	m.loop(ctx)
	return m.exit
}

// inboxPollInterval is how often an open window looks for stragglers.
// Short enough that a late file appears while the person is still
// looking at the list, cheap enough to run for as long as the window is
// open: it is one Stat on a file that is usually not there.
const inboxPollInterval = 250 * time.Millisecond

// watchInbox drains the inbox into dropped until stop is closed.
func watchInbox(box *jobs.Inbox, dropped chan<- []string, stop <-chan struct{}) {
	ticker := time.NewTicker(inboxPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			paths, err := box.Take()
			if err != nil {
				slog.Warn("signing window: reading the shell inbox failed", "error", err)
				continue
			}
			if len(paths) == 0 {
				continue
			}
			slog.Info("signing window: adding documents that arrived after the window opened", "count", len(paths))
			select {
			case dropped <- paths:
			case <-stop:
				return
			}
		}
	}
}

// loop is the window's event loop: drops in, clicks in, everything else
// driven from here on one goroutine, so nothing touches the queue
// concurrently.
func (m *mainWindow) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.closed:
			// The user closed the window. A run in flight is asked to
			// stop after the document it is on — never interrupted, so
			// no half-written file is left behind (F6 §7).
			if m.runner != nil {
				m.runner.Stop()
			}
			return
		case paths := <-m.dropped:
			// Documents are added at the step that is about them.
			// Anywhere else the drop is not lost and not silently
			// folded into a batch that has already been approved
			// (SPEC §6.5) — it is refused, loudly in the log.
			if m.step != stepDocuments || m.showingReport {
				slog.Info("signing window: files dropped away from the document step are ignored", "count", len(paths))
				continue
			}
			m.addPaths(paths)
		case msg := <-m.messages:
			switch msg.Type {
			case ui.MessageTypeSelectCertificate:
				m.selected = msg.Thumbprint
			case ui.MessageTypeCancel:
				if m.runner != nil {
					m.runner.Stop()
					return
				}
				if m.cancelFlow() {
					return
				}
			case ui.MessageTypeApprove:
				if done := m.handleAction(ctx); done {
					return
				}
			}
		}
	}
}

// mainAction is what the page recorded before sending its one approve
// message (D-083: the page->Go surface stays at exactly three types).
type mainAction struct {
	Action string `json:"action"`
	Index  int    `json:"index"`
}

func (m *mainWindow) readAction() mainAction {
	raw, err := m.win.Eval("window.__liroAction()")
	if err != nil {
		slog.Warn("signing window: reading the action failed", "error", err)
		return mainAction{}
	}
	var jsonStr string
	if err := json.Unmarshal([]byte(raw), &jsonStr); err != nil {
		slog.Warn("signing window: decoding the action envelope failed", "error", err)
		return mainAction{}
	}
	var a mainAction
	if err := json.Unmarshal([]byte(jsonStr), &a); err != nil {
		slog.Warn("signing window: decoding the action failed", "error", err)
		return mainAction{}
	}
	return a
}

// handleAction runs one page action. It returns true when the window
// should close.
func (m *mainWindow) handleAction(ctx context.Context) bool {
	// The method step reports its whole form in one read, so it is
	// read as a form and not as a bare action.
	if !m.showingReport && !m.failed && m.step == stepMethod {
		return m.handleStampAction(ctx)
	}

	a := m.readAction()
	switch a.Action {
	case "browse":
		m.browse()
	case "remove":
		m.removeAt(a.Index)
	case "clear":
		m.queue.Clear()
		m.notices = nil
		m.postFiles()
	case "chooseOutputFolder":
		m.chooseOutputFolder()
	case "clearOutputFolder":
		m.clearOutputFolder()
	case "next":
		return m.next(ctx)
	case "back":
		m.back()
	case "approve":
		return m.approveCertificate(ctx)
	case "stop":
		if m.runner != nil {
			m.runner.Stop()
		}
	case "openOutput":
		m.openOutputFolder()
	case "exportReport":
		m.exportReport()
	case "newBatch":
		m.queue.Clear()
		m.notices = nil
		m.report = nil
		m.selected = ""
		m.show(stepDocuments)
	case "finish":
		// The expected end of a batch, and the report screen's primary
		// action: the work is done, so the window closes. Signing more
		// is the other button, beside it.
		return true
	default:
		slog.Warn("signing window: approve with no action recorded")
	}
	return false
}

// next is the primary action of whichever step is showing.
func (m *mainWindow) next(ctx context.Context) bool {
	if m.step == stepDocuments {
		if m.queue.Len() == 0 {
			return false
		}
		m.readInputs()
		if !m.gatherCertificates(ctx) {
			return false
		}
	}
	return m.advance(ctx)
}

func (m *mainWindow) addPaths(paths []string) {
	before := m.queue.Len()
	added, notices := m.queue.Add(paths)
	m.notices = notices
	// Counts only — never a name or a path (SPEC §18.3). This is what
	// settles "it added a duplicate anyway" from a machine that is not
	// this one: added plus duplicates plus anything unreadable accounts
	// for every path that arrived, and the queue length says what the
	// list should be showing.
	duplicates := 0
	unreadable := 0
	for _, n := range notices {
		switch n.Kind {
		case jobs.NoticeDuplicate:
			duplicates++
		case jobs.NoticeUnreadable:
			unreadable++
		}
	}
	slog.Info("signing window: documents added",
		"arrived", len(paths), "added", added,
		"duplicates", duplicates, "unreadable", unreadable,
		"queueWas", before, "queueNow", m.queue.Len())
	m.postFiles()
}

func (m *mainWindow) removeAt(index int) {
	items := m.queue.Items()
	if index < 0 || index >= len(items) {
		// The page's index and Go's list disagreeing means a stale
		// click, not an attack — but acting on it would remove the
		// wrong document, so it is dropped and logged.
		slog.Warn("signing window: remove for an index that is not in the queue", "index", index)
		return
	}
	m.queue.Remove(items[index].Path)
	m.notices = nil
	m.postFiles()
}

func (m *mainWindow) browse() {
	paths, ok, err := ui.ChooseFiles(m.win.Handle(),
		m.c.T("main.choose_files"),
		m.c.T("main.file_filter_pdf"),
		m.c.T("main.file_filter_all"))
	if err != nil {
		slog.Warn("signing window: the file chooser failed", "error", err)
		return
	}
	if !ok || len(paths) == 0 {
		return
	}
	m.addPaths(paths)
}

// chooseOutputFolder asks for a folder to write signatures into.
//
// The chooser starts on the folder already chosen, so an accidental OK
// keeps what was there instead of answering "the Desktop" — which is
// what it did answer, silently, and why every signed document was
// landing there (docs/decisions.md).
func (m *mainWindow) chooseOutputFolder() {
	dir, ok, err := ui.ChooseFolder(m.win.Handle(), m.c.T("main.choose_output_folder"), m.cfg.OutputFolder)
	if err != nil {
		slog.Warn("signing window: the folder chooser failed", "error", err)
		return
	}
	if !ok {
		return
	}
	m.cfg.OutputFolder = dir
	m.saveConfig()
	m.postFiles()
}

// clearOutputFolder goes back to the default: each signature beside its
// own input. Without it a chosen folder was a one-way door — the main
// window could set one and had no way to unset it, so the only way back
// was the Settings window's text field or editing config.json by hand.
func (m *mainWindow) clearOutputFolder() {
	m.cfg.OutputFolder = ""
	m.saveConfig()
	m.postFiles()
}

func (m *mainWindow) saveConfig() {
	if err := config.Save(config.DefaultPath(), m.cfg); err != nil {
		slog.Warn("signing window: saving the configuration failed", "error", err)
	}
}

// ---- payloads ------------------------------------------------------

func (m *mainWindow) postFiles() {
	if err := m.win.PostJSON(m.filesPayload("files")); err != nil {
		slog.Warn("signing window: posting the file list failed", "error", err)
	}
}

type jsFile struct {
	Name string `json:"name"`
	// Folder is set only for a document whose name alone does not
	// identify it in this list (jobs.NeedsFolder). Empty for every
	// other row, which is nearly all of them.
	Folder    string `json:"folder,omitempty"`
	SizeText  string `json:"sizeText"`
	State     string `json:"state,omitempty"`
	StateText string `json:"stateText,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type jsNotice struct {
	Text    string `json:"text"`
	Problem bool   `json:"problem"`
}

func (m *mainWindow) filesPayload(kind string) map[string]any {
	items := m.queue.Items()
	needsFolder := jobs.NeedsFolder(items)
	files := make([]jsFile, 0, len(items))
	for i, it := range items {
		f := jsFile{Name: it.DisplayName, SizeText: m.sizeText(it)}
		if needsFolder[i] {
			f.Folder = it.Folder
		}
		files = append(files, f)
	}
	payload := map[string]any{
		"type":             kind,
		"files":            files,
		"countText":        m.countText(),
		"notices":          m.noticeTexts(),
		"outputFolderText": m.outputFolderText(),
		"step":             m.headerFor(stepDocuments),
		"primaryLabel":     m.primaryLabelFor(stepDocuments),
		// Only shown when there is something to undo: a "beside each
		// document" button next to a row that already says exactly that
		// is a button that does nothing.
		"outputFolderChosen": m.cfg.OutputFolder != "",
	}
	if kind == "init" {
		payload["strings"] = m.staticStrings()
	}
	return payload
}

func (m *mainWindow) sizeText(it jobs.Item) string {
	if !it.SizeKnown {
		return m.c.T("main.size_unknown")
	}
	return jobs.FormatSize(it.Size)
}

func (m *mainWindow) countText() string {
	n := m.queue.Len()
	if n == 0 {
		return ""
	}
	total, complete := m.queue.TotalSize()
	size := jobs.FormatSize(total)
	if !complete {
		size += " " + m.c.T("main.size_unknown")
	}
	if n == 1 {
		return fmt.Sprintf(m.c.T("main.document_count_one"), size)
	}
	return fmt.Sprintf(m.c.T("main.document_count"), n, size)
}

func (m *mainWindow) noticeTexts() []jsNotice {
	out := make([]jsNotice, 0, len(m.notices))
	for _, n := range m.notices {
		switch n.Kind {
		case jobs.NoticeFolderScanned:
			out = append(out, jsNotice{Text: fmt.Sprintf(m.c.T("main.notice_folder_scanned"), n.Name, n.Count)})
		case jobs.NoticeFolderEmpty:
			out = append(out, jsNotice{Text: fmt.Sprintf(m.c.T("main.notice_folder_empty"), n.Name), Problem: true})
		case jobs.NoticeDuplicate:
			out = append(out, jsNotice{Text: fmt.Sprintf(m.c.T("main.notice_duplicate"), n.Name)})
		case jobs.NoticeUnreadable:
			out = append(out, jsNotice{Text: fmt.Sprintf(m.c.T("main.notice_unreadable"), n.Name), Problem: true})
		}
	}
	return out
}

func (m *mainWindow) outputFolderText() string {
	if m.cfg.OutputFolder == "" {
		return m.c.T("main.output_beside_input")
	}
	return m.cfg.OutputFolder
}

// staticStrings is every data-i18n key this page resolves, resolved in
// Go so the page never sees a catalogue or a locale.
func (m *mainWindow) staticStrings() map[string]string {
	keys := []string{
		"main.empty_title", "main.empty_hint", "main.browse", "main.clear",
		"main.sign_opening", "main.remove_file",
		"main.output_label", "main.output_change",
		"main.output_beside", "main.stop", "main.stopping",
		"main.report_failures_title", "main.report_output_label",
		"main.report_level_label", "main.open_output", "main.export_report",
		"main.new_batch", "main.finish", "consent.per_signature_pin_warning",
		// The questions asked between the approval and the first
		// signature live on this page now, so its static labels do too.
		"consent.tsa_choice_title", "consent.tsa_choice_explain",
		"consent.tsa_save_without_timestamp", "consent.tsa_configure",
		"consent.output_exists_title", "consent.output_exists_explain",
		"consent.output_exists_path_label", "consent.output_exists_overwrite",
		"consent.state_failed", "consent.copy_technical_details",
		"consent.close", "consent.cancel",
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = m.c.T(k)
	}
	return out
}

// ---- the run -------------------------------------------------------

// startSigning is everything between the last step and the first
// signature: the timestamp question, the output paths, and the card
// session — in that order, and all of them before the card is touched,
// so a person who cancels has not spent a PIN entry on a batch that was
// never going to be saved (D-095, D-104).
//
// All of it happens on the same window, which by now shows the page
// that will carry the progress and the report — and shows the progress
// screen itself from the moment Sign is pressed, not from the moment
// the first signature completes.
//
// That last is not cosmetic. Navigating here used to put the document
// list back on screen, and it stayed there while the card session was
// opened — measured at about a second, and the whole point of the
// "Preparing card…" state (SPEC §12.9) is that this is exactly the
// interval a person must not be told nothing is happening in. Worse,
// what it showed was step 1: the flow appeared to jump back to where it
// started.
func (m *mainWindow) startSigning(ctx context.Context) bool {
	if m.queue.Len() == 0 || m.selected == "" {
		return false
	}
	if !m.gotoPage(pageMain) {
		return false
	}
	m.resize(stepDocumentsWidth, stepDocumentsHeight)
	m.postPreparingCard()

	level := interactiveLevel(m.cfg)
	var tsaClient *tsa.Client
	allowBB := level == pades.LevelBB
	if level != pades.LevelBB {
		client, err := buildTSAClient(m.cfg)
		if err != nil {
			m.fail(err)
			return false
		}
		tsaClient = client
		if tsaClient == nil {
			cfg, next, bb, proceed := resolveTSAChoice(m.win, m.messages, m.c, m.cfg, m.locale, consent.TSAReasonNotConfigured)
			if !proceed {
				m.deny()
				m.backToStart()
				return false
			}
			m.cfg, tsaClient, allowBB = cfg, next, bb
		}
	}

	outputs, settled := resolveOutputsIn(m.win, m.messages, m.c, m.inputs, m.cfg.OutputFolder, m.cfg.OutputSuffix, m.force)
	if !settled {
		m.deny()
		m.backToStart()
		return false
	}

	// Either question above puts its own screen up. Whichever way they
	// were answered, the progress screen is what covers the card being
	// opened — the slowest thing that happens with nothing to show for
	// it.
	m.postPreparingCard()

	session, err := openInteractiveSession(ctx, keysource.Thumbprint(m.selected), m.win.Handle())
	if err != nil {
		m.fail(err)
		return false
	}
	defer func() { _ = session.Close() }()

	m.runBatch(ctx, consentDecision{
		approved:   true,
		thumbprint: m.selected,
		session:    session,
		level:      level,
		tsaClient:  tsaClient,
		allowBB:    allowBB,
		outputs:    outputs,
		cfg:        m.cfg,
	})
	return false
}

// backToStart is where cancelling a question asked after the approval
// lands: the document list when there is one, and nowhere otherwise —
// a run that brought its own documents has nothing to go back to, so
// the window closes and the loop ends on the next cancel.
func (m *mainWindow) backToStart() {
	if m.documentsSupplied {
		m.exit = 0
		close(m.closed)
		return
	}
	m.selected = ""
	m.show(stepDocuments)
}

// readInputs reads every queued document for the digest the batch
// fingerprint is built from (SPEC §6.6).
func (m *mainWindow) readInputs() {
	items := m.queue.Items()
	inputs := make([]interactiveInput, 0, len(items))
	for _, it := range items {
		in, err := newInteractiveInput(it.Path)
		if err != nil {
			// A document that cannot even be read for its digest is
			// not something to ask consent about. It is reported in
			// the queue instead, where the rest of the batch's own
			// failures are.
			slog.Warn("signing window: a document could not be read for the batch fingerprint",
				"error", err)
			in = interactiveInput{path: it.Path, digest: nil}
		}
		inputs = append(inputs, in)
	}
	m.inputs = inputs
}

// runBatch signs the queue and shows the report.
func (m *mainWindow) runBatch(ctx context.Context, d consentDecision) {
	runner := &jobs.Runner{}
	m.runner = runner
	defer func() { m.runner = nil }()

	wrapped := signing.WrapSession(d.session)
	trustStore := interactiveTrustStore(ctx)
	stampOpts := stampOptionsFor(m.c, m.cfg)

	// SPEC §12.8: a timestamp authority that stops answering mid-batch
	// presents the choice, it does not end the batch. The question has
	// to be asked on the goroutine that owns the window's messages,
	// which is this one — so the signing goroutine hands a reply
	// channel across and waits for the answer.
	tsaAsk := make(chan chan tsaAnswer)
	tsaClient, allowBB := d.tsaClient, d.allowBB

	// The run is on its own goroutine so the message loop keeps
	// answering — Stop must reach the runner while it is signing, and a
	// window that stops repainting mid-batch is the freeze F6 §3 asks
	// to avoid.
	reportCh := make(chan jobs.Report, 1)
	go func() {
		reportCh <- runner.Run(ctx, &m.queue, func(rctx context.Context, i int, item jobs.Item) (jobs.Outcome, error) {
			out := d.outputs[i]
			opts := interactiveSignOptions{
				level:      d.level,
				trustStore: trustStore,
				tsaClient:  tsaClient,
				outPath:    out.path,
				overwrite:  out.overwrite,
				allowBB:    allowBB,
				stamp:      stampOpts,
			}
			var result *pades.Result
			var err error
			for {
				result, err = signInteractiveOne(rctx, item.Path, wrapped, opts)
				if err == nil || !isTSAFailure(err) {
					break
				}
				reply := make(chan tsaAnswer, 1)
				select {
				case tsaAsk <- reply:
				case <-rctx.Done():
					return jobs.Outcome{}, err
				}
				answer := <-reply
				if !answer.proceed {
					return jobs.Outcome{}, err
				}
				// The answer holds for every document after this one
				// too: a batch of a hundred must not ask a hundred
				// times about one dead authority.
				tsaClient, allowBB = answer.client, answer.allowBB
				opts.tsaClient, opts.allowBB = answer.client, answer.allowBB
			}
			if err != nil {
				return jobs.Outcome{}, err
			}
			return jobs.Outcome{
				OutputPath:    out.path,
				AchievedLevel: string(result.AchievedLevel),
				// A remembered position that did not fit this document
				// as it stood is reported, not hidden (F6b §3).
				StampAdjusted: result.StampMoved || result.StampPageFellBack,
			}, nil
		}, jobs.Hooks{
			OnProgress: func(p jobs.Progress) { m.postQueue(p, runner.Stopped()) },
		})
	}()

	// Pump the window's own events while the run proceeds. Stop and
	// close both arrive here.
	var report jobs.Report
	for done := false; !done; {
		select {
		case report = <-reportCh:
			done = true
		case <-m.closed:
			runner.Stop()
			// Wait for the document in flight rather than abandoning
			// it: F6 §3 and SPEC §18.10 both forbid leaving a
			// half-written file, and the runner only stops between
			// documents.
			report = <-reportCh
			done = true
		case msg := <-m.messages:
			if msg.Type == ui.MessageTypeApprove {
				if a := m.readAction(); a.Action == "stop" {
					runner.Stop()
					m.postQueue(jobs.Progress{Phase: jobs.PhaseSigning, Total: m.queue.Len()}, true)
				}
			}
		case reply := <-tsaAsk:
			// The timestamp authority stopped answering. The same three
			// actions as before signing began, now about an authority
			// that was actually tried.
			cfg, client, bb, proceed := resolveTSAChoice(m.win, m.messages, m.c, m.cfg, m.locale, consent.TSAReasonUnreachable)
			m.cfg = cfg
			reply <- tsaAnswer{client: client, allowBB: bb, proceed: proceed}
			if proceed {
				// The window is showing the question; put the queue
				// back before the next document finishes.
				m.postQueue(jobs.Progress{Phase: jobs.PhaseSigning, Total: m.queue.Len()}, runner.Stopped())
			}
		case paths := <-m.dropped:
			// Files dropped mid-batch are not silently lost, and not
			// added to a batch already approved either — the consent
			// screen approved a specific set (SPEC §6.5). They wait.
			slog.Info("signing window: files dropped during a run are ignored", "count", len(paths))
		}
	}

	m.report = &report
	m.recordAudit(d, report)
	m.postReport(report)
}

// tsaAnswer is what the message loop tells the signing goroutine after
// asking SPEC §12.8's question mid-batch.
type tsaAnswer struct {
	client  *tsa.Client
	allowBB bool
	proceed bool
}

// stampPageNumber turns the configured page selection into the page
// number pades.StampOptions expects: 1 for the first page, -1 for the
// last (its own existing convention), or the number itself.
//
// An unparseable value cannot arrive here — config.ValidStampPage has
// already replaced it — and lands on the first page if it somehow does,
// which is SPEC §13's own default rather than a refusal.
func stampPageNumber(page string) int {
	switch page {
	case config.StampPageFirst:
		return 1
	case config.StampPageLast:
		return -1
	default:
		n, err := strconv.Atoi(page)
		if err != nil || n < 1 {
			return 1
		}
		return n
	}
}

func (m *mainWindow) recordAudit(d consentDecision, report jobs.Report) {
	open := m.auditStore
	if open == nil {
		open = newAuditStore
	}
	store, err := open()
	outcome := audit.OutcomeApproved
	switch {
	case report.Succeeded == 0 && (report.Stopped || report.Aborted):
		outcome = audit.OutcomeDenied
	case report.Succeeded == 0:
		outcome = audit.OutcomeFailed
	case report.Failed > 0 || report.Skipped > 0:
		outcome = audit.OutcomePartial
	}
	var lastErr error
	if len(report.Failures) > 0 {
		lastErr = errs.New(report.Failures[len(report.Failures)-1].Code, nil)
	}
	recordInteractiveAudit(store, err, d.thumbprint, m.queue.Len(), outcome, lastErr,
		d.session.Certificate().IsTestKey, report.AchievedLevel)
}

// ---- queue and report payloads -------------------------------------

// postPreparingCard is the progress screen before there is any progress
// to report: the queue with every document still waiting, the label
// SPEC §12.9 asks for, and an indeterminate bar rather than a
// percentage — nothing has been signed, so 0% would be a number
// pretending to be information.
//
// It is the same screen jobs.Runner's own first progress hook posts, in
// the same phase, so nothing changes on screen when the run begins.
func (m *mainWindow) postPreparingCard() {
	m.postQueue(jobs.Progress{Phase: jobs.PhasePreparingCard, Total: m.queue.Len()}, false)
}

func (m *mainWindow) postQueue(p jobs.Progress, stopping bool) {
	m.showingReport = false
	if err := m.win.PostJSON(m.queuePayload(m.queue.Items(), p, stopping)); err != nil {
		slog.Warn("signing window: posting progress failed", "error", err)
	}
}

// queuePayload is what the queue screen renders. Split from postQueue so
// the rendering can be exercised with a queue in any combination of
// states — including all five at once, which a real run passes through
// but never rests in.
func (m *mainWindow) queuePayload(items []jobs.Item, p jobs.Progress, stopping bool) map[string]any {
	// The same list, one screen later, so the same rule: a name that
	// appears twice says which folder it came from. Watching two rows
	// called "ugovor.pdf" and being told one of them failed is the
	// document-list defect happening two seconds further on.
	needsFolder := jobs.NeedsFolder(items)
	files := make([]jsFile, 0, len(items))
	for i, it := range items {
		f := jsFile{
			Name:      it.DisplayName,
			State:     string(it.State),
			StateText: m.stateText(it.State),
		}
		if needsFolder[i] {
			f.Folder = it.Folder
		}
		if it.State == jobs.StateFailed {
			f.Reason = cli.ErrorMessage(errs.New(it.FailureCode, nil), m.c)
		}
		files = append(files, f)
	}

	percent := 0
	if p.Total > 0 {
		percent = p.Current * 100 / p.Total
	}
	return map[string]any{
		"type":            "queue",
		"files":           files,
		"label":           m.queueLabel(p),
		"indeterminate":   p.Phase == jobs.PhasePreparingCard,
		"percent":         percent,
		"etaText":         m.etaText(p),
		"perSignaturePIN": p.PerSignaturePIN,
		"stopping":        stopping,
	}
}

func (m *mainWindow) queueLabel(p jobs.Progress) string {
	if p.Phase == jobs.PhasePreparingCard {
		return m.c.T("consent.state_preparing_card")
	}
	current := p.Current
	if current < 1 {
		current = 1
	}
	return fmt.Sprintf(m.c.T("consent.state_signing"), current, p.Total)
}

func (m *mainWindow) etaText(p jobs.Progress) string {
	if !p.ETAKnown {
		return ""
	}
	return fmt.Sprintf(m.c.T("consent.eta_label"), formatETA(p.ETA))
}

// formatETA renders a remaining time as a short phrase. Seconds below a
// minute, whole minutes above — nobody reads "1m 47.328s".
func formatETA(d time.Duration) string {
	if d < time.Minute {
		s := int(d.Round(time.Second).Seconds())
		if s < 1 {
			s = 1
		}
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dmin", int(d.Round(time.Minute).Minutes()))
}

func (m *mainWindow) stateText(s jobs.State) string {
	switch s {
	case jobs.StateSigning:
		return m.c.T("main.state_signing")
	case jobs.StateDone:
		return m.c.T("main.state_done")
	case jobs.StateFailed:
		return m.c.T("main.state_failed")
	case jobs.StateSkipped:
		return m.c.T("main.state_skipped")
	default:
		return m.c.T("main.state_waiting")
	}
}

type jsFailure struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func (m *mainWindow) postReport(r jobs.Report) {
	failures := make([]jsFailure, 0, len(r.Failures))
	for _, f := range r.Failures {
		// F6 §5: by file name, with the reason in words. The code never
		// reaches the screen (SPEC §7).
		failures = append(failures, jsFailure{
			Name:   f.Name,
			Reason: cli.ErrorMessage(errs.New(f.Code, nil), m.c),
		})
	}

	title := m.c.T("main.report_title")
	if r.Stopped {
		title = m.c.T("main.report_stopped_title")
	}

	abortMessage := ""
	switch {
	case r.Aborted && r.AbortCode == errs.CodeCardNotPresent:
		abortMessage = m.c.T("main.aborted_card")
	case r.Aborted && r.AbortCode == errs.CodePINLocked:
		abortMessage = m.c.T("main.aborted_pin")
	}

	outputText := m.c.T("main.report_output_various")
	if r.OutputDir != "" {
		outputText = r.OutputDir
	}
	levelText := r.AchievedLevel
	if levelText == "" {
		levelText = "—"
	}

	payload := map[string]any{
		"type":          "report",
		"title":         title,
		"counts":        m.countsText(r),
		"failures":      failures,
		"outputText":    outputText,
		"levelText":     levelText,
		"canOpenOutput": r.OutputDir != "",
		"abortMessage":  abortMessage,
		// F6b §3: a remembered position reused across a batch of
		// differently shaped documents is adjusted to fit some of them,
		// and the person is told how many rather than left to notice.
		"stampAdjusted": stampAdjustedText(m.c, r.StampAdjusted),
		// SPEC §18.11: the level a batch actually reached is stated,
		// and B-B — a signature with no proof of when it was made —
		// says so in words rather than being left to read as just
		// another level.
		"levelIntent": achievedLevelIntent(r.AchievedLevel),
		"levelNote":   achievedLevelNote(m.c, r.AchievedLevel),
		// Signing more means going back to a document list, which a run
		// that brought its own documents does not have.
		"canSignMore": !m.documentsSupplied,
	}
	m.showingReport = true
	if err := m.win.PostJSON(payload); err != nil {
		slog.Warn("signing window: posting the report failed", "error", err)
	}
}

// stampAdjustedText says how many documents needed the saved stamp
// position moved to fit them. Empty when none did, which is the
// ordinary case and deserves no line of its own.
func stampAdjustedText(c *i18n.Catalogue, n int) string {
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return c.T("sign.stamp_adjusted_one")
	default:
		return fmt.Sprintf(c.T("sign.stamp_adjusted_many"), n)
	}
}

func (m *mainWindow) countsText(r jobs.Report) string {
	parts := []string{fmt.Sprintf(m.c.T("main.report_succeeded"), r.Succeeded)}
	if r.Failed > 0 {
		parts = append(parts, fmt.Sprintf(m.c.T("main.report_failed"), r.Failed))
	}
	if r.Skipped > 0 {
		parts = append(parts, fmt.Sprintf(m.c.T("main.report_skipped"), r.Skipped))
	}
	return strings.Join(parts, " · ")
}

// ---- report actions ------------------------------------------------

// openOutputFolder shows the output folder in Explorer.
//
// The path handed to explorer.exe is one this process computed from the
// queue's own inputs and the configured folder — never a string the
// page supplied — so there is nothing here for a page to steer.
func (m *mainWindow) openOutputFolder() {
	if m.report == nil || m.report.OutputDir == "" {
		return
	}
	cmd := exec.Command("explorer.exe", m.report.OutputDir) //nolint:gosec // a path this process computed, never page input
	if err := cmd.Start(); err != nil {
		slog.Warn("signing window: opening the output folder failed", "error", err)
	}
}

// exportReport writes the finished run's report where the user chooses
// (F6 §5). The audit log already records the batch; this is for the
// person, so it is plain text they can read, attach or print — not the
// audit log's hash-chained JSON.
func (m *mainWindow) exportReport() {
	if m.report == nil {
		return
	}
	dir, ok, err := ui.ChooseFolder(m.win.Handle(), m.c.T("main.export_report_title"), m.cfg.OutputFolder)
	if err != nil || !ok {
		if err != nil {
			slog.Warn("signing window: the folder chooser failed", "error", err)
		}
		return
	}
	name := fmt.Sprintf("liro-report-%s.txt", time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(m.reportText(*m.report)), 0o600); err != nil {
		slog.Warn("signing window: writing the report failed", "error", err)
		m.postStatus(m.c.T("main.export_report_failed"), "negative")
		return
	}
	m.postStatus(fmt.Sprintf(m.c.T("main.export_report_done"), dir), "positive")
}

// reportText renders the report as plain text.
//
// File names appear here because this file is the person's own record
// of their own documents, written where they chose. That is a different
// thing from the audit log, which SPEC §6.7 forbids file names in
// precisely because it is the permanent, chained record — and which is
// written separately and is not affected by this.
func (m *mainWindow) reportText(r jobs.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\r\n", m.c.T("main.report_title"))
	fmt.Fprintf(&b, "%s\r\n\r\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "%s\r\n", m.countsText(r))
	if r.AchievedLevel != "" {
		fmt.Fprintf(&b, "%s: %s\r\n", m.c.T("main.report_level_label"), r.AchievedLevel)
	}
	if r.OutputDir != "" {
		fmt.Fprintf(&b, "%s: %s\r\n", m.c.T("main.report_output_label"), r.OutputDir)
	}
	if len(r.Failures) > 0 {
		fmt.Fprintf(&b, "\r\n%s\r\n", m.c.T("main.report_failures_title"))
		for _, f := range r.Failures {
			fmt.Fprintf(&b, "  %s — %s\r\n", f.Name, cli.ErrorMessage(errs.New(f.Code, nil), m.c))
		}
	}
	return b.String()
}

func (m *mainWindow) postStatus(text, intent string) {
	if err := m.win.PostJSON(map[string]any{
		"type": "status", "text": text, "intent": intent,
	}); err != nil {
		slog.Warn("signing window: posting a status line failed", "error", err)
	}
}
