package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// TestAnEmptyListOnLinuxIsExplainedFromTheModulesSlots is open item A20: with
// no reader listing, the modules' own slots decide which of the three
// whole-machine reasons applies. Each case is a state measured on the F12 VM,
// named for what was plugged in and installed.
func TestAnEmptyListOnLinuxIsExplainedFromTheModulesSlots(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cards *CardSlots
		want  errs.Code
	}{
		{
			// A stock Ubuntu desktop with only this package: gnome-keyring
			// and p11-kit-trust answer, with no slot a card goes into.
			name:  "no card program installed",
			cards: &CardSlots{},
			want:  errs.CodeNoReader,
		},
		{
			// SafeSign's four placeholder slots, and nothing in them.
			name:  "a card program, and no card anywhere",
			cards: &CardSlots{ReaderSlots: 5},
			want:  errs.CodeCardNotPresent,
		},
		{
			// The MUP card in the passed-through reader: SafeSign and OpenSC
			// each saw it and neither recognised it. The window said "no
			// reader was found".
			name:  "a card no module recognises",
			cards: &CardSlots{ReaderSlots: 6, CardsPresent: 2, CardsUnrecognised: 2},
			want:  errs.CodeCertNotFound,
		},
		{
			// A card that was read and offered nothing to sign with.
			name:  "a card read, nothing on it to sign with",
			cards: &CardSlots{ReaderSlots: 1, CardsPresent: 1},
			want:  errs.CodeCertNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Report{Cards: tc.cards}).NothingUsableReason(); got != tc.want {
				t.Errorf("reason = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTheSlotsDecideNothingWhereReadersAreListed keeps Windows as it was:
// where SCardListReaders answered, it is the witness, and a survey — which
// Windows never produces — could not overrule it if one appeared.
func TestTheSlotsDecideNothingWhereReadersAreListed(t *testing.T) {
	r := Report{
		Readers: []platform.ReaderState{{Name: "Reader 0", CardPresent: false}},
		Cards:   &CardSlots{ReaderSlots: 1, CardsPresent: 1, CardsUnrecognised: 1},
	}
	if got := r.NothingUsableReason(); got != errs.CodeCardNotPresent {
		t.Errorf("reason = %q, want the reader listing's CARD_NOT_PRESENT", got)
	}
}

// TestSlotsDoNotHideAUsableCertificate: the survey explains an empty list and
// never empties one.
func TestSlotsDoNotHideAUsableCertificate(t *testing.T) {
	r := Report{
		Certificates: []CertRow{hardwareRow(true, "")},
		Cards:        &CardSlots{ReaderSlots: 2, CardsPresent: 2, CardsUnrecognised: 1},
	}
	if got := r.NothingUsableReason(); got != "" {
		t.Errorf("reason = %q with a usable certificate offered", got)
	}
}

// TestCertsSaysWhyItsListIsEmpty is the terminal half of A20: `certs` printed
// "Sertifikati: 0" and nothing else to a person with a MUP card in the
// reader, while the window said why. On Linux it now says what the window
// says, from the same key; elsewhere its output is unchanged.
func TestCertsSaysWhyItsListIsEmpty(t *testing.T) {
	c := i18n.Load("sr-Latn")
	mup := Report{Cards: &CardSlots{ReaderSlots: 6, CardsPresent: 2, CardsUnrecognised: 2}}
	var out bytes.Buffer
	RenderText(&out, mup, c, time.Now(), false)

	want := c.T(NothingUsableKey(errs.CodeCertNotFound))
	said := strings.Contains(out.String(), want)
	if runtime.GOOS == "linux" {
		if !said {
			t.Fatalf("certs did not say why its list is empty; want %q in:\n%s", want, out.String())
		}
		for _, claim := range []string{"MUP", "Pošte Srbije", "Halcom"} {
			if !strings.Contains(want, claim) {
				t.Errorf("the sentence does not say which issuers are supported (%s missing): %q", claim, want)
			}
		}
		if strings.Contains(out.String(), c.T(i18n.CodeKey(errs.CodeNoReader))) {
			t.Errorf("certs told a person with a card in the reader that there is no reader:\n%s", out.String())
		}
	} else if said {
		t.Errorf("certs changed its output off Linux:\n%s", out.String())
	}

	// A usable certificate: nothing is explained, because nothing is missing.
	out.Reset()
	RenderText(&out, Report{Certificates: []CertRow{hardwareRow(true, "")}, Cards: mup.Cards}, c, time.Now(), false)
	if strings.Contains(out.String(), want) {
		t.Errorf("certs explained an empty list that was not empty:\n%s", out.String())
	}
}
