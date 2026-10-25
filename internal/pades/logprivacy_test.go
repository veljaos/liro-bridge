package pades

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/logcheck"
)

// personalSubject is a signer's subject the way a MUP card carries one:
// a person's name in Cyrillic and a JMBG in the serial-number attribute.
// Both are made up.
var personalSubject = pkix.Name{
	Country:      []string{"RS"},
	CommonName:   "Петар Петровић",
	SerialNumber: "PNORS-0101990710006",
}

// TestTheLogCarriesNoPersonalData is the check D-480 found missing
// (the owner, D-482): every path through SignDocument that touches a
// certificate, signed by a person, with the agent's real logger writing
// a real bridge.log. Then the log, every note and every error are read
// for a JMBG, a written-out subject, and any certificate's name. The
// notes are read because cmd's logSignNotes puts them in the log; the
// errors because they reach the person's terminal and the calling
// program.
//
// Not covered: the paths it does not drive (logcheck says what it cannot
// see), and the window's and protocol's own log lines beyond the notes.
func TestTheLogCarriesNoPersonalData(t *testing.T) {
	dir := t.TempDir()
	old := slog.Default()
	_, closer, err := config.SetupLogging("debug", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closer.Close(); slog.SetDefault(old) })

	var messages []string
	var known []string
	seen := map[string]bool{}
	knowCert := func(c *x509.Certificate) {
		for _, v := range []string{c.Subject.String(), c.Subject.CommonName, c.Subject.SerialNumber} {
			if v != "" && !seen[v] {
				seen[v] = true
				known = append(known, v)
			}
		}
	}
	known = append(known, "Петар", "Петровић", "0101990710006")
	record := func(path string, r *Result, err error) {
		if err != nil {
			messages = append(messages, path+" error: "+err.Error())
		}
		if r != nil {
			messages = append(messages, path+" notes: "+strings.Join(r.Notes, "; "))
		}
	}
	personal := func(f *bltFixture, ocsp []string) {
		f.session = newChainedSessionFor(t, personalSubject, f.ca, f.caKey, ocsp, nil)
		knowCert(f.session.cert)
		knowCert(f.ca)
		knowCert(f.tsaCert)
	}

	// B-LT reached.
	f := newBLTFixture(t)
	personal(f, f.session.cert.OCSPServer)
	r, err := f.sign(t)
	if err != nil || r.AchievedLevel != LevelBLT {
		t.Fatalf("the reached path gave %v, %v; it is a control and must reach B-LT", r, err)
	}
	record("reached", r, err)

	// The signer revoked: CERT_REVOKED, nothing saved.
	f = newBLTFixture(t)
	personal(f, f.session.cert.OCSPServer)
	f.revoke(f.session.cert)
	r, err = f.sign(t)
	record("signer revoked", r, err)

	// The timestamp authority revoked: B-T, and a note.
	f = newBLTFixture(t)
	personal(f, f.session.cert.OCSPServer)
	f.revoke(f.tsaCert)
	r, err = f.signKeepingBT(t)
	record("authority revoked", r, err)

	// The signer's responder never answers: B-T, a note, and dss's
	// warning for a certificate with no evidence.
	f = newBLTFixture(t)
	silent := httptest.NewServer(nil)
	silentURL := silent.URL + "/ocsp"
	silent.Close()
	personal(f, []string{silentURL})
	r, err = f.signKeepingBT(t)
	record("no evidence", r, err)

	// No issuer to be found for the signer: dss's other warning.
	f = newBLTFixture(t)
	personal(f, f.session.cert.OCSPServer)
	f.session.caDER = nil
	r, err = f.signKeepingBT(t)
	record("no issuer", r, err)

	_ = closer.Close()
	logs, _ := filepath.Glob(filepath.Join(dir, "bridge.log*"))
	if len(logs) == 0 {
		t.Fatal("no bridge.log was written; this test reads nothing")
	}
	var logText strings.Builder
	for _, p := range logs {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		logText.Write(b)
	}
	// Each path must have said something, or this test reads nothing.
	for _, want := range []string{"no issuer found for a certificate", "no OCSP response or CRL could be had"} {
		if !strings.Contains(logText.String(), want) {
			t.Fatalf("bridge.log does not say %q, so that path did not run:\n%s", want, logText.String())
		}
	}
	all := strings.Join(messages, "\n")
	for _, want := range []string{"signer revoked error: CERT_REVOKED", "authority revoked notes: B-LT not claimed", "no evidence notes: B-LT requested"} {
		if !strings.Contains(all, want) {
			t.Fatalf("no message %q, so that path did not run:\n%s", want, all)
		}
	}
	for _, found := range logcheck.Find(logText.String(), known...) {
		t.Errorf("bridge.log carries %s", found)
	}
	for _, m := range messages {
		for _, found := range logcheck.Find(m, known...) {
			t.Errorf("%s — carries %s", m, found)
		}
	}
}
