package main

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"strings"
	"testing"
	"time"
)

// The needle is shaped like what a real Serbian identity certificate
// carries in its common name: a name, then the holder's JMBG, then the
// key's purpose. It is invented — no real person's number appears in
// this repository, which is the whole point of the thing being guarded
// (D-487).
const (
	needleName = "PETAR PETROVIC"
	needleJMBG = "0101990710099"
	needleCN   = needleName + " " + needleJMBG + " Sign"
)

func aCertificate(cn string) *x509.Certificate {
	return &x509.Certificate{
		Raw:       []byte("not a real certificate, but enough bytes to hash"),
		Subject:   pkix.Name{CommonName: cn},
		Issuer:    pkix.Name{CommonName: "MUP Gradjani CA 4"},
		NotBefore: time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2031, 9, 22, 0, 0, 0, 0, time.UTC),
	}
}

// TestSignerSubjectNeverReachesTheOutput is D59's guard. This command
// writes to standard output, so whatever it prints reaches terminals,
// transcripts, CI logs and anything pasted into an issue — and on an
// identity card the signer's common name is a person's name and their
// national identity number. D-480 took those out of every message the
// agent writes and this verifier was not changed with it; D-487 is what
// that cost when a third party's number reached a public repository.
func TestSignerSubjectNeverReachesTheOutput(t *testing.T) {
	var out bytes.Buffer
	printCert(&out, "  signer", aCertificate(needleCN), false)
	got := out.String()

	for _, forbidden := range []string{needleCN, needleName, needleJMBG} {
		if strings.Contains(got, forbidden) {
			t.Errorf("the signer's subject reached the output: %q is in\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, subjectWithheld) {
		t.Errorf("nothing said what was withheld; output was\n%s", got)
	}
}

// TestWithheldSubjectStillIdentifiesTheCertificate: withholding the name
// must not cost the reader the ability to tie this signature to a log
// line or an audit entry. The thumbprint is what those name it by.
func TestWithheldSubjectStillIdentifiesTheCertificate(t *testing.T) {
	c := aCertificate(needleCN)
	var out bytes.Buffer
	printCert(&out, "  signer", c, false)
	got := out.String()

	for _, want := range []string{
		thumbprintOf(c),     // the same form the audit log uses
		"MUP Gradjani CA 4", // the issuer is an organisation, not a person
		"2026-09-22 .. 2031-09-22",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing from\n%s", want, got)
		}
	}
}

// TestAMachineSubjectIsStillPrinted: a timestamp unit is not somebody,
// and hiding its name would cost a reader the one thing that says which
// authority stamped the document.
func TestAMachineSubjectIsStillPrinted(t *testing.T) {
	const unit = "TEST Timestamp server 290000560"
	var out bytes.Buffer
	printCert(&out, "  timestamp", aCertificate(unit), true)
	got := out.String()

	if !strings.Contains(got, unit) {
		t.Errorf("the timestamp unit's subject was withheld; output was\n%s", got)
	}
	if strings.Contains(got, subjectWithheld) {
		t.Errorf("a machine's subject was treated as a person's; output was\n%s", got)
	}
}

// TestThumbprintIsTheFormEverythingElseUses: uppercase hex SHA-1, as in
// the audit chain, Windows' certificate store and the agent's warnings.
// Written out in this command rather than imported, because this is the
// second implementation SPEC §16.4 requires (D-044) — so it is worth a
// test that the duplicate agrees in shape.
func TestThumbprintIsTheFormEverythingElseUses(t *testing.T) {
	got := thumbprintOf(aCertificate(needleCN))
	if len(got) != 40 {
		t.Errorf("a SHA-1 thumbprint is 40 hex characters; got %d (%q)", len(got), got)
	}
	if got != strings.ToUpper(got) {
		t.Errorf("thumbprints are uppercase elsewhere in this project; got %q", got)
	}
	if strings.ContainsAny(got, "ghijklmnopqrstuvwxyz ") {
		t.Errorf("not hex: %q", got)
	}
}
