//go:build windows

package main

// Step 3 of the three-step flow — the signing method — driven against a
// real WebView2 window.

import (
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestStepThreeIsOneChoiceWithThreeOutcomes is the shape of this
// screen: three options, one under another, in the order the person
// was promised, each a button-sized target — and no checkbox anywhere,
// because a control that changes which controls exist is exactly the
// nesting the three options replace.
func TestStepThreeIsOneChoiceWithThreeOutcomes(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	win, _ := sharedStampWindow(t, c, cfg, stampRoleStep)

	methods := evalText(t, win,
		`Array.from(document.querySelectorAll('input[name="stamp-method"]')).map(function(i){return i.value}).join(',')`)
	if want := strings.Join([]string{stampMethodPlaced, stampMethodCorners, stampMethodNone}, ","); methods != want {
		t.Fatalf("the methods are %q, want %q", methods, want)
	}

	// Each one carries the catalogue's own words, and each is a real
	// target rather than a line of text: a rendered box with a height a
	// finger or a pointer can land on.
	for i, key := range []string{
		"stampwindow.method_placed", "stampwindow.method_corners", "stampwindow.method_none",
	} {
		label := evalText(t, win,
			"document.querySelectorAll('.stamp-method-label')["+itoaTest(i)+"].textContent")
		if label != c.T(key) {
			t.Errorf("option %d reads %q, want %q", i+1, label, c.T(key))
		}
		h := evalNumber(t, win,
			"document.querySelectorAll('.stamp-method')["+itoaTest(i)+"].getBoundingClientRect().height")
		if h < 32 {
			t.Errorf("option %d is %v points tall, which is not a button-sized target", i+1, h)
		}
	}

	// The checkbox is gone: the three options make it redundant.
	if n := evalNumber(t, win, "document.querySelectorAll('#stamp-methods input[type=\"checkbox\"]').length"); n != 0 {
		t.Errorf("the method screen still carries %v checkbox(es)", n)
	}

	// Step 3 asks one thing: the standing preferences are not on this
	// screen at all when this window is the last step before signing.
	if d := evalText(t, win, "getComputedStyle(document.getElementById('stamp-more')).display"); d != "none" {
		t.Fatalf("step 3 shows the standing preferences (display: %s), so it asks more than one thing", d)
	}
}

// TestTheCornersAppearOnlyUnderTheSecondOption is the behaviour the
// three options are for: the corner choice is inline beneath the
// option that needs it, on the same screen, and nowhere else.
func TestTheCornersAppearOnlyUnderTheSecondOption(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	win, _ := sharedStampWindow(t, c, cfg, stampRoleStep)

	// A corner placement is what config.Default() means, so the corners
	// are already showing, in the order they sit on a page.
	corners := evalText(t, win,
		`Array.from(document.querySelectorAll('input[name="stamp-corner"]')).map(function(i){return i.value}).join(',')`)
	want := strings.Join([]string{
		consent.StampPositionTopLeft, consent.StampPositionTopRight,
		consent.StampPositionBottomLeft, consent.StampPositionBottomRight,
	}, ",")
	if corners != want {
		t.Fatalf("the corners are %q, want %q", corners, want)
	}
	for _, key := range []string{
		"stampwindow.position_top_left", "stampwindow.position_top_right",
		"stampwindow.position_bottom_left", "stampwindow.position_bottom_right",
	} {
		if !strings.Contains(evalText(t, win, "document.getElementById('stamp-corner-grid').textContent"), c.T(key)) {
			t.Errorf("the corner grid does not carry %q", c.T(key))
		}
	}

	// D-106's trap: `hidden` being true is not the same as the element
	// being invisible, because an author `display` rule outranks the
	// user-agent's [hidden] rule. Read what is rendered.
	for _, tc := range []struct {
		method  string
		showing string
		hidden  []string
	}{
		// As a step of signing, only the second method reveals
		// anything: the first has a step of its own after it and the
		// third has nothing left to ask.
		{stampMethodPlaced, "", []string{"stamp-corners", "stamp-placed"}},
		{stampMethodCorners, "stamp-corners", []string{"stamp-placed"}},
		{stampMethodNone, "", []string{"stamp-placed", "stamp-corners"}},
	} {
		chooseStampMethod(t, win, tc.method)
		if tc.showing != "" {
			if d := evalText(t, win, "getComputedStyle(document.getElementById('"+tc.showing+"')).display"); d == "none" {
				t.Errorf("%s: %s is not on screen", tc.method, tc.showing)
			}
		}
		for _, id := range tc.hidden {
			if d := evalText(t, win, "getComputedStyle(document.getElementById('"+id+"')).display"); d != "none" {
				t.Errorf("%s: %s still renders (display: %s)", tc.method, id, d)
			}
		}
	}
}

// TestTheThirdOptionExplainsItselfInItsOwnTitle is Task 2: the line
// under "sign with no visual mark" said what the option's own title
// already says, and it is gone — from the page and from all three
// catalogues, so nobody wires it back.
//
// What the option has to stay is unmistakable: "no visual mark", never
// "a mark somewhere I cannot see", which is exactly how the invisible
// default was first reported (D-103). That work is done by the title.
func TestTheThirdOptionExplainsItselfInItsOwnTitle(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		if got := c.T("stampwindow.method_none_hint"); got != "stampwindow.method_none_hint" {
			t.Errorf("%s: the deleted explanation is still in the catalogue as %q", locale, got)
		}
		win, _ := sharedStampWindow(t, c, config.Default(), stampRoleStep)
		if !evalBool(t, win, "document.getElementById('stamp-none-note') === null") {
			t.Errorf("%s: the third option still carries an explanation under it", locale)
		}
		chooseStampMethod(t, win, stampMethodNone)
		// The title still says it, and says it as a real target.
		if got := evalText(t, win, "document.querySelectorAll('.stamp-method-label')[2].textContent"); got != c.T("stampwindow.method_none") {
			t.Errorf("%s: the third option reads %q, want %q", locale, got, c.T("stampwindow.method_none"))
		}
		// And nothing about a corner is on screen: an invisible
		// signature has no position to argue about.
		if d := evalText(t, win, "getComputedStyle(document.getElementById('stamp-corners')).display"); d != "none" {
			t.Errorf("%s: the corners still render for a signature nobody will see", locale)
		}
	}
}

// TestTheMethodUsedLastTimeIsPreselected is what makes a returning user
// confirm rather than decide — and it needs no new setting: the three
// methods are what the configuration already says.
func TestTheMethodUsedLastTimeIsPreselected(t *testing.T) {
	c := i18n.Load("sr-Latn")

	corners := config.Default()
	corners.VisibleStamp = true
	corners.StampPosition = consent.StampPositionTopLeft

	invisible := config.Default()
	invisible.VisibleStamp = false

	placed := config.Default()
	placed.VisibleStamp = true
	placed.StampPosition = config.StampPositionCustom
	placed.StampPlacedPage = 2
	placed.StampX, placed.StampY = 100, 200

	for _, tc := range []struct {
		name   string
		cfg    config.Config
		method string
		corner string
	}{
		{"a corner", corners, stampMethodCorners, consent.StampPositionTopLeft},
		{"nothing shown", invisible, stampMethodNone, consent.StampPositionBottomRight},
		{"a placed position", placed, stampMethodPlaced, consent.StampPositionBottomRight},
	} {
		if got := stampMethodOf(tc.cfg); got != tc.method {
			t.Errorf("%s: stampMethodOf = %q, want %q", tc.name, got, tc.method)
		}
		win, _ := sharedStampWindow(t, c, tc.cfg, stampRoleStep)
		got := evalText(t, win, `document.querySelector('input[name="stamp-method"]:checked').value`)
		if got != tc.method {
			t.Errorf("%s: the window preselected %q, want %q", tc.name, got, tc.method)
		}
		// The selected option looks selected, not merely reports itself
		// as such: the radio inside the card is invisible by design.
		if !evalBool(t, win, `document.querySelector('input[name="stamp-method"]:checked').parentElement.classList.contains("stamp-method-selected")`) {
			t.Errorf("%s: the chosen option is not marked on screen", tc.name)
		}
		corner := evalText(t, win, `document.querySelector('input[name="stamp-corner"]:checked').value`)
		if corner != tc.corner {
			t.Errorf("%s: the corner offered is %q, want %q", tc.name, corner, tc.corner)
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
		{stampRoleStep, c.T("stampwindow.sign"), c.T("stampwindow.cancel")},
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

// TestSettingsKeepsTheStandingPreferencesAndTheSameWords: Settings sets
// a default rather than making a choice for one batch, so it holds
// what a preference owns and a batch does not — and it says "method"
// and "position" in the same words step 3 does, because a second
// vocabulary for one choice is how two screens come to disagree.
func TestSettingsKeepsTheStandingPreferencesAndTheSameWords(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	win, _ := sharedStampWindow(t, c, cfg, stampRoleSettings)

	for _, id := range []string{"stamp-page", "stamp-reference", "stamp-document-id", "stamp-page-number"} {
		if evalBool(t, win, "document.getElementById('"+id+"') === null") {
			t.Fatalf("the settings role has no %s control", id)
		}
	}
	if d := evalText(t, win, "getComputedStyle(document.getElementById('stamp-more')).display"); d == "none" {
		t.Fatal("the settings role hides the standing preferences it exists to hold")
	}

	// The same three methods, with the same words.
	methods := evalText(t, win,
		`Array.from(document.querySelectorAll('input[name="stamp-method"]')).map(function(i){return i.value}).join(',')`)
	if want := strings.Join([]string{stampMethodPlaced, stampMethodCorners, stampMethodNone}, ","); methods != want {
		t.Fatalf("Settings offers %q, want the same three methods %q", methods, want)
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

	// And nothing a person is not drawing is on screen: choosing "no
	// visual mark" takes the standing preferences with it.
	chooseStampMethod(t, win, stampMethodNone)
	if d := evalText(t, win, "getComputedStyle(document.getElementById('stamp-more')).display"); d != "none" {
		t.Fatalf("the standing preferences still render for a signature nobody will see (display: %s)", d)
	}
}

// TestStampWindowPageNumberAppearsOnlyForASpecificPage covers the same
// rule one level down.
func TestStampWindowPageNumberAppearsOnlyForASpecificPage(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPage = config.StampPageFirst
	win, _ := sharedStampWindow(t, c, cfg, stampRoleSettings)

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
	win, messages := sharedStampWindow(t, c, cfg, stampRoleSettings)

	chooseStampMethod(t, win, stampMethodCorners)
	script := strings.Join([]string{
		"document.getElementById('corner-top-left').click();",
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
	if form.Method != stampMethodCorners {
		t.Errorf("method = %q, want %q", form.Method, stampMethodCorners)
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

// TestChoosingTheThirdOptionTurnsTheStampOff is the same round trip for
// the outcome that draws nothing.
func TestChoosingTheThirdOptionTurnsTheStampOff(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = consent.StampPositionTopRight
	win, messages := sharedStampWindow(t, c, cfg, stampRoleStep)

	chooseStampMethod(t, win, stampMethodNone)
	drain(messages)
	if _, err := win.Eval("document.getElementById('save-btn').click(); 'ok'"); err != nil {
		t.Fatal(err)
	}
	<-messages

	form, err := readStampSettings(win)
	if err != nil {
		t.Fatal(err)
	}
	if form.Method != stampMethodNone {
		t.Fatalf("method = %q, want %q", form.Method, stampMethodNone)
	}
	got := applyStampSettings(cfg, form)
	if got.VisibleStamp {
		t.Error("choosing 'nothing shown' left the stamp on")
	}
	// The corner is kept rather than thrown away: turning the stamp
	// back on must not lose the corner that was chosen for it.
	if got.StampPosition != consent.StampPositionTopRight {
		t.Errorf("the corner was lost: %q", got.StampPosition)
	}
	// And nothing is drawn: SPEC §13.4's default path, untouched.
	if stampOptionsFor(c, got) != nil {
		t.Error("a signature nobody will see still asked for a stamp to be drawn")
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
			labels := evalText(t, win, "document.getElementById('stamp-methods').textContent")
			if strings.Contains(labels, "stampwindow.") {
				t.Errorf("the three options rendered a raw key in %s: %q", locale, labels)
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
// other size proves nothing about the window a person opens. And each
// is measured in every method, because which one is chosen decides
// what is revealed underneath it.
func TestStampWindowFitsBothRolesWithoutScrolling(t *testing.T) {
	c := i18n.Load("sr-Cyrl") // the longest labels of the three
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPage = "3" // the page-number box shown too
	cfg.StampReference = "Ugovor 2026/114"
	// And with a placed position remembered, which is what the first
	// method has to show. A window sized for its emptiest state is a
	// window that scrolls the moment somebody uses it.
	cfg.StampPlacedPage = 12
	cfg.StampX, cfg.StampY = 393.32, 12

	for _, tc := range []struct {
		role   stampWindowRole
		height int
	}{
		{stampRoleStep, stepMethodHeight},
		{stampRoleSettings, stampSettingsHeight},
	} {
		role, roleHeight := tc.role, tc.height
		win, err := ui.NewWindow(ui.Options{
			Title:       c.T("stampwindow.title"),
			Width:       stampWindowWidth,
			Height:      roleHeight,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/stamp.html",
		})
		if err != nil {
			t.Fatalf("NewWindow(role %v): %v", role, err)
		}
		for _, method := range []string{stampMethodPlaced, stampMethodCorners, stampMethodNone} {
			payload := buildStampInit(c, cfg, role, method)
			if role == stampRoleStep {
				// With the step header the real screen carries. Measuring
				// without it is how this test passed at 345 points while
				// the third option was cut off on screen.
				m := newMainWindow(cfg, "sr-Cyrl")
				m.method = method
				payload["step"] = m.headerFor(stepMethod)
			}
			if err := win.PostJSON(payload); err != nil {
				t.Fatalf("PostJSON(role %v, method %s): %v", role, method, err)
			}

			scrolls := evalBool(t, win,
				"document.querySelector('.stamp-form').scrollHeight > document.querySelector('.stamp-form').clientHeight + 1")
			if scrolls {
				h := evalNumber(t, win, "document.querySelector('.stamp-form').scrollHeight")
				cH := evalNumber(t, win, "document.querySelector('.stamp-form').clientHeight")
				t.Errorf("role %v, method %s at %d points scrolls: %v of %v visible",
					role, method, roleHeight, cH, h)
			}
			assertNoPageScroll(t, win)
			bottom := evalNumber(t, win, "document.getElementById('save-btn').getBoundingClientRect().bottom")
			if height := evalNumber(t, win, "window.innerHeight"); bottom > height {
				t.Errorf("role %v, method %s: the primary action's bottom edge is at %v, past the window's %v",
					role, method, bottom, height)
			}
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
		Method:    stampMethodCorners,
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
		Saved:    true,
		Method:   stampMethodCorners,
		Position: consent.StampPositionBottomRight,
		Page:     "middle",
	})
	if got.StampPage != "3" {
		t.Fatalf("StampPage = %q, want the previous value kept rather than an unrecognised one written", got.StampPage)
	}
}

// TestStampSettingsRejectAMethodItDoesNotUnderstand is the same rule
// for the choice this screen is: a value the page could not have
// produced leaves the configuration as it was rather than turning the
// stamp off by accident.
func TestStampSettingsRejectAMethodItDoesNotUnderstand(t *testing.T) {
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = consent.StampPositionTopLeft
	got := applyStampSettings(cfg, stampSettings{
		Saved: true, Method: "sideways", Page: config.StampPageFirst,
	})
	if !got.VisibleStamp || got.StampPosition != consent.StampPositionTopLeft {
		t.Fatalf("an unrecognised method changed the configuration: %+v", got)
	}
}

// TestStepThreeIsTitledForWhatItAsksAndSaysNothingElse is Task 4 of the
// first-use fix pass, kept: the window is named for the question it
// asks — the method of signing — and carries no subtitle. The one it
// had said "the last step, everything else is already decided", which
// tells a person what they can already see, on a window whose whole
// value is being small. Settings keeps a subtitle, because there the
// window is a standing preference and the line says which.
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
		// The deleted keys are gone from every catalogue, not merely
		// unused by Go: a key left behind is a key someone wires back.
		for _, key := range []string{
			"stampwindow.subtitle",
			"stampwindow.mode_label", "stampwindow.mode_visible", "stampwindow.mode_invisible",
			"stampwindow.position_placed",
			// Gone with the flow becoming one window: the way back is
			// the step header now, in one place, from one key.
			"stampwindow.method_none_hint", "stampwindow.back",
			// Gone with the position step, which asked nothing: its
			// title and the line it showed when nothing was placed.
			"stampwindow.position_step_title", "stampwindow.placed_none",
		} {
			if s := c.T(key); s != key {
				t.Fatalf("%s: %s is still in the catalogue as %q", locale, key, s)
			}
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
		if err := win.PostJSON(buildStampInit(c, config.Default(), stampRoleSettings, stampMethodCorners)); err != nil {
			t.Fatal(err)
		}
		if evalBool(t, win, "document.getElementById('stamp-subtitle').hidden") {
			t.Fatalf("%s: the settings role lost its subtitle too", locale)
		}
	}
}

// chooseStampMethod clicks one of the three options through the page's
// own DOM — D-094's carve-out, which touches nothing outside it.
func chooseStampMethod(t *testing.T, win ui.Window, method string) {
	t.Helper()
	script := "(function(){var i=document.getElementById('method-" + method + "');" +
		"i.checked=true;i.dispatchEvent(new Event('change'));return 'ok';})()"
	if _, err := win.Eval(script); err != nil {
		t.Fatalf("Eval(choose %s): %v", method, err)
	}
}
