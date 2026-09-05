//go:build windows

package main

// Step 3 of the three-step flow — how to sign — driven against a real
// WebView2 window.

import (
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestStampWindowAsksVisibleOrInvisibleAndWhichCorner is step 3's one
// question. Everything else the stamp can carry is still reachable, and
// still here — behind the disclosure, where a decision about this batch
// is not competing with a standing preference.
func TestStampWindowAsksVisibleOrInvisibleAndWhichCorner(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	win, _ := sharedStampWindow(t, c, cfg, stampRoleStep)

	modes := evalText(t, win,
		"Array.from(document.getElementById('stamp-mode').options).map(function(o){return o.value}).join(',')")
	if modes != "visible,invisible" {
		t.Fatalf("modes = %q, want visible,invisible", modes)
	}
	if got := evalText(t, win, "document.getElementById('stamp-mode').value"); got != "visible" {
		t.Fatalf("mode = %q for a configuration with the stamp on, want visible", got)
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

	// Step 3 asks one thing: the standing preferences are not on this
	// screen at all when this window is the last step before signing.
	if d := evalText(t, win, "getComputedStyle(document.getElementById('stamp-more')).display"); d != "none" {
		t.Fatalf("step 3 shows the standing preferences (display: %s), so it asks three questions instead of one", d)
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

// TestStampWindowHidesTheCornerForAnInvisibleSignature: controls for
// something nobody is drawing are noise (the same judgement D-103 made
// for the corner selector on the consent screen).
func TestStampWindowHidesTheCornerForAnInvisibleSignature(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = false
	// Settings' role, because that is the one that shows every control
	// an invisible signature has to hide.
	win, _ := sharedStampWindow(t, c, cfg, stampRoleSettings)

	// D-106's trap: `hidden` being true is not the same as the element
	// being invisible, because an author `display` rule outranks the
	// user-agent's [hidden] rule. Read what is rendered.
	for _, id := range []string{"stamp-position-field", "stamp-more"} {
		if d := evalText(t, win, "getComputedStyle(document.getElementById('"+id+"')).display"); d != "none" {
			t.Fatalf("%s still renders for an invisible signature (display: %s)", id, d)
		}
	}

	if _, err := win.Eval("document.getElementById('stamp-mode').value = 'visible'; document.getElementById('stamp-mode').dispatchEvent(new Event('change'))"); err != nil {
		t.Fatalf("Eval(choose visible): %v", err)
	}
	for _, id := range []string{"stamp-position-field", "stamp-more"} {
		if d := evalText(t, win, "getComputedStyle(document.getElementById('"+id+"')).display"); d == "none" {
			t.Fatalf("%s stays hidden after choosing a visible signature", id)
		}
	}
}

// TestStampWindowLabelsItsPrimaryActionForWhereItWasOpenedFrom: the
// same window ends a signature and edits a preference, and says which.
func TestStampWindowLabelsItsPrimaryActionForWhereItWasOpenedFrom(t *testing.T) {
	c := i18n.Load("sr-Latn")
	for _, tc := range []struct {
		role               stampWindowRole
		primary, secondary string
	}{
		{stampRoleStep, c.T("stampwindow.sign"), c.T("stampwindow.back")},
		{stampRoleSettings, c.T("stampwindow.save"), c.T("stampwindow.cancel")},
	} {
		win, _ := sharedStampWindow(t, c, config.Default(), tc.role)
		if got := evalText(t, win, "document.getElementById('save-btn').textContent"); got != tc.primary {
			t.Errorf("primary action = %q, want %q", got, tc.primary)
		}
		if got := evalText(t, win, "document.getElementById('cancel-btn').textContent"); got != tc.secondary {
			t.Errorf("secondary action = %q, want %q", got, tc.secondary)
		}
	}
}

// TestStampWindowPageNumberAppearsOnlyForASpecificPage covers the same
// rule one level down.
func TestStampWindowPageNumberAppearsOnlyForASpecificPage(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPage = config.StampPageFirst
	win, _ := sharedStampWindow(t, c, cfg, stampRoleStep)

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
	win, messages := sharedStampWindow(t, c, cfg, stampRoleStep)

	script := strings.Join([]string{
		"document.getElementById('stamp-mode').value = 'visible';",
		"document.getElementById('stamp-mode').dispatchEvent(new Event('change'));",
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
	win, _ := sharedStampWindow(t, c, config.Default(), stampRoleStep)

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
			win, _ := sharedStampWindow(t, c, config.Default(), stampRoleStep)
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

// TestStampWindowFitsBothRolesWithoutScrolling is why this window has
// two sizes rather than one. Step 3 asks one thing; Settings shows the
// standing preferences too. Neither may scroll — a form that scrolls is
// a form whose last field can be the one nobody sees (D-106).
//
// Each role is measured in a window of its own actual size, not in the
// shared one: a size constant checked against a window created at some
// other size proves nothing about the window a person opens.
func TestStampWindowFitsBothRolesWithoutScrolling(t *testing.T) {
	c := i18n.Load("sr-Cyrl") // the longest labels of the three
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPage = "3" // the page-number box shown too
	cfg.StampReference = "Ugovor 2026/114"

	for _, role := range []stampWindowRole{stampRoleStep, stampRoleSettings} {
		win, err := ui.NewWindow(ui.Options{
			Title:       c.T("stampwindow.title"),
			Width:       stampWindowWidth,
			Height:      stampWindowHeight(role),
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/stamp.html",
		})
		if err != nil {
			t.Fatalf("NewWindow(role %v): %v", role, err)
		}
		if err := win.PostJSON(buildStampInit(c, cfg, role)); err != nil {
			t.Fatalf("PostJSON(role %v): %v", role, err)
		}

		scrolls := evalBool(t, win,
			"document.querySelector('.stamp-form').scrollHeight > document.querySelector('.stamp-form').clientHeight + 1")
		if scrolls {
			h := evalNumber(t, win, "document.querySelector('.stamp-form').scrollHeight")
			cH := evalNumber(t, win, "document.querySelector('.stamp-form').clientHeight")
			t.Errorf("role %v at %d points scrolls: %v of %v visible", role, stampWindowHeight(role), cH, h)
		}
		assertNoPageScroll(t, win)
		bottom := evalNumber(t, win, "document.getElementById('save-btn').getBoundingClientRect().bottom")
		if height := evalNumber(t, win, "window.innerHeight"); bottom > height {
			t.Errorf("role %v: the primary action's bottom edge is at %v, past the window's %v", role, bottom, height)
		}
		_ = win.Close()
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

// TestStepThreeIsTitledForWhatItAsksAndSaysNothingElse is Task 4 of the
// first-use fix pass.
//
// The window is named for the question it asks — the method of signing
// — rather than for the act of asking it, and it carries no subtitle:
// the one it had said "the last step, everything else is already
// decided", which tells a person what they can already see, on a window
// whose whole value is being small. Settings keeps a subtitle, because
// there the window is a standing preference and the line says which.
func TestStepThreeIsTitledForWhatItAsksAndSaysNothingElse(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		title := c.T("stampwindow.title")
		if title == "stampwindow.title" {
			t.Fatalf("%s: no title in the catalogue", locale)
		}
		for _, gone := range []string{"Kako potpisati", "Како потписати", "How to sign"} {
			if title == gone {
				t.Fatalf("%s: the window is still titled %q", locale, gone)
			}
		}
		// The deleted key is gone from every catalogue, not merely
		// unused by Go: a key left behind is a key someone wires back.
		if s := c.T("stampwindow.subtitle"); s != "stampwindow.subtitle" {
			t.Fatalf("%s: stampwindow.subtitle is still in the catalogue as %q", locale, s)
		}

		win, messages := sharedStampWindow(t, c, config.Default(), stampRoleStep)
		_ = messages
		if !evalBool(t, win, "document.getElementById('stamp-subtitle').hidden") {
			text := evalText(t, win, "document.getElementById('stamp-subtitle').textContent")
			t.Fatalf("%s: step 3 still shows a subtitle: %q", locale, text)
		}
		if h := evalNumber(t, win, "document.getElementById('stamp-subtitle').getBoundingClientRect().height"); h != 0 {
			t.Fatalf("%s: the empty subtitle still takes %v points of the window", locale, h)
		}
		if got := evalText(t, win, "document.querySelector('.liro-display').textContent"); got != title {
			t.Fatalf("%s: the window shows %q, want %q", locale, got, title)
		}

		// Settings' way in is the same window and does still explain
		// itself.
		if err := win.PostJSON(buildStampInit(c, config.Default(), stampRoleSettings)); err != nil {
			t.Fatal(err)
		}
		if evalBool(t, win, "document.getElementById('stamp-subtitle').hidden") {
			t.Fatalf("%s: the settings role lost its subtitle too", locale)
		}
	}
}
