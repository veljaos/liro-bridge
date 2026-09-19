package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// collapseCert is the certificate every test in this file uses: the same
// issuer-faithful MUP signing certificate the rest of this package's tests use,
// with a thumbprint of this file's own choosing.
//
// The thumbprint is what the collapse turns on, and it is deliberately the
// *given* value rather than one computed here: Gather takes the enumerating
// backend's word for it, exactly as it does in the product, so a test that
// recomputed it would be checking something Gather never does.
const collapseThumb = "AF5063BB74378BD503AB46DD08AEAD205BA2AA54"

func collapseCertificate(t *testing.T) ([]byte, string) {
	t.Helper()
	return loadDER(t, "mup_signing.der"), collapseThumb
}

// collapseDeps builds a Deps whose CNG half returns cngCerts and whose PKCS#11
// half returns moduleCerts, with a Trusted List that knows nothing.
//
// The Trusted List not knowing these certificates is deliberate and does not
// weaken anything here: what is being measured is whether one certificate
// becomes one row, which is a question about identity and not about trust.
func collapseDeps(t *testing.T, cngCerts []windowscng.Certificate, moduleCerts []ModuleCertificate, moduleFailures []ModuleFailure) Deps {
	t.Helper()
	store := &fakeStore{list: bundledTSLList(t), prov: tsl.Provenance{Source: tsl.SourceEmbedded, Sequence: 36, IssuedAt: referenceTime}}
	return Deps{
		Readers: func(context.Context) ([]platform.ReaderState, error) {
			return []platform.ReaderState{{Name: "a reader", CardPresent: true}}, nil
		},
		PresenceCheck: func(context.Context, keysource.Thumbprint) (bool, error) {
			// False on purpose. The PKCS#11 sighting is what should establish
			// presence for a collapsed row, and a probe that already said yes
			// would hide whether it did.
			return false, nil
		},
		Enumerate: func(context.Context) ([]windowscng.Certificate, error) { return cngCerts, nil },
		Store:     store,
		ModuleCertificates: func(context.Context) ([]ModuleCertificate, []ModuleFailure, error) {
			return moduleCerts, moduleFailures, nil
		},
	}
}

// TestOneCardSeenThroughBothBackendsIsOneRow is F11 §4 step 3, and it is the
// property the whole step exists for.
//
// D-310 measured a Pošta certificate read through aetpkss1.dll giving
// byte-for-byte the thumbprint the Windows store gives for the same
// certificate. Without the collapse a person with one card would be offered it
// twice and have no way to tell the two entries apart, which is worse than
// offering it once through the wrong backend.
func TestOneCardSeenThroughBothBackendsIsOneRow(t *testing.T) {
	der, thumb := collapseCertificate(t)

	report, err := Gather(context.Background(), collapseDeps(t,
		[]windowscng.Certificate{{Thumbprint: thumb, DER: der, OnHardware: true}},
		[]ModuleCertificate{{Thumbprint: thumb, DER: der, ModulePath: `C:\Windows\System32\aetpkss1.dll`}},
		nil), referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	if len(report.Certificates) != 1 {
		t.Fatalf("one card through two backends produced %d rows, want 1", len(report.Certificates))
	}
	row := report.Certificates[0]
	if len(row.Backends) != 2 || row.Backends[0] != backendCNG || row.Backends[1] != backendPKCS11 {
		t.Errorf("Backends = %v, want [%s %s] — the collapse must not lose the fact "+
			"that there were two sightings", row.Backends, backendCNG, backendPKCS11)
	}
	if len(row.Modules) != 1 || row.Modules[0] != `C:\Windows\System32\aetpkss1.dll` {
		t.Errorf("Modules = %v, want the one module that saw it", row.Modules)
	}
	// CNG is first, which is the order the backends were asked in and therefore
	// the order D-311 will sign in.
	if row.Backends[0] != backendCNG {
		t.Errorf("Backends[0] = %q; CNG is asked first and signs first (D-311)", row.Backends[0])
	}
}

// TestAModuleSightingEstablishesThatTheCardIsThere. PKCS#11 enumeration only
// ever looks at slots with a token present, so a certificate read off one is
// evidence the card is in the reader — better evidence than the presence probe,
// because the bytes came off the card.
//
// The probe in these deps answers false, so a row that comes out usable can
// only have got there through the module sighting.
func TestAModuleSightingEstablishesThatTheCardIsThere(t *testing.T) {
	der, thumb := collapseCertificate(t)

	withModule, err := Gather(context.Background(), collapseDeps(t,
		[]windowscng.Certificate{{Thumbprint: thumb, DER: der, OnHardware: true}},
		[]ModuleCertificate{{Thumbprint: thumb, DER: der, ModulePath: "a.dll"}},
		nil), referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	// The control: the same listing with no module at all. If this one also
	// reports the card as present then the assertion above measures nothing.
	withoutModule, err := Gather(context.Background(), collapseDeps(t,
		[]windowscng.Certificate{{Thumbprint: thumb, DER: der, OnHardware: true}},
		nil, nil), referenceTime)
	if err != nil {
		t.Fatalf("Gather (control): %v", err)
	}

	got := withModule.Certificates[0].Info.NotUsableReason
	control := withoutModule.Certificates[0].Info.NotUsableReason
	if got == control {
		t.Fatalf("a module sighting made no difference to the row: reason %q either way.\n"+
			"Either the sighting is not establishing presence, or this test's control "+
			"is not a control.", got)
	}
	if control != errs.CodeCardNotPresent {
		t.Fatalf("the control's reason is %q, want CARD_NOT_PRESENT — the probe answers "+
			"false, so that is the only reason this row should be unusable", control)
	}
	if got == errs.CodeCardNotPresent {
		t.Errorf("a certificate read off a token is reported as having no card present")
	}
}

// TestACertificateOnlyAModuleCanSeeGetsItsOwnRow. The collapse must not become
// a filter: a card Windows has no minidriver for is exactly the case F12 exists
// for, and its certificate appears in no CNG listing at all.
func TestACertificateOnlyAModuleCanSeeGetsItsOwnRow(t *testing.T) {
	der, thumb := collapseCertificate(t)

	report, err := Gather(context.Background(), collapseDeps(t, nil,
		[]ModuleCertificate{{Thumbprint: thumb, DER: der, ModulePath: "only.dll"}},
		nil), referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(report.Certificates) != 1 {
		t.Fatalf("got %d rows, want 1", len(report.Certificates))
	}
	row := report.Certificates[0]
	if !row.OnHardware {
		t.Error("a certificate read off a token is not marked as being on hardware")
	}
	if len(row.Backends) != 1 || row.Backends[0] != backendPKCS11 {
		t.Errorf("Backends = %v, want [%s]", row.Backends, backendPKCS11)
	}
	// The card is in the reader — the bytes came off it. The probe in these
	// deps answers false and is never consulted for a certificate CNG never
	// enumerated, so a row reported as having no card present would mean the
	// sighting's own evidence was thrown away.
	if row.Info.NotUsableReason == errs.CodeCardNotPresent {
		t.Errorf("a certificate read off a token is reported as CARD_NOT_PRESENT; " +
			"PKCS#11 enumeration only looks at slots with a token in them")
	}
}

// TestTwoModulesSeeingOneCardIsStillOneRow. This project's own machine has
// NetSeT installed at two paths in two builds five years apart, and both see
// the same card identically (D-271). That is two sightings of one certificate
// and it is one row, with both modules named — which is what a person needs
// when one of those builds is twenty-seven times slower than the other (D-305).
func TestTwoModulesSeeingOneCardIsStillOneRow(t *testing.T) {
	der, thumb := collapseCertificate(t)

	report, err := Gather(context.Background(), collapseDeps(t, nil,
		[]ModuleCertificate{
			{Thumbprint: thumb, DER: der, ModulePath: `C:\Program Files\TrustEdgeID\netsetpkcs11_x64.dll`},
			{Thumbprint: thumb, DER: der, ModulePath: `C:\Program Files\MUP RS\Celik\netsetpkcs11_x64.dll`},
		}, nil), referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(report.Certificates) != 1 {
		t.Fatalf("one card through two modules produced %d rows, want 1", len(report.Certificates))
	}
	if got := len(report.Certificates[0].Modules); got != 2 {
		t.Errorf("the row names %d modules, want 2 — which one is being used is the "+
			"question D-305 makes worth asking", got)
	}
}

// TestAModuleThatCouldNotBeAskedIsARowAndNotASilence. F11 §3's rule, at the
// level a person reads: the listing survives, and the reason their certificate
// is missing is in the report rather than in a log file they will not open.
func TestAModuleThatCouldNotBeAskedIsARowAndNotASilence(t *testing.T) {
	der, thumb := collapseCertificate(t)

	report, err := Gather(context.Background(), collapseDeps(t,
		[]windowscng.Certificate{{Thumbprint: thumb, DER: der, OnHardware: true}},
		nil,
		[]ModuleFailure{{Path: `C:\broken.dll`, Origin: "configured", Reason: "the module did not load"}},
	), referenceTime)
	if err != nil {
		t.Fatalf("a module that would not load failed the whole listing: %v", err)
	}
	if len(report.Certificates) != 1 {
		t.Errorf("the listing lost its certificates: %d rows", len(report.Certificates))
	}
	if len(report.ModuleFailures) != 1 {
		t.Fatalf("got %d module failures, want 1", len(report.ModuleFailures))
	}
	if report.ModuleFailures[0].Path != `C:\broken.dll` {
		t.Errorf("the failure names %q", report.ModuleFailures[0].Path)
	}
}

// TestNothingChangesWhenThereIsNoPKCS11Path. Nil ModuleCertificates is every
// build without a PKCS#11 path and every machine with no module installed, and
// it must produce exactly what it always did.
func TestNothingChangesWhenThereIsNoPKCS11Path(t *testing.T) {
	der, thumb := collapseCertificate(t)
	deps := collapseDeps(t, []windowscng.Certificate{{Thumbprint: thumb, DER: der, OnHardware: true}}, nil, nil)
	deps.ModuleCertificates = nil

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(report.Certificates) != 1 {
		t.Fatalf("got %d rows, want 1", len(report.Certificates))
	}
	if len(report.ModuleFailures) != 0 {
		t.Errorf("ModuleFailures = %v, want none", report.ModuleFailures)
	}
	if got := report.Certificates[0].Modules; len(got) != 0 {
		t.Errorf("Modules = %v, want none", got)
	}
}

// TestTheJSONOutputShowsThatTheCollapseHappened. The collapse is only
// checkable if the output says which backends offered a row.
//
// Without it, a person looking at one row for one card cannot tell a correct
// collapse from one backend having found nothing — which are very different
// machines to be standing in front of, and the second is what F12 exists to
// fix. This is the field the owner's own verification reads.
func TestTheJSONOutputShowsThatTheCollapseHappened(t *testing.T) {
	der, thumb := collapseCertificate(t)

	report, err := Gather(context.Background(), collapseDeps(t,
		[]windowscng.Certificate{{Thumbprint: thumb, DER: der, OnHardware: true}},
		[]ModuleCertificate{{Thumbprint: thumb, DER: der, ModulePath: `C:\Windows\System32\aetpkss1.dll`}},
		[]ModuleFailure{{Path: `C:\broken.dll`, Origin: "configured", Reason: "the module did not load"}},
	), referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	var buf bytes.Buffer
	if err := RenderJSON(&buf, report, referenceTime); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var got struct {
		Certificates []struct {
			Thumbprint string   `json:"thumbprint"`
			Backends   []string `json:"backends"`
			Modules    []string `json:"modules"`
		} `json:"certificates"`
		ModuleFailures []struct {
			Path   string `json:"path"`
			Origin string `json:"origin"`
			Reason string `json:"reason"`
		} `json:"moduleFailures"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("the output is not JSON: %v\n%s", err, buf.String())
	}
	if len(got.Certificates) != 1 {
		t.Fatalf("got %d certificates in the JSON, want 1", len(got.Certificates))
	}
	c := got.Certificates[0]
	if len(c.Backends) != 2 {
		t.Errorf("backends = %v, want both", c.Backends)
	}
	if len(c.Modules) != 1 || c.Modules[0] != `C:\Windows\System32\aetpkss1.dll` {
		t.Errorf("modules = %v", c.Modules)
	}
	if len(got.ModuleFailures) != 1 || got.ModuleFailures[0].Path != `C:\broken.dll` {
		t.Errorf("moduleFailures = %v", got.ModuleFailures)
	}
}

// TestTheJSONIsUnchangedWhereThereIsNoPKCS11. omitempty on all three fields, so
// a build with no PKCS#11 path or a machine with no module produces exactly the
// bytes it always did — which matters because F2 §6.1's OpenSSL recipe reads
// this output with jq.
func TestTheJSONIsUnchangedWhereThereIsNoPKCS11(t *testing.T) {
	der, thumb := collapseCertificate(t)
	deps := collapseDeps(t, []windowscng.Certificate{{Thumbprint: thumb, DER: der, OnHardware: true}}, nil, nil)
	deps.ModuleCertificates = nil

	report, err := Gather(context.Background(), deps, referenceTime)
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var buf bytes.Buffer
	if err := RenderJSON(&buf, report, referenceTime); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	for _, key := range []string{"modules", "moduleFailures"} {
		if bytes.Contains(buf.Bytes(), []byte(`"`+key+`"`)) {
			t.Errorf("the JSON names %q on a machine with no PKCS#11 module:\n%s", key, buf.String())
		}
	}
	// backends is the exception and is present: a CNG row says so, which is
	// new information rather than an empty field, and F1 §6.1's contract is
	// that --json reports the same data the text view classifies.
	if !bytes.Contains(buf.Bytes(), []byte(`"backends"`)) {
		t.Errorf("the JSON does not say which backend found the certificate:\n%s", buf.String())
	}
}
