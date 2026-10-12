package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource"
	"github.com/veljaos/liro-bridge/internal/logcheck"
	"github.com/veljaos/liro-bridge/internal/pades"
)

// notesSession is a key in memory with a self-signed certificate: enough
// for pades.SignDocument to build a real signature.
type notesSession struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func (s *notesSession) SignDigest(_ context.Context, _ keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	return rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest)
}
func (s *notesSession) Certificate() keysource.Certificate {
	return keysource.Certificate{Thumbprint: "NOTESTEST", DER: s.cert.Raw}
}
func (s *notesSession) Chain() [][]byte { return nil }
func (s *notesSession) Close() error    { return nil }

func onePagePDF() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	var offsets [5]int
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	}
	for i, body := range objects {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	xref := buf.Len()
	buf.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", xref)
	return buf.Bytes()
}

// TestTheWindowLogsTheSignersNotes is worklist 1.1's precondition
// (D-477): a signature that falls short says why in the agent's log,
// not only on the CLI. Here the timestamp authority is missing and B-B
// was accepted, so the signer leaves a note; before D-477 the window
// dropped it.
//
// Not shown: the protocol path's call (signRemoteOne needs a whole
// mainWindow), or a B-LT shortfall's notes — the card run reads those.
func TestTheWindowLogsTheSignersNotes(t *testing.T) {
	var logBuf bytes.Buffer
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(77),
		// A person's subject, so the scan below has something to find
		// if the window ever logs it (D-482).
		Subject:   pkix.Name{Country: []string{"RS"}, CommonName: "Петар Петровић", SerialNumber: "PNORS-0101990710006"},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(in, onePagePDF(), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := signInteractiveOne(t.Context(), in, &notesSession{cert: cert, key: key}, interactiveSignOptions{
		level:   pades.LevelBT,
		outPath: filepath.Join(dir, "out.pdf"),
		allowBB: true,
	})
	if err != nil {
		t.Fatalf("signing with B-B accepted: %v", err)
	}
	if result.AchievedLevel != pades.LevelBB || len(result.Notes) == 0 {
		t.Fatalf("achieved %s with %d notes; the test needs a B-B with a note to read", result.AchievedLevel, len(result.Notes))
	}
	got := logBuf.String()
	for _, found := range logcheck.Find(got, cert.Subject.String(), cert.Subject.CommonName, cert.Subject.SerialNumber) {
		t.Errorf("the window's log carries %s", found)
	}
	for _, want := range []string{"the signer's notes on this signature", "path=window", "requested=B-T", "achieved=B-B", "no timestamp (saved at B-B)"} {
		if !strings.Contains(got, want) {
			t.Errorf("the log does not say %q; it says:\n%s", want, got)
		}
	}
}
