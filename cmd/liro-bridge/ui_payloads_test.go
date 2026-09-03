package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/consent"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
)

// TestConsentInitRendersCyrillic is F5 §9/§10's named requirement:
// "Cyrillic must be tested, not assumed: a test that renders the
// consent window in sr-Cyrl and asserts the expected strings reach the
// page." Window content itself cannot be unit-tested (F5 §10), so this
// tests the actual boundary that can be: the JSON payload
// Window.PostJSON sends to the page, which is where every localised
// string this project controls is resolved (D-085's "view model has no
// window dependency" — buildConsentInit is exactly that dependency-free
// layer, reused here with the real sr-Cyrl catalogue instead of a
// window).
func TestConsentInitRendersCyrillic(t *testing.T) {
	c := i18n.Load("sr-Cyrl")
	vm := consent.BuildViewModel(consent.ApplicationLocal, [][]byte{{1, 2, 3}}, []string{"уговор.pdf"},
		[]classify.Info{
			{Thumbprint: "AABBCCDD", Purpose: classify.PurposeSigning, Usable: true, Subject: classify.Subject{DisplayName: "ВЕЉКО СТАНОЈЕВИЋ"}},
		})

	payload := buildConsentInit(c, vm)

	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshalling payload: %v", err)
	}
	rendered := string(b)

	// Every static label must have been resolved to Cyrillic, not left
	// as its English/Latin default or as the raw key.
	wantCyrillic := []string{
		"Откажи",         // consent.cancel
		"Одобри",         // consent.approve
		"Апликација",     // consent.application_label
		"Детаљи",         // consent.details_toggle
		"за потписивање", // role text for PurposeSigning
	}
	for _, want := range wantCyrillic {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered sr-Cyrl payload missing %q\npayload: %s", want, rendered)
		}
	}

	// The signer's display name must survive untouched — SPEC §9.3: a
	// Cyrillic name is never itself "localised" or transliterated.
	if !strings.Contains(rendered, "ВЕЉКО СТАНОЈЕВИЋ") {
		t.Error("rendered payload lost the signer's Cyrillic display name")
	}

	// Untrusted file name content still round-trips as Cyrillic too,
	// proving sanitisation does not mangle non-Latin scripts.
	if !strings.Contains(rendered, "уговор.pdf") {
		t.Error("rendered payload lost the (sanitised) Cyrillic file name")
	}
}

// TestConsentInitRendersLatinAndEnglishToo covers the other two
// locales alongside sr-Cyrl, the same way the i18n package's own
// completeness test treats all three symmetrically.
func TestConsentInitRendersLatinAndEnglishToo(t *testing.T) {
	tests := []struct {
		locale string
		want   string
	}{
		{"sr-Latn", "Otkaži"},
		{"en", "Cancel"},
	}
	for _, tc := range tests {
		c := i18n.Load(tc.locale)
		vm := consent.BuildViewModel(consent.ApplicationLocal, nil, nil, nil)
		b, err := json.Marshal(buildConsentInit(c, vm))
		if err != nil {
			t.Fatalf("%s: marshalling payload: %v", tc.locale, err)
		}
		if !strings.Contains(string(b), tc.want) {
			t.Errorf("%s: rendered payload missing %q", tc.locale, tc.want)
		}
	}
}
