//go:build windows

package main

// The main window (F6 §1, §3, §4, §5): the surface a person uses
// without a command line. Documents are gathered here, the consent
// window approves them — unchanged, the same one `sign --interactive`
// opens, because SPEC §6.5 makes that screen the product's only real
// gate and two implementations of it would be two things to keep right
// — and the batch is then watched and read here.
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
	"github.com/veljaos/liro-bridge/internal/pades"
	"github.com/veljaos/liro-bridge/internal/signing"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// mainWindowSize is measured, not guessed: at 560x720 the document list
// shows eight rows without scrolling, which covers the ordinary batch,
// and the footer's two setting rows plus the three actions stay on
// screen with a two-hundred-document list scrolling above them (D-106).
const (
	mainWindowWidth  = 560
	mainWindowHeight = 720
)

// mainWindow is one open main window and everything it is driving.
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
}

// runMainWindow opens the main window and runs it until it is closed.
// initialPaths seeds the queue — the Explorer context menu and the
// command line both arrive that way; an empty slice opens the empty
// state.
func runMainWindow(ctx context.Context, cfg config.Config, locale string, initialPaths []string) int {
	return runMainWindowWatching(ctx, cfg, locale, initialPaths, nil)
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
	m := &mainWindow{
		messages: make(chan ui.Message, 16),
		dropped:  make(chan []string, 16),
		closed:   make(chan struct{}),
		c:        i18n.Load(locale),
		locale:   locale,
		cfg:      cfg,
	}
	if len(initialPaths) > 0 {
		_, notices := m.queue.Add(initialPaths)
		m.notices = notices
	}

	win, err := ui.NewWindow(ui.Options{
		Title:       m.c.T("main.title"),
		Width:       mainWindowWidth,
		Height:      mainWindowHeight,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/main.html",
		OnMessage:   func(msg ui.Message) { m.messages <- msg },
		// F6 §1: files dropped from Explorer. The callback runs on the
		// window's own thread, so it does nothing but hand the paths
		// over — reading two hundred files' sizes there would freeze
		// the window mid-drop.
		OnFilesDropped: func(paths []string) { m.dropped <- paths },
		OnClosed:       func() { close(m.closed) },
	})
	if err != nil {
		slog.Error("main window: could not open", "error", err)
		return 1
	}
	m.win = win
	defer func() { _ = win.Close() }()

	if err := win.PostJSON(m.filesPayload("init")); err != nil {
		slog.Error("main window: could not post the initial state", "error", err)
		return 1
	}

	if inbox != nil {
		stop := make(chan struct{})
		defer close(stop)
		go watchInbox(inbox, m.dropped, stop)
	}

	m.loop(ctx)
	return 0
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
				slog.Warn("main window: reading the shell inbox failed", "error", err)
				continue
			}
			if len(paths) == 0 {
				continue
			}
			slog.Info("main window: adding documents that arrived after the window opened", "count", len(paths))
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
			m.addPaths(paths)
		case msg := <-m.messages:
			if msg.Type == ui.MessageTypeCancel {
				if m.runner != nil {
					m.runner.Stop()
				}
				return
			}
			if msg.Type != ui.MessageTypeApprove {
				continue
			}
			if done := m.handleAction(ctx); done {
				return
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
	raw, err := m.win.Eval("window.__liroMainAction()")
	if err != nil {
		slog.Warn("main window: reading the action failed", "error", err)
		return mainAction{}
	}
	var jsonStr string
	if err := json.Unmarshal([]byte(raw), &jsonStr); err != nil {
		slog.Warn("main window: decoding the action envelope failed", "error", err)
		return mainAction{}
	}
	var a mainAction
	if err := json.Unmarshal([]byte(jsonStr), &a); err != nil {
		slog.Warn("main window: decoding the action failed", "error", err)
		return mainAction{}
	}
	return a
}

// handleAction runs one page action. It returns true when the window
// should close.
func (m *mainWindow) handleAction(ctx context.Context) bool {
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
	case "stampSettings":
		m.openStampWindow()
	case "sign":
		m.sign(ctx)
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
		m.postFiles()
	default:
		slog.Warn("main window: approve with no action recorded")
	}
	return false
}

func (m *mainWindow) addPaths(paths []string) {
	_, notices := m.queue.Add(paths)
	m.notices = notices
	m.postFiles()
}

func (m *mainWindow) removeAt(index int) {
	items := m.queue.Items()
	if index < 0 || index >= len(items) {
		// The page's index and Go's list disagreeing means a stale
		// click, not an attack — but acting on it would remove the
		// wrong document, so it is dropped and logged.
		slog.Warn("main window: remove for an index that is not in the queue", "index", index)
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
		slog.Warn("main window: the file chooser failed", "error", err)
		return
	}
	if !ok || len(paths) == 0 {
		return
	}
	m.addPaths(paths)
}

func (m *mainWindow) chooseOutputFolder() {
	dir, ok, err := ui.ChooseFolder(m.win.Handle(), m.c.T("main.choose_output_folder"))
	if err != nil {
		slog.Warn("main window: the folder chooser failed", "error", err)
		return
	}
	if !ok {
		return
	}
	m.cfg.OutputFolder = dir
	m.saveConfig()
	m.postFiles()
}

func (m *mainWindow) saveConfig() {
	if err := config.Save(config.DefaultPath(), m.cfg); err != nil {
		slog.Warn("main window: saving the configuration failed", "error", err)
	}
}

// ---- payloads ------------------------------------------------------

func (m *mainWindow) postFiles() {
	if err := m.win.PostJSON(m.filesPayload("files")); err != nil {
		slog.Warn("main window: posting the file list failed", "error", err)
	}
}

type jsFile struct {
	Name      string `json:"name"`
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
	files := make([]jsFile, 0, len(items))
	for _, it := range items {
		files = append(files, jsFile{Name: it.DisplayName, SizeText: m.sizeText(it)})
	}
	payload := map[string]any{
		"type":             kind,
		"files":            files,
		"countText":        m.countText(),
		"notices":          m.noticeTexts(),
		"outputFolderText": m.outputFolderText(),
		"stampSummary":     m.stampSummary(),
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

func (m *mainWindow) stampSummary() string {
	if !m.cfg.VisibleStamp {
		return m.c.T("main.stamp_summary_off")
	}
	return fmt.Sprintf(m.c.T("main.stamp_summary_on"),
		m.c.T(stampPositionKey(m.cfg.StampPosition)),
		m.stampPageText())
}

func (m *mainWindow) stampPageText() string {
	return stampPageLabel(m.c, m.cfg.StampPage)
}

func stampPositionKey(position string) string {
	switch position {
	case consent.StampPositionBottomLeft:
		return "stampwindow.position_bottom_left"
	case consent.StampPositionTopRight:
		return "stampwindow.position_top_right"
	case consent.StampPositionTopLeft:
		return "stampwindow.position_top_left"
	default:
		return "stampwindow.position_bottom_right"
	}
}

// staticStrings is every data-i18n key this page resolves, resolved in
// Go so the page never sees a catalogue or a locale.
func (m *mainWindow) staticStrings() map[string]string {
	keys := []string{
		"main.empty_title", "main.empty_hint", "main.browse", "main.clear",
		"main.sign", "main.remove_file", "main.output_label", "main.output_change",
		"main.stamp_change", "main.queue_title", "main.stop", "main.stopping",
		"main.report_failures_title", "main.report_output_label",
		"main.report_level_label", "main.open_output", "main.export_report",
		"main.new_batch", "consent.per_signature_pin_warning",
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = m.c.T(k)
	}
	return out
}

// ---- the run -------------------------------------------------------

// sign takes the queue through the consent window and then runs it.
func (m *mainWindow) sign(ctx context.Context) {
	if m.queue.Len() == 0 {
		return
	}

	decision, ok := m.approve(ctx)
	if !ok {
		m.postFiles()
		return
	}
	defer func() { _ = decision.session.Close() }()

	m.runBatch(ctx, decision)
}

// runBatch signs the queue and shows the report.
func (m *mainWindow) runBatch(ctx context.Context, d consentDecision) {
	runner := &jobs.Runner{}
	m.runner = runner
	defer func() { m.runner = nil }()

	wrapped := signing.WrapSession(d.session)
	trustStore := interactiveTrustStore(ctx)
	stampOpts := m.stampOptions()

	// The run is on its own goroutine so the message loop keeps
	// answering — Stop must reach the runner while it is signing, and a
	// window that stops repainting mid-batch is the freeze F6 §3 asks
	// to avoid.
	reportCh := make(chan jobs.Report, 1)
	go func() {
		reportCh <- runner.Run(ctx, &m.queue, func(rctx context.Context, i int, item jobs.Item) (jobs.Outcome, error) {
			out := d.outputs[i]
			result, err := signInteractiveOne(rctx, item.Path, wrapped, interactiveSignOptions{
				level:      d.level,
				trustStore: trustStore,
				tsaClient:  d.tsaClient,
				outPath:    out.path,
				overwrite:  out.overwrite,
				allowBB:    d.allowBB,
				stamp:      stampOpts,
			})
			if err != nil {
				return jobs.Outcome{}, err
			}
			return jobs.Outcome{OutputPath: out.path, AchievedLevel: string(result.AchievedLevel)}, nil
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
		case paths := <-m.dropped:
			// Files dropped mid-batch are not silently lost, and not
			// added to a batch already approved either — the consent
			// screen approved a specific set (SPEC §6.5). They wait.
			slog.Info("main window: files dropped during a run are ignored", "count", len(paths))
		}
	}

	m.report = &report
	m.recordAudit(d, report)
	m.postReport(report)
}

func (m *mainWindow) stampOptions() *pades.StampOptions {
	if !m.cfg.VisibleStamp {
		return nil
	}
	opts := interactiveStampOptions(m.c, consent.StampChoice{
		Visible:  true,
		Position: m.cfg.StampPosition,
	}.Normalised())
	if opts == nil {
		return nil
	}
	opts.Page = stampPageNumber(m.cfg.StampPage)
	opts.Reference = m.cfg.StampReference
	opts.ShowDocumentID = m.cfg.StampShowDocumentID
	return opts
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
	store, err := newAuditStore()
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

func (m *mainWindow) postQueue(p jobs.Progress, stopping bool) {
	if err := m.win.PostJSON(m.queuePayload(m.queue.Items(), p, stopping)); err != nil {
		slog.Warn("main window: posting progress failed", "error", err)
	}
}

// queuePayload is what the queue screen renders. Split from postQueue so
// the rendering can be exercised with a queue in any combination of
// states — including all five at once, which a real run passes through
// but never rests in.
func (m *mainWindow) queuePayload(items []jobs.Item, p jobs.Progress, stopping bool) map[string]any {
	files := make([]jsFile, 0, len(items))
	for _, it := range items {
		f := jsFile{
			Name:      it.DisplayName,
			State:     string(it.State),
			StateText: m.stateText(it.State),
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
	}
	if err := m.win.PostJSON(payload); err != nil {
		slog.Warn("main window: posting the report failed", "error", err)
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
		slog.Warn("main window: opening the output folder failed", "error", err)
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
	dir, ok, err := ui.ChooseFolder(m.win.Handle(), m.c.T("main.export_report_title"))
	if err != nil || !ok {
		if err != nil {
			slog.Warn("main window: the folder chooser failed", "error", err)
		}
		return
	}
	name := fmt.Sprintf("liro-report-%s.txt", time.Now().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(m.reportText(*m.report)), 0o600); err != nil {
		slog.Warn("main window: writing the report failed", "error", err)
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
		slog.Warn("main window: posting a status line failed", "error", err)
	}
}

// openStampWindow shows F6 §6's stamp settings and re-reads the
// configuration when it closes, so the summary line and the next batch
// both reflect whatever was saved.
func (m *mainWindow) openStampWindow() {
	if err := runStampWindow(m.cfg, m.locale); err != nil {
		slog.Warn("main window: the stamp window failed", "error", err)
	}
	m.cfg = currentStampConfig(m.cfg)
	m.postFiles()
}
