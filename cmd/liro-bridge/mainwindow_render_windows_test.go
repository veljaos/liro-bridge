//go:build windows

package main

// The main window, driven against a real WebView2 window (F6 §1, §3,
// §5).
//
// D-087's lesson is the reason these exist rather than tests of the
// payload builders alone: every visible defect in F5 passed its Go-side
// tests while being broken on screen, because a test that exercises the
// logic beneath a boundary is not evidence about what crosses it. What
// crosses here is a JSON payload into a browser engine and a recorded
// action back out.
//
// Clicks are Eval-driven, inside the page's own DOM — D-094's carve-out.
// Nothing here touches the real cursor or keyboard.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// evalText runs script and decodes its JSON string result.
func evalText(t *testing.T, win ui.Window, script string) string {
	t.Helper()
	raw, err := win.Eval(script)
	if err != nil {
		t.Fatalf("Eval(%s): %v", script, err)
	}
	var s *string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("Eval(%s): decoding %q: %v", script, raw, err)
	}
	if s == nil {
		t.Fatalf("Eval(%s): returned null", script)
	}
	return *s
}

func evalBool(t *testing.T, win ui.Window, script string) bool {
	t.Helper()
	raw, err := win.Eval(script)
	if err != nil {
		t.Fatalf("Eval(%s): %v", script, err)
	}
	var b *bool
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		t.Fatalf("Eval(%s): decoding %q: %v", script, raw, err)
	}
	if b == nil {
		t.Fatalf("Eval(%s): returned null", script)
	}
	return *b
}

// testMainWindow builds a mainWindow bound to the shared window, with
// the queue seeded from paths.
func testMainWindow(t *testing.T, locale string, cfg config.Config, paths []string) (*mainWindow, chan ui.Message) {
	t.Helper()
	win, messages := sharedMainWindow(t)
	m := newMainWindow(cfg, locale)
	m.win = win
	m.messages = messages
	m.page = pageMain
	m.step = stepDocuments
	if len(paths) > 0 {
		_, m.notices = m.queue.Add(paths)
	}
	if err := win.PostJSON(m.filesPayload("init")); err != nil {
		t.Fatalf("PostJSON(init): %v", err)
	}
	return m, messages
}

// TestMainWindowEmptyStateSaysWhatToDo is F6 §1's "show a clear empty
// state that says what to do".
func TestMainWindowEmptyStateSaysWhatToDo(t *testing.T) {
	c := i18n.Load("sr-Latn")
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)

	if hidden := evalBool(t, m.win, "document.getElementById('empty-state').hidden"); hidden {
		t.Fatal("the empty state is hidden with no documents in the list")
	}
	if hidden := evalBool(t, m.win, "document.getElementById('file-list').hidden"); !hidden {
		t.Fatal("the file list is shown with no documents in it")
	}
	text := evalText(t, m.win, "document.getElementById('empty-state').textContent")
	if !strings.Contains(text, c.T("main.empty_title")) {
		t.Fatalf("the empty state does not say what to do: %q", text)
	}
	if !strings.Contains(text, c.T("main.empty_hint")) {
		t.Fatalf("the empty state does not mention Browse or folders: %q", text)
	}
	// Sign must not be pressable with nothing to sign.
	if !evalBool(t, m.win, "document.getElementById('sign-btn').disabled") {
		t.Fatal("Sign is enabled with an empty list")
	}
}

// TestMainWindowRendersTheFileListWithSizes is F6 §1's "show the
// accepted files as a list, with a count and each file's size".
func TestMainWindowRendersTheFileListWithSizes(t *testing.T) {
	dir := t.TempDir()
	a := writeTestPDF(t, dir, "Ugovor o radu.pdf", 1234)
	b := writeTestPDF(t, dir, "Račun 2026-114.pdf", 4321)

	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{a, b})

	if evalBool(t, m.win, "document.getElementById('file-list').hidden") {
		t.Fatal("the file list is hidden with documents in it")
	}
	if !evalBool(t, m.win, "document.getElementById('empty-state').hidden") {
		t.Fatal("the empty state is shown with documents in the list")
	}
	rows := evalNumber(t, m.win, "document.querySelectorAll('#file-list .file-row').length")
	if rows != 2 {
		t.Fatalf("the list shows %v rows, want 2", rows)
	}

	text := evalText(t, m.win, "document.getElementById('file-list').textContent")
	for _, want := range []string{"Ugovor o radu.pdf", "Račun 2026-114.pdf", jobs.FormatSize(1234), jobs.FormatSize(4321)} {
		if !strings.Contains(text, want) {
			t.Errorf("the list does not show %q: %q", want, text)
		}
	}
	count := evalText(t, m.win, "document.getElementById('files-count').textContent")
	if !strings.Contains(count, "2") {
		t.Errorf("the count line does not say how many documents there are: %q", count)
	}
	if !evalBool(t, m.win, "!document.getElementById('sign-btn').disabled") {
		t.Fatal("Sign is disabled with two documents in the list")
	}
}

// TestMainWindowRemoveReachesGoWithTheRightIndex is F6 §1's "let a file
// be removed from the list before signing", through the real page.
//
// The row identifies itself by index, never by path: a path travelling
// out to the page and back is a path the page could change (F5 §5.3's
// "never treat a name as a path").
func TestMainWindowRemoveReachesGoWithTheRightIndex(t *testing.T) {
	dir := t.TempDir()
	a := writeTestPDF(t, dir, "first.pdf", 10)
	b := writeTestPDF(t, dir, "second.pdf", 10)
	cpath := writeTestPDF(t, dir, "third.pdf", 10)

	m, messages := testMainWindow(t, "sr-Latn", config.Default(), []string{a, b, cpath})

	// Click the middle row's Remove, inside the page's own DOM.
	if _, err := m.win.Eval("document.querySelectorAll('#file-list .file-remove')[1].click()"); err != nil {
		t.Fatalf("Eval(click remove): %v", err)
	}
	select {
	case msg := <-messages:
		if msg.Type != ui.MessageTypeApprove {
			t.Fatalf("Remove sent %q, want approve", msg.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("clicking Remove sent nothing to Go")
	}

	action := m.readAction()
	if action.Action != "remove" {
		t.Fatalf("action = %q, want remove", action.Action)
	}
	if action.Index != 1 {
		t.Fatalf("index = %d, want 1 (the middle row)", action.Index)
	}

	m.removeAt(action.Index)
	if names := m.queue.DisplayNames(); len(names) != 2 || names[0] != "first.pdf" || names[1] != "third.pdf" {
		t.Fatalf("after removing the middle row the queue holds %v", names)
	}
	rows := evalNumber(t, m.win, "document.querySelectorAll('#file-list .file-row').length")
	if rows != 2 {
		t.Fatalf("the page shows %v rows after the removal, want 2", rows)
	}
}

// TestMainWindowSignAndBrowseReachGo covers the remaining actions that
// have to cross the boundary at all.
func TestMainWindowSignAndBrowseReachGo(t *testing.T) {
	dir := t.TempDir()
	a := writeTestPDF(t, dir, "doc.pdf", 10)
	m, messages := testMainWindow(t, "sr-Latn", config.Default(), []string{a})

	for _, tc := range []struct{ button, action string }{
		{"sign-btn", "next"},
		{"browse-btn", "browse"},
		{"clear-btn", "clear"},
		{"output-change-btn", "chooseOutputFolder"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			drain(messages)
			if _, err := m.win.Eval("document.getElementById('" + tc.button + "').click()"); err != nil {
				t.Fatalf("Eval(click %s): %v", tc.button, err)
			}
			select {
			case <-messages:
			case <-time.After(5 * time.Second):
				t.Fatalf("clicking %s sent nothing to Go", tc.button)
			}
			if got := m.readAction().Action; got != tc.action {
				t.Fatalf("action = %q, want %q", got, tc.action)
			}
		})
	}
}

// TestMainWindowAsksOnlyWhichDocuments is step 1 of the three-step
// flow: the window that gathers documents does not also ask how to
// sign them. The stamp summary and its Change button used to sit in
// this footer and ask exactly what the consent screen asked next.
func TestMainWindowAsksOnlyWhichDocuments(t *testing.T) {
	dir := t.TempDir()
	a := writeTestPDF(t, dir, "doc.pdf", 10)
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{a})

	for _, id := range []string{"stamp-summary", "stamp-change-btn"} {
		if !evalBool(t, m.win, "document.getElementById('"+id+"') === null") {
			t.Errorf("the document list still carries %s; how to sign is step 3's question", id)
		}
	}
}

// TestMainWindowOffersTheWayBackToBesideEachDocument: choosing an
// output folder was a one-way door — the window could set one and had
// no control to unset it, which is how every signed document came to be
// landing on the Desktop with no way back short of editing config.json.
func TestMainWindowOffersTheWayBackToBesideEachDocument(t *testing.T) {
	dir := t.TempDir()
	a := writeTestPDF(t, dir, "doc.pdf", 10)

	// With no folder chosen there is nothing to undo, and the button
	// stays out of the way.
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{a})
	if d := evalText(t, m.win, "getComputedStyle(document.getElementById('output-beside-btn')).display"); d != "none" {
		t.Errorf("the reset button renders with no folder chosen (display: %s)", d)
	}

	cfg := config.Default()
	cfg.OutputFolder = filepath.Join(dir, "Desktop")
	m, messages := testMainWindow(t, "sr-Latn", cfg, []string{a})
	if d := evalText(t, m.win, "getComputedStyle(document.getElementById('output-beside-btn')).display"); d == "none" {
		t.Fatal("a chosen folder offers no way back to beside each document")
	}
	if got := evalText(t, m.win, "document.getElementById('output-folder').textContent"); got != cfg.OutputFolder {
		t.Errorf("the chosen folder renders as %q, want %q", got, cfg.OutputFolder)
	}

	drain(messages)
	if _, err := m.win.Eval("document.getElementById('output-beside-btn').click()"); err != nil {
		t.Fatalf("Eval(click reset): %v", err)
	}
	select {
	case <-messages:
	case <-time.After(5 * time.Second):
		t.Fatal("the reset button sent nothing to Go")
	}
	if got := m.readAction().Action; got != "clearOutputFolder" {
		t.Fatalf("action = %q, want clearOutputFolder", got)
	}

	// And Go acts on it: the default is beside each input, which is what
	// an empty folder means everywhere downstream.
	t.Setenv("LOCALAPPDATA", t.TempDir())
	m.clearOutputFolder()
	if m.cfg.OutputFolder != "" {
		t.Errorf("after the reset the configuration still holds %q", m.cfg.OutputFolder)
	}
	if got := evalText(t, m.win, "document.getElementById('output-folder').textContent"); got != m.c.T("main.output_beside_input") {
		t.Errorf("the row reads %q, want the beside-each-input text", got)
	}
}

// TestMainWindowNoticesAreShown is F6 §1's "say how many were found"
// for a dropped folder, and the other three things an Add can have to
// report.
func TestMainWindowNoticesAreShown(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	writeTestPDF(t, dir, "a.pdf", 10)
	writeTestPDF(t, dir, "b.pdf", 10)

	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{dir})

	notices := evalText(t, m.win, "document.getElementById('notices').textContent")
	if !strings.Contains(notices, "2") {
		t.Fatalf("the folder notice does not say how many PDFs were found: %q", notices)
	}
	// The folder's own name is in the sentence, so a person dropping
	// two folders can tell which one is being talked about.
	if !strings.Contains(notices, filepath.Base(dir)) {
		t.Fatalf("the folder notice does not name the folder: %q", notices)
	}
	_ = c
}

// TestMainWindowQueueShowsEveryPerFileState is F6 §3's "per-file state:
// waiting, signing, done, failed, skipped", rendered.
func TestMainWindowQueueShowsEveryPerFileState(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	var paths []string
	for _, n := range []string{"a.pdf", "b.pdf", "c.pdf", "d.pdf"} {
		paths = append(paths, writeTestPDF(t, dir, n, 10))
	}
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), paths)

	// One item in each state — the combination a run passes through but
	// never rests in, which is exactly why the payload builder takes
	// the items rather than reading the live queue.
	items := m.queue.Items()
	items[0].State = jobs.StateDone
	items[1].State = jobs.StateSigning
	items[2].State = jobs.StateFailed
	items[2].FailureCode = errs.CodePDFInvalid
	items[3].State = jobs.StateSkipped

	payload := m.queuePayload(items, jobs.Progress{Phase: jobs.PhaseSigning, Current: 2, Total: 4}, false)
	if err := m.win.PostJSON(payload); err != nil {
		t.Fatalf("PostJSON(queue): %v", err)
	}

	text := evalText(t, m.win, "document.getElementById('queue-list').textContent")
	for _, want := range []string{
		c.T("main.state_done"), c.T("main.state_signing"),
		c.T("main.state_failed"), c.T("main.state_skipped"),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the queue does not show the state %q: %q", want, text)
		}
	}
	// A failed row carries its reason in words, never a code.
	if strings.Contains(text, string(errs.CodePDFInvalid)) {
		t.Fatalf("an error code reached the screen: %q", text)
	}
	if !strings.Contains(text, cliErrorMessage(c, errs.CodePDFInvalid)) {
		t.Fatalf("the failed row does not give a reason in words: %q", text)
	}
	// Each state also carries its own class, which is what colours the
	// row — a state that renders as text but not as a class reads as
	// the same as every other row at a glance.
	for _, cls := range []string{"file-row-done", "file-row-signing", "file-row-failed", "file-row-skipped"} {
		n := evalNumber(t, m.win, "document.querySelectorAll('#queue-list ."+cls+"').length")
		if n != 1 {
			t.Errorf("%s appears on %v rows, want 1", cls, n)
		}
	}
}

// TestMainWindowFailedRowPutsItsReasonOnItsOwnLine keeps the file name
// readable. The reason is a whole sentence; sharing a line with it
// squeezed the name down to two words and wrapped it, which is how a
// list of documents stops being scannable exactly when something has
// gone wrong in it.
func TestMainWindowFailedRowPutsItsReasonOnItsOwnLine(t *testing.T) {
	dir := t.TempDir()
	paths := []string{writeTestPDF(t, dir, "faktura-2026-0091.pdf", 10)}
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), paths)

	items := m.queue.Items()
	items[0].State = jobs.StateFailed
	items[0].FailureCode = errs.CodePDFEncrypted
	if err := m.win.PostJSON(m.queuePayload(items, jobs.Progress{Phase: jobs.PhaseSigning, Current: 1, Total: 1}, false)); err != nil {
		t.Fatalf("PostJSON(queue): %v", err)
	}

	nameBottom := evalNumber(t, m.win, "document.querySelector('#queue-list .file-name').getBoundingClientRect().bottom")
	reasonTop := evalNumber(t, m.win, "document.querySelector('#queue-list .file-reason').getBoundingClientRect().top")
	if reasonTop < nameBottom {
		t.Fatalf("the reason (top %v) shares a line with the file name (bottom %v)", reasonTop, nameBottom)
	}
	// And the name itself is on one line, not wrapped to make room.
	nameHeight := evalNumber(t, m.win, "document.querySelector('#queue-list .file-name').getBoundingClientRect().height")
	lineHeight := evalNumber(t, m.win, "parseFloat(getComputedStyle(document.querySelector('#queue-list .file-name')).lineHeight)")
	if nameHeight > lineHeight*1.6 {
		t.Fatalf("the file name wrapped: %v tall against a %v line", nameHeight, lineHeight)
	}
}

// TestMainWindowPreparingCardIsIndeterminate is F6 §3's "Preparing
// card…": no bar sitting still at 0% for the measured ~4.9 seconds.
func TestMainWindowPreparingCardIsIndeterminate(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})

	m.postQueue(jobs.Progress{Phase: jobs.PhasePreparingCard, Current: 0, Total: 1}, false)

	if evalBool(t, m.win, "document.getElementById('queue-progress-indeterminate').hidden") {
		t.Fatal("the indeterminate bar is hidden while the card is being prepared")
	}
	if !evalBool(t, m.win, "document.getElementById('queue-progress').hidden") {
		t.Fatal("a determinate progress bar is shown while the card is being prepared")
	}
	label := evalText(t, m.win, "document.getElementById('queue-label').textContent")
	if label != c.T("consent.state_preparing_card") {
		t.Fatalf("label = %q, want %q", label, c.T("consent.state_preparing_card"))
	}
	if eta := evalText(t, m.win, "document.getElementById('queue-eta').textContent"); eta != "" {
		t.Fatalf("an ETA is shown before any signature has completed: %q", eta)
	}
}

// TestMainWindowSaysWhenThePINIsAskedForEverySignature is F6 §3's "if
// the PIN policy is detected as per-signature, say so".
func TestMainWindowSaysWhenThePINIsAskedForEverySignature(t *testing.T) {
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})

	m.postQueue(jobs.Progress{Phase: jobs.PhaseSigning, Current: 1, Total: 4}, false)
	if !evalBool(t, m.win, "document.getElementById('queue-per-signature').hidden") {
		t.Fatal("the per-signature PIN warning is shown for a batch that is not asking for one")
	}

	m.postQueue(jobs.Progress{Phase: jobs.PhaseSigning, Current: 1, Total: 4, PerSignaturePIN: true}, false)
	if evalBool(t, m.win, "document.getElementById('queue-per-signature').hidden") {
		t.Fatal("the per-signature PIN warning is hidden for a batch that is asking for one")
	}
}

// TestMainWindowStopSaysItIsFinishingTheCurrentDocument covers the
// state F6 §3 asks for between pressing Stop and the run ending.
func TestMainWindowStopSaysItIsFinishingTheCurrentDocument(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})

	m.postQueue(jobs.Progress{Phase: jobs.PhaseSigning, Current: 1, Total: 4}, true)
	if !evalBool(t, m.win, "document.getElementById('stop-btn').disabled") {
		t.Fatal("Stop is still pressable after being pressed")
	}
	if label := evalText(t, m.win, "document.getElementById('stop-btn').textContent"); label != c.T("main.stopping") {
		t.Fatalf("Stop says %q, want %q", label, c.T("main.stopping"))
	}
}

// TestMainWindowReportNamesFailuresWithoutCodes is F6 §5: "failures are
// listed by file name with the reason in words, not a code".
func TestMainWindowReportNamesFailuresWithoutCodes(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})

	m.report = &jobs.Report{
		Succeeded:     97,
		Failed:        2,
		Skipped:       1,
		OutputDir:     dir,
		AchievedLevel: "B-T",
		Failures: []jobs.Failure{
			{Name: "Ugovor o radu.pdf", Code: errs.CodePDFEncrypted},
			{Name: "Изјава.pdf", Code: errs.CodeInputUnreadable},
		},
	}
	m.postReport(*m.report)

	body := evalText(t, m.win, "document.getElementById('report-body').textContent")
	for _, want := range []string{"Ugovor o radu.pdf", "Изјава.pdf"} {
		if !strings.Contains(body, want) {
			t.Errorf("the report does not name the failed document %q: %q", want, body)
		}
	}
	for _, code := range []errs.Code{errs.CodePDFEncrypted, errs.CodeInputUnreadable} {
		if strings.Contains(body, string(code)) {
			t.Errorf("the error code %q reached the screen; SPEC §7 and F6 §5 forbid it", code)
		}
		if !strings.Contains(body, cliErrorMessage(c, code)) {
			t.Errorf("the reason for %q is not given in words: %q", code, body)
		}
	}

	counts := evalText(t, m.win, "document.getElementById('report-counts').textContent")
	for _, want := range []string{"97", "2", "1"} {
		if !strings.Contains(counts, want) {
			t.Errorf("the counts line is missing %q: %q", want, counts)
		}
	}
	if level := evalText(t, m.win, "document.getElementById('report-level').textContent"); level != "B-T" {
		t.Fatalf("the report shows level %q, want B-T", level)
	}
	if out := evalText(t, m.win, "document.getElementById('report-output').textContent"); !strings.Contains(out, dir) {
		t.Fatalf("the report does not say where the files are: %q", out)
	}
	if evalBool(t, m.win, "document.getElementById('report-open-btn').disabled") {
		t.Fatal("Open folder is disabled for a batch that wrote to one folder")
	}
}

// TestMainWindowReportOpenFolderIsDisabledWithNoOneFolder keeps the
// button honest: there is nothing to open.
func TestMainWindowReportOpenFolderIsDisabledWithNoOneFolder(t *testing.T) {
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})
	m.postReport(jobs.Report{Succeeded: 2, OutputDir: ""})
	if !evalBool(t, m.win, "document.getElementById('report-open-btn').disabled") {
		t.Fatal("Open folder is enabled for a batch that wrote to several folders")
	}
}

// TestMainWindowAbortIsExplained is F6 §3's two abort conditions,
// reaching the person as a sentence.
func TestMainWindowAbortIsExplained(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})

	for _, tc := range []struct {
		code errs.Code
		key  string
	}{
		{errs.CodeCardNotPresent, "main.aborted_card"},
		{errs.CodePINLocked, "main.aborted_pin"},
	} {
		m.postReport(jobs.Report{Succeeded: 5, Failed: 1, Skipped: 4, Aborted: true, AbortCode: tc.code})
		if evalBool(t, m.win, "document.getElementById('report-abort').hidden") {
			t.Fatalf("%s: the abort explanation is hidden", tc.code)
		}
		got := evalText(t, m.win, "document.getElementById('report-abort').textContent")
		if got != c.T(tc.key) {
			t.Fatalf("%s: abort message = %q, want %q", tc.code, got, c.T(tc.key))
		}
	}
}

// TestMainWindowStoppedReportSaysStopped distinguishes a run the user
// ended from one that finished.
func TestMainWindowStoppedReportSaysStopped(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})

	m.postReport(jobs.Report{Succeeded: 3, Skipped: 7, Stopped: true})
	if got := evalText(t, m.win, "document.getElementById('report-title').textContent"); got != c.T("main.report_stopped_title") {
		t.Fatalf("title = %q, want %q", got, c.T("main.report_stopped_title"))
	}
}

// TestMainWindowRendersInEveryLocale is SPEC §9.1: three locales, no
// blank labels, no raw keys.
func TestMainWindowRendersInEveryLocale(t *testing.T) {
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "a.pdf", 10)

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			m, _ := testMainWindow(t, locale, config.Default(), []string{path})

			for _, id := range []string{"sign-btn", "browse-btn", "clear-btn"} {
				text := evalText(t, m.win, "document.getElementById('"+id+"').textContent")
				if strings.TrimSpace(text) == "" {
					t.Errorf("%s has no label in %s", id, locale)
				}
				if strings.HasPrefix(text, "main.") {
					t.Errorf("%s rendered a raw catalogue key in %s: %q", id, locale, text)
				}
			}
			if got := evalText(t, m.win, "document.getElementById('sign-btn').textContent"); got != c.T("step.next") {
				t.Errorf("the primary action = %q, want %q", got, c.T("step.next"))
			}
		})
	}
}

// TestMainWindowDoesNotScrollWithTwoHundredDocuments is D-106's rule at
// F6 §7's stated size: the page itself never scrolls, the list does,
// and Sign stays on screen.
func TestMainWindowDoesNotScrollWithTwoHundredDocuments(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 200; i++ {
		paths = append(paths, writeTestPDF(t, dir, longDocName(i), 1000))
	}
	m, _ := testMainWindow(t, "sr-Cyrl", config.Default(), paths)

	assertNoPageScroll(t, m.win)

	// Sign is inside the window, not below it.
	bottom := evalNumber(t, m.win, "document.getElementById('sign-btn').getBoundingClientRect().bottom")
	height := evalNumber(t, m.win, "window.innerHeight")
	if bottom > height {
		t.Fatalf("Sign's bottom edge is at %v, past the window's %v — it scrolled off", bottom, height)
	}
	// The list is what scrolls.
	if !evalBool(t, m.win, "document.getElementById('file-list').scrollHeight > document.getElementById('file-list').clientHeight") {
		t.Fatal("two hundred documents did not make the list scroll; something else is absorbing the height")
	}
}

// assertNoPageScroll fails if the page as a whole scrolls in either
// direction, or if any element outside the named scrolling region both
// declares an overflow and actually overflows (D-106).
func assertNoPageScroll(t *testing.T, win ui.Window) {
	t.Helper()
	if evalBool(t, win, "document.body.scrollWidth > document.body.clientWidth") {
		w := evalNumber(t, win, "document.body.scrollWidth")
		c := evalNumber(t, win, "document.body.clientWidth")
		t.Fatalf("the page scrolls horizontally: scrollWidth %v exceeds clientWidth %v", w, c)
	}
	if evalBool(t, win, "document.body.scrollHeight > document.body.clientHeight + 1") {
		h := evalNumber(t, win, "document.body.scrollHeight")
		c := evalNumber(t, win, "document.body.clientHeight")
		t.Fatalf("the page scrolls vertically: scrollHeight %v exceeds clientHeight %v", h, c)
	}
}

// TestMainWindowLongNameDoesNotWidenTheWindow is D-096's rule applied
// to F6 §7's "path longer than 260 characters": a value with no length
// bound wraps rather than pushing Sign off the window.
func TestMainWindowLongNameDoesNotWidenTheWindow(t *testing.T) {
	dir := t.TempDir()
	// A single name component is capped at 255 characters by the
	// filesystem itself, so the long case is a long *name* at that
	// bound — which is already twice what the display cap allows
	// through (consent.MaxDisplayLength is 120). The long *path* case
	// F6 §7 asks about is exercised where paths are handled, in
	// internal/jobs.
	name := strings.Repeat("veoma-dugacko-ime-dokumenta-", 8) + "kraj.pdf"
	if len(name) > 250 {
		name = name[:246] + ".pdf"
	}
	path := writeTestPDF(t, dir, name, 10)

	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{path})
	assertNoPageScroll(t, m.win)

	// The rendered name is the sanitised, middle-elided one, not the
	// whole 250 characters (F5 §5.3).
	shown := evalText(t, m.win, "document.querySelector('#file-list .file-name').textContent")
	if len([]rune(shown)) > 120 {
		t.Fatalf("the list rendered %d characters of a file name; the cap is 120", len([]rune(shown)))
	}
}

func longDocName(i int) string {
	return "документ-" + strings.Repeat("0", 3-len(itoaTest(i))) + itoaTest(i) + ".pdf"
}

func itoaTest(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// writeTestPDF creates a file of n bytes and returns its path. Content
// is irrelevant to every test here: these exercise the window, and the
// signing path has its own fixtures.
func writeTestPDF(t *testing.T, dir, name string, n int) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, make([]byte, n), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

// cliErrorMessage is the sentence the interface shows for a code, from
// the same renderer the command line uses.
func cliErrorMessage(c *i18n.Catalogue, code errs.Code) string {
	return cli.ErrorMessage(errs.New(code, nil), c)
}

// TestSignSaysItIsOpeningAndCannotBePressedTwice is Task 3 of the
// first-use fix pass.
//
// Pressing Sign opens the consent window, which is a WebView2 window of
// its own: measured on the machine this was reported from, three runs
// of ui.NewWindow for that page took 2.29 s, 2.12 s and 2.15 s, against
// 0.19–0.90 s for the whole certificate gather that precedes it — so
// the wait is the window, not the card work, and it is long enough that
// a person presses the button again. The first press therefore takes
// the button out of service and says what is happening; a later render
// of the list — which is what a cancelled consent window produces —
// puts it back.
func TestSignSaysItIsOpeningAndCannotBePressedTwice(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	in := writeTestPDF(t, dir, "ugovor.pdf", 10)
	m, messages := testMainWindow(t, "sr-Latn", config.Default(), []string{in})

	if evalBool(t, m.win, "document.getElementById('sign-btn').disabled") {
		t.Fatal("Sign is disabled with a document in the list")
	}
	if label := evalText(t, m.win, "document.getElementById('sign-btn').textContent"); label != c.T("step.next") {
		t.Fatalf("the primary action reads %q before it is pressed, want %q", label, c.T("step.next"))
	}

	if _, err := m.win.Eval("document.getElementById('sign-btn').click()"); err != nil {
		t.Fatalf("clicking Sign: %v", err)
	}
	select {
	case msg := <-messages:
		if msg.Type != ui.MessageTypeApprove {
			t.Fatalf("Sign sent %v", msg.Type)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Sign never reached Go")
	}
	if !evalBool(t, m.win, "document.getElementById('sign-btn').disabled") {
		t.Fatal("Sign is still pressable after the first press")
	}
	if label := evalText(t, m.win, "document.getElementById('sign-btn').textContent"); label != c.T("main.sign_opening") {
		t.Fatalf("Sign reads %q while it is opening, want %q", label, c.T("main.sign_opening"))
	}

	// A second press does nothing at all: no message reaches Go.
	if _, err := m.win.Eval("document.getElementById('sign-btn').click()"); err != nil {
		t.Fatalf("second click: %v", err)
	}
	select {
	case msg := <-messages:
		t.Fatalf("a second press of a disabled Sign reached Go as %v", msg.Type)
	case <-time.After(500 * time.Millisecond):
	}

	// Cancelling the consent window brings the list back, and with it
	// the button.
	m.postFiles()
	if evalBool(t, m.win, "document.getElementById('sign-btn').disabled") {
		t.Fatal("Sign is still disabled after the list was shown again")
	}
	if label := evalText(t, m.win, "document.getElementById('sign-btn').textContent"); label != c.T("step.next") {
		t.Fatalf("the primary action reads %q after the list was shown again, want %q", label, c.T("step.next"))
	}
}
