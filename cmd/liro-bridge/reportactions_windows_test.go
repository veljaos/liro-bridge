//go:build windows

package main

// The completion screen's actions.
//
// A person who has just signed a hundred documents is finished.
// Signing more was the primary action — the uncommon case dressed as
// the expected one — so Finish is the primary action now, in brand
// blue, and it closes the window; signing more is the neutral button
// beside it; the folder button is unchanged.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/jobs"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestFinishIsThePrimaryActionOnTheReport reads the rendered screen:
// the weights, the words and the order, in all three locales.
func TestFinishIsThePrimaryActionOnTheReport(t *testing.T) {
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "a.pdf", 10)

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		m, _ := testMainWindow(t, locale, config.Default(), []string{path})
		m.postReport(jobs.Report{Succeeded: 100, OutputDir: dir, AchievedLevel: "B-LT"})

		if got := evalText(t, m.win, "document.getElementById('report-finish-btn').textContent"); got != c.T("main.finish") {
			t.Errorf("%s: Finish reads %q, want %q", locale, got, c.T("main.finish"))
		}
		if got := evalText(t, m.win, "document.getElementById('report-again-btn').textContent"); got != c.T("main.new_batch") {
			t.Errorf("%s: the other button reads %q, want %q", locale, got, c.T("main.new_batch"))
		}

		// Finish is the one in brand blue, and it is the only one.
		if !evalBool(t, m.win, "document.getElementById('report-finish-btn').classList.contains('liro-btn-primary')") {
			t.Errorf("%s: Finish is not the primary action", locale)
		}
		if evalBool(t, m.win, "document.getElementById('report-again-btn').classList.contains('liro-btn-primary')") {
			t.Errorf("%s: signing more is still dressed as the primary action", locale)
		}
		if !evalBool(t, m.win, "document.getElementById('report-again-btn').classList.contains('liro-btn-secondary')") {
			t.Errorf("%s: signing more is not the neutral button beside Finish", locale)
		}
		// The folder button is untouched.
		if !evalBool(t, m.win, "document.getElementById('report-open-btn').classList.contains('liro-btn-secondary')") {
			t.Errorf("%s: the folder button changed weight", locale)
		}

		// It reads as blue, not merely as carrying the class: the token
		// is what the rule resolves to, and a colour is what a person
		// actually judges "primary" by.
		blue := evalText(t, m.win, "getComputedStyle(document.getElementById('report-finish-btn')).backgroundColor")
		neutral := evalText(t, m.win, "getComputedStyle(document.getElementById('report-again-btn')).backgroundColor")
		if blue == neutral {
			t.Errorf("%s: Finish and signing more are the same colour (%s)", locale, blue)
		}

		// Side by side, with Finish last, where every primary action in
		// this program sits, and both on screen.
		finishTop := evalNumber(t, m.win, "document.getElementById('report-finish-btn').getBoundingClientRect().top")
		againTop := evalNumber(t, m.win, "document.getElementById('report-again-btn').getBoundingClientRect().top")
		if finishTop != againTop {
			t.Errorf("%s: Finish is not beside signing more — they are on different lines (%v and %v)",
				locale, finishTop, againTop)
		}
		finishLeft := evalNumber(t, m.win, "document.getElementById('report-finish-btn').getBoundingClientRect().left")
		againLeft := evalNumber(t, m.win, "document.getElementById('report-again-btn').getBoundingClientRect().left")
		if finishLeft <= againLeft {
			t.Errorf("%s: Finish is not last in its row (%v vs %v)", locale, finishLeft, againLeft)
		}
		assertNoPageScroll(t, m.win)
		for _, id := range []string{"report-finish-btn", "report-again-btn", "report-open-btn", "report-export-btn"} {
			bottom := evalNumber(t, m.win, "document.getElementById('"+id+"').getBoundingClientRect().bottom")
			if height := evalNumber(t, m.win, "window.innerHeight"); bottom > height {
				t.Errorf("%s: %s is at %v, past the window's %v", locale, id, bottom, height)
			}
		}
	}
}

// TestFinishClosesTheWindow is the half a rendering test cannot see:
// the primary action ends the run rather than only looking like it
// should. Signing more, next to it, does not.
func TestFinishClosesTheWindow(t *testing.T) {
	dir := t.TempDir()
	m, _ := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})
	m.report = &jobs.Report{Succeeded: 100, OutputDir: dir, AchievedLevel: "B-LT"}
	m.postReport(*m.report)

	// Signing more clears the batch and stays open.
	if _, err := m.win.Eval("document.getElementById('report-again-btn').click(); 'ok'"); err != nil {
		t.Fatal(err)
	}
	if done := m.handleAction(context.Background()); done {
		t.Fatal("signing more closed the window")
	}
	if m.report != nil || m.queue.Len() != 0 {
		t.Errorf("signing more left the finished batch behind: report %v, %d documents", m.report, m.queue.Len())
	}

	// Finish ends it.
	m.postReport(jobs.Report{Succeeded: 100, OutputDir: dir, AchievedLevel: "B-LT"})
	if _, err := m.win.Eval("document.getElementById('report-finish-btn').click(); 'ok'"); err != nil {
		t.Fatal(err)
	}
	if done := m.handleAction(context.Background()); !done {
		t.Fatal("Finish did not close the window")
	}
}

// TestFinishReachesGoAsItsOwnAction: the page->Go surface is still the
// three message types F5 §2.4 fixes, so a new button is a new recorded
// action rather than a fourth message (D-083).
func TestFinishReachesGoAsItsOwnAction(t *testing.T) {
	dir := t.TempDir()
	m, messages := testMainWindow(t, "sr-Latn", config.Default(), []string{writeTestPDF(t, dir, "a.pdf", 10)})
	m.postReport(jobs.Report{Succeeded: 1, OutputDir: dir})

	drain(messages)
	if _, err := m.win.Eval("document.getElementById('report-finish-btn').click(); 'ok'"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-messages:
		if msg.Type != ui.MessageTypeApprove {
			t.Fatalf("Finish sent %q, want approve", msg.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Finish sent nothing to Go")
	}
	if got := m.readAction().Action; got != "finish" {
		t.Fatalf("Finish recorded action %q", got)
	}
}

// TestReportActionsRenderInEveryLocale: four buttons, no blank labels,
// no raw keys.
func TestReportActionsRenderInEveryLocale(t *testing.T) {
	dir := t.TempDir()
	path := writeTestPDF(t, dir, "a.pdf", 10)
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		m, _ := testMainWindow(t, locale, config.Default(), []string{path})
		m.postReport(jobs.Report{Succeeded: 3, OutputDir: dir})
		for _, id := range []string{"report-export-btn", "report-open-btn", "report-again-btn", "report-finish-btn"} {
			text := evalText(t, m.win, "document.getElementById('"+id+"').textContent")
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s: %s has no label", locale, id)
			}
			if strings.HasPrefix(text, "main.") {
				t.Errorf("%s: %s rendered a raw catalogue key: %q", locale, id, text)
			}
		}
	}
}
