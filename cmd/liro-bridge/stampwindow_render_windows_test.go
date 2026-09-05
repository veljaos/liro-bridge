//go:build windows

package main

// The stamp window, driven against a real WebView2 window (F6 §6).

import (
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestStampWindowOffersEverythingF6Asks pins the window's controls:
// on/off, the four corners, the page, the reference line and the
// document-number toggle.
func TestStampWindowOffersEverythingF6Asks(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	win, _ := sharedStampWindow(t, c, cfg)

	if !evalBool(t, win, "document.getElementById('stamp-visible').checked") {
		t.Fatal("the visible-stamp box is unticked for a configuration that has it on")
	}

	corners := evalText(t, win,
		"Array.from(document.getElementById('stamp-position').options).map(function(o){return o.value}).join(',')")
	want := strings.Join([]string{
		consent.StampPositionBottomRight, consent.StampPositionBottomLeft,
		consent.StampPositionTopRight, consent.StampPositionTopLeft,
	}, ",")
	if corners != want {
		t.Fatalf("corners = %q, want %q — SPEC §13.1's four, its own default first", corners, want)
	}

	pages := evalText(t, win,
		"Array.from(document.getElementById('stamp-page').options).map(function(o){return o.value}).join(',')")
	if pages != "first,last,number" {
		t.Fatalf("pages = %q, want first,last,number", pages)
	}

	for _, id := range []string{"stamp-reference", "stamp-document-id", "stamp-page-number"} {
		if evalBool(t, win, "document.getElementById('"+id+"') === null") {
			t.Fatalf("the window has no %s control", id)
		}
	}

	// SPEC §13.5: the identity document number is never the default.
	if evalBool(t, win, "document.getElementById('stamp-document-id').checked") {
		t.Fatal("the identity-document-number box is ticked by default")
	}

	// F6 §6's margin rule is stated on the screen, not only in code.
	text := evalText(t, win, "document.body.textContent")
	if !strings.Contains(text, c.T("stampwindow.margin_note")) {
		t.Fatalf("the window does not mention the enforced margin: %q", text)
	}
}

// TestStampWindowHidesTheDetailsWhenTheStampIsOff: controls for
// something nobody is drawing are noise (the same judgement D-103 made
// for the corner selector on the consent screen).
func TestStampWindowHidesTheDetailsWhenTheStampIsOff(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = false
	win, _ := sharedStampWindow(t, c, cfg)

	// D-106's trap: `hidden` being true is not the same as the element
	// being invisible, because an author `display` rule outranks the
	// user-agent's [hidden] rule. Read what is rendered.
	display := evalText(t, win, "getComputedStyle(document.getElementById('stamp-details')).display")
	if display != "none" {
		t.Fatalf("the stamp details still render with the stamp off (display: %s)", display)
	}

	if _, err := win.Eval("document.getElementById('stamp-visible').checked = true; document.getElementById('stamp-visible').dispatchEvent(new Event('change'))"); err != nil {
		t.Fatalf("Eval(tick the box): %v", err)
	}
	display = evalText(t, win, "getComputedStyle(document.getElementById('stamp-details')).display")
	if display == "none" {
		t.Fatal("the stamp details stay hidden after the stamp is switched on")
	}
}

// TestStampWindowPageNumberAppearsOnlyForASpecificPage covers the same
// rule one level down.
func TestStampWindowPageNumberAppearsOnlyForASpecificPage(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPage = config.StampPageFirst
	win, _ := sharedStampWindow(t, c, cfg)

	if d := evalText(t, win, "getComputedStyle(document.getElementById('stamp-page-number-field')).display"); d != "none" {
		t.Fatalf("the page-number box renders for 'first page' (display: %s)", d)
	}

	if _, err := win.Eval("document.getElementById('stamp-page').value = 'number'; document.getElementById('stamp-page').dispatchEvent(new Event('change'))"); err != nil {
		t.Fatalf("Eval(choose a specific page): %v", err)
	}
	if d := evalText(t, win, "getComputedStyle(document.getElementById('stamp-page-number-field')).display"); d == "none" {
		t.Fatal("the page-number box stays hidden after choosing a specific page")
	}
}

// TestStampWindowReportsWhatWasSaved is the round trip: the form's
// answers reach Go through the one read D-083 allows, and Save is
// distinguishable from Cancel.
func TestStampWindowReportsWhatWasSaved(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	win, messages := sharedStampWindow(t, c, cfg)

	script := strings.Join([]string{
		"document.getElementById('stamp-visible').checked = true;",
		"document.getElementById('stamp-visible').dispatchEvent(new Event('change'));",
		"document.getElementById('stamp-position').value = 'top-left';",
		"document.getElementById('stamp-page').value = 'number';",
		"document.getElementById('stamp-page').dispatchEvent(new Event('change'));",
		"document.getElementById('stamp-page-number').value = '7';",
		"document.getElementById('stamp-reference').value = 'Ugovor 2026/114';",
		"document.getElementById('stamp-document-id').checked = true;",
		"document.getElementById('save-btn').click();",
	}, "")
	if _, err := win.Eval(script); err != nil {
		t.Fatalf("Eval(fill and save): %v", err)
	}
	select {
	case msg := <-messages:
		if msg.Type != ui.MessageTypeApprove {
			t.Fatalf("Save sent %q, want approve", msg.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Save sent nothing to Go")
	}

	form, err := readStampSettings(win)
	if err != nil {
		t.Fatalf("readStampSettings: %v", err)
	}
	if !form.Saved {
		t.Fatal("the form does not report that Save was pressed")
	}
	if form.Position != consent.StampPositionTopLeft {
		t.Errorf("position = %q, want top-left", form.Position)
	}
	if form.Page != "7" {
		t.Errorf("page = %q, want 7", form.Page)
	}
	if form.Reference != "Ugovor 2026/114" {
		t.Errorf("reference = %q", form.Reference)
	}
	if !form.ShowDocumentID {
		t.Error("the document-number toggle did not come back")
	}

	// And the same form folded onto a configuration.
	got := applyStampSettings(config.Default(), form)
	if got.StampPosition != consent.StampPositionTopLeft || got.StampPage != "7" ||
		got.StampReference != "Ugovor 2026/114" || !got.StampShowDocumentID || !got.VisibleStamp {
		t.Fatalf("applyStampSettings produced %+v", got)
	}
}

// TestStampWindowCancelIsNotASave: closing without pressing Save must
// change nothing.
func TestStampWindowCancelIsNotASave(t *testing.T) {
	c := i18n.Load("sr-Latn")
	win, _ := sharedStampWindow(t, c, config.Default())

	form, err := readStampSettings(win)
	if err != nil {
		t.Fatalf("readStampSettings: %v", err)
	}
	if form.Saved {
		t.Fatal("a freshly-opened window reports that Save was pressed")
	}
}

// TestStampWindowRendersInEveryLocale is SPEC §9.1 for the new window.
func TestStampWindowRendersInEveryLocale(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win, _ := sharedStampWindow(t, c, config.Default())
			for _, id := range []string{"save-btn", "cancel-btn"} {
				text := evalText(t, win, "document.getElementById('"+id+"').textContent")
				if strings.TrimSpace(text) == "" {
					t.Errorf("%s has no label in %s", id, locale)
				}
				if strings.HasPrefix(text, "stampwindow.") {
					t.Errorf("%s rendered a raw key in %s: %q", id, locale, text)
				}
			}
		})
	}
}

// TestStampWindowDoesNotScroll is D-106's rule for the new window.
func TestStampWindowDoesNotScroll(t *testing.T) {
	c := i18n.Load("sr-Cyrl") // the longest labels of the three
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampReference = strings.Repeat("Ugovor 2026/114 ", 12)
	win, _ := sharedStampWindow(t, c, cfg)

	assertNoPageScroll(t, win)
	bottom := evalNumber(t, win, "document.getElementById('save-btn').getBoundingClientRect().bottom")
	height := evalNumber(t, win, "window.innerHeight")
	if bottom > height {
		t.Fatalf("Save's bottom edge is at %v, past the window's %v", bottom, height)
	}
}

// TestStampWindowShowsItsWholeFormWithoutScrolling is why the window is
// 680 points rather than the 560 it started at. Every control shown at
// once — the stamp on, a specific page chosen — must fit, because the
// line that scrolled off at 560 was the margin note, which is the one
// line here a person reads once and needs to have seen.
func TestStampWindowShowsItsWholeFormWithoutScrolling(t *testing.T) {
	c := i18n.Load("sr-Cyrl") // the longest labels of the three
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPage = "3"
	win, _ := sharedStampWindow(t, c, cfg)

	if _, err := win.Eval("document.getElementById('stamp-page').value='number';document.getElementById('stamp-page').dispatchEvent(new Event('change'))"); err != nil {
		t.Fatalf("Eval(choose a specific page): %v", err)
	}

	scrolls := evalBool(t, win,
		"document.querySelector('.stamp-form').scrollHeight > document.querySelector('.stamp-form').clientHeight + 1")
	if scrolls {
		h := evalNumber(t, win, "document.querySelector('.stamp-form').scrollHeight")
		cH := evalNumber(t, win, "document.querySelector('.stamp-form').clientHeight")
		t.Fatalf("the form scrolls with every control shown: %v of %v visible", cH, h)
	}
}

// TestStampSettingsSanitiseTheReferenceLine: the reference is free text
// a person types and this project draws into a PDF other people read.
// F5 §5.3's rule about untrusted display text applies to text the user
// supplies about themselves too.
func TestStampSettingsSanitiseTheReferenceLine(t *testing.T) {
	form := stampSettings{
		Saved:     true,
		Visible:   true,
		Position:  consent.StampPositionBottomRight,
		Page:      config.StampPageFirst,
		Reference: "Ugovor\u202E2026\u0001/114",
	}
	got := applyStampSettings(config.Default(), form)
	if strings.ContainsRune(got.StampReference, '\u202E') {
		t.Fatalf("a direction override survived into the stamp reference: %q", got.StampReference)
	}
	if strings.ContainsRune(got.StampReference, '\u0001') {
		t.Fatalf("a control character survived into the stamp reference: %q", got.StampReference)
	}
	if got.StampReference != "Ugovor2026/114" {
		t.Fatalf("reference = %q, want the printable characters kept", got.StampReference)
	}
}

// TestStampSettingsRejectAPageItDoesNotUnderstand keeps a value the
// page could never legitimately produce out of the configuration file.
func TestStampSettingsRejectAPageItDoesNotUnderstand(t *testing.T) {
	cfg := config.Default()
	cfg.StampPage = "3"
	got := applyStampSettings(cfg, stampSettings{
		Saved: true, Visible: true,
		Position: consent.StampPositionBottomRight,
		Page:     "middle",
	})
	if got.StampPage != "3" {
		t.Fatalf("StampPage = %q, want the previous value kept rather than an unrecognised one written", got.StampPage)
	}
}
