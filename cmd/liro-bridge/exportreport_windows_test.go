//go:build windows

package main

// The second thing the owner reported: exporting the audit log writes
// two files and the screen says nothing about either of them. The
// verification report was opened and asked about —
//
//	{ "entryCount": 137, "result": { "OK": true, "BrokenAt": -1 } }
//
// — which is correct output, and is the hash-chain check SPEC §6.7
// requires the log to be provable against. Nothing on screen said which
// file was which or what the report was for.
//
// So the confirmation names both files and says, in one line each, what
// they are. The line that matters most is the one nobody has seen yet:
// when the chain does not verify, it has to say the log was altered and
// where, in words, not as "OK: false, BrokenAt: 42".

import (
	"fmt"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestExportNamesBothFilesAndSaysWhatTheyAre covers the Go half in all
// three catalogues: two lines, in order, each naming its own file, each
// carrying a sentence from the catalogue rather than a key or a number.
func TestExportNamesBothFilesAndSaysWhatTheyAre(t *testing.T) {
	const (
		entriesName = "liro-audit-20260905-143515.jsonl"
		reportName  = "liro-audit-20260905-143515-report.json"
	)

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)

		ok := audit.ExportReport{EntryCount: 137, Result: audit.VerifyResult{OK: true, BrokenAt: -1}}
		lines := exportedFileLines(c, entriesName, reportName, ok)
		if len(lines) != 2 {
			t.Fatalf("%s: an export produced %d lines, want one per file", locale, len(lines))
		}
		if lines[0].Name != entriesName || lines[1].Name != reportName {
			t.Fatalf("%s: the lines name %q and %q", locale, lines[0].Name, lines[1].Name)
		}
		if want := fmt.Sprintf(c.T("settings.export_entries"), 137); lines[0].Detail != want {
			t.Errorf("%s: the log line says %q, want %q", locale, lines[0].Detail, want)
		}
		if !strings.Contains(lines[0].Detail, "137") {
			t.Errorf("%s: the log line does not say how many entries it holds: %q", locale, lines[0].Detail)
		}
		if want := c.T("settings.export_check_ok"); lines[1].Detail != want {
			t.Errorf("%s: the report line says %q, want %q", locale, lines[1].Detail, want)
		}
		for _, l := range lines {
			if strings.HasPrefix(l.Detail, "settings.") {
				t.Errorf("%s: a line is showing its own catalogue key: %q", locale, l.Detail)
			}
		}

		// One entry is a sentence of its own, the way every other count
		// in this project is (main.document_count_one).
		one := audit.ExportReport{EntryCount: 1, Result: audit.VerifyResult{OK: true, BrokenAt: -1}}
		if got, want := exportedFileLines(c, entriesName, reportName, one)[0].Detail, c.T("settings.export_entries_one"); got != want {
			t.Errorf("%s: a one-entry log reads %q, want %q", locale, got, want)
		}

		// The case the report exists for. BrokenAt is an index into the
		// entries; the log holds one per line, so it is named as the
		// line a person would count to.
		broken := audit.ExportReport{EntryCount: 137, Result: audit.VerifyResult{OK: false, BrokenAt: 41}}
		brokenLines := exportedFileLines(c, entriesName, reportName, broken)
		want := fmt.Sprintf(c.T("settings.export_check_broken"), 42)
		if brokenLines[1].Detail != want {
			t.Errorf("%s: a broken chain reads %q, want %q", locale, brokenLines[1].Detail, want)
		}
		if !strings.Contains(brokenLines[1].Detail, "42") {
			t.Errorf("%s: a broken chain does not name the entry it broke at: %q", locale, brokenLines[1].Detail)
		}
		if brokenLines[1].Detail == c.T("settings.export_check_ok") {
			t.Errorf("%s: a broken chain reads the same as an intact one", locale)
		}
		// The log line is unchanged by the break: both files were
		// written, and the count is still true.
		if brokenLines[0].Detail != lines[0].Detail {
			t.Errorf("%s: the log's own line changed because the chain broke: %q", locale, brokenLines[0].Detail)
		}
	}

	// The key that used to carry this — one sentence about the folder,
	// with no file names in it — is gone from the catalogues rather than
	// merely unused.
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		if got := i18n.Load(locale).T("settings.export_chain_broken"); got != "settings.export_chain_broken" {
			t.Errorf("%s: settings.export_chain_broken is still in the catalogue: %q", locale, got)
		}
	}
}

// TestExportConfirmationRendersBothFiles is the same thing on screen, in
// the window a person is actually looking at: the heading says where the
// files went, and under it each file names itself beside what it is —
// with a real rendered height, and without widening the window (D-096).
func TestExportConfirmationRendersBothFiles(t *testing.T) {
	const (
		target      = `C:\Users\Veljko\Desktop\Izvoz`
		entriesName = "liro-audit-20260905-143515.jsonl"
		reportName  = "liro-audit-20260905-143515-report.json"
	)

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		report := audit.ExportReport{EntryCount: 137, Result: audit.VerifyResult{OK: true, BrokenAt: -1}}
		lines := exportedFileLines(c, entriesName, reportName, report)

		for _, w := range []struct {
			name string
			win  ui.Window
		}{
			{"audit log", auditLogWindowWithEntries(t, c)},
			{"settings", settingsWindowFor(t, c)},
		} {
			postWindowStatusFiles(w.win, fmt.Sprintf(c.T("settings.export_done"), target), lines, ui.IntentPositive)

			text := evalText(t, w.win, "document.getElementById('action-status').textContent")
			for _, want := range []string{target, entriesName, reportName, lines[0].Detail, lines[1].Detail} {
				if !strings.Contains(text, want) {
					t.Errorf("%s/%s: the confirmation does not say %q: %q", locale, w.name, want, text)
				}
			}
			if h := evalNumber(t, w.win, "document.getElementById('action-status-files').getBoundingClientRect().height"); h <= 0 {
				t.Errorf("%s/%s: the file lines have no height on screen", locale, w.name)
			}
			if n := evalNumber(t, w.win, "document.getElementById('action-status-files').children.length"); n != 4 {
				t.Errorf("%s/%s: the file list holds %v cells, want two rows of name and detail", locale, w.name, n)
			}
			assertNoPageScroll(t, w.win)

			// A status with no files — "check for updates", an export
			// that failed before writing anything — shows none, rather
			// than an empty grid taking a line.
			postWindowStatus(w.win, c.T("settings.updates_not_available"), ui.IntentWarning)
			if !evalBool(t, w.win, "document.getElementById('action-status-files').hidden") {
				t.Errorf("%s/%s: the file list is still shown for a status that wrote no files", locale, w.name)
			}
		}
	}
}

// TestExportConfirmationSaysPlainlyWhenTheChainIsBroken is the case the
// verification report exists for, on screen: the same two files, the
// same shape, and the report's line saying what happened in the negative
// colour family rather than in a number.
func TestExportConfirmationSaysPlainlyWhenTheChainIsBroken(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win := auditLogWindowWithEntries(t, c)

	broken := audit.ExportReport{EntryCount: 137, Result: audit.VerifyResult{OK: false, BrokenAt: 41}}
	lines := exportedFileLines(c, "liro-audit-x.jsonl", "liro-audit-x-report.json", broken)
	postWindowStatusFiles(win, fmt.Sprintf(c.T("settings.export_done"), `C:\Izvoz`), lines, ui.IntentNegative)

	text := evalText(t, win, "document.getElementById('action-status').textContent")
	if !strings.Contains(text, lines[1].Detail) {
		t.Errorf("the window does not say the chain is broken: %q", text)
	}
	if !strings.Contains(text, "42") {
		t.Errorf("the window does not name the entry the log broke at: %q", text)
	}
	class := evalText(t, win, "document.getElementById('action-status').className")
	if !strings.Contains(class, "liro-outcome-"+string(ui.IntentNegative)) {
		t.Errorf("a broken chain is not shown in the negative family: class %q", class)
	}
	// Nothing in the negative family gets quietened down: the report's
	// own line must not be greyed out relative to the heading.
	heading := evalText(t, win, "getComputedStyle(document.getElementById('action-status-text')).color")
	detail := evalText(t, win, "getComputedStyle(document.querySelectorAll('.liro-status-file-detail')[1]).color")
	if heading != detail {
		t.Errorf("the report's line is coloured %q while the heading is %q", detail, heading)
	}
}

func auditLogWindowWithEntries(t *testing.T, c *i18n.Catalogue) ui.Window {
	t.Helper()
	win, _ := sharedAuditLogWindowWithMessages(t)
	if err := win.PostJSON(buildAuditLogInit(c, twentyAuditEntries())); err != nil {
		t.Fatalf("PostJSON(audit log init): %v", err)
	}
	return win
}

func settingsWindowFor(t *testing.T, c *i18n.Catalogue) ui.Window {
	t.Helper()
	win, _ := sharedSettingsWindow(t, c, populatedSettings())
	return win
}
