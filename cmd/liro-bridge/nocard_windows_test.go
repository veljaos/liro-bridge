//go:build windows

package main

// What `sign` does on a machine that has nothing to sign with.
//
// Measured on windows-latest, which has no reader and no smart card
// service at all: `liro-bridge sign --in x.pdf` enumerated first, the
// enumeration failed in 2ms, and the command returned 1 after 1.011s
// having produced no window, nothing on stdout, and one ERROR line in a
// JSON log file inside a temporary directory nobody would look in. That
// is F10's stranger — somebody who installs the agent before plugging
// the reader in — and it is also every developer's first run.
//
// The window comes first now and says which of the three states this
// machine is in (D-236). These tests are how that is checked on a
// machine that is in none of them: the enumeration is a field, so the
// window can be given the answer a reader-less machine would give and
// then read.

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/audit"
	"github.com/veljaos/liro-bridge/internal/cli"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
)

// swapGather puts a listing of this test's choosing behind every signing
// window opened while it runs, and puts the real one back afterwards.
func swapGather(t *testing.T, gather func(context.Context) (cli.Report, error)) {
	t.Helper()
	prev := interactiveGather
	interactiveGather = gather
	t.Cleanup(func() { interactiveGather = prev })
}

func blankPDFIn(t *testing.T, dir string) string {
	t.Helper()
	in := filepath.Join(dir, "ugovor.pdf")
	blank, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pdfs", "blank.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, blank, 0o600); err != nil {
		t.Fatal(err)
	}
	return in
}

// TestTheSignWindowIsOnScreenBeforeTheCertificateListIs is the ordering,
// asserted as an ordering rather than as a duration.
//
// The enumeration is held open by this test, so "the window appeared
// while the certificates had not" is a state observed rather than a
// stopwatch read (D-201). Against the code as it stood the window is not
// merely late here — it never comes at all, because the enumeration this
// test is holding is the thing that used to run first.
func TestTheSignWindowIsOnScreenBeforeTheCertificateListIs(t *testing.T) {
	tempConfigHome(t)

	release := make(chan struct{})
	asked := make(chan struct{})
	swapGather(t, func(context.Context) (cli.Report, error) {
		close(asked)
		<-release
		return cli.Report{}, errs.New(errs.CodeSmartCardServiceDown, errors.New("smart card service is not running"))
	})

	in := blankPDFIn(t, t.TempDir())
	const title = "Liro Bridge"
	before := liroWindowsTitled(t, title)

	done := make(chan int, 1)
	go func() { done <- run([]string{"sign", "--in", in}, io.Discard) }()

	select {
	case <-asked:
	case <-time.After(30 * time.Second):
		t.Fatal("the certificate enumeration was never started")
	}

	hwnd := waitForNewWindowTitled(t, title, before, 60*time.Second)
	select {
	case code := <-done:
		t.Fatalf("sign returned %d before its window was answered", code)
	default:
	}

	// Only now is the enumeration allowed to finish: everything above
	// happened with the machine's answer still unknown.
	close(release)

	const wmClose = 0x0010
	_, _, _ = procPostMessageT.Call(hwnd, wmClose, 0, 0)
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("sign did not return after its window was closed")
	}
}

// TestSignExitsNonZeroWhenThereIsNothingToSignWith: a person sees a
// window and a sentence. A script sees only the exit code, and a machine
// with nothing to sign with is a failed `sign` rather than a refused one.
//
// The window is closed only after the sentence is on it, because closing
// it before the listing has landed is a different thing — that is a
// person who changed their mind, and it is not a failure.
func TestSignExitsNonZeroWhenThereIsNothingToSignWith(t *testing.T) {
	tempConfigHome(t)
	c := i18n.Load("sr-Latn")

	m, exit := openSigningFlowForTest(t, "sr-Latn", func(context.Context) (cli.Report, error) {
		return cli.Report{}, nil // no readers: NO_READER
	})
	waitForConsentNotice(t, m, c.T("error.no_reader"))

	const wmClose = 0x0010
	_, _, _ = procPostMessageT.Call(m.win.Handle(), wmClose, 0, 0)
	if code := exit(); code == 0 {
		t.Error("sign reported success on a machine with no card reader")
	}
}

// TestTheCertificateStepSaysWhichKindOfNothingThisIs: the three states a
// person can act on are three different remedies — a cable, a card, a
// Windows service — and the screen has to say which one they are in.
//
// It reads the page, not the payload: the sentence has to be on screen,
// and the element it goes in was previously filled by a static label the
// page resolved for itself ("None of the certificates on this card can
// be used for signing"), which asserts a card that may not exist and
// which nothing in this suite had ever looked at.
func TestTheCertificateStepSaysWhichKindOfNothingThisIs(t *testing.T) {
	locale := "sr-Latn"
	c := i18n.Load(locale)

	for _, tc := range []struct {
		name    string
		listing func(context.Context) (cli.Report, error)
		want    string
	}{
		{
			name: "no smart card service",
			listing: func(context.Context) (cli.Report, error) {
				return cli.Report{}, errs.New(errs.CodeSmartCardServiceDown, errors.New("no service"))
			},
			want: c.T("error.smart_card_service_down"),
		},
		{
			name: "no reader",
			listing: func(context.Context) (cli.Report, error) {
				return cli.Report{}, nil
			},
			want: c.T("error.no_reader"),
		},
		{
			name: "a reader with no card in it",
			listing: func(context.Context) (cli.Report, error) {
				return cli.Report{Readers: []platform.ReaderState{
					{Name: "Generic Smart Card Reader Interface 0"},
				}}, nil
			},
			want: c.T("error.card_not_present"),
		},
		{
			name: "a card with nothing on it",
			listing: func(context.Context) (cli.Report, error) {
				return cli.Report{Readers: []platform.ReaderState{
					{Name: "Generic Smart Card Reader Interface 0", CardPresent: true},
				}}, nil
			},
			want: c.T("consent.no_certificate_found"),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tempConfigHome(t)
			m, _ := openSigningFlowForTest(t, locale, tc.listing)

			waitForConsentNotice(t, m, tc.want)

			// Nothing on this screen can be approved: there is nothing to
			// approve it with.
			if !evalBool(t, m.win, "document.getElementById('approve-btn').disabled") {
				t.Error("Approve is pressable on a screen offering no certificate")
			}
			if !evalBool(t, m.win, "document.getElementById('select-prompt').hidden") {
				t.Error("the screen still asks the person to select a certificate")
			}
		})
	}
}

// TestTheCertificateStepNeverAssertsACardBeforeItHasLooked: the second
// the window is up and the enumeration has not answered, the screen must
// not already be claiming that nothing here can sign — that is a verdict
// on a question nobody has asked yet.
func TestTheCertificateStepNeverAssertsACardBeforeItHasLooked(t *testing.T) {
	tempConfigHome(t)
	locale := "sr-Latn"
	c := i18n.Load(locale)

	release := make(chan struct{})
	m, _ := openSigningFlowForTest(t, locale, func(context.Context) (cli.Report, error) {
		<-release
		return cli.Report{}, nil
	})
	t.Cleanup(func() { close(release) })

	got := evalString(t, m.win, "document.getElementById('cert-notice').textContent")
	if got != c.T("consent.looking_for_certificates") {
		t.Errorf("while the listing was still running the screen said %q, want %q",
			got, c.T("consent.looking_for_certificates"))
	}
	if evalBool(t, m.win, "document.getElementById('cert-notice').hidden") {
		t.Error("the screen says nothing at all while it looks")
	}
}

// TestALiveListSaysNothingAboveItself: a list with rows on it explains
// itself row by row, and a summary above repeating one row's reason is
// what a photograph of the real window caught — "Ubacite karticu u
// čitač." printed twice on one screen, once as the machine's verdict and
// once as the certificate's own.
func TestALiveListSaysNothingAboveItself(t *testing.T) {
	tempConfigHome(t)
	unusable := stampTestCertificate()
	unusable.Usable = false
	unusable.NotUsableReason = errs.CodeCardNotPresent

	m, _ := openSigningFlowForTest(t, "sr-Latn", func(context.Context) (cli.Report, error) {
		return cli.Report{
			Readers:      []platform.ReaderState{{Name: "Reader 0"}},
			Certificates: []cli.CertRow{{OnHardware: true, Info: unusable}},
		}, nil
	})

	// The row is there and says why it cannot be chosen...
	waitForCertRows(t, m, 1)
	if got := evalString(t, m.win, "document.querySelector('.cert-reason').textContent"); got == "" {
		t.Error("the unusable row carries no reason of its own")
	}
	// ...and nothing above it says the same thing again.
	if got := evalString(t, m.win, "document.getElementById('cert-notice').textContent"); got != "" {
		t.Errorf("the screen repeats the row's own reason above the list: %q", got)
	}
	if !evalBool(t, m.win, "document.getElementById('approve-btn').disabled") {
		t.Error("Approve is pressable with no usable certificate on the list")
	}
}

// waitForCertRows waits for the list to hold n rows, which is how this
// test knows the listing has landed without timing anything.
func waitForCertRows(t *testing.T, m *mainWindow, n int) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var got float64
	for time.Now().Before(deadline) {
		got = evalNumber(t, m.win, "document.querySelectorAll('.liro-cert-row').length")
		if int(got) == n {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the certificate list holds %v rows, want %d", got, n)
}

// TestAUsableCertificateStillGetsNoNotice: the line exists for the
// screens that have nothing to offer, and must not appear on the one
// that does.
func TestAUsableCertificateStillGetsNoNotice(t *testing.T) {
	tempConfigHome(t)
	m, _ := openSigningFlowForTest(t, "sr-Latn", func(context.Context) (cli.Report, error) {
		return cli.Report{
			Readers:      []platform.ReaderState{{Name: "Reader 0", CardPresent: true}},
			Certificates: []cli.CertRow{{OnHardware: true, Info: usableTestCertificateInfo()}},
		}, nil
	})

	waitForConsentNotice(t, m, "")
	if evalBool(t, m.win, "document.getElementById('select-prompt').hidden") {
		t.Error("a screen with a usable certificate does not ask the person to choose one")
	}
}

// TestAProtocolRequestIsToldWhichKindOfNothingThisIs: the other front
// door does not open a window at a person to explain a program's
// problem — it answers the program with a code (SPEC §7). It was
// answering `INTERNAL` for every listing failure, hardcoded, which meant
// `SMART_CARD_SERVICE_DOWN` — a code docs/PROTOCOL.md documents with its
// own status and its own remedy — could not be produced by any request
// this agent has ever served.
func TestAProtocolRequestIsToldWhichKindOfNothingThisIs(t *testing.T) {
	tempConfigHome(t)
	for _, tc := range []struct {
		name    string
		listing func(context.Context) (cli.Report, error)
		want    errs.Code
	}{
		{
			name: "no smart card service",
			listing: func(context.Context) (cli.Report, error) {
				return cli.Report{}, errs.New(errs.CodeSmartCardServiceDown, errors.New("no service"))
			},
			want: errs.CodeSmartCardServiceDown,
		},
		{
			name: "something nobody classified",
			listing: func(context.Context) (cli.Report, error) {
				return cli.Report{}, errors.New("winscard is on fire")
			},
			want: errs.CodeInternal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			swapGather(t, tc.listing)
			req := digestRequest(1, "")
			got := runProtocolFlow(context.Background(), config.Default(), "sr-Latn", req, testJob(t, 1))
			if got.Code != tc.want {
				t.Errorf("the caller was told %q, want %q", got.Code, tc.want)
			}
		})
	}
}

// openSigningFlowForTest opens the real `sign` window over a listing of
// the test's choosing and returns it, closed at the end of the test.
//
// It builds the flow with newSigningFlow — the same constructor
// runSigningFlow uses, so what is on screen here is what is on screen
// there — rather than reproducing it, which is how a settings window
// came to be checked against the value it was handed instead of the one
// on disk (D-134).
// The returned function waits for the flow to end and yields its exit
// code; it is called by the test's own cleanup too, so calling it is
// optional and calling it twice is the same as calling it once.
func openSigningFlowForTest(t *testing.T, locale string, gather func(context.Context) (cli.Report, error)) (*mainWindow, func() int) {
	t.Helper()
	in := blankPDFIn(t, t.TempDir())
	input, err := newInteractiveInput(in)
	if err != nil {
		t.Fatal(err)
	}

	m := newSigningFlow(config.Default(), locale, flowRequest{inputs: []interactiveInput{input}})
	m.gather = gather
	m.auditStore = func() (*audit.Store, error) { return audit.NewStore(t.TempDir()) }

	ctx, cancel := context.WithCancel(context.Background())
	m.startListing(ctx)
	exited := make(chan int, 1)
	go func() { exited <- m.open(ctx, nil, stepCertificate) }()

	var once sync.Once
	var code int
	wait := func() int {
		once.Do(func() {
			select {
			case code = <-exited:
			case <-time.After(30 * time.Second):
				t.Error("the signing window never closed")
			}
		})
		return code
	}
	t.Cleanup(func() {
		cancel()
		wait()
	})

	select {
	case <-m.ready:
	case <-time.After(60 * time.Second):
		t.Fatal("the signing window never opened")
	}
	return m, wait
}

// waitForConsentNotice waits for the notice line to say want. It waits
// for the text rather than for a moment: the listing arrives into the
// window's own loop, which is a different goroutine from this one, and
// the only thing worth waiting for is the sentence itself (D-201).
func waitForConsentNotice(t *testing.T, m *mainWindow, want string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		got = evalString(t, m.win, "document.getElementById('cert-notice').textContent")
		if got == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the certificate step says %q, want %q", got, want)
}

func usableTestCertificateInfo() classify.Info {
	info := stampTestCertificate()
	info.Usable = true
	info.NotUsableReason = ""
	return info
}
