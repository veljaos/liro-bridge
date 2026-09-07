package api

import (
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// The property the whole mechanism rests on: the code travels through a
// person. If it came back in the response, an application could pair
// itself with nobody watching, and the window would prove nothing.
func TestTheCodeIsShownInTheWindowAndIsNotInTheResponse(t *testing.T) {
	h := newHarness(t)

	id, ttl, e := h.flow.Request("My ERP", "https://erp.example.com")
	if e != nil {
		t.Fatalf("Request: %v", e)
	}
	if ttl != PairingCodeTTL {
		t.Fatalf("ttl is %v, want %v", ttl, PairingCodeTTL)
	}
	win := h.ui.last()
	if win == nil {
		t.Fatal("no pairing window was shown")
	}
	code := win.prompt.Code
	if len(code) != 6 {
		t.Fatalf("the code is %q, want six digits", code)
	}
	if strings.Trim(code, "0123456789") != "" {
		t.Fatalf("the code is %q, want digits only", code)
	}
	// The request identifier is the only thing the caller receives, and
	// it must not be the code, nor contain it.
	if id == code || strings.Contains(id, code) {
		t.Fatalf("the request identifier %q gives the code %q away", id, code)
	}
}

// Six digits means leading zeros exist and must survive. "042317" is a
// code; "42317" is a code nobody can type.
func TestPairingCodesKeepTheirLeadingZeros(t *testing.T) {
	for range 200 {
		code, err := newPairingCode()
		if err != nil {
			t.Fatalf("newPairingCode: %v", err)
		}
		if len(code) != 6 {
			t.Fatalf("code %q is %d characters, want 6", code, len(code))
		}
	}
}

func TestPairingWindowShowsTheNameAndOriginVerbatim(t *testing.T) {
	h := newHarness(t)

	// http:// rather than https:// is exactly the difference a person
	// must be able to notice; nothing prettifies it, and the scheme is
	// not stripped.
	const origin = "http://erp.example.com:8080/callback"
	if _, _, e := h.flow.Request("My ERP", origin); e != nil {
		t.Fatalf("Request: %v", e)
	}
	win := h.ui.last()
	if win.prompt.Origin != origin {
		t.Fatalf("the window was shown %q, want %q verbatim", win.prompt.Origin, origin)
	}
	if win.prompt.Name != "My ERP" {
		t.Fatalf("the window was shown the name %q", win.prompt.Name)
	}
}

// A name is untrusted display text and is sanitised the way file names
// are (SPEC §6.6). What is bound is the sanitised form, so the name at
// pairing and the name above every later signature are the same bytes.
func TestAnApplicationNameIsSanitisedAndTheSanitisedFormIsBound(t *testing.T) {
	h := newHarness(t)

	hostile := "Liro\u202e\x07 Bridge"
	if _, _, e := h.flow.Request(hostile, "https://erp.example.com"); e != nil {
		t.Fatalf("Request: %v", e)
	}
	win := h.ui.last()
	if strings.ContainsRune(win.prompt.Name, 0x202E) || strings.ContainsRune(win.prompt.Name, 7) {
		t.Fatalf("the window was shown unsanitised text: %q", win.prompt.Name)
	}

	result, e := h.flow.Confirm(win.prompt.RequestID, win.prompt.Code, "https://erp.example.com")
	if e != nil {
		t.Fatalf("Confirm: %v", e)
	}
	if result.Pairing.Name != win.prompt.Name {
		t.Fatalf("the bound name %q differs from the one shown %q",
			result.Pairing.Name, win.prompt.Name)
	}
}

// An origin is refused rather than sanitised: SPEC §6.2 requires it to
// be shown verbatim, and a value altered on its way to the screen is
// not verbatim.
func TestAnOriginThatCannotBeShownVerbatimIsRefused(t *testing.T) {
	h := newHarness(t)

	cases := []struct{ name, origin string }{
		{"empty", ""},
		{"a direction override", "https://erp.example.com\u202e"},
		{"a control character", "https://erp.example.com\x00"},
		{"whitespace", "https://erp example.com"},
		{"too long", "https://" + strings.Repeat("a", MaxOriginLength) + ".example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, e := h.flow.Request("My ERP", tc.origin)
			if e == nil {
				t.Fatal("the origin was accepted")
			}
			if e.Code != errs.CodeRequestInvalid {
				t.Fatalf("code is %s, want %s", e.Code, errs.CodeRequestInvalid)
			}
			if e.Details["field"] != "origin" {
				t.Fatalf("details are %v, want the offending field named", e.Details)
			}
		})
	}
}

func TestAnEmptyApplicationNameIsRefused(t *testing.T) {
	h := newHarness(t)
	for _, name := range []string{"", "   ", "\u202e\u202e"} {
		_, _, e := h.flow.Request(name, "https://erp.example.com")
		if e == nil || e.Code != errs.CodeRequestInvalid || e.Details["field"] != "applicationName" {
			t.Fatalf("name %q was answered %v, want REQUEST_INVALID naming applicationName", name, e)
		}
	}
}

func TestFiveWrongCodesVoidTheRequest(t *testing.T) {
	h := newHarness(t)

	id, _, e := h.flow.Request("My ERP", "https://erp.example.com")
	if e != nil {
		t.Fatalf("Request: %v", e)
	}
	win := h.ui.last()
	wrong := wrongCode(win.prompt.Code)

	for attempt := 1; attempt < MaxPairingAttempts; attempt++ {
		_, e := h.flow.Confirm(id, wrong, "https://erp.example.com")
		if e == nil || e.Code != errs.CodePairingCodeIncorrect {
			t.Fatalf("attempt %d was answered %v, want %s", attempt, e, errs.CodePairingCodeIncorrect)
		}
		want := MaxPairingAttempts - attempt
		if got, _ := e.Details["attemptsRemaining"].(int); got != want {
			t.Fatalf("attempt %d reported %v attempts remaining, want %d", attempt, e.Details["attemptsRemaining"], want)
		}
	}

	// The fifth voids it.
	_, e = h.flow.Confirm(id, wrong, "https://erp.example.com")
	if e == nil || e.Code != errs.CodePairingExpired {
		t.Fatalf("the fifth wrong code was answered %v, want %s", e, errs.CodePairingExpired)
	}
	if closed, _ := win.state(); !closed {
		t.Fatal("the pairing window was left open after the request was voided")
	}

	// And the right code no longer works: a voided request is voided.
	_, e = h.flow.Confirm(id, win.prompt.Code, "https://erp.example.com")
	if e == nil {
		t.Fatal("the right code was accepted after the request had been voided")
	}
	if h.secrets.count() != 0 {
		t.Fatal("a device secret was issued for a voided pairing request")
	}
}

func TestAnExpiredRequestCannotBeConfirmed(t *testing.T) {
	h := newHarness(t)

	id, _, e := h.flow.Request("My ERP", "https://erp.example.com")
	if e != nil {
		t.Fatalf("Request: %v", e)
	}
	win := h.ui.last()

	h.clock.Advance(PairingCodeTTL)
	_, e = h.flow.Confirm(id, win.prompt.Code, "https://erp.example.com")
	if e == nil || e.Code != errs.CodePairingExpired {
		t.Fatalf("a confirm exactly at expiry was answered %v, want %s", e, errs.CodePairingExpired)
	}
	if h.secrets.count() != 0 {
		t.Fatal("a device secret was issued for an expired pairing request")
	}
}

// The window closes when the code expires, so a person does not go on
// looking at a code that no longer works.
func TestTheWindowClosesWhenTheCodeExpires(t *testing.T) {
	h := newHarness(t)

	if _, _, e := h.flow.Request("My ERP", "https://erp.example.com"); e != nil {
		t.Fatalf("Request: %v", e)
	}
	win := h.ui.last()

	h.expire <- time.Time{} // the watcher's five minutes are up
	waitFor(t, func() bool { closed, _ := win.state(); return closed })
}

func TestConfirmFromADifferentOriginIsRefused(t *testing.T) {
	h := newHarness(t)

	id, _, e := h.flow.Request("My ERP", "https://erp.example.com")
	if e != nil {
		t.Fatalf("Request: %v", e)
	}
	win := h.ui.last()

	_, e = h.flow.Confirm(id, win.prompt.Code, "https://evil.example.com")
	if e == nil || e.Code != errs.CodePairingOriginMismatch {
		t.Fatalf("confirm from another origin was answered %v, want %s", e, errs.CodePairingOriginMismatch)
	}
	if h.secrets.count() != 0 {
		t.Fatal("a device secret was issued to a different origin")
	}

	// The request survives: an integrator that declared two different
	// origins can fix the second call without starting over.
	if _, e := h.flow.Confirm(id, win.prompt.Code, "https://erp.example.com"); e != nil {
		t.Fatalf("the corrected confirm was refused: %v", e)
	}
}

func TestTheSecretIsIssuedExactlyOnce(t *testing.T) {
	h := newHarness(t)

	id, _, e := h.flow.Request("My ERP", "https://erp.example.com")
	if e != nil {
		t.Fatalf("Request: %v", e)
	}
	win := h.ui.last()

	first, e := h.flow.Confirm(id, win.prompt.Code, "https://erp.example.com")
	if e != nil {
		t.Fatalf("Confirm: %v", e)
	}
	if len(first.DeviceSecret) != DeviceSecretLength {
		t.Fatalf("the device secret is %d bytes, want %d", len(first.DeviceSecret), DeviceSecretLength)
	}

	_, e = h.flow.Confirm(id, win.prompt.Code, "https://erp.example.com")
	if e == nil || e.Code != errs.CodePairingExpired {
		t.Fatalf("a second confirm was answered %v, want %s", e, errs.CodePairingExpired)
	}
	if got := len(h.pairings.List()); got != 1 {
		t.Fatalf("%d pairings exist after confirming twice, want 1", got)
	}
}

// The window is told the pairing succeeded rather than simply vanishing
// at the moment the person was reading a code out of it.
func TestASuccessfulConfirmTellsTheWindowSo(t *testing.T) {
	h := newHarness(t)
	h.pair("My ERP", "https://erp.example.com")

	closed, confirmed := h.ui.last().state()
	if !confirmed {
		t.Fatal("the window was not told the pairing succeeded")
	}
	if closed {
		t.Fatal("the window was closed rather than shown the result")
	}
}

func TestOnlyOnePairingRequestIsOpenAtATime(t *testing.T) {
	h := newHarness(t)

	if _, _, e := h.flow.Request("First", "https://first.example.com"); e != nil {
		t.Fatalf("Request: %v", e)
	}
	_, _, e := h.flow.Request("Second", "https://second.example.com")
	if e == nil || e.Code != errs.CodePairingInProgress {
		t.Fatalf("a second pairing request was answered %v, want %s", e, errs.CodePairingInProgress)
	}
	if got := h.ui.count(); got != 1 {
		t.Fatalf("%d pairing windows were opened, want 1", got)
	}
}

func TestAnOriginGetsThreePairingRequestsAMinute(t *testing.T) {
	h := newHarness(t)
	const origin = "https://erp.example.com"

	// Each request opens a window, so each one has to be got out of the
	// way before the next; denial is what a person does to a window
	// they did not ask for.
	for i := range PairingRequestsPerMinute {
		if _, _, e := h.flow.Request("My ERP", origin); e != nil {
			t.Fatalf("request %d was refused: %v", i+1, e)
		}
		win := h.ui.last()
		win.deny()
		waitFor(t, func() bool { closed, _ := win.state(); return closed })
	}

	_, _, e := h.flow.Request("My ERP", origin)
	if e == nil || e.Code != errs.CodeRateLimited {
		t.Fatalf("the fourth request in a minute was answered %v, want %s", e, errs.CodeRateLimited)
	}
	if secs, _ := e.Details["retryAfterSeconds"].(int); secs <= 0 || secs > 60 {
		t.Fatalf("retryAfterSeconds is %v, want a number of seconds inside the minute", e.Details["retryAfterSeconds"])
	}

	// A different origin is unaffected: the limit is per origin.
	if _, _, e := h.flow.Request("Other", "https://other.example.com"); e != nil {
		t.Fatalf("another origin was rate limited too: %v", e)
	}
	other := h.ui.last()
	other.deny()
	// The watcher clears the flow's single slot before it closes the
	// window, so "closed" is the point at which the next request can
	// get one — waiting on it is what keeps this test from racing the
	// allowance away on retries.
	waitFor(t, func() bool { closed, _ := other.state(); return closed })

	// And the allowance comes back.
	h.clock.Advance(pairingRateWindow)
	if _, _, e := h.flow.Request("My ERP", origin); e != nil {
		t.Fatalf("the allowance did not come back after a minute: %v", e)
	}
}

// A refused request is charged against the limit too. A limiter that
// only counts the requests it allowed does not limit a loop at all.
func TestRefusedPairingRequestsCountAgainstTheLimit(t *testing.T) {
	h := newHarness(t)
	const origin = "https://erp.example.com"

	// The first opens a window and stays open; the next two are refused
	// with PAIRING_IN_PROGRESS but still spend the allowance.
	if _, _, e := h.flow.Request("My ERP", origin); e != nil {
		t.Fatalf("Request: %v", e)
	}
	for i := range PairingRequestsPerMinute - 1 {
		if _, _, e := h.flow.Request("My ERP", origin); e == nil || e.Code != errs.CodePairingInProgress {
			t.Fatalf("refused request %d was answered %v", i+2, e)
		}
	}
	_, _, e := h.flow.Request("My ERP", origin)
	if e == nil || e.Code != errs.CodeRateLimited {
		t.Fatalf("the fourth request was answered %v, want %s", e, errs.CodeRateLimited)
	}
}

func TestARefusedPairingIsAnAnswerRatherThanAnExpiry(t *testing.T) {
	h := newHarness(t)

	id, _, e := h.flow.Request("My ERP", "https://erp.example.com")
	if e != nil {
		t.Fatalf("Request: %v", e)
	}
	win := h.ui.last()
	win.deny()
	waitFor(t, func() bool { closed, _ := win.state(); return closed })

	_, e = h.flow.Confirm(id, win.prompt.Code, "https://erp.example.com")
	if e == nil || e.Code != errs.CodePairingDenied {
		t.Fatalf("confirm after a refusal was answered %v, want %s", e, errs.CodePairingDenied)
	}
	if h.secrets.count() != 0 {
		t.Fatal("a device secret was issued for a refused pairing")
	}

	// Refusing one request does not block the next: a person who
	// pressed Deny by accident can be asked again.
	if _, _, e := h.flow.Request("My ERP", "https://erp.example.com"); e != nil {
		t.Fatalf("a new request after a refusal was answered %v", e)
	}
}

func TestAnUnknownRequestIdentifierLooksExactlyLikeAnExpiredOne(t *testing.T) {
	h := newHarness(t)

	_, e := h.flow.Confirm("0123456789abcdef0123456789abcdef", "123456", "https://erp.example.com")
	if e == nil || e.Code != errs.CodePairingExpired {
		t.Fatalf("an unknown identifier was answered %v, want %s", e, errs.CodePairingExpired)
	}
	_, e = h.flow.Confirm("", "123456", "https://erp.example.com")
	if e == nil || e.Code != errs.CodePairingExpired {
		t.Fatalf("an empty identifier was answered %v, want %s", e, errs.CodePairingExpired)
	}
}

// wrongCode returns a six-digit code that is not the given one.
func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

// waitFor spins until cond holds, or fails the test. Used only where a
// goroutine this package owns has to get somewhere; nothing here waits
// on a wall clock.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for a condition this package's own goroutine should have reached")
}
