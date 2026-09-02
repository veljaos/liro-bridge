package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func fakeDeps(t *testing.T, certs []windowscng.Certificate, anyCard bool, store tsl.Store) Deps {
	t.Helper()
	return Deps{
		Readers: func(context.Context) ([]platform.ReaderState, error) {
			return []platform.ReaderState{{Name: "Test Reader", CardPresent: anyCard}}, nil
		},
		AnyCardPresent: func(context.Context) (bool, error) { return anyCard, nil },
		Enumerate:      func(context.Context) ([]windowscng.Certificate, error) { return certs, nil },
		Store:          store,
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

func TestHiddenDoesNotHideUnusableAuthenticationCertificates(t *testing.T) {
	// The authentication certificate in F1's own §6.1 example is shown
	// by default even though it is not usable — only PurposeUnknown +
	// NotQualified together are hidden (SPEC §11.10/F1 §5.4).
	der := loadDER(t, "halcom_auth.der")
	store := &fakeStore{list: bundledTSLList(t)}
	deps := fakeDeps(t, []windowscng.Certificate{{DER: der, OnHardware: true}}, true, store)

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if report.Certificates[0].Hidden() {
		t.Fatal("an authentication certificate must not be Hidden() by default")
	}
}
