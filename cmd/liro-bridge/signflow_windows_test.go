//go:build windows

package main

// The flow itself: which steps a run has, what the header says about
// them, what the window is sized for, and the two properties the
// certificate step must not lose for being a step of something.

import (
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestTheFlowIsOneWindowWithThreeSteps is the shape of the task:
// documents, certificate, method, all on one window — and no fourth
// step for the method that opens the placement picker. The picker is
// the consequence of choosing that method, not a screen before it.
func TestTheFlowIsOneWindowWithThreeSteps(t *testing.T) {
	corners := config.Default()
	corners.VisibleStamp = true

	placed := corners
	placed.StampPosition = config.StampPositionCustom
	placed.StampPlacedPage = 2
	placed.StampX, placed.StampY = 100, 200

	none := config.Default()
	none.VisibleStamp = false

	want := []flowStep{stepDocuments, stepCertificate, stepMethod}
	for _, tc := range []struct {
		name string
		cfg  config.Config
	}{
		{"a set corner", corners},
		{"choosing the position", placed},
		{"nothing drawn", none},
	} {
		m := newMainWindow(tc.cfg, "sr-Latn")
		got := m.stepsFor()
		if len(got) != len(want) {
			t.Fatalf("%s: steps = %v, want %v", tc.name, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%s: steps = %v, want %v", tc.name, got, want)
			}
		}
	}

	// Three pages for three steps, and the placement picker is not one
	// of them: it is the only window this flow still opens.
	if stepDocuments.page() != pageMain {
		t.Errorf("the documents step is on %q", stepDocuments.page())
	}
	if stepCertificate.page() != pageConsent {
		t.Errorf("the certificate step is on %q", stepCertificate.page())
	}
	if stepMethod.page() != pageStamp {
		t.Errorf("the method step is on %q", stepMethod.page())
	}
}

// TestBackIsAlwaysAvailableExceptOnTheFirstStep: someone who picked the
// wrong certificate goes back one step, not out of the flow and into it
// again. The first step has nowhere to go back to, and a run that is
// only the approval has no header at all.
func TestBackIsAlwaysAvailableExceptOnTheFirstStep(t *testing.T) {
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = 1

	m := newMainWindow(cfg, "sr-Latn")
	for i, step := range m.stepsFor() {
		h := m.headerFor(step)
		if h == nil {
			t.Fatalf("step %d carries no header at all", i+1)
		}
		if h.Index != i+1 {
			t.Errorf("step %d reports itself as %d", i+1, h.Index)
		}
		if h.Total != 3 {
			t.Errorf("step %d says the run is %d steps long, want 3", i+1, h.Total)
		}
		if want := i > 0; h.Back != want {
			t.Errorf("step %d offers back = %v, want %v", i+1, h.Back, want)
		}
	}

	// A caller that supplied everything: the approval alone, and no
	// header — a single dot is not a sequence.
	supplied := consent.StampChoice{Visible: false}
	only := newMainWindow(cfg, "sr-Latn")
	only.documentsSupplied = true
	only.suppliedStamp = &supplied
	if h := only.headerFor(stepCertificate); h != nil {
		t.Errorf("a one-step run drew a header: %+v", h)
	}
}

// TestTheStepHeaderReadsTheSameInEveryLanguage: the dots are what the
// eye sees, and the words are what a screen reader says. Both come from
// the catalogue, in all three locales, and the number of dots is the
// number of steps this run actually has.
func TestTheStepHeaderReadsTheSameInEveryLanguage(t *testing.T) {
	cfg := config.Default()
	cfg.VisibleStamp = true

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		m := newMainWindow(cfg, locale)
		h := m.headerFor(stepCertificate)
		if h == nil {
			t.Fatalf("%s: no header", locale)
		}
		if h.BackText != c.T("step.back") || strings.HasPrefix(h.BackText, "step.") {
			t.Errorf("%s: back reads %q", locale, h.BackText)
		}
		if strings.HasPrefix(h.Label, "step.") || !strings.Contains(h.Label, "2") {
			t.Errorf("%s: the header says %q", locale, h.Label)
		}
	}

	// And on the page: one dot per step, exactly one of them current.
	c := i18n.Load("sr-Latn")
	m := newMainWindow(cfg, "sr-Latn")
	win, _ := sharedConsentWindow(t)
	payload := buildConsentInit(c, consent.BuildViewModel(consent.ApplicationLocal,
		[][]byte{{1}}, []string{"ugovor.pdf"}, []classify.Info{stampTestCertificate()}))
	payload["step"] = m.headerFor(stepCertificate)
	if err := win.PostJSON(payload); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	if got := evalNumber(t, win, "document.querySelectorAll('#step-dots .liro-steps-dot').length"); got != 3 {
		t.Errorf("the header drew %v dots, want 3", got)
	}
	if got := evalNumber(t, win, "document.querySelectorAll('.liro-steps-dot-current').length"); got != 1 {
		t.Errorf("%v dots are marked current, want exactly 1", got)
	}
	if evalBool(t, win, "document.getElementById('step-back-btn').hidden") {
		t.Error("the certificate step offers no way back to the documents")
	}
	if got := evalString(t, win, "document.getElementById('step-header').getAttribute('aria-label')"); !strings.Contains(got, "2") {
		t.Errorf("the header's accessible name is %q, which does not say which step this is", got)
	}
}

// TestApproveIsNotTheDefaultFocusAndNeedsADeliberatePress is the
// property SPEC §6.5 and §10.3 make non-negotiable, checked on the
// certificate step as it now is: a step of a sequence, with a way back
// beside it.
//
// Three things: Approve is not what the keyboard lands on, it cannot be
// pressed at all until a certificate has been chosen, and pressing it
// without one sends nothing to Go. The way back must not become the
// focused control either — the way out of a decision is refusing it.
func TestApproveIsNotTheDefaultFocusAndNeedsADeliberatePress(t *testing.T) {
	c := i18n.Load("sr-Latn")
	m := newMainWindow(config.Default(), "sr-Latn")
	win, messages := sharedConsentWindow(t)

	payload := buildConsentInit(c, consent.BuildViewModel(consent.ApplicationLocal,
		[][]byte{{1}}, []string{"ugovor.pdf"}, []classify.Info{stampTestCertificate()}))
	payload["step"] = m.headerFor(stepCertificate)
	if err := win.PostJSON(payload); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	if got := evalString(t, win, "document.activeElement.id"); got != "cancel-btn" {
		t.Fatalf("the initially focused control is %q, want cancel-btn", got)
	}
	if !evalBool(t, win, "document.getElementById('approve-btn').disabled") {
		t.Fatal("Approve is pressable before a certificate has been chosen")
	}

	drain(messages)
	if _, err := win.Eval("document.getElementById('approve-btn').click(); 'ok'"); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-messages:
		t.Fatalf("pressing Approve with no certificate chosen sent %v", msg.Type)
	case <-time.After(500 * time.Millisecond):
	}

	// Choosing one is what makes the approval possible, and the press
	// is still the person's.
	if _, err := win.Eval("document.querySelector('.liro-cert-row').click(); 'ok'"); err != nil {
		t.Fatal(err)
	}
	if msg := recvMessage(t, messages, 5*time.Second); msg.Type != ui.MessageTypeSelectCertificate {
		t.Fatalf("choosing a certificate sent %v", msg.Type)
	}
	if evalBool(t, win, "document.getElementById('approve-btn').disabled") {
		t.Fatal("Approve is still not pressable with a certificate chosen")
	}
	if got := evalString(t, win, "document.activeElement.id"); got == "approve-btn" {
		t.Fatal("choosing a certificate moved the focus onto Approve")
	}
	if _, err := win.Eval("document.getElementById('approve-btn').click(); 'ok'"); err != nil {
		t.Fatal(err)
	}
	if msg := recvMessage(t, messages, 5*time.Second); msg.Type != ui.MessageTypeApprove {
		t.Fatalf("Approve sent %v, want approve", msg.Type)
	}
	if got := readWindowAction(win, "the approval"); got != "approve" {
		t.Errorf("Approve reported the action %q, want approve", got)
	}
}

// TestEveryMethodEndsTheFlowOnTheSameWord: the method step is the last
// step whichever of the three is chosen, so its primary action says
// Sign for all of them and the header draws the same three dots.
//
// Choosing to place the stamp by looking at the page used to say Next,
// because it led to a fourth step that asked nothing before opening the
// picker. The picker opens from here now, so there is no "next" left to
// promise.
func TestEveryMethodEndsTheFlowOnTheSameWord(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = 1

	m := newMainWindow(cfg, "sr-Latn")
	win, _ := sharedStampWindow(t, c, cfg, stampRoleStep)
	m.win = win
	m.method = stampMethodPlaced
	m.postStampStep()

	for _, method := range []string{stampMethodPlaced, stampMethodCorners, stampMethodNone} {
		chooseStampMethod(t, win, method)
		if got := evalNumber(t, win, "document.querySelectorAll('#step-dots .liro-steps-dot').length"); got != 3 {
			t.Errorf("method %s drew %v dots, want 3", method, got)
		}
		if got := evalNumber(t, win, "document.querySelectorAll('.liro-steps-dot-current').length"); got != 1 {
			t.Errorf("method %s: %v dots are marked current, want exactly 1", method, got)
		}
		if got := evalString(t, win, "document.getElementById('save-btn').textContent"); got != c.T("stampwindow.sign") {
			t.Errorf("method %s: the primary action reads %q, want %q", method, got, c.T("stampwindow.sign"))
		}
	}
}

// TestTheDocumentsScreenIsNotShownOnTheWayToSigning is the flash the
// owner reported, as a test: pressing Sign put step 1 back on screen
// for about a second while the card was being opened.
//
// The cause was that navigating to the main page posted the document
// list's whole payload — which is also what *renders* the document
// list — and nothing replaced it until the runner's first progress
// hook, on the far side of the card session. So the two halves are
// checked here: arriving on the page shows no screen at all, and the
// screen that covers the card is the progress screen, in the state SPEC
// §12.9 asks for.
func TestTheDocumentsScreenIsNotShownOnTheWayToSigning(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, err := ui.NewWindow(ui.Options{
		Title:       "on the way to signing",
		Width:       stepDocumentsWidth,
		Height:      stepDocumentsHeight,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   pageStamp,
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer func() { _ = win.Close() }()

	m := newMainWindow(config.Default(), "sr-Latn")
	m.win = win
	m.page = pageStamp

	// Exactly what startSigning does on the way from the method step.
	if !m.gotoPage(pageMain) {
		t.Fatal("the flow could not reach its main page")
	}

	for _, id := range []string{
		"state-files", "state-queue", "state-report",
		"state-tsachoice", "state-outputexists", "state-failed",
	} {
		script := "getComputedStyle(document.getElementById('" + id + "')).display"
		if got := evalString(t, win, script); got != "none" {
			t.Errorf("%s is on screen (display %q) after navigating, before any screen was asked for", id, got)
		}
	}
	// The strings did arrive, which is the whole reason anything is
	// posted here at all.
	if got := evalString(t, win, "window.liroT('main.stop')"); got != c.T("main.stop") {
		t.Errorf("the page's strings are %q, want the catalogue's %q", got, c.T("main.stop"))
	}

	// And the screen that covers the card session.
	m.postPreparingCard()
	if got := evalString(t, win, "getComputedStyle(document.getElementById('state-files')).display"); got != "none" {
		t.Errorf("the document list is what covers the card session (display %q)", got)
	}
	if evalBool(t, win, "document.getElementById('state-queue').hidden") {
		t.Fatal("the progress screen is not showing before the card session opens")
	}
	if got := evalString(t, win, "document.getElementById('queue-label').textContent"); got != c.T("consent.state_preparing_card") {
		t.Errorf("the progress screen reads %q, want %q", got, c.T("consent.state_preparing_card"))
	}
	if evalBool(t, win, "document.getElementById('queue-progress-indeterminate').hidden") {
		t.Error("the bar is not indeterminate while the card is being opened")
	}
	if !evalBool(t, win, "document.getElementById('queue-progress').hidden") {
		t.Error("a percentage is shown for a batch in which nothing has been signed yet")
	}
}

// TestTheWindowResizesToItsContent: the window is not one size for four
// screens. Each step is measured in a window of its own actual size —
// a size constant checked against a window created at some other size
// proves nothing about the window a person opens.
func TestTheWindowResizesToItsContent(t *testing.T) {
	win, err := ui.NewWindow(ui.Options{
		Title:       "resize",
		Width:       stepDocumentsWidth,
		Height:      stepDocumentsHeight,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   pageMain,
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer func() { _ = win.Close() }()

	for _, step := range []flowStep{stepDocuments, stepCertificate, stepMethod} {
		w, h := step.size()
		if err := win.Navigate(step.page()); err != nil {
			t.Fatalf("Navigate(%v): %v", step, err)
		}
		// The page must reach the size the step asks for. It is waited
		// for rather than read once, because Resize returns before the
		// page has been relaid out and a single read straight after it
		// answers out of the previous step's layout — the same defect,
		// in the same package, that D-201 found in the pairing window.
		resizeAndSettle(t, win, w, h)
	}

	// And the steps are genuinely different sizes: the point of
	// resizing is that a list of documents and three method cards do
	// not need the same window.
	dw, dh := stepDocuments.size()
	mw, mh := stepMethod.size()
	if dw <= mw || dh <= mh {
		t.Errorf("the documents step (%dx%d) is not larger than the method step (%dx%d)", dw, dh, mw, mh)
	}
}

// TestNavigatingBetweenStepsKeepsOneWindow is the whole of "one window
// whose content changes": the native window the person is looking at is
// the same object, at the same handle, on every step.
func TestNavigatingBetweenStepsKeepsOneWindow(t *testing.T) {
	win, err := ui.NewWindow(ui.Options{
		Title:       "one window",
		Width:       stepDocumentsWidth,
		Height:      stepDocumentsHeight,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   pageMain,
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	defer func() { _ = win.Close() }()

	handle := win.Handle()
	for _, step := range []flowStep{stepCertificate, stepMethod, stepDocuments} {
		if err := win.Navigate(step.page()); err != nil {
			t.Fatalf("Navigate(%v): %v", step, err)
		}
		if win.Handle() != handle {
			t.Fatalf("step %v is a different native window", step)
		}
		// The page that arrived is the page that was asked for, and its
		// scripts have run by the time Navigate returned — otherwise
		// the first payload after it is dropped on the floor (D-098).
		if !evalBool(t, win, "typeof window.__liroReceive === 'function'") {
			t.Fatalf("step %v: the page's bridge had not run when Navigate returned", step)
		}
		if got := evalString(t, win, "window.location.pathname"); got != step.page() {
			t.Fatalf("navigating to %v landed on %q", step, got)
		}
	}
}
