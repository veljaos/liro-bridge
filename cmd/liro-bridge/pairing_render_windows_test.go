//go:build windows

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// testPrompt is the prompt these tests render: a real-shaped name, an
// origin whose scheme is the thing a person is meant to be able to
// notice, and a code with a leading zero.
func testPrompt() api.PairingPrompt {
	return api.PairingPrompt{
		RequestID: "0123456789abcdef0123456789abcdef",
		Code:      "042317",
		Name:      "Knjigovodstvo d.o.o. — ERP",
		Origin:    "http://erp.knjigovodstvo.rs:8443/liro/callback",
		ExpiresAt: time.Now().Add(api.PairingCodeTTL),
	}
}

// The window shows the code, and shows it whole. Six digits with a
// leading zero is a code; five is a code nobody can type.
func TestThePairingWindowShowsTheCodeAndNoAllowButton(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win, _ := sharedPairingWindow(t, c, testPrompt())

			if got := evalString(t, win, "document.getElementById('pairing-code').textContent"); got != "042317" {
				t.Fatalf("the code renders as %q, want %q", got, "042317")
			}
			if h := evalNumber(t, win, "document.getElementById('pairing-code').getBoundingClientRect().height"); h < 20 {
				t.Fatalf("the code's rendered height is %v; it is meant to be read across a desk", h)
			}

			// There is no Allow button, and that is the design: approval
			// is the six digits travelling through a person, not a
			// click. A button here would prove somebody was at the
			// machine and nothing about who they were talking to.
			buttons := evalString(t, win,
				"Array.from(document.querySelectorAll('button')).map(function(b){return b.id}).join(',')")
			for _, id := range strings.Split(buttons, ",") {
				if strings.Contains(id, "allow") || strings.Contains(id, "approve") {
					t.Fatalf("the pairing window carries an affirmative button %q; buttons are %q", id, buttons)
				}
			}

			// And the catalogues no longer carry the string one would
			// have used, so it cannot be quietly wired back in.
			if got := c.T("pairing.allow"); got != "pairing.allow" {
				t.Fatalf("pairing.allow is still in the %s catalogue as %q", locale, got)
			}
		})
	}
}

// The name and the origin are shown exactly as the application declared
// them: no prettifying, no stripping of the scheme. A person who sees
// http:// where they expected https:// must be able to notice
// (SPEC §6.2, F7 §2.1).
func TestThePairingWindowShowsTheNameAndOriginVerbatim(t *testing.T) {
	prompt := testPrompt()
	win, _ := sharedPairingWindow(t, i18n.Load("sr-Latn"), prompt)

	if got := evalString(t, win, "document.getElementById('app-name').textContent"); got != prompt.Name {
		t.Fatalf("the name renders as %q, want %q", got, prompt.Name)
	}
	if got := evalString(t, win, "document.getElementById('app-origin').textContent"); got != prompt.Origin {
		t.Fatalf("the origin renders as %q, want %q verbatim", got, prompt.Origin)
	}
}

// A caller's origin has no length bound, so it wraps rather than
// widening the window — the rule the batch fingerprint and the audit
// log's chain-break line each had to learn once (D-096, D-167).
func TestALongOriginWrapsRatherThanWideningThePairingWindow(t *testing.T) {
	prompt := testPrompt()
	prompt.Origin = "https://" + strings.Repeat("a-very-long-subdomain.", 8) + "example.com/callback"
	if len(prompt.Origin) > api.MaxOriginLength {
		prompt.Origin = prompt.Origin[:api.MaxOriginLength]
	}

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			win, _ := sharedPairingWindow(t, i18n.Load(locale), prompt)
			assertPageDoesNotScroll(t, win, "pairing ("+locale+")", ".identity")
			assertButtonsVisible(t, win, "pairing ("+locale+")", ".identity")
		})
	}
}

// A caller's *name* has no length bound either, and it is the one thing
// on this screen a person is meant to read before typing a code into
// somebody else's application. Measured at the full 120 characters
// before the identity block became the page's scrolling region: the
// page overflowed its own window, deny-btn.focus() scrolled that
// overflow, and #app-name's first line rendered at -8 on the code
// screen and -53 on the connected one — above the top of the window in
// both. An application calling itself 120 characters of padding
// followed by its real name would put the real name out of sight, and
// the person would approve what they could not see.
//
// Both screens, at the size each is really shown at, in all three
// locales. The assertion is the two properties that were false: the
// name's first line is inside the window, and the page does not scroll.
func TestALongApplicationNameStaysReadableInThePairingWindow(t *testing.T) {
	prompt := testPrompt()
	// consent.MaxDisplayLength is where a name arrives from
	// api.validatePairingRequest, so this is the longest one that can
	// reach this window at all. The padding-then-real-name shape is the
	// case the finding names: the part that identifies the application
	// is last, so it is what a truncated or scrolled-away name loses.
	prompt.Name = strings.Repeat("Padding ", 12) + "Knjigovodstvo d.o.o. ERP"
	prompt.Name = consent.SanitizeDisplayText(prompt.Name)
	prompt.Name = consent.TruncateMiddle(prompt.Name, consent.MaxDisplayLength)
	if got := len([]rune(prompt.Name)); got != consent.MaxDisplayLength {
		t.Fatalf("the name under test is %d characters, want the full %d", got, consent.MaxDisplayLength)
	}

	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			win, _ := sharedPairingWindow(t, i18n.Load(locale), prompt)

			assertPageDoesNotScroll(t, win, "pairing, long name ("+locale+")", ".identity")
			assertButtonsVisible(t, win, "pairing, long name ("+locale+")", ".identity")
			assertNameStartsInsideTheWindow(t, win, "pairing, long name ("+locale+")", "app-name")

			// And the same at the size the connected screen is shown at,
			// once the page has actually reached it (D-201).
			resizeAndSettle(t, win, pairingWindowWidth, pairingConnectedHeight)
			defer resizeAndSettle(t, win, pairingWindowWidth, pairingWindowHeight)
			if err := win.PostJSON(map[string]any{"type": "connected"}); err != nil {
				t.Fatalf("PostJSON(connected): %v", err)
			}
			assertPageDoesNotScroll(t, win, "pairing connected, long name ("+locale+")", ".identity")
			assertButtonsVisible(t, win, "pairing connected, long name ("+locale+")", ".identity")
			assertNameStartsInsideTheWindow(t, win, "pairing connected, long name ("+locale+")", "connected-name")
		})
	}
}

// The other side of making the identity block scrollable: for an
// ordinary name it must not actually scroll. A scrollbar beside a
// two-line company name is what the remedy would cost if the block were
// even a point short of what it needs, and a point is exactly what it
// was short of on the connected screen when this was first built — the
// page did not scroll, every layout test passed, and the shipped window
// had a scrollbar in it (D-167's pattern for the third time).
func TestAnOrdinaryNameLeavesTheIdentityBlockUnscrolled(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			win, _ := sharedPairingWindow(t, i18n.Load(locale), testPrompt())
			assertRegionDoesNotScroll(t, win, "pairing ("+locale+")", "#state-code .identity")

			resizeAndSettle(t, win, pairingWindowWidth, pairingConnectedHeight)
			defer resizeAndSettle(t, win, pairingWindowWidth, pairingWindowHeight)
			if err := win.PostJSON(map[string]any{"type": "connected"}); err != nil {
				t.Fatalf("PostJSON(connected): %v", err)
			}
			assertRegionDoesNotScroll(t, win, "pairing connected ("+locale+")", "#state-connected .identity")
		})
	}
}

// assertRegionDoesNotScroll is the opposite of assertScrolls: a region
// that is allowed to scroll, given content that should not make it.
func assertRegionDoesNotScroll(t *testing.T, win ui.Window, window, selector string) {
	t.Helper()
	box := evalNumbers(t, win,
		"document.querySelector("+jsStringLiteral(selector)+").scrollHeight",
		"document.querySelector("+jsStringLiteral(selector)+").clientHeight",
		"0", "0")
	if box[0] > box[1] {
		t.Errorf("%s: %s scrolls for an ordinary name — %v of %v", window, selector, box[1], box[0])
	}
}

// assertNameStartsInsideTheWindow is the half a scrolling check cannot
// see: a page that does not scroll can still hold a block scrolled past
// its own first line, and the first line is where a name begins.
//
// The four numbers come from one Eval for the reason
// assertPageDoesNotScroll takes its four that way (D-201): two
// questions asked at two moments can straddle a relayout and compare
// numbers from different layouts.
func assertNameStartsInsideTheWindow(t *testing.T, win ui.Window, window, id string) {
	t.Helper()
	box := evalNumbers(t, win,
		"document.getElementById('"+id+"').getBoundingClientRect().top",
		"document.getElementById('"+id+"').getBoundingClientRect().bottom",
		"window.innerHeight",
		"document.getElementById('"+id+"').getBoundingClientRect().height")
	top, bottom, viewport, height := box[0], box[1], box[2], box[3]
	if height <= 0 {
		t.Fatalf("%s: #%s has no rendered height; nothing was measured", window, id)
	}
	if top < -0.5 {
		t.Errorf("%s: #%s renders at %.0f..%.0f of a %.0f-point window — its first line is above the top",
			window, id, top, bottom, viewport)
	}
	if top > viewport+0.5 {
		t.Errorf("%s: #%s renders at %.0f..%.0f of a %.0f-point window — it starts below the bottom",
			window, id, top, bottom, viewport)
	}
}

// Deny is a refusal that reaches Go. Everything else about the pairing
// depends on it: the flow closes the window, records the refusal, and
// answers the application that asked.
func TestDenyingAPairingReachesGo(t *testing.T) {
	win, messages := sharedPairingWindow(t, i18n.Load("sr-Latn"), testPrompt())

	if _, err := win.Eval("document.getElementById('deny-btn').click()"); err != nil {
		t.Fatalf("Eval(click deny-btn): %v", err)
	}
	msg := recvMessage(t, messages, 5*time.Second)
	if msg.Type != ui.MessageTypeCancel {
		t.Fatalf("Deny sent %+v, want cancel", msg)
	}
}

// Nothing affirmative is ever the initially focused control (F5 §5.6).
// Here the only button is the refusal, so it is the one that has focus.
func TestThePairingWindowFocusesTheOnlyButtonItHas(t *testing.T) {
	win, _ := sharedPairingWindow(t, i18n.Load("sr-Latn"), testPrompt())

	if got := evalString(t, win, "document.activeElement.id"); got != "deny-btn" {
		t.Fatalf("the initially focused control is %q, want deny-btn", got)
	}
}

// A successful pairing says so rather than the window vanishing at the
// moment the person was reading a code out of it — and the code is gone
// from the screen once it has been spent.
func TestASuccessfulPairingShowsTheConnectedScreen(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		t.Run(locale, func(t *testing.T) {
			c := i18n.Load(locale)
			win, _ := sharedPairingWindow(t, c, testPrompt())

			// At the size the connected screen is actually shown at:
			// a layout measured in a differently-sized window says
			// nothing about the window a person opens (D-124).
			//
			// And measured at that size once the page has actually
			// reached it. Resize returns before the page has been
			// relaid out, so a measurement taken straight afterwards
			// can be answered out of the 420x330 layout the window had
			// a moment ago — which is what "the page itself scrolls"
			// meant on the runner (D-201).
			resizeAndSettle(t, win, pairingWindowWidth, pairingConnectedHeight)
			defer resizeAndSettle(t, win, pairingWindowWidth, pairingWindowHeight)
			if err := win.PostJSON(map[string]any{"type": "connected"}); err != nil {
				t.Fatalf("PostJSON(connected): %v", err)
			}
			if display := evalString(t, win,
				"getComputedStyle(document.getElementById('state-code')).display"); display != "none" {
				t.Fatalf("the code screen still renders (display: %s) after the pairing succeeded", display)
			}
			if display := evalString(t, win,
				"getComputedStyle(document.getElementById('state-connected')).display"); display == "none" {
				t.Fatal("the connected screen did not appear")
			}
			// The sentence that matters on that screen: pairing did not
			// buy the application a signature, only the right to ask.
			body := evalString(t, win, "document.body.textContent")
			if !strings.Contains(body, c.T("pairing.connected_explain")) {
				t.Fatalf("the connected screen does not say who still approves each signature:\n%s", body)
			}
			if strings.Contains(body, "042317") {
				t.Fatal("the spent code is still on screen")
			}
			assertPageDoesNotScroll(t, win, "pairing connected ("+locale+")", ".identity")
			assertButtonsVisible(t, win, "pairing connected ("+locale+")", ".identity")
		})
	}
}

// buildPairingInit is what the window actually renders, so this asserts
// on it rather than on a hand-built payload: every key the page asks
// for by data-i18n must be in it, or the label renders as its own key.
func TestThePairingPayloadCarriesEveryStringThePageAsksFor(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := i18n.Load(locale)
		payload := buildPairingInit(c, testPrompt())
		strs, ok := payload["strings"].(map[string]string)
		if !ok {
			t.Fatalf("%s: the payload carries no strings map", locale)
		}
		for key, value := range strs {
			if value == key {
				t.Fatalf("%s: %q has no message in the catalogue and would render as its own key", locale, key)
			}
		}
	}
}
