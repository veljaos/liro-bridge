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

// formatVerbs returns the fmt verbs in a message, in the order they
// appear. %% is a literal per cent and is not one.
//
// The pattern is deliberately wider than what this project writes: it
// accepts flags, widths, precisions, argument indices and any verb
// letter, so a translation that turns %s into %-10s or %[1]d is
// reported rather than passed over as "close enough".
func formatVerbs(s string) []string {
	var out []string
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		j := i + 1
		if j < len(s) && s[j] == '%' { // an escaped per cent, not a verb
			i = j
			continue
		}
		if j < len(s) && s[j] == '[' { // an explicit argument index
			for j < len(s) && s[j] != ']' {
				j++
			}
			j++
		}
		for j < len(s) && strings.ContainsRune("-+# 0123456789.*", rune(s[j])) {
			j++
		}
		if j < len(s) {
			out = append(out, s[i:j+1])
			i = j
		}
	}
	return out
}

// TestCataloguesAgreeOnFormatVerbs is the other half of the key-set
// test above, and the half that was missing.
//
// Identical key sets say nothing about what is inside the values. A
// translation that drops a %s, adds one, or reorders two of them
// compiles, passes every other test in this repository, and then
// renders %!s(MISSING) — or silently prints the wrong argument — in
// exactly one locale. That is a defect a user finds rather than CI: the
// developer's own locale is the one that looks right.
//
// Every string in this catalogue reaches fmt.Sprintf on some path, so
// the check is over all three catalogues rather than over a list of
// keys somebody keeps in step (D-158's method: a list maintained by
// hand is a check that quietly stops checking).
func TestCataloguesAgreeOnFormatVerbs(t *testing.T) {
	const reference = "sr-Latn" // the source of truth; the other two follow it
	ref := Load(reference)

	for _, locale := range []string{"sr-Cyrl", "en"} {
		c := Load(locale)
		for key, want := range ref.data {
			got, ok := c.data[key]
			if !ok {
				continue // the key-set test above owns that failure
			}
			a, b := formatVerbs(want), formatVerbs(got)
			if len(a) != len(b) {
				t.Errorf("%s: %s has %d format verbs %v, %s has %d %v",
					key, reference, len(a), a, locale, len(b), b)
				continue
			}
			for i := range a {
				if a[i] != b[i] {
					t.Errorf("%s: verb %d is %s in %s and %s in %s (%v vs %v)",
						key, i+1, a[i], reference, b[i], locale, a, b)
					break
				}
			}
		}
	}
}

// TestTheFormatVerbCheckWouldActuallyFire is the other half of the
// check above, because a matcher that finds nothing passes for ever.
// Each case is a way a translation really does go wrong.
func TestTheFormatVerbCheckWouldActuallyFire(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []string
	}{
		{"nothing here", nil},
		{"%d dokumenata, %s", []string{"%d", "%s"}},
		{"%s do %s", []string{"%s", "%s"}},
		{"x %.0f y %.0f", []string{"%.0f", "%.0f"}},
		{"100%% sigurno", nil},     // escaped, not a verb
		{"%%d is not a verb", nil}, // nor is this
		{"%[2]s then %[1]s", []string{"%[2]s", "%[1]s"}},
		{"%-10s padded", []string{"%-10s"}},
		{"trailing %", nil},                 // nothing follows it
		{"AppData\\Local\\Liro\\logs", nil}, // a path, no verbs
	} {
		got := formatVerbs(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("formatVerbs(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("formatVerbs(%q) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}

	// And the comparison itself: a dropped verb, an added one and a
	// reordered pair must each be caught.
	for _, tc := range []struct{ a, b string }{
		{"%d of %d", "%d of"},        // dropped
		{"%d documents", "%d of %d"}, // added
		{"%s do %d", "%d do %s"},     // reordered
	} {
		x, y := formatVerbs(tc.a), formatVerbs(tc.b)
		same := len(x) == len(y)
		for i := 0; same && i < len(x); i++ {
			same = x[i] == y[i]
		}
		if same {
			t.Errorf("formatVerbs(%q)=%v and formatVerbs(%q)=%v compare equal; the check would miss it", tc.a, x, tc.b, y)
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
	// main.title rather than the app.name key this used to read: that
	// key was never referenced by anything but this test, and every
	// window that needs the product name uses main.title, so it was
	// deleted rather than left for someone to wire up later.
	if got := c.T("main.title"); got != "Liro Bridge" {
		t.Fatalf("T(main.title) = %q", got)
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
