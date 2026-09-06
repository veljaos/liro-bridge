//go:build windows

package main

// Task 5, from the outside: after a chain has been broken and continued,
// what the report screen says, what the export says, and what the audit
// log window shows.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

// brokenAndContinuedStore seeds a log, truncates its last line as a
// power cut would, and records one more batch — which starts a new chain
// beside the broken file. It returns the store, the broken file's path,
// and its bytes as they stood after the truncation.
func brokenAndContinuedStore(t *testing.T) (store *audit.Store, brokenFile string, brokenBytes []byte) {
	t.Helper()
	dir, file := seedAuditLog(t, 5)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, raw[:len(raw)-30], 0o600); err != nil {
		t.Fatal(err)
	}
	brokenBytes, err = os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	store, err = audit.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d := recordInteractiveAudit(store, nil, "AB12AB12", 2, audit.OutcomeApproved, nil, false, "B-T"); d == nil {
		t.Fatal("the seeded break did not start a new chain")
	}
	return store, file, brokenBytes
}

// TestTheReportSaysTheLogContinuedInANewFile is the notice itself: on
// the report screen, once, as a notice rather than an error, naming the
// file the log continued in and where it is.
func TestTheReportSaysTheLogContinuedInANewFile(t *testing.T) {
	for _, locale := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			store, brokenFile, _ := brokenAndContinuedStore(t)

			d := &audit.Discontinuity{
				PreviousChain:   1,
				PreviousFile:    filepath.Base(brokenFile),
				LastSequence:    3,
				HasLastSequence: true,
				Line:            5,
				Reason:          audit.BreakUnparseable,
			}
			notice := auditChainNotice(c, store, d)
			if notice == "" {
				t.Fatal("no notice was produced")
			}
			if strings.Contains(notice, "%") || strings.Contains(notice, "chain_continued") {
				t.Fatalf("the notice is not a finished sentence: %q", notice)
			}
			if !strings.Contains(notice, filepath.Base(brokenFile)) {
				t.Errorf("the notice does not name the file that broke: %q", notice)
			}
			newFile, err := store.LatestChainFile()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(notice, newFile) {
				t.Errorf("the notice does not name the file the log continued in (%s): %q", newFile, notice)
			}
			if !strings.Contains(notice, store.Dir()) {
				t.Errorf("the notice does not say where the log is: %q", notice)
			}
			// Nothing at all when there is nothing to say.
			if got := auditChainNotice(c, store, nil); got != "" {
				t.Errorf("a batch with no break produced %q", got)
			}
		})
	}
}

// emptyReportForTest is a finished run with nothing remarkable in it,
// so a test can post the report screen and look at one line of it.
func emptyReportForTest() jobs.Report {
	return jobs.Report{Succeeded: 1, AchievedLevel: "B-T"}
}

// TestTheReportScreenShowsTheNoticeOnce drives the real window: the
// notice appears on the report screen, in the caution family rather than
// as an error, and a report with no notice shows nothing.
func TestTheReportScreenShowsTheNoticeOnce(t *testing.T) {
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)

	m.auditNotice = ""
	m.postReport(emptyReportForTest())
	if hidden := evalString(t, m.win, "String(document.getElementById('report-audit-notice').hidden)"); hidden != "true" {
		t.Error("the audit notice is shown on a report that has none")
	}

	m.auditNotice = "Dnevnik revizije je nastavljen u novom fajlu."
	m.postReport(emptyReportForTest())
	if hidden := evalString(t, m.win, "String(document.getElementById('report-audit-notice').hidden)"); hidden != "false" {
		t.Fatal("the audit notice did not appear on the report screen")
	}
	text := evalString(t, m.win, "document.getElementById('report-audit-notice').textContent")
	if !strings.Contains(text, "novom fajlu") {
		t.Errorf("the notice on screen is %q", text)
	}
	if h := evalString(t, m.win, "String(document.getElementById('report-audit-notice').getBoundingClientRect().height)"); h == "0" {
		t.Error("the notice has no rendered height")
	}
	class := evalString(t, m.win, "document.getElementById('report-audit-notice').className")
	if !strings.Contains(class, "liro-outcome-caution") {
		t.Errorf("the notice is not in the caution family: %q — it is a notice, not an error", class)
	}
}

// TestTheExportSaysHowManyChainsAndWhereTheyBroke is the verification
// surface: "three chains, each intact, breaks…" rather than one verdict
// over a log that is now in two pieces.
func TestTheExportSaysHowManyChainsAndWhereTheyBroke(t *testing.T) {
	store, _, _ := brokenAndContinuedStore(t)
	dir := t.TempDir()
	entriesPath := filepath.Join(dir, "liro-audit.jsonl")
	reportPath := filepath.Join(dir, "liro-audit-report.json")
	report, _ := store.Export(entriesPath, reportPath)

	for _, locale := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		c := i18n.Load(locale)
		summary := chainsSummary(c, report)
		if summary == "" {
			t.Fatalf("%s: a log holding two chains produced no summary", locale)
		}
		if !strings.Contains(summary, "2") {
			t.Errorf("%s: the summary does not say how many chains: %q", locale, summary)
		}
		if strings.Contains(summary, "export_chains") || strings.Contains(summary, "%") {
			t.Errorf("%s: %q is not a finished sentence", locale, summary)
		}
		// The break is named, with a date and a reason.
		if !strings.Contains(summary, time.Now().Format("02.01.2006.")) {
			t.Errorf("%s: the summary does not date the break: %q", locale, summary)
		}
		lines := exportedFileLines(c, "liro-audit.jsonl", "liro-audit-report.json", report)
		if len(lines) != 2 {
			t.Fatalf("%s: %d file lines, want 2", locale, len(lines))
		}
		if !strings.Contains(lines[1].Detail, summary) {
			t.Errorf("%s: the report file's line does not carry the chain summary: %q", locale, lines[1].Detail)
		}
	}
}

// TestAnOrdinaryExportSaysNothingAboutChains: a log that has never been
// interrupted holds one chain, and saying so every time is noise.
func TestAnOrdinaryExportSaysNothingAboutChains(t *testing.T) {
	dir, _ := seedAuditLog(t, 3)
	store, err := audit.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	report, exportErr := store.Export(filepath.Join(out, "e.jsonl"), filepath.Join(out, "r.json"))
	if exportErr != nil {
		t.Fatalf("Export of an intact log: %v", exportErr)
	}
	if got := chainsSummary(i18n.Load("en"), report); got != "" {
		t.Errorf("an intact one-chain log produced %q", got)
	}
}

// TestTheAuditWindowShowsWhereTheChainRestarted: this window is where a
// person looks at the log, so a gap between two chains has to be legible
// in it.
func TestTheAuditWindowShowsWhereTheChainRestarted(t *testing.T) {
	store, brokenFile, _ := brokenAndContinuedStore(t)
	entries, err := store.All()
	if err != nil {
		t.Fatal(err)
	}
	c := i18n.Load("sr-Latn")
	init := buildAuditLogInit(c, entries)
	model, ok := init["model"].(map[string]any)
	if !ok {
		t.Fatal("the payload has no model")
	}
	js, ok := model["entries"].([]jsAuditEntry)
	if !ok {
		t.Fatalf("the payload's entries are %T", model["entries"])
	}

	breaks := 0
	for _, e := range js {
		if e.ChainBreakText == "" {
			continue
		}
		breaks++
		if !strings.Contains(e.ChainBreakText, filepath.Base(brokenFile)) {
			t.Errorf("the break line does not name the file that broke: %q", e.ChainBreakText)
		}
		if strings.Contains(e.ChainBreakText, "%") || strings.Contains(e.ChainBreakText, "chain_break") {
			t.Errorf("the break line is not a finished sentence: %q", e.ChainBreakText)
		}
	}
	if breaks != 1 {
		t.Fatalf("%d entries carry a break line, want exactly 1", breaks)
	}
}

// TestTheBrokenFileIsNeverTouchedByAnythingTheWindowDoes: reading the
// log, exporting it and signing again must all leave the evidence
// exactly as it is. Never overwrite, truncate or delete a broken chain
// file.
func TestTheBrokenFileIsNeverTouchedByAnythingTheWindowDoes(t *testing.T) {
	store, brokenFile, brokenBytes := brokenAndContinuedStore(t)

	if _, err := store.All(); err != nil {
		t.Fatalf("All: %v", err)
	}
	if _, err := store.Verify(); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	out := t.TempDir()
	_, _ = store.Export(filepath.Join(out, "e.jsonl"), filepath.Join(out, "r.json"))
	recordInteractiveAudit(store, nil, "AB12AB12", 1, audit.OutcomeApproved, nil, false, "B-T")

	after, err := os.ReadFile(brokenFile)
	if err != nil {
		t.Fatalf("the broken file is gone: %v", err)
	}
	if !bytes.Equal(brokenBytes, after) {
		t.Fatal("the broken chain's file was modified")
	}
}

// TestTheChainBreakLineWrapsRatherThanWideningTheWindow is the defect
// this line actually had, found by running the built binary and looking
// at it rather than by any assertion: the sentence carried
// `.liro-outcome`, whose `white-space: nowrap` and right alignment are
// for the one-word outcome badge on the right of a row. On screen it
// became a single line the entry list had to scroll sideways to show,
// with most of the sentence off the edge.
//
// D-096's rule, a third time: a value with no length bound wraps, it
// does not widen its container.
func TestTheChainBreakLineWrapsRatherThanWideningTheWindow(t *testing.T) {
	for _, locale := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win := sharedAuditLogWindow(t)

			entries := twentyAuditEntries()
			entries[3].Discontinuity = &audit.Discontinuity{
				PreviousChain:   1,
				PreviousFile:    "2026-09-001.jsonl",
				LastSequence:    4,
				HasLastSequence: true,
				Line:            5,
				Reason:          audit.BreakUnparseable,
			}
			if err := win.PostJSON(buildAuditLogInit(c, entries)); err != nil {
				t.Fatalf("PostJSON: %v", err)
			}

			if got := evalNumber(t, win, "document.querySelectorAll('.audit-chain-break').length"); got != 1 {
				t.Fatalf("%v break lines rendered, want 1", got)
			}
			// The entry list may scroll down. It must never scroll
			// sideways: nothing in this window is wide content.
			scrollW := evalNumber(t, win, "document.getElementById('entry-list').scrollWidth")
			clientW := evalNumber(t, win, "document.getElementById('entry-list').clientWidth")
			if scrollW > clientW+1 {
				t.Errorf("the entry list scrolls sideways: scrollWidth %v exceeds clientWidth %v", scrollW, clientW)
			}
			// And the sentence is actually on screen rather than cut
			// off at the right edge.
			right := evalNumber(t, win, "document.querySelector('.audit-chain-break').getBoundingClientRect().right")
			innerW := evalNumber(t, win, "window.innerWidth")
			if right > innerW+0.5 {
				t.Errorf("the break line runs past the window's right edge: %v > %v", right, innerW)
			}
			if ws := evalString(t, win, "getComputedStyle(document.querySelector('.audit-chain-break')).whiteSpace"); ws == "nowrap" {
				t.Error("the break line is nowrap, so a long sentence cannot wrap")
			}
		})
	}
}

// TestTheReportNoticeWrapsToo is the same check for the report screen's
// own notice, which names two file names and a folder path and so is
// the longest sentence either screen carries.
func TestTheReportNoticeWrapsToo(t *testing.T) {
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), nil)
	m.auditNotice = "Dnevnik revizije nije mogao da se nastavi u 2026-09-001.jsonl, pa je nastavljen u novom fajlu, 2026-09-001.c2.jsonl, u C:/Users/Veljko/AppData/Local/Liro/audit. Stari fajl je ostavljen tačno onakav kakav je bio."
	m.postReport(emptyReportForTest())

	right := evalNumber(t, m.win, "document.getElementById('report-audit-notice').getBoundingClientRect().right")
	innerW := evalNumber(t, m.win, "window.innerWidth")
	if right > innerW+0.5 {
		t.Errorf("the notice runs past the window's right edge: %v > %v", right, innerW)
	}
	if ws := evalString(t, m.win, "getComputedStyle(document.getElementById('report-audit-notice')).whiteSpace"); ws == "nowrap" {
		t.Error("the notice is nowrap, so a long sentence cannot wrap")
	}
	assertPageDoesNotScroll(t, m.win, "report with an audit notice", "#report-body")
	assertButtonsVisible(t, m.win, "report with an audit notice", "#report-body")
}

// TestTheExportLineNeverContradictsItself: a log that could not be read
// to its end must not be reported as "integrity check: passed" and then,
// in the same line, as "not all intact". That is what a two-way
// OK/broken split produced, and this screen is the one that must not
// have a mixed signal on it (D-135).
func TestTheExportLineNeverContradictsItself(t *testing.T) {
	store, _, _ := brokenAndContinuedStore(t)
	out := t.TempDir()
	report, _ := store.Export(filepath.Join(out, "e.jsonl"), filepath.Join(out, "r.json"))

	for _, locale := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		c := i18n.Load(locale)
		line := exportedFileLines(c, "e.jsonl", "r.json", report)[1].Detail
		if strings.Contains(line, c.T("settings.export_check_ok")) {
			t.Errorf("%s: a log that could not be read to its end says %q: %q",
				locale, c.T("settings.export_check_ok"), line)
		}
		if !strings.Contains(line, c.T("settings.export_check_incomplete")) {
			t.Errorf("%s: the line does not say what actually happened: %q", locale, line)
		}
	}
}

// TestTheChainCountReadsAsSerbianDoes: Serbian has two plural stems
// where English has one, and a number ending in 2-4 takes the smaller
// one except in the teens. On the screen that reports the integrity of
// an audit log, a form that reads as though a machine wrote it is not
// the impression to give.
func TestTheChainCountReadsAsSerbianDoes(t *testing.T) {
	few := map[int]bool{2: true, 3: true, 4: true, 22: true, 33: true, 104: true}
	many := map[int]bool{5: true, 9: true, 11: true, 12: true, 13: true, 14: true, 25: true, 100: true}
	for n := range few {
		if got := chainCountKey(n); got != "settings.export_chains_few" {
			t.Errorf("%d -> %s, want the 2-4 form", n, got)
		}
	}
	for n := range many {
		if got := chainCountKey(n); got != "settings.export_chains_many" {
			t.Errorf("%d -> %s, want the 5+ form", n, got)
		}
	}
}
