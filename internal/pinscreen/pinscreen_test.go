package pinscreen

import (
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// locales is every catalogue this program ships (SPEC §9.1), and every test
// here runs over all three.
//
// D-161's lesson is the reason: a test asserting the right property against
// the wrong fixture keeps passing, and one locale is the wrong fixture for a
// screen whose whole job is saying something to a person in their own
// language. D-265 found the same thing from the other side, in a check that
// compared column widths in English only while the Serbian ones were ragged.
var locales = []string{"sr-Latn", "sr-Cyrl", "en"}

// posta is the request as this project's own card actually produces it: the
// values D-273 measured off the Pošta token through SafeSign 3.9.32.1, with
// the module path this machine loads it from.
func posta() pkcs11.PINRequest {
	return pkcs11.PINRequest{
		TokenLabel:       "Savka Odžić 200100123",
		TokenSerial:      "2353120973204924",
		CertificateLabel: "Savka Odžić 200100123",
		ModulePath:       `C:\Windows\System32\aetpkss1.dll`,
		MinLength:        5,
	}
}

// TestTheHeadingNamesThisProgramInEveryLocale is SPEC §6.5.1's sixth clause.
//
// "The PIN screen says whose it is. A person must be able to tell they are
// giving their PIN to Liro Bridge rather than to the card or to Windows."
// D-277 and D-278 both record that this matters *more* on this window than it
// would elsewhere, precisely because the window deliberately looks like a
// system dialog — the familiarity that makes a native dialog the right shape
// is the same thing clause 6 exists to defeat.
func TestTheHeadingNamesThisProgramInEveryLocale(t *testing.T) {
	for _, loc := range locales {
		p := Text(i18n.Load(loc), posta(), 15)
		if !strings.Contains(p.Heading, "Liro Bridge") {
			t.Errorf("%s: the heading is %q and does not name Liro Bridge — "+
				"SPEC §6.5.1 clause 6 is the whole of what that line is for", loc, p.Heading)
		}
	}
}

// TestTheHintCarriesTheTokensOwnLimits is clause 7 as D-276 amended it.
//
// "The limits are the token's, read when they are needed: measured, a MUP
// token declares 4 and 8 where a Pošta token declares 5 and 15, and both live
// in one person's drawer. A screen built around either pair is wrong for the
// other card."
//
// So both real pairs are driven through, and each must produce its own
// numbers. A screen that hard-coded either would pass for one card and fail
// here for the other, which is exactly the failure the clause names.
func TestTheHintCarriesTheTokensOwnLimits(t *testing.T) {
	cards := []struct {
		name     string
		min, max int
	}{
		{"Pošta, as D-273 measured it", 5, 15},
		{"MUP, as D-268 measured it", 4, 8},
	}
	for _, card := range cards {
		for _, loc := range locales {
			req := posta()
			req.MinLength = card.min
			p := Text(i18n.Load(loc), req, card.max)

			wantMin := itoa(card.min)
			wantMax := itoa(card.max)
			if !strings.Contains(p.Hint, wantMin) || !strings.Contains(p.Hint, wantMax) {
				t.Errorf("%s / %s: the hint is %q and does not carry this token's own %s and %s",
					card.name, loc, p.Hint, wantMin, wantMax)
			}
			// The order matters and a swapped pair reads as a rule nobody can
			// satisfy — "accepts 15 to 5 characters". Checked rather than
			// assumed, because the catalogue's own %d verbs are positional and
			// a translator reordering them is the defect D-265's format-verb
			// check exists for one layer down.
			if i, j := strings.Index(p.Hint, wantMin), strings.Index(p.Hint, wantMax); i > j {
				t.Errorf("%s / %s: the hint states the maximum before the minimum: %q",
					card.name, loc, p.Hint)
			}
		}
	}
}

// TestTheSubjectNamesTheCardAndNothingElse.
//
// The card's own label is what a person with two cards in two readers needs.
// The serial is not something anybody reads, and the module path is diagnostic
// — the only thing that tells two sightings of one card apart (D-271, D-272) —
// which is a reason to keep it in a log and not a reason to put a DLL path in
// front of somebody about to type a secret.
func TestTheSubjectNamesTheCardAndNothingElse(t *testing.T) {
	req := posta()
	for _, loc := range locales {
		p := Text(i18n.Load(loc), req, 15)
		if !strings.Contains(p.Subject, req.TokenLabel) {
			t.Errorf("%s: the subject is %q and does not name the card", loc, p.Subject)
		}
		for _, mustNot := range []string{req.TokenSerial, req.ModulePath} {
			if strings.Contains(p.Subject, mustNot) {
				t.Errorf("%s: the subject carries %q, which is not for this screen", loc, mustNot)
			}
		}
	}
}

// TestTheCardIsNamedOnceEvenWhenTwoLabelsAgree is why the certificate label is
// not shown beside the token label.
//
// On the Pošta card they are the same string, so showing both would show a
// person their own name twice — the defect D-149 removed from the certificate
// list, where a Serbian card's authentication twin put the holder's own name
// on a second row, struck through, under a sentence about a distinction they
// have no vocabulary for.
func TestTheCardIsNamedOnceEvenWhenTwoLabelsAgree(t *testing.T) {
	req := posta()
	if req.TokenLabel != req.CertificateLabel {
		t.Fatal("this test's premise is that the Pošta card's two labels are the same string")
	}
	for _, loc := range locales {
		p := Text(i18n.Load(loc), req, 15)
		if n := strings.Count(p.Subject, req.TokenLabel); n != 1 {
			t.Errorf("%s: the card is named %d times in %q, want once", loc, n, p.Subject)
		}
	}
}

// TestACardWithNoLabelStillSaysWhichOne. A token that reports no label of its
// own falls back to the certificate's, because a subject reading "Card: " is
// worse than one naming the certificate.
func TestACardWithNoLabelStillSaysWhichOne(t *testing.T) {
	req := posta()
	req.TokenLabel = ""
	p := Text(i18n.Load("sr-Latn"), req, 15)
	if !strings.Contains(p.Subject, req.CertificateLabel) {
		t.Errorf("with no token label the subject is %q and names nothing", p.Subject)
	}
}

// TestNoFieldOfThePromptIsEmptyOrAKey.
//
// i18n.T falls back to the catalogue key when a message is missing, so a
// missing key renders as "pindialog.heading" on screen rather than failing.
// That is the right fallback and it is a bad thing to ship, and it is exactly
// what would happen if these seven keys were ever swept as unread — D-279
// recorded them as deliberately kept for a reader that had not arrived yet,
// and this is that reader.
func TestNoFieldOfThePromptIsEmptyOrAKey(t *testing.T) {
	for _, loc := range locales {
		p := Text(i18n.Load(loc), posta(), 15)
		fields := map[string]string{
			"Title": p.Title, "Heading": p.Heading, "Subject": p.Subject,
			"Label": p.Label, "Hint": p.Hint, "OK": p.OK, "Cancel": p.Cancel,
		}
		for name, value := range fields {
			if value == "" {
				t.Errorf("%s: %s is empty", loc, name)
			}
			if strings.HasPrefix(value, "pindialog.") {
				t.Errorf("%s: %s rendered as its own catalogue key (%q) — the key is "+
					"missing from this catalogue", loc, name, value)
			}
			if strings.Contains(value, "%!") {
				t.Errorf("%s: %s has a format-verb fault: %q", loc, name, value)
			}
		}
	}
}

// itoa avoids strconv for a one-line helper in a test whose subject is text.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
