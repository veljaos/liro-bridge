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

			// The mark and the word are outside the scrolling region
			// for this case and no other: a screen whose whole message
			// is "connected" must not push that message off its own top
			// to make room for the caller's padding (D-208).
			assertNameStartsInsideTheWindow(t, win, "pairing connected, long name ("+locale+")", "connected-check")
			assertNameStartsInsideTheWindow(t, win, "pairing connected, long name ("+locale+")", "connected-title")
		})
	}
}

// The strings the redesign took off these two screens are gone from the
// catalogues, not merely unreferenced by the pages (D-208). An unused
// message is one edit away from being wired back in; a missing one is
// not. This is the check TestThePairingWindowShowsTheCodeAndNoAllowButton
// already makes for pairing.allow, for the two that followed it.
func TestTheStringsTheRedesignRemovedAreGoneFromEveryCatalogue(t *testing.T) {
	for _, locale := range everyLocale {
		c := i18n.Load(locale)
		for _, key := range []string{"pairing.wants_to_connect", "pairing.connected_explain"} {
			if got := c.T(key); got != key {
				t.Errorf("%s is still in the %s catalogue as %q", key, locale, got)
			}
		}
	}
}

// One typeface on the whole screen, and the difference between a label
// and its value made with size, weight and colour rather than with a
// second font (D-208). The origin used to be monospace inside a card,
// which made two fields that say the same kind of thing look like two
// different kinds of thing.
func TestThePairingWindowIsOneTypeface(t *testing.T) {
	win, _ := sharedPairingWindow(t, i18n.Load("sr-Latn"), testPrompt())

	// Every piece of text on both screens, hidden or not: a computed
	// font-family is resolved even for a display:none subtree.
	families := evalString(t, win, "(function(){var seen={};"+
		"document.querySelectorAll('#state-code p, #state-connected p, button').forEach(function(el){"+
		"seen[getComputedStyle(el).fontFamily]=1;});"+
		"return Object.keys(seen).join(' | ');})()")
	if strings.Contains(families, "|") {
		t.Errorf("the pairing window renders in more than one font family: %s", families)
	}
	if body := evalString(t, win, "getComputedStyle(document.body).fontFamily"); body != families {
		t.Errorf("the page's text is in %q, the body's family is %q", families, body)
	}

	// And the label/value distinction is the three things it is
	// allowed to be.
	got := evalNumbers(t, win,
		"parseFloat(getComputedStyle(document.querySelector('#state-code .field-label')).fontSize)",
		"parseFloat(getComputedStyle(document.querySelector('#state-code .field-label')).fontWeight)",
		"parseFloat(getComputedStyle(document.getElementById('app-name')).fontSize)",
		"parseFloat(getComputedStyle(document.getElementById('app-name')).fontWeight)")
	if got[2] <= got[0] {
		t.Errorf("a value renders at %v points and its label at %v; the value is meant to be the larger", got[2], got[0])
	}
	if got[3] <= got[1] {
		t.Errorf("a value renders at weight %v and its label at %v; the value is meant to be the stronger", got[3], got[1])
	}
	colours := evalString(t, win,
		"getComputedStyle(document.querySelector('#state-code .field-label')).color+' | '+"+
			"getComputedStyle(document.getElementById('app-name')).color")
	if parts := strings.Split(colours, " | "); parts[0] == parts[1] {
		t.Errorf("a label and its value are the same colour (%s); the label is meant to be the quieter", parts[0])
	}
}

// The code is separated from the identity by more than the two fields
// are separated from each other — that difference is the only thing on
// the screen saying that the six digits are not a third field about who
// is asking, but the thing being read out loud (D-208).
func TestTheGapAboveTheCodeIsLargerThanTheGapBetweenTheFields(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) {
			win, _ := sharedPairingWindow(t, i18n.Load(locale), testPrompt())
			gaps := evalNumbers(t, win,
				"document.querySelectorAll('#state-code .field')[1].getBoundingClientRect().top - "+
					"document.querySelectorAll('#state-code .field')[0].getBoundingClientRect().bottom",
				"document.querySelector('#state-code .code-block').getBoundingClientRect().top - "+
					"document.querySelector('#state-code .identity').getBoundingClientRect().bottom",
				"0", "0")
			if gaps[1] <= gaps[0] {
				t.Errorf("the code sits %v points below the identity and the fields are %v points apart; "+
					"the code is meant to be the more separated", gaps[1], gaps[0])
			}
		})
	}
}

// The code, its label and its two-line note sit on the window's own
// centre line, apart from the left-aligned table above them (D-208).
// Centring the digits needs one correction that is invisible until it
// is missing: letter-spacing adds its gap after the last character too,
// so a centred string of six spaced digits sits left of centre by that
// much unless the text-indent puts it back.
func TestTheCodeIsCentredOnTheWindow(t *testing.T) {
	for _, locale := range everyLocale {
		t.Run(locale, func(t *testing.T) {
			win, _ := sharedPairingWindow(t, i18n.Load(locale), testPrompt())

			// The label and the note are centred by their boxes, which is
			// what .code-block's align-items does.
			for _, sel := range []string{".code-block .field-label", ".code-note"} {
				box := evalNumbers(t, win,
					"document.querySelector("+jsStringLiteral(sel)+").getBoundingClientRect().left",
					"document.querySelector("+jsStringLiteral(sel)+").getBoundingClientRect().right",
					"window.innerWidth", "0")
				centre, page := (box[0]+box[1])/2, box[2]/2
				if diff := centre - page; diff > 1 || diff < -1 {
					t.Errorf("%s is centred on %.1f, the window on %.1f — %.1f points off",
						sel, centre, page, diff)
				}
			}

			// The digits are measured as ink, not as a box. Their box is
			// shrink-wrapped and centred by the flex container whatever
			// the type inside it does, so it sits on the window's centre
			// line even when the digits do not — which is exactly how the
			// trailing letter-space goes unnoticed. The run's own
			// rectangle ends after that trailing gap, so the last
			// letter-space comes off the right edge to leave the glyphs.
			ink := evalNumbers(t, win,
				"(function(){var r=document.createRange();"+
					"r.selectNodeContents(document.getElementById('pairing-code'));"+
					"return r.getBoundingClientRect().left;})()",
				"(function(){var r=document.createRange();"+
					"r.selectNodeContents(document.getElementById('pairing-code'));"+
					"return r.getBoundingClientRect().right;})()",
				"parseFloat(getComputedStyle(document.getElementById('pairing-code')).letterSpacing)",
				"window.innerWidth")
			centre, page := (ink[0]+ink[1]-ink[2])/2, ink[3]/2
			if diff := centre - page; diff > 1 || diff < -1 {
				t.Errorf("the code's digits are centred on %.1f, the window on %.1f — %.1f points off",
					centre, page, diff)
			}

			// And the note really is two lines, not one that happens to
			// wrap: two elements, each one line high, in every locale.
			lines := evalString(t, win, "(function(){var out=[];"+
				"document.querySelectorAll('.code-note p').forEach(function(p){"+
				"out.push(Math.round(p.getBoundingClientRect().height/"+
				"parseFloat(getComputedStyle(p).lineHeight)));});return out.join(',');})()")
			if lines != "1,1" {
				t.Errorf("the note under the code renders as %q lines per paragraph, want two paragraphs of one", lines)
			}
		})
	}
}

// The mark on the connected screen is drawn in the page, in the design
// system's positive intent colour, at the size the token says — never a
// literal, and never an icon library for one glyph (SPEC §10.1, D-208).
func TestTheConnectedScreenMarkIsDrawnInThePositiveIntentColour(t *testing.T) {
	win, _ := sharedPairingWindow(t, i18n.Load("sr-Latn"), testPrompt())

	// The token's own value, resolved on the page and converted to the
	// form getComputedStyle answers in, so the comparison is against
	// tokens.css rather than against a colour written down twice.
	report := evalString(t, win, "(function(){"+
		"var root=getComputedStyle(document.documentElement);"+
		"var hex=root.getPropertyValue('--liro-color-positive').trim();"+
		"var m=/^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex);"+
		"var want=m?'rgb('+parseInt(m[1],16)+', '+parseInt(m[2],16)+', '+parseInt(m[3],16)+')':hex;"+
		"var svg=document.getElementById('connected-check');"+
		"return [svg.tagName.toLowerCase(),"+
		"svg.querySelectorAll('circle').length+'+'+svg.querySelectorAll('path').length,"+
		"getComputedStyle(svg).color,want,"+
		"getComputedStyle(svg.querySelector('circle')).stroke,"+
		"root.getPropertyValue('--liro-icon-size-lg').trim(),"+
		"getComputedStyle(svg).width].join('|');})()")

	parts := strings.Split(report, "|")
	if len(parts) != 7 {
		t.Fatalf("the mark reported %q, which is not the seven values asked for", report)
	}
	tag, shapes, colour, want, stroke, token, width := parts[0], parts[1], parts[2], parts[3], parts[4], parts[5], parts[6]
	if tag != "svg" {
		t.Errorf("the mark is a <%s>; it is meant to be inline SVG drawn in the page", tag)
	}
	if shapes != "1+1" {
		t.Errorf("the mark is %s circles+paths, want a circle and a check", shapes)
	}
	if colour != want {
		t.Errorf("the mark renders in %s; --liro-color-positive is %s", colour, want)
	}
	if stroke != want {
		t.Errorf("the mark's circle is stroked %s, not the colour the SVG was given (%s)", stroke, want)
	}
	if width != token {
		t.Errorf("the mark renders %s wide; --liro-icon-size-lg is %s", width, token)
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
	// One whole CSS pixel, and the numbers are reported with their
	// fractions rather than rounded. Both for the same reason, measured
	// on this window: every box on this page has a fractional height —
	// .identity 140.016, .field 87.969, .field-label 16.797,
	// .field-value 67.172, .code-block 105.984, .pairing-code 47.594 —
	// so an edge measured against an integer viewport sits on a
	// boundary by construction, and half a pixel is below the
	// granularity of the thing being compared.
	//
	// Rounding the numbers in the message is what made the F9b failure
	// F10 §7 carries forward unreadable: "renders at 353..370 of 369"
	// is what a 369.6 bottom edge looks like once %.0f has been applied
	// to it, and it is indistinguishable from a real one-pixel overflow.
	// A tolerance below the measurement's own precision produces
	// failures nobody can act on; printing the fraction is what makes
	// the next one diagnosable in one reading.
	if top < -layoutEpsilon {
		t.Errorf("%s: #%s renders at %.3f..%.3f of a %.3f-point window — its first line is above the top",
			window, id, top, bottom, viewport)
	}
	if top > viewport+layoutEpsilon {
		t.Errorf("%s: #%s renders at %.3f..%.3f of a %.3f-point window — it starts below the bottom",
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
			// What that screen says is now three things and no
			// sentence (D-208): the mark, the word, and who it is
			// that connected.
			body := evalString(t, win, "document.body.textContent")
			if !strings.Contains(body, c.T("pairing.connected_title")) {
				t.Fatalf("the connected screen does not say that it connected:\n%s", body)
			}
			if !strings.Contains(body, testPrompt().Name) {
				t.Fatalf("the connected screen does not say which application connected:\n%s", body)
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
