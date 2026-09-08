package i18n

import (
	"sort"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// TestCataloguesHaveIdenticalKeySets is the completeness test from F0 §5.4:
// a key present in one locale but missing from another must fail the build.
func TestCataloguesHaveIdenticalKeySets(t *testing.T) {
	locales := []string{"sr-Latn", "sr-Cyrl", "en"}
	var reference []string
	for i, locale := range locales {
		c := Load(locale)
		keys := make([]string, 0, len(c.data))
		for k := range c.data {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		if i == 0 {
			reference = keys
			continue
		}
		if len(keys) != len(reference) {
			t.Fatalf("%s has %d keys, %s has %d keys", locale, len(keys), locales[0], len(reference))
		}
		for j, k := range keys {
			if k != reference[j] {
				t.Fatalf("%s key set differs from %s: %q vs %q", locale, locales[0], k, reference[j])
			}
		}
	}
}

func TestLoadFallsBackOnUnknownLocale(t *testing.T) {
	c := Load("fr")
	if c.locale != defaultLocale {
		t.Fatalf("locale = %q, want %q", c.locale, defaultLocale)
	}
}

func TestLoadFallsBackOnEmptyLocale(t *testing.T) {
	c := Load("")
	if c.locale != defaultLocale {
		t.Fatalf("locale = %q, want %q", c.locale, defaultLocale)
	}
}

// TestBareSrDoesNotResolveToCyrillic guards SPEC §9.1: in CLDR, bare "sr"
// resolves to Cyrillic. A bare "sr" here must fall back to sr-Latn instead.
func TestBareSrDoesNotResolveToCyrillic(t *testing.T) {
	c := Load("sr")
	if c.locale != "sr-Latn" {
		t.Fatalf("bare \"sr\" resolved to %q, want sr-Latn (never sr-Cyrl)", c.locale)
	}
}

func TestLoadExactMatches(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := Load(locale)
		if c.locale != locale {
			t.Fatalf("Load(%q).locale = %q", locale, c.locale)
		}
	}
}

func TestTReturnsMessage(t *testing.T) {
	c := Load("en")
	if got := c.T("app.name"); got != "Liro Bridge" {
		t.Fatalf("T(app.name) = %q", got)
	}
}

func TestTFallsBackToEnglishThenToKey(t *testing.T) {
	c := Load("sr-Latn")
	// Present nowhere: must fall back all the way to the key itself.
	if got := c.T("does.not.exist"); got != "does.not.exist" {
		t.Fatalf("T(missing) = %q, want the key echoed back", got)
	}
}

func TestCodeKey(t *testing.T) {
	cases := map[errs.Code]string{
		errs.CodeCardNotPresent: "error.card_not_present",
		errs.CodeNoReader:       "error.no_reader",
		errs.CodeInternal:       "error.internal",
	}
	for code, want := range cases {
		if got := CodeKey(code); got != want {
			t.Fatalf("CodeKey(%s) = %q, want %q", code, got, want)
		}
	}
}

// TestEveryErrorCodeHasAMessageInEveryCatalogue is Task 4's general
// check of the error-code mapping, mechanised: a code with no message
// renders its own key ("error.pin_locked") to the user, and nothing
// before this test would have said so. It is the reason errs.AllCodes
// exists.
func TestEveryErrorCodeHasAMessageInEveryCatalogue(t *testing.T) {
	for _, locale := range []string{"sr-Latn", "sr-Cyrl", "en"} {
		c := Load(locale)
		for _, code := range errs.AllCodes() {
			key := CodeKey(code)
			if _, ok := c.data[key]; !ok {
				t.Errorf("locale %s has no message for %s (key %q)", locale, code, key)
			}
		}
	}
}

// TestStampLabelIsTheElectronicWordingInEveryCatalogue is D-209's
// wording, and the check that the old wording is gone rather than
// merely unused.
//
// "Digitalno potpisano" said the signature was digital, which is a
// statement about how it was made; "Elektronski potpisano" says it was
// electronically signed, which is the term the law and the people
// reading the document use. English is unchanged: "Digitally signed"
// is the established English form and reads correctly as it is.
//
// The old strings are searched for across the whole of each catalogue,
// not only under sign.stamp_label, because a phrase left behind under
// some other key would still reach a user's screen.
func TestStampLabelIsTheElectronicWordingInEveryCatalogue(t *testing.T) {
	want := map[string]string{
		"sr-Latn": "Elektronski potpisano",
		"sr-Cyrl": "Електронски потписано",
		"en":      "Digitally signed",
	}
	gone := []string{"Digitalno potpisano", "Дигитално потписано", "Digitalno potpisao", "Дигитално потписао"}

	for locale, wantLabel := range want {
		c := Load(locale)
		if got := c.T("sign.stamp_label"); got != wantLabel {
			t.Errorf("%s: sign.stamp_label = %q, want %q", locale, got, wantLabel)
		}
		for key, value := range c.data {
			for _, old := range gone {
				if strings.Contains(value, old) {
					t.Errorf("%s: %s still carries the old stamp wording %q: %q", locale, key, old, value)
				}
			}
		}
	}
}
