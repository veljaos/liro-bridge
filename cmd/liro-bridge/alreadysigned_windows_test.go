//go:build windows

package main

// J-3 in the window: when the list holds documents whose names already
// end in the configured output suffix, say how many and offer to skip
// them, once, for the whole batch.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

func waitForAlreadySignedScreen(t *testing.T, win ui.Window) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if evalString(t, win, "String(document.getElementById('state-alreadysigned').hidden)") == "false" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the already-signed screen never appeared")
}

func alreadySignedInputs(t *testing.T, dir string, names ...string) []interactiveInput {
	t.Helper()
	inputs := make([]interactiveInput, 0, len(names))
	for _, name := range names {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("%PDF-1.4\n"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		inputs = append(inputs, interactiveInput{path: p})
	}
	return inputs
}

// TestAListWithNoAlreadySignedDocumentsAsksNothing keeps the addition
// informative rather than obstructive: the ordinary batch never sees
// this screen at all.
func TestAListWithNoAlreadySignedDocumentsAsksNothing(t *testing.T) {
	c := i18n.Load("sr-Latn")
	m, messages := testMainWindow(t, "sr-Latn", config.Default(), nil)
	dir := t.TempDir()
	inputs := alreadySignedInputs(t, dir, "ugovor.pdf", "racun.pdf")

	skip, proceed := resolveAlreadySigned(m.win, messages, m.closed, c, inputs, "-signed")
	if !proceed {
		t.Fatal("a list with nothing already signed did not proceed")
	}
	if len(skip) != 0 {
		t.Fatalf("nothing should have been skipped, got %v", skip)
	}
	if evalString(t, m.win, "String(document.getElementById('state-alreadysigned').hidden)") != "true" {
		t.Error("the already-signed screen was shown for a list that has none")
	}
}

// TestTheAlreadySignedScreenSaysHowManyAndOffersToSkipThem is the
// screen itself: the count, in words, and the two proceeding actions
// plus Cancel — the same shape as the output-file choice, with the
// action that loses nothing as the primary one.
func TestTheAlreadySignedScreenSaysHowManyAndOffersToSkipThem(t *testing.T) {
	for _, locale := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			m, _ := testMainWindow(t, locale, config.Default(), nil)
			win := m.win

			if err := win.PostJSON(askAlreadySignedPayload(3, "-signed", c)); err != nil {
				t.Fatalf("PostJSON: %v", err)
			}
			waitForAlreadySignedScreen(t, win)

			explain := evalString(t, win, "document.getElementById('already-signed-explain').textContent")
			if !strings.Contains(explain, "3") {
				t.Errorf("the screen does not say how many: %q", explain)
			}
			if !strings.Contains(explain, "-signed") {
				t.Errorf("the screen does not name the suffix: %q", explain)
			}
			for _, id := range []string{"already-skip-btn", "already-sign-btn", "already-cancel-btn"} {
				label := evalString(t, win, "document.getElementById('"+id+"').textContent")
				if strings.TrimSpace(label) == "" {
					t.Errorf("%s has no label in %s", id, locale)
				}
			}
			// Nothing that signs anything is focused first — the same
			// rule the output-file and timestamp screens follow.
			if focused := evalString(t, win, "document.activeElement.id"); focused != "already-cancel-btn" {
				t.Errorf("initial focus is %q, want already-cancel-btn", focused)
			}
			// Weights: skipping loses nothing and is the primary
			// action; signing them again is the neutral one.
			primary := evalString(t, win, "document.getElementById('already-skip-btn').className")
			if !strings.Contains(primary, "liro-btn-primary") {
				t.Errorf("the skip action is not the primary one: %q", primary)
			}
		})
	}
}

// TestOneAlreadySignedDocumentReadsAsOne pins the singular wording: a
// button that says "Skip them" for one document reads as though the
// program has miscounted.
func TestOneAlreadySignedDocumentReadsAsOne(t *testing.T) {
	c := i18n.Load("en")
	m, _ := testMainWindow(t, "en", config.Default(), nil)

	if err := m.win.PostJSON(askAlreadySignedPayload(1, "-signed", c)); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	waitForAlreadySignedScreen(t, m.win)

	explain := evalString(t, m.win, "document.getElementById('already-signed-explain').textContent")
	if !strings.Contains(explain, "1 of these documents is") {
		t.Errorf("the singular sentence did not render: %q", explain)
	}
	skip := evalString(t, m.win, "document.getElementById('already-skip-btn').textContent")
	if !strings.Contains(skip, "Skip it") {
		t.Errorf("the skip button is not singular: %q", skip)
	}
}

// TestTheAlreadySignedAnswerAppliesToTheWholeBatch drives the whole
// loop against a real window, through the page's own DOM (D-094's
// Eval carve-out; nothing simulates the real cursor): one question,
// one answer, applied to every already-signed document in the list.
func TestTheAlreadySignedAnswerAppliesToTheWholeBatch(t *testing.T) {
	c := i18n.Load("sr-Latn")
	m, messages := testMainWindow(t, "sr-Latn", config.Default(), nil)
	win := m.win

	dir := t.TempDir()
	inputs := alreadySignedInputs(t, dir,
		"ugovor.pdf", "ugovor-signed.pdf", "racun.pdf", "racun-signed.pdf")

	type result struct {
		skip    map[string]bool
		proceed bool
	}

	t.Run("skip", func(t *testing.T) {
		done := make(chan result, 1)
		go func() {
			s, ok := resolveAlreadySigned(win, messages, m.closed, c, inputs, "-signed")
			done <- result{s, ok}
		}()
		waitForAlreadySignedScreen(t, win)
		if _, err := win.Eval("document.getElementById('already-skip-btn').click()"); err != nil {
			t.Fatalf("Eval(click): %v", err)
		}
		select {
		case got := <-done:
			if !got.proceed {
				t.Fatal("skipping did not proceed with the rest of the batch")
			}
			if len(got.skip) != 2 {
				t.Fatalf("skip = %v, want both already-signed documents", got.skip)
			}
			for _, name := range []string{"ugovor-signed.pdf", "racun-signed.pdf"} {
				if !got.skip[filepath.Join(dir, name)] {
					t.Errorf("%s was not skipped", name)
				}
			}
			for _, name := range []string{"ugovor.pdf", "racun.pdf"} {
				if got.skip[filepath.Join(dir, name)] {
					t.Errorf("%s was skipped and should not have been", name)
				}
			}
		case <-time.After(10 * time.Second):
			t.Fatal("resolveAlreadySigned never returned after Skip was clicked")
		}
	})

	t.Run("sign them too", func(t *testing.T) {
		done := make(chan result, 1)
		go func() {
			s, ok := resolveAlreadySigned(win, messages, m.closed, c, inputs, "-signed")
			done <- result{s, ok}
		}()
		waitForAlreadySignedScreen(t, win)
		if _, err := win.Eval("document.getElementById('already-sign-btn').click()"); err != nil {
			t.Fatalf("Eval(click): %v", err)
		}
		select {
		case got := <-done:
			if !got.proceed {
				t.Fatal("signing them too did not proceed")
			}
			if len(got.skip) != 0 {
				t.Fatalf("skip = %v, want nothing skipped", got.skip)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("resolveAlreadySigned never returned after Sign was clicked")
		}
	})

	t.Run("cancel", func(t *testing.T) {
		done := make(chan result, 1)
		go func() {
			s, ok := resolveAlreadySigned(win, messages, m.closed, c, inputs, "-signed")
			done <- result{s, ok}
		}()
		waitForAlreadySignedScreen(t, win)
		if _, err := win.Eval("document.getElementById('already-cancel-btn').click()"); err != nil {
			t.Fatalf("Eval(click): %v", err)
		}
		select {
		case got := <-done:
			if got.proceed {
				t.Fatal("cancel proceeded with the batch")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("resolveAlreadySigned never returned after Cancel was clicked")
		}
	})
}

// TestTheAlreadySignedQuestionFollowsTheConfiguredSuffix: a person who
// has configured a different output suffix gets the question about
// their own suffix, not about "-signed".
func TestTheAlreadySignedQuestionFollowsTheConfiguredSuffix(t *testing.T) {
	c := i18n.Load("sr-Latn")
	m, messages := testMainWindow(t, "sr-Latn", config.Default(), nil)

	dir := t.TempDir()
	inputs := alreadySignedInputs(t, dir, "ugovor-potpisan.pdf", "ugovor-signed.pdf")

	// Against "-potpisan", the "-signed" document is an ordinary input.
	done := make(chan map[string]bool, 1)
	go func() {
		s, _ := resolveAlreadySigned(m.win, messages, m.closed, c, inputs, "-potpisan")
		done <- s
	}()
	waitForAlreadySignedScreen(t, m.win)
	if _, err := m.win.Eval("document.getElementById('already-skip-btn').click()"); err != nil {
		t.Fatalf("Eval(click): %v", err)
	}
	select {
	case skip := <-done:
		if len(skip) != 1 || !skip[filepath.Join(dir, "ugovor-potpisan.pdf")] {
			t.Fatalf("skip = %v, want only the document matching the configured suffix", skip)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("resolveAlreadySigned never returned")
	}
}

// TestTheReportSaysWhyDocumentsWereSkipped: "skipped" in the counts does
// not say why, and why is the whole of what the person needs.
func TestTheReportSaysWhyDocumentsWereSkipped(t *testing.T) {
	for _, locale := range []string{"en", "sr-Latn", "sr-Cyrl"} {
		c := i18n.Load(locale)
		if got := alreadySignedReportText(c, 0); got != "" {
			t.Errorf("%s: a batch that skipped nothing produced %q", locale, got)
		}
		one := alreadySignedReportText(c, 1)
		many := alreadySignedReportText(c, 4)
		for _, got := range []string{one, many} {
			if got == "" || strings.Contains(got, "already_signed") || strings.Contains(got, "%d") {
				t.Errorf("%s: %q is not a finished sentence", locale, got)
			}
		}
		if !strings.Contains(many, "4") {
			t.Errorf("%s: the plural sentence does not say how many: %q", locale, many)
		}
	}
}
