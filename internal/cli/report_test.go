package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

const testdataDir = "../../testdata/certs"

func loadDER(t *testing.T, filename string) []byte {
	t.Helper()
	der, err := os.ReadFile(filepath.Join(testdataDir, filename))
	if err != nil {
		t.Fatalf("reading %s: %v", filename, err)
	}
	return der
}

// fakeStore implements tsl.Store without any network access, so
// internal/cli's own logic is testable independent of the Trusted
// List's fetch/verify machinery (already covered in internal/trust/tsl).
type fakeStore struct {
	list       *tsl.List
	prov       tsl.Provenance
	refreshErr error
	refreshed  int
}

func (f *fakeStore) Current(context.Context) (*tsl.List, tsl.Provenance, error) {
	return f.list, f.prov, nil
}

func (f *fakeStore) Refresh(context.Context) error {
	f.refreshed++
	return f.refreshErr
}

func bundledTSLList(t *testing.T) *tsl.List {
	t.Helper()
	raw, err := os.ReadFile("../trust/tsl/seed/TSL-RS.xml")
	if err != nil {
		t.Fatalf("reading bundled seed: %v", err)
	}
	list, err := tsl.Parse(raw)
	if err != nil {
		t.Fatalf("parsing bundled seed: %v", err)
	}
	return list
}

var referenceTime = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

// fakeDeps builds Deps whose PresenceCheck returns anyCard uniformly for
// every certificate — that is enough for every existing test here, none
// of which has more than one hardware-backed certificate. Task 2's own
// discriminating test (TestGatherPresenceIsPerCertificateNotGlobal)
// builds Deps directly instead, with a PresenceCheck that answers
// differently per thumbprint.
func fakeDeps(t *testing.T, certs []windowscng.Certificate, anyCard bool, store tsl.Store) Deps {
	t.Helper()
	return Deps{
		Readers: func(context.Context) ([]platform.ReaderState, error) {
			return []platform.ReaderState{{Name: "Test Reader", CardPresent: anyCard}}, nil
		},
		PresenceCheck: func(context.Context, keysource.Thumbprint) (bool, error) { return anyCard, nil },
		Enumerate:     func(context.Context) ([]windowscng.Certificate, error) { return certs, nil },
		Store:         store,
	}
}

// TestGatherClassifiesAgainstBundledTSL is the F1 §5.4/§6 integration
// point: real (synthetic-but-issuer-faithful) certificate DER, run
// through actual enumeration-shaped Deps and the real bundled Trusted
// List, produces a qualified, usable row.
func TestGatherClassifiesAgainstBundledTSL(t *testing.T) {
	der := loadDER(t, "mup_signing.der")
	store := &fakeStore{list: bundledTSLList(t), prov: tsl.Provenance{Source: tsl.SourceEmbedded, Sequence: 36, IssuedAt: referenceTime}}

	deps := fakeDeps(t, []windowscng.Certificate{
		{Thumbprint: "ABCDEF0123456789", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
	}, true, store)

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(report.Certificates) != 1 {
		t.Fatalf("len(Certificates) = %d, want 1", len(report.Certificates))
	}
	row := report.Certificates[0]
	if row.Info.Qualification != classify.QualificationQualified {
		t.Errorf("Qualification = %v, want QualificationQualified", row.Info.Qualification)
	}
	if !row.Info.Usable {
		t.Errorf("Usable = false, want true (reason=%q)", row.Info.NotUsableReason)
	}
	if row.Info.Thumbprint != "ABCDEF0123456789" {
		t.Errorf("Thumbprint = %q, want the value supplied by enumeration", row.Info.Thumbprint)
	}
	if store.refreshed != 1 {
		t.Errorf("Store.Refresh called %d times, want exactly 1 (F1 §4.8: refresh at startup)", store.refreshed)
	}
}

func TestGatherSkipsMalformedCertificateWithoutFailing(t *testing.T) {
	store := &fakeStore{list: nil, prov: tsl.Provenance{Source: tsl.SourceEmbedded}}
	deps := fakeDeps(t, []windowscng.Certificate{
		{Thumbprint: "BAD", DER: []byte("not a certificate"), OnHardware: false},
	}, false, store)

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v, want nil (a malformed store entry should be skipped, not fatal)", err)
	}
	if len(report.Certificates) != 0 {
		t.Fatalf("len(Certificates) = %d, want 0", len(report.Certificates))
	}
}

// TestGatherIgnoresRefreshFailure proves Gather never fails just because
// the Trusted List refresh failed (F1 §4.8: network failure is never
// fatal) — the existing list is still classified against successfully.
func TestGatherIgnoresRefreshFailure(t *testing.T) {
	store := &fakeStore{
		list:       bundledTSLList(t),
		prov:       tsl.Provenance{Source: tsl.SourceCache, Sequence: 36, IssuedAt: referenceTime},
		refreshErr: errors.New("connection refused"),
	}
	deps := fakeDeps(t, nil, false, store)

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v, want nil despite the refresh failure", err)
	}
	if report.TSL.Source != tsl.SourceCache {
		t.Fatalf("TSL.Source = %v, want SourceCache (unchanged by the failed refresh)", report.TSL.Source)
	}
}

// TestGatherPresenceIsPerCertificateNotGlobal is Task 2's own reported
// scenario: a machine with only one card in the reader nevertheless
// reported a second, physically absent certificate as usable, because
// presence was decided once (AnyCardPresent) and applied to every
// hardware-backed certificate. Two certificates with the same
// (otherwise qualified and usable) DER but different thumbprints, and a
// PresenceCheck that answers differently per thumbprint, must produce
// two different Usable results — not the same one twice.
func TestGatherPresenceIsPerCertificateNotGlobal(t *testing.T) {
	der := loadDER(t, "mup_signing.der")
	store := &fakeStore{list: bundledTSLList(t), prov: tsl.Provenance{Source: tsl.SourceEmbedded, Sequence: 36, IssuedAt: referenceTime}}

	deps := Deps{
		Readers: func(context.Context) ([]platform.ReaderState, error) {
			return []platform.ReaderState{{Name: "Test Reader", CardPresent: true}}, nil
		},
		PresenceCheck: func(_ context.Context, thumbprint keysource.Thumbprint) (bool, error) {
			return thumbprint == "PRESENT", nil
		},
		Enumerate: func(context.Context) ([]windowscng.Certificate, error) {
			return []windowscng.Certificate{
				{Thumbprint: "PRESENT", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
				{Thumbprint: "ABSENT", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
			}, nil
		},
		Store: store,
	}

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(report.Certificates) != 2 {
		t.Fatalf("len(Certificates) = %d, want 2", len(report.Certificates))
	}

	byThumbprint := map[string]classify.Info{}
	for _, row := range report.Certificates {
		byThumbprint[row.Info.Thumbprint] = row.Info
	}
	if !byThumbprint["PRESENT"].Usable {
		t.Errorf("PRESENT certificate Usable = false, want true (reason=%q)", byThumbprint["PRESENT"].NotUsableReason)
	}
	if byThumbprint["ABSENT"].Usable {
		t.Fatal("ABSENT certificate Usable = true, want false — this is the exact bug Task 2 fixes: one card present marking every hardware-backed certificate usable")
	}
	if byThumbprint["ABSENT"].NotUsableReason != errs.CodeCardNotPresent {
		t.Errorf("ABSENT certificate NotUsableReason = %q, want CARD_NOT_PRESENT", byThumbprint["ABSENT"].NotUsableReason)
	}
}

func TestHiddenHidesUnknownPurposeUnqualifiedCertificates(t *testing.T) {
	der := loadDER(t, "selfsigned_unrelated.der")
	store := &fakeStore{list: bundledTSLList(t)}
	deps := fakeDeps(t, []windowscng.Certificate{{DER: der, OnHardware: false}}, false, store)

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(report.Certificates) != 1 {
		t.Fatalf("len(Certificates) = %d, want 1", len(report.Certificates))
	}
	if !report.Certificates[0].Hidden() {
		t.Fatal("a not-qualified, unknown-purpose certificate must be Hidden()")
	}
}

// TestHiddenHidesAuthenticationCertificates is the reversal of F1
// §6.1's "shown disabled, not hidden" rule. The real Halcom pair is
// what makes the case: the two certificates' Subject DNs are identical
// byte for byte (SPEC §11.5), so leaving the authentication one in the
// list shows the person their own name twice, the second time struck
// through. It is not a choice, and `--all` still shows it.
func TestHiddenHidesAuthenticationCertificates(t *testing.T) {
	der := loadDER(t, "halcom_auth.der")
	store := &fakeStore{list: bundledTSLList(t)}
	deps := fakeDeps(t, []windowscng.Certificate{{DER: der, OnHardware: true}}, true, store)

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if !report.Certificates[0].Hidden() {
		t.Fatal("an authentication certificate must be Hidden() by default")
	}
}

// TestHiddenKeepsASigningCertificateThatCannotBeUsedNow is the other
// half of the same rule, and the one that must not slip: a signing
// certificate whose card is absent is a real choice temporarily
// unavailable. Hiding *that* is what would make a card look broken.
func TestHiddenKeepsASigningCertificateThatCannotBeUsedNow(t *testing.T) {
	der := loadDER(t, "halcom_signing.der")
	store := &fakeStore{list: bundledTSLList(t)}
	deps := fakeDeps(t, []windowscng.Certificate{{DER: der, OnHardware: true}}, false, store)

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if report.Certificates[0].Info.Usable {
		t.Fatal("fixture assumption broken: with no card present this certificate must be unusable")
	}
	if report.Certificates[0].Hidden() {
		t.Fatal("a signing certificate whose card is absent must stay visible, disabled with its reason")
	}
}

// TestGatherProbesEachCertificateOnce is J-7's own change: within one
// listing, the same certificate is asked about once, however many times
// it is enumerated.
//
// The probe is not cheap — measured on real hardware at 457 ms for a
// certificate whose card is present and 855 ms for one whose card is
// not — so a store that lists a certificate twice used to pay for it
// twice, for a question whose answer cannot change while a single list
// is being built.
func TestGatherProbesEachCertificateOnce(t *testing.T) {
	der := loadDER(t, "mup_signing.der")
	store := &fakeStore{list: bundledTSLList(t), prov: tsl.Provenance{Source: tsl.SourceEmbedded, Sequence: 36, IssuedAt: referenceTime}}

	probes := map[string]int{}
	deps := Deps{
		Readers: func(context.Context) ([]platform.ReaderState, error) {
			return []platform.ReaderState{{Name: "Test Reader", CardPresent: true}}, nil
		},
		PresenceCheck: func(_ context.Context, tp keysource.Thumbprint) (bool, error) {
			probes[string(tp)]++
			return string(tp) == "AAAA", nil
		},
		Enumerate: func(context.Context) ([]windowscng.Certificate, error) {
			return []windowscng.Certificate{
				{Thumbprint: "AAAA", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
				{Thumbprint: "BBBB", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
				{Thumbprint: "AAAA", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
				{Thumbprint: "BBBB", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
			}, nil
		},
		Store: store,
	}

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(report.Certificates) != 4 {
		t.Fatalf("len(Certificates) = %d, want 4 — the memo must not drop rows", len(report.Certificates))
	}
	for tp, n := range probes {
		if n != 1 {
			t.Errorf("%s was probed %d times in one listing, want 1", tp, n)
		}
	}
	// The remembered answer must be the right one, not merely one
	// answer: the two thumbprints disagree, and both rows of each must
	// carry that thumbprint's own result.
	for i, row := range report.Certificates {
		want := row.Info.Thumbprint == "AAAA"
		if row.Info.Usable != want {
			t.Errorf("row %d (%s): Usable = %v, want %v", i, row.Info.Thumbprint, row.Info.Usable, want)
		}
	}
}

// TestGatherAsksAgainOnTheNextListing is the other half, and the one
// that matters for SPEC §11.10: the memo lives inside one Gather. A card
// can be inserted between one listing and the next, and reporting a
// certificate as available when it is not is the worst outcome this
// product has.
func TestGatherAsksAgainOnTheNextListing(t *testing.T) {
	der := loadDER(t, "mup_signing.der")
	store := &fakeStore{list: bundledTSLList(t), prov: tsl.Provenance{Source: tsl.SourceEmbedded, Sequence: 36, IssuedAt: referenceTime}}

	present := false
	calls := 0
	deps := Deps{
		Readers: func(context.Context) ([]platform.ReaderState, error) {
			return []platform.ReaderState{{Name: "Test Reader", CardPresent: present}}, nil
		},
		PresenceCheck: func(context.Context, keysource.Thumbprint) (bool, error) {
			calls++
			return present, nil
		},
		Enumerate: func(context.Context) ([]windowscng.Certificate, error) {
			return []windowscng.Certificate{
				{Thumbprint: "AAAA", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
			}, nil
		},
		Store: store,
	}

	first, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if first.Certificates[0].Info.Usable {
		t.Fatal("with no card present the certificate must not be usable")
	}

	present = true // the card was inserted between the two listings
	second, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if !second.Certificates[0].Info.Usable {
		t.Fatal("the second listing did not ask again: a card inserted between two listings must be seen")
	}
	if calls != 2 {
		t.Fatalf("PresenceCheck called %d times across two listings, want 2", calls)
	}
}

// TestGatherRemembersAFailedProbeToo pins the cost of the conservative
// answer: a probe that errors is treated as "not present" and that
// answer is remembered, so a certificate that fails to probe does not
// pay for the failure once per enumeration entry.
func TestGatherRemembersAFailedProbeToo(t *testing.T) {
	der := loadDER(t, "mup_signing.der")
	store := &fakeStore{list: bundledTSLList(t), prov: tsl.Provenance{Source: tsl.SourceEmbedded, Sequence: 36, IssuedAt: referenceTime}}

	calls := 0
	deps := Deps{
		Readers: func(context.Context) ([]platform.ReaderState, error) {
			return []platform.ReaderState{{Name: "Test Reader", CardPresent: true}}, nil
		},
		PresenceCheck: func(context.Context, keysource.Thumbprint) (bool, error) {
			calls++
			return false, errors.New("probe failed")
		},
		Enumerate: func(context.Context) ([]windowscng.Certificate, error) {
			return []windowscng.Certificate{
				{Thumbprint: "AAAA", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
				{Thumbprint: "AAAA", DER: der, Provider: "Microsoft Smart Card Key Storage Provider", OnHardware: true},
			}, nil
		},
		Store: store,
	}

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if calls != 1 {
		t.Fatalf("a failing probe was retried within one listing (%d calls), want 1", calls)
	}
	for i, row := range report.Certificates {
		if row.Info.Usable {
			t.Errorf("row %d: Usable = true after a failed probe, want false (conservative)", i)
		}
	}
}
