//go:build windows

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
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
			assertPageDoesNotScroll(t, win, "pairing ("+locale+")", ".origin-row")
			assertButtonsVisible(t, win, "pairing ("+locale+")", ".origin-row")
		})
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
			if err := win.Resize(pairingWindowWidth, pairingConnectedHeight); err != nil {
				t.Fatalf("Resize: %v", err)
			}
			defer func() { _ = win.Resize(pairingWindowWidth, pairingWindowHeight) }()
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
			assertPageDoesNotScroll(t, win, "pairing connected ("+locale+")", ".origin-row")
			assertButtonsVisible(t, win, "pairing connected ("+locale+")", ".origin-row")
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
