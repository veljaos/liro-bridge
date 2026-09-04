//go:build windows

package main

// Task 2 (F5 second-real-run review): the batch fingerprint is 64
// unbreakable hex characters, and rendering it whole pushed the Details
// card — and with it the window's layout — wider than the window. The
// Go-side view model was right the whole time, which is exactly why this
// test measures the rendered DOM: a layout defect is invisible to every
// test that only looks at the data.
import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// fullLengthFingerprint is a real 64-character hex SHA-256 — the exact
// value from the F5 second-real-run report.
const fullLengthFingerprint = "d54aeba8571c16922cb7cd1f6b758824bc7b26e6865daa4bba47be47c906135e"

func TestConsentDetailsFingerprintDoesNotWidenItsContainer(t *testing.T) {
	c := i18n.Load("sr-Latn")

	cert := classify.Info{
		Thumbprint:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Subject:       classify.Subject{DisplayName: "Test Testić"},
		IssuerCN:      "Test CA",
		Qualification: classify.QualificationQualified,
		Purpose:       classify.PurposeSigning,
		Usable:        true,
	}

	win, _ := sharedConsentWindow(t)

	// The card's width with no fingerprint at all is the baseline the
	// full-length one must not change.
	postConsentFingerprint(t, win, c, cert, "")
	baseline := evalNumber(t, win, "document.querySelector('#details .liro-card').getBoundingClientRect().width")
	if baseline <= 0 {
		t.Fatalf("baseline Details card width is %v — the Details section did not render", baseline)
	}

	postConsentFingerprint(t, win, c, cert, fullLengthFingerprint)
	withFingerprint := evalNumber(t, win, "document.querySelector('#details .liro-card').getBoundingClientRect().width")
	if withFingerprint != baseline {
		t.Errorf("Details card width changed from %v to %v when a full-length fingerprint was rendered", baseline, withFingerprint)
	}

	// And the page as a whole still fits: nothing scrolls sideways.
	bodyScroll := evalNumber(t, win, "document.body.scrollWidth")
	bodyClient := evalNumber(t, win, "document.body.clientWidth")
	if bodyScroll > bodyClient {
		t.Errorf("body scrollWidth %v exceeds clientWidth %v — something overflows horizontally", bodyScroll, bodyClient)
	}
	cardScroll := evalNumber(t, win, "document.querySelector('#details .liro-card').scrollWidth")
	cardClient := evalNumber(t, win, "document.querySelector('#details .liro-card').clientWidth")
	if cardScroll > cardClient {
		t.Errorf("Details card scrollWidth %v exceeds clientWidth %v — its content overflows", cardScroll, cardClient)
	}

	// What is on screen is the elided form; the full value is only ever
	// on the clipboard.
	shown := evalString(t, win, "document.getElementById('fingerprint').textContent")
	want := consent.ShortFingerprint(fullLengthFingerprint)
	if shown != want {
		t.Errorf("fingerprint rendered %q, want %q", shown, want)
	}
	if strings.Contains(evalString(t, win, "document.body.textContent"), fullLengthFingerprint) {
		t.Error("the full 64-character fingerprint is rendered somewhere in the page; only the elided form should be")
	}
}

// TestConsentLongFileNameDoesNotWidenTheWindow covers the other
// unbounded value the Details section renders — a file name, which
// arrives from a potentially hostile caller (SPEC §6.6).
func TestConsentLongFileNameDoesNotWidenTheWindow(t *testing.T) {
	c := i18n.Load("sr-Latn")
	cert := classify.Info{
		Thumbprint:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		Subject:       classify.Subject{DisplayName: "Test Testić"},
		IssuerCN:      "Test CA",
		Qualification: classify.QualificationQualified,
		Purpose:       classify.PurposeSigning,
		Usable:        true,
	}

	win, _ := sharedConsentWindow(t)

	longName := strings.Repeat("a", 300) + ".pdf"
	vm := consent.BuildViewModel(consent.ApplicationLocal, [][]byte{{1}}, []string{longName}, []classify.Info{cert})
	if err := win.PostJSON(buildConsentInit(c, vm)); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	openConsentDetails(t, win)

	bodyScroll := evalNumber(t, win, "document.body.scrollWidth")
	bodyClient := evalNumber(t, win, "document.body.clientWidth")
	if bodyScroll > bodyClient {
		t.Errorf("body scrollWidth %v exceeds clientWidth %v with a 300-character file name", bodyScroll, bodyClient)
	}
}

// postConsentFingerprint renders the consent screen with exactly the
// given fingerprint, overriding whatever BuildViewModel computed — the
// point of the measurement is the rendered string's length, not which
// digests produced it.
func postConsentFingerprint(t *testing.T, win ui.Window, c *i18n.Catalogue, cert classify.Info, fingerprint string) {
	t.Helper()
	vm := consent.BuildViewModel(consent.ApplicationLocal, [][]byte{{1, 2, 3}}, []string{"document.pdf"}, []classify.Info{cert})
	vm.Fingerprint = fingerprint
	vm.FingerprintShort = consent.ShortFingerprint(fingerprint)
	if err := win.PostJSON(buildConsentInit(c, vm)); err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	openConsentDetails(t, win)
}

// openConsentDetails opens the collapsed <details> section, since a
// closed one has no laid-out content to measure.
func openConsentDetails(t *testing.T, win ui.Window) {
	t.Helper()
	if _, err := win.Eval("document.getElementById('details').open = true"); err != nil {
		t.Fatalf("Eval(open details): %v", err)
	}
}

// evalNumber runs script via Window.Eval and decodes its JSON-encoded
// result as a float64.
func evalNumber(t *testing.T, win ui.Window, script string) float64 {
	t.Helper()
	raw, err := win.Eval(script)
	if err != nil {
		t.Fatalf("Eval(%s): %v", script, err)
	}
	var v *float64
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("Eval(%s): decoding result %q: %v", script, raw, err)
	}
	if v == nil {
		t.Fatalf("Eval(%s): returned null", script)
	}
	return *v
}
