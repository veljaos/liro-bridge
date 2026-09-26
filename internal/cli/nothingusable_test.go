package cli

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
)

// A machine with no reader, or a reader with no card, or no smart card
// service at all, is not an exotic state. It is the state of every
// machine where this agent is installed before the hardware is, and it
// is the permanent state of the Windows CI runner — where `sign` was
// measured giving up in 1.011s with no window and nothing said, because
// nothing in this report could tell those three apart from each other or
// from "there is simply nothing here" (D-236).

func hardwareRow(usable bool, reason errs.Code) CertRow {
	return CertRow{
		OnHardware: true,
		Info: classify.Info{
			Subject:         classify.Subject{CommonName: "Petar Petrović 123456"},
			Purpose:         classify.PurposeSigning,
			Qualification:   classify.QualificationQualified,
			Usable:          usable,
			NotUsableReason: reason,
			NotBefore:       time.Now().Add(-time.Hour),
			NotAfter:        time.Now().Add(time.Hour),
		},
	}
}

func TestTheReportSaysWhyItHasNothingToSignWith(t *testing.T) {
	reader := func(name string, card bool) platform.ReaderState {
		return platform.ReaderState{Name: name, CardPresent: card}
	}

	for _, tc := range []struct {
		name   string
		report Report
		want   errs.Code
	}{
		{
			name:   "no reader at all",
			report: Report{},
			want:   errs.CodeNoReader,
		},
		{
			name:   "a reader, and nothing in it",
			report: Report{Readers: []platform.ReaderState{reader("Generic Smart Card Reader Interface 0", false)}},
			want:   errs.CodeCardNotPresent,
		},
		{
			name: "two readers, neither holding a card",
			report: Report{Readers: []platform.ReaderState{
				reader("Reader 0", false), reader("Reader 1", false),
			}},
			want: errs.CodeCardNotPresent,
		},
		{
			// A card is in, and this agent has nothing to offer off it:
			// a different remedy from either of the two above, and the
			// person needs to be told which one they are in.
			name:   "a card, and nothing on it",
			report: Report{Readers: []platform.ReaderState{reader("Reader 0", true)}},
			want:   errs.CodeCertNotFound,
		},
		{
			name: "one certificate, its card out",
			report: Report{
				Readers:      []platform.ReaderState{reader("Reader 0", false)},
				Certificates: []CertRow{hardwareRow(false, errs.CodeCardNotPresent)},
			},
			want: errs.CodeCardNotPresent,
		},
		{
			name: "two certificates, unusable for two different reasons",
			report: Report{
				Readers: []platform.ReaderState{reader("Reader 0", true)},
				Certificates: []CertRow{
					hardwareRow(false, errs.CodeCardNotPresent),
					hardwareRow(false, errs.CodeCertExpired),
				},
			},
			want: errs.CodeCertNotUsable,
		},
		{
			name: "one usable certificate among unusable ones",
			report: Report{
				Readers: []platform.ReaderState{reader("Reader 0", true)},
				Certificates: []CertRow{
					hardwareRow(false, errs.CodeCertExpired),
					hardwareRow(true, ""),
				},
			},
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.report.NothingUsableReason(); got != tc.want {
				t.Errorf("NothingUsableReason() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A row nobody is offered is not evidence that there is something to
// sign with. The Windows-internal artefacts every machine's store is
// full of are hidden from every listing this project shows, so a machine
// with nothing but those and no reader is a machine with no reader.
func TestHiddenRowsAreNotSomethingToSignWith(t *testing.T) {
	internal := CertRow{
		Info: classify.Info{
			Subject:       classify.Subject{CommonName: "690414da-410d-4d2b-b233-585d193482b3"},
			SelfSigned:    true,
			Qualification: classify.QualificationNotQualified,
		},
	}
	if !internal.Hidden() {
		t.Fatal("this fixture is meant to be a hidden row and is not; the test below proves nothing")
	}
	r := Report{Certificates: []CertRow{internal}}
	if got := r.NothingUsableReason(); got != errs.CodeNoReader {
		t.Errorf("NothingUsableReason() = %q over hidden rows only, want %q", got, errs.CodeNoReader)
	}
}

// SMART_CARD_SERVICE_DOWN has existed since F1 and was never once
// produced: platform.ErrSmartCardServiceDown travelled as a bare wrapped
// error, so every caller above saw an unclassified failure. This is the
// test that it is a code now, and that an unrelated failure does not
// borrow it.
func TestAStoppedSmartCardServiceIsItsOwnCode(t *testing.T) {
	deps := func(readerErr error) Deps {
		return Deps{
			Readers:   func(context.Context) ([]platform.ReaderState, error) { return nil, readerErr },
			Enumerate: nil,
			Store:     &fakeStore{},
		}
	}

	_, err := Gather(context.Background(), deps(platform.ErrSmartCardServiceDown), time.Now())
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != errs.CodeSmartCardServiceDown {
		t.Fatalf("a stopped service produced %v, want code %s", err, errs.CodeSmartCardServiceDown)
	}
	if !errors.Is(err, platform.ErrSmartCardServiceDown) {
		t.Error("the cause was lost on the way out; errors.Is no longer finds it")
	}

	_, err = Gather(context.Background(), deps(errors.New("winscard is on fire")), time.Now())
	if !errors.As(err, &e) || e.Code != errs.CodeInternal {
		t.Fatalf("an unclassified reader failure produced %v, want code %s", err, errs.CodeInternal)
	}
}

// TestAStoppedPCSCDExplainsAnEmptyListAndHidesNothing is F12 §9 on Linux
// (D-358). pcscd off is the reason no card gives a certificate, and it is
// never a reason to list nothing: a soft token or SoftHSM needs no pcscd,
// so the listing goes on and the report carries the reason beside it.
func TestAStoppedPCSCDExplainsAnEmptyListAndHidesNothing(t *testing.T) {
	deps := func(serviceErr error) Deps {
		return Deps{
			Readers:     func(context.Context) ([]platform.ReaderState, error) { return []platform.ReaderState{}, nil },
			Enumerate:   func(context.Context) ([]windowscng.Certificate, error) { return nil, nil },
			Store:       &fakeStore{},
			CardService: func(context.Context) error { return serviceErr },
		}
	}

	report, err := Gather(context.Background(), deps(fmt.Errorf("%w: pcscd does not answer", platform.ErrSmartCardServiceDown)), time.Now())
	if err != nil {
		t.Fatalf("a stopped pcscd failed the whole listing: %v", err)
	}
	if !report.CardServiceDown {
		t.Fatal("a stopped pcscd is not in the report")
	}
	if got := report.NothingUsableReason(); got != errs.CodeSmartCardServiceDown {
		t.Errorf("with nothing listed and pcscd stopped, the reason is %s, want %s", got, errs.CodeSmartCardServiceDown)
	}

	// A usable certificate from somewhere that needs no pcscd is still
	// something to sign with.
	report.Certificates = []CertRow{hardwareRow(true, "")}
	if got := report.NothingUsableReason(); got != "" {
		t.Errorf("a usable certificate was hidden behind a stopped pcscd: reason %s", got)
	}

	report, err = Gather(context.Background(), deps(nil), time.Now())
	if err != nil || report.CardServiceDown {
		t.Errorf("pcscd answering gave err %v, down %v", err, report.CardServiceDown)
	}
	if got := report.NothingUsableReason(); got != errs.CodeNoReader {
		t.Errorf("pcscd answering and nothing listed gave %s, want %s", got, errs.CodeNoReader)
	}

	// A check that failed for some other reason is not a stopped service.
	report, err = Gather(context.Background(), deps(errors.New("something else")), time.Now())
	if err != nil || report.CardServiceDown {
		t.Errorf("an unrelated check failure gave err %v, down %v; it must not claim pcscd is off", err, report.CardServiceDown)
	}
}
