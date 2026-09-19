//go:build windows || softtoken

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// quietLog is a logger with nowhere to write. What openInOrder logs is a fact
// about a run, not a property under test, and a test that asserted on log text
// would be asserting on wording.
func quietLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// recorder is one backend's attempt, which records that it was asked.
//
// Whether a backend was asked *at all* is most of what these tests measure:
// asking one is not free — it can put a PIN screen in front of a person and
// spend an attempt on their card — so "did not sign here" and "was never asked"
// are different outcomes and the difference is the substance.
type recorder struct {
	asked int
	sess  keysource.Session
	orig  signerOrigin
	err   error
}

func (r *recorder) open(context.Context, keysource.Thumbprint) (keysource.Session, signerOrigin, error) {
	r.asked++
	return r.sess, r.orig, r.err
}

// fakeSession is a session that cannot sign. Nothing here signs; these tests
// are about which backend is asked, and a session that did anything would
// invite a test about what it did.
type fakeSession struct{ keysource.Session }

func notFoundInCNG() error { return errs.New(errs.CodeCertNotFound, nil) }

func succeeding(name string) *recorder {
	return &recorder{sess: fakeSession{}, orig: signerOrigin{backend: name}}
}

func failing(err error) *recorder { return &recorder{err: err} }

// TestTheDefaultOrderIsCNGThenTheModule is D-311, and the assertion that
// matters is the second one: when CNG signs, the module is never asked.
//
// Never asked, not merely unused. Asking a PKCS#11 module for a session means
// C_Login, which means a PIN screen and an attempt on somebody's card. A chain
// that asked both and preferred the first would be a chain that costs a PIN for
// a signature CNG had already made.
func TestTheDefaultOrderIsCNGThenTheModule(t *testing.T) {
	cng := succeeding("windows-cng")
	p11 := succeeding("pkcs11")

	_, origin, err := openInOrder(context.Background(), "ABCD", false, cng.open, p11.open, nil, quietLog())
	if err != nil {
		t.Fatalf("openInOrder: %v", err)
	}
	if origin.backend != "windows-cng" {
		t.Errorf("signed through %q, want windows-cng", origin.backend)
	}
	if p11.asked != 0 {
		t.Errorf("the module was asked %d times for a certificate CNG had. "+
			"Asking it means C_Login, which means a PIN screen and an attempt "+
			"on the card.", p11.asked)
	}
}

// TestThePreferenceInvertsTheOrderAndCNGIsNeverAsked is the option's whole
// purpose, and the same "never asked" rule in the other direction.
func TestThePreferenceInvertsTheOrderAndCNGIsNeverAsked(t *testing.T) {
	cng := succeeding("windows-cng")
	p11 := succeeding("pkcs11")

	_, origin, err := openInOrder(context.Background(), "ABCD", true, cng.open, p11.open, nil, quietLog())
	if err != nil {
		t.Fatalf("openInOrder: %v", err)
	}
	if origin.backend != "pkcs11" {
		t.Errorf("signed through %q, want pkcs11", origin.backend)
	}
	if cng.asked != 0 {
		t.Errorf("CNG was asked %d times although the module signed", cng.asked)
	}
}

// TestPreferringTheModuleNeverAsksOneCardForTwoPINs is the rule the whole
// arrangement turns on, and it is the one a future change is most likely to
// break by accident: the fallback path after CNG must not run a second PKCS#11
// walk when the preference already did one.
//
// SPEC §6.5.1 clause 5 is that one wrong PIN is one attempt, and on a machine
// with one card behind two builds of one vendor's module (D-271) a second walk
// spends a second attempt on the same mistake. Three attempts block the card,
// and for a national identity card that means a visit to a police station.
func TestPreferringTheModuleNeverAsksOneCardForTwoPINs(t *testing.T) {
	// The module does not have it, CNG does not have it either: the path that
	// runs the most backends, and therefore the one where a duplicate walk
	// would hide.
	p11 := failing(pkcs11.ErrCertificateNotFound)
	cng := failing(notFoundInCNG())

	_, _, err := openInOrder(context.Background(), "ABCD", true, cng.open, p11.open, nil, quietLog())
	if err == nil {
		t.Fatal("a certificate no backend has was opened")
	}
	if p11.asked != 1 {
		t.Fatalf("the module was asked %d times for one signature, want exactly 1. "+
			"Each ask is a C_Login and an attempt on the card; three block it.", p11.asked)
	}
}

// TestAPreferredModuleThatFailsLeavesNobodyWorseOff. Somebody turns the
// preference on because their minidriver is broken. Somebody else turns it on,
// finds their module is the broken one, and must still be able to sign.
//
// This is the one place a non-not-found error is deliberately walked past, and
// the test exists because that is the opposite of the rule everywhere else
// (D-033) and will look like a mistake to whoever reads it next.
func TestAPreferredModuleThatFailsLeavesNobodyWorseOff(t *testing.T) {
	p11 := failing(errors.New("pkcs11 worker: the worker process stopped answering"))
	cng := succeeding("windows-cng")

	_, origin, err := openInOrder(context.Background(), "ABCD", true, cng.open, p11.open, nil, quietLog())
	if err != nil {
		t.Fatalf("a broken preferred module cost a signature CNG could have made: %v", err)
	}
	if origin.backend != "windows-cng" {
		t.Errorf("signed through %q, want windows-cng", origin.backend)
	}
}

// TestWhenThePreferredModuleFailsItsReasonIsTheOneReported. The person
// configured that path. "CNG has no such certificate" explains nothing about
// why the module they chose did not work, and is what they would otherwise be
// shown.
func TestWhenThePreferredModuleFailsItsReasonIsTheOneReported(t *testing.T) {
	moduleErr := errors.New("pkcs11 worker: C:\\vendor.dll: the worker process stopped answering")
	p11 := failing(moduleErr)
	cng := failing(notFoundInCNG())

	_, _, err := openInOrder(context.Background(), "ABCD", true, cng.open, p11.open, nil, quietLog())
	if !errors.Is(err, moduleErr) {
		t.Fatalf("err = %v\nwant the preferred module's own reason, not CNG's", err)
	}
}

// TestARealCNGFailureStopsTheChain is D-033, unchanged by any of this: a card
// that was removed, a blocked PIN or a missing reader is a real answer, and
// trying an unrelated backend afterwards produces a confusing failure about the
// wrong thing.
func TestARealCNGFailureStopsTheChain(t *testing.T) {
	cardGone := errs.New(errs.CodeCardNotPresent, nil)
	cng := failing(cardGone)
	p11 := succeeding("pkcs11")
	soft := succeeding("softtoken")

	_, _, err := openInOrder(context.Background(), "ABCD", false, cng.open, p11.open, soft.open, quietLog())
	if !errors.Is(err, cardGone) {
		t.Fatalf("err = %v, want the card-not-present answer CNG gave", err)
	}
	if p11.asked != 0 || soft.asked != 0 {
		t.Errorf("a removed card sent the chain on to other backends: module asked %d, "+
			"soft token asked %d", p11.asked, soft.asked)
	}
}

// TestTheSoftTokenIsLastAndOnlyWhenThereIsOne. nil soft is every release build;
// the tag adds it and must not take the card away (F2 §6.1).
func TestTheSoftTokenIsLastAndOnlyWhenThereIsOne(t *testing.T) {
	t.Run("it is tried when nothing else has the certificate", func(t *testing.T) {
		soft := succeeding("softtoken")
		_, origin, err := openInOrder(context.Background(), "ABCD", false,
			failing(notFoundInCNG()).open, failing(pkcs11.ErrCertificateNotFound).open, soft.open, quietLog())
		if err != nil {
			t.Fatalf("openInOrder: %v", err)
		}
		if origin.backend != "softtoken" {
			t.Errorf("signed through %q, want softtoken", origin.backend)
		}
	})

	t.Run("a release build has none and says what CNG said", func(t *testing.T) {
		_, _, err := openInOrder(context.Background(), "ABCD", false,
			failing(notFoundInCNG()).open, failing(pkcs11.ErrCertificateNotFound).open, nil, quietLog())
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeCertNotFound {
			t.Fatalf("err = %v, want CNG's CERT_NOT_FOUND — the answer a person can act on", err)
		}
	})
}
