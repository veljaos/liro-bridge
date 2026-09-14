//go:build windows

package main

// An option that cannot work on this path is not offered on it.
//
// The method screen's first option — sign, choosing where the signature
// goes — opens the placement picker on the batch's first document. A
// batch that arrived over the protocol has no first document on disk,
// so the picker has nothing to open, and choosing it could only ever
// end in a refusal. The refusal is still there, and is now the backstop
// rather than the thing a person meets.

import (
	"context"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// TestOnlyABatchWithFilesIsOfferedThePicker is the rule itself, over the
// two kinds of batch this program has, built by the functions the
// product calls rather than by assembling the state here (D-134).
func TestOnlyABatchWithFilesIsOfferedThePicker(t *testing.T) {
	local := newMainWindow(config.Default(), "sr-Latn")
	local.inputs = []interactiveInput{{path: `C:\docs\ugovor.pdf`}}
	if !local.canPlaceByLooking() {
		t.Error("a dropped document is a file, so the picker has a page to open")
	}
	if got := local.methodsOffered(); !equalStrings(got, allStampMethods) {
		t.Errorf("a local batch is offered %v, want all three %v", got, allStampMethods)
	}

	remote := protocolWindowFor(t, documentRequest(t, 2))
	if remote.canPlaceByLooking() {
		t.Error("a protocol batch's documents are not files; there is nothing for the picker to open")
	}
	if got := remote.methodsOffered(); !equalStrings(got, offeredStampMethods) {
		t.Errorf("a protocol batch is offered %v, want %v", got, offeredStampMethods)
	}
}

// TestTheMethodScreenLeavesOutTheOptionItCannotOffer measures the
// rendered screen, because the finding this closes was about what a
// person sees: a green suite is not evidence about what a window shows
// (D-087, D-122, D-161, D-172, D-219, D-247).
//
// Absent, not disabled, and the difference is asserted rather than
// assumed: the card must not render at all, and nothing on the screen
// may be a disabled control — a greyed row invites "why not", which is
// a sentence nobody has written.
func TestTheMethodScreenLeavesOutTheOptionItCannotOffer(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		cfg := config.Default()
		cfg.VisibleStamp = true
		win, _ := sharedStampWindowOffering(t, c, cfg, stampRoleStep, offeredStampMethods)

		// What is on screen is the two that can work, in order.
		visible := evalText(t, win, `Array.from(document.querySelectorAll('.stamp-method'))
			.filter(function(el){ return getComputedStyle(el).display !== 'none' })
			.map(function(el){ return el.querySelector('input').value }).join(',')`)
		if want := strings.Join(offeredStampMethods, ","); visible != want {
			t.Errorf("%s: the screen offers %q, want %q", locale, visible, want)
		}

		// The first card renders nothing at all, so it is in no tab
		// order, no arrow-key group and no accessibility tree.
		if d := evalText(t, win,
			`getComputedStyle(document.getElementById('method-placed').parentElement).display`); d != "none" {
			t.Errorf("%s: the option that cannot work still renders (display: %s)", locale, d)
		}
		if h := evalNumber(t, win,
			`document.getElementById('method-placed').parentElement.getBoundingClientRect().height`); h != 0 {
			t.Errorf("%s: the absent option still takes %v points of the screen", locale, h)
		}

		// Absent rather than disabled: nothing anywhere on this screen
		// is a control a person can see and cannot use.
		if n := evalNumber(t, win, `document.querySelectorAll('[disabled],[aria-disabled="true"]').length`); n != 0 {
			t.Errorf("%s: the screen carries %v disabled control(s); the rule is absent, not greyed", locale, n)
		}

		// And it is not the chosen one, which would be a method nobody
		// could see, change or tab to.
		if got := evalText(t, win,
			`(document.querySelector('input[name="stamp-method"]:checked')||{}).value || ''`); got != stampMethodCorners {
			t.Errorf("%s: the chosen method is %q, want %q", locale, got, stampMethodCorners)
		}
		// The corners it names are on screen, so the screen still asks
		// its one question completely.
		if d := evalText(t, win, `getComputedStyle(document.getElementById('stamp-corners')).display`); d == "none" {
			t.Errorf("%s: the corners are not on screen for the method that needs them", locale)
		}

		// Nothing was refused, so nothing is said. The status line is
		// where a refusal would have gone.
		if !evalBool(t, win, `document.getElementById('stamp-status').hidden`) {
			t.Errorf("%s: the screen explains a refusal that never happened", locale)
		}
	}
}

// TestTheMethodScreenFitsWhateverItOffers measures the window at its
// own size for each offering, which is the only way this can be
// asserted: a size constant checked against a differently-sized window
// proves nothing about the window a person opens (D-124).
//
// Both directions. The two-method screen must not leave the white a
// window sized for three would, and the three-method screen must still
// fit — a single lower height would have made it scroll, which is the
// defect D-202 and D-208 each shipped once by arithmetic.
func TestTheMethodScreenFitsWhateverItOffers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		offered []string
		height  int
	}{
		{"three methods", allStampMethods, stepMethodHeight},
		{"two methods", offeredStampMethods, stepMethodTwoHeight},
	} {
		win, err := ui.NewWindow(ui.Options{
			Title:       "method " + tc.name,
			Width:       stepMethodWidth,
			Height:      tc.height,
			Assets:      assetsFS,
			VirtualHost: liroVirtualHost,
			StartPage:   "/pages/stamp.html",
		})
		if err != nil {
			t.Fatalf("%s: NewWindow: %v", tc.name, err)
		}
		for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
			c := i18n.Load(locale)
			cfg := config.Default()
			cfg.VisibleStamp = true
			// The corners chosen: the fullest either screen gets, since
			// the four corners are taller than anything the other
			// methods reveal.
			payload := buildStampInit(c, cfg, stampRoleStep, stampMethodCorners, tc.offered)
			m := newMainWindow(cfg, locale)
			m.method = stampMethodCorners
			payload["step"] = m.headerFor(stepMethod)
			if err := win.PostJSON(payload); err != nil {
				t.Fatalf("%s/%s: PostJSON: %v", tc.name, locale, err)
			}

			form := evalNumber(t, win, `document.querySelector('.stamp-form').scrollHeight`)
			room := evalNumber(t, win, `document.querySelector('.stamp-form').clientHeight`)
			if form > room+1 {
				t.Errorf("%s/%s: the form needs %v of the %v it has, so it scrolls",
					tc.name, locale, form, room)
			}
			// And the slack under the last card is the headroom the
			// three-method screen was sized for, not a card's worth of
			// nothing.
			slack := evalNumber(t, win,
				`document.querySelector('.actions').getBoundingClientRect().top -
				 document.getElementById('stamp-methods').getBoundingClientRect().bottom`)
			if slack > 46 {
				t.Errorf("%s/%s: %v points of white under the last card, which is more than a card and its gap",
					tc.name, locale, slack)
			}
			if slack < 0 {
				t.Errorf("%s/%s: the cards overrun the actions by %v points", tc.name, locale, -slack)
			}
		}
		if err := win.Close(); err != nil {
			t.Errorf("%s: Close: %v", tc.name, err)
		}
	}
}

// TestABatchOfFilesStillGetsAllThreeMethods is the other half: the
// local path is untouched. Without it the test above passes just as
// happily against a screen that never offers the picker to anybody.
func TestABatchOfFilesStillGetsAllThreeMethods(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	win, _ := sharedStampWindowOffering(t, c, cfg, stampRoleStep, allStampMethods)

	visible := evalText(t, win, `Array.from(document.querySelectorAll('.stamp-method'))
		.filter(function(el){ return getComputedStyle(el).display !== 'none' })
		.map(function(el){ return el.querySelector('input').value }).join(',')`)
	if want := strings.Join(allStampMethods, ","); visible != want {
		t.Fatalf("a local batch is offered %q, want %q", visible, want)
	}
	// And the option is a real target rather than a row that happens to
	// have a height.
	chooseStampMethod(t, win, stampMethodPlaced)
	if got := evalText(t, win,
		`(document.querySelector('input[name="stamp-method"]:checked')||{}).value || ''`); got != stampMethodPlaced {
		t.Fatalf("the first method cannot be chosen locally: chosen is %q", got)
	}
}

// TestSettingsAlwaysOffersAllThree pins the window's other role. A
// standing preference is about how this person's own documents should
// look, and every one of the three is reachable for a document they
// dropped themselves — so nothing about a protocol batch may reach it.
func TestSettingsAlwaysOffersAllThree(t *testing.T) {
	c := i18n.Load("sr-Latn")
	payload := buildStampInit(c, config.Default(), stampRoleSettings, stampMethodOf(config.Default()), allStampMethods)
	got, ok := payload["methods"].([]string)
	if !ok || !equalStrings(got, allStampMethods) {
		t.Fatalf("Settings offers %v, want all three %v", payload["methods"], allStampMethods)
	}
}

// TestAnUnofferedMethodIsNeverNamedAsTheChosenOne is the Go half of the
// same property. The page would otherwise have to check a control
// nobody can see, and a person would have no way of knowing what they
// were about to sign with.
func TestAnUnofferedMethodIsNeverNamedAsTheChosenOne(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cfg := config.Default()
	cfg.VisibleStamp = true
	cfg.StampPosition = config.StampPositionCustom
	cfg.StampPlacedPage = 2
	cfg.StampX, cfg.StampY = 100, 200
	if stampMethodOf(cfg) != stampMethodPlaced {
		t.Fatal("the fixture does not mean a placed position, so this test proves nothing")
	}

	payload := buildStampInit(c, cfg, stampRoleStep, stampMethodPlaced, offeredStampMethods)
	model, _ := payload["model"].(map[string]any)
	if got := model["method"]; got != stampMethodCorners {
		t.Fatalf("the payload names %q as chosen while offering %v", got, offeredStampMethods)
	}
}

// TestTheRefusalIsStillThereForABatchWithNoFiles keeps the backstop
// honest. methodsOffered has made it unreachable from the screen, which
// is exactly why it is worth a test: an unreachable branch is one that
// can be deleted or broken with nothing to say so, and it is what
// protects this path if anything ever reaches the method by another
// route.
//
// It also pins which of the two sentences is used. place.unavailable
// says a document cannot be *displayed*, which is true of a local file
// this project's rasteriser cannot draw and wrong here: there is no
// document on disk to display at all.
func TestTheRefusalIsStillThereForABatchWithNoFiles(t *testing.T) {
	c := i18n.Load("sr-Latn")
	m := protocolWindowFor(t, documentRequest(t, 1))
	rec := &recordingWindow{}
	m.win = rec
	m.c = c
	m.method = stampMethodPlaced

	if m.signAtAChosenPosition(context.Background()) {
		t.Fatal("a batch with no files signed at a position nobody could have chosen")
	}
	if m.method != stampMethodCorners {
		t.Errorf("after the refusal the method is %q, want the corners", m.method)
	}

	want := c.T("place.unavailable_no_file")
	if want == "place.unavailable_no_file" {
		t.Fatal("the catalogue has no sentence for a batch whose documents are not files")
	}
	if want == c.T("place.unavailable") {
		t.Fatal("the two refusals say the same thing, so one of them is describing the wrong condition")
	}
	if !recordedStatus(rec, want) {
		t.Errorf("the refusal did not say why; the window was sent %v", rec.posted("status"))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// recordedStatus reports whether the window was sent a status line
// carrying text, in the warning intent.
func recordedStatus(rec *recordingWindow, text string) bool {
	for _, p := range rec.posted("status") {
		st, ok := p["status"].(map[string]any)
		if ok && st["text"] == text && st["intent"] == string(ui.IntentWarning) {
			return true
		}
	}
	return false
}
