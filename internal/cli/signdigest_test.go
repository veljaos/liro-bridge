//go:build softtoken

// Tests for sign-digest, which exists only in a build made with the
// "softtoken" tag (D-227).
package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

// fakeSignSession is a minimal keysource.Session used only to test
// internal/cli's own flag parsing, validation and output — the real
// signing behaviour is internal/signing's responsibility and is
// exhaustively tested there.
type fakeSignSession struct {
	cert    keysource.Certificate
	signErr error
	closed  bool
}

func (f *fakeSignSession) SignDigest(context.Context, keysource.DigestAlgorithm, []byte) ([]byte, error) {
	if f.signErr != nil {
		return nil, f.signErr
	}
	return []byte("raw-signature-bytes"), nil
}
func (f *fakeSignSession) Certificate() keysource.Certificate { return f.cert }
func (f *fakeSignSession) Chain() [][]byte                    { return nil }
func (f *fakeSignSession) Close() error                       { f.closed = true; return nil }

func validDigestHex() string {
	sum := sha256.Sum256([]byte("liro-bridge test input"))
	return hex.EncodeToString(sum[:])
}

func openDeps(sess keysource.Session, openErr error) SignDeps {
	return SignDeps{Open: func(context.Context, keysource.Thumbprint) (keysource.Session, error) {
		if openErr != nil {
			return nil, openErr
		}
		return sess, nil
	}}
}

func TestSignDigestWritesBase64ToStdoutByDefault(t *testing.T) {
	sess := &fakeSignSession{}
	var stdout, stderr bytes.Buffer
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", validDigestHex()}, &stdout, &stderr, "en", openDeps(sess, nil))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	got, err := base64.StdEncoding.DecodeString(strings.TrimSpace(stdout.String()))
	if err != nil {
		t.Fatalf("stdout is not valid base64: %q", stdout.String())
	}
	if string(got) != "raw-signature-bytes" {
		t.Fatalf("decoded signature = %q, want raw-signature-bytes", got)
	}
	if !sess.closed {
		t.Fatal("session must be closed")
	}
}

func TestSignDigestWritesRawBytesToOutFile(t *testing.T) {
	sess := &fakeSignSession{}
	var stdout, stderr bytes.Buffer
	outPath := filepath.Join(t.TempDir(), "sig.bin")
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", validDigestHex(), "--out", outPath}, &stdout, &stderr, "en", openDeps(sess, nil))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must be empty when --out is used, got: %q", stdout.String())
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	if string(raw) != "raw-signature-bytes" {
		t.Fatalf("file contents = %q, want raw-signature-bytes", raw)
	}
}

// TestSignDigestRejectsWrongLengthDigestBeforeOpen is the failing test
// for F2 §6: an invalid --digest must never reach deps.Open (which may
// touch the card / trigger a PIN prompt).
func TestSignDigestRejectsWrongLengthDigestBeforeOpen(t *testing.T) {
	sess := &fakeSignSession{}
	opened := false
	deps := SignDeps{Open: func(context.Context, keysource.Thumbprint) (keysource.Session, error) {
		opened = true
		return sess, nil
	}}
	var stdout, stderr bytes.Buffer
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", "not-64-hex-chars"}, &stdout, &stderr, "en", deps)
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero for an invalid digest")
	}
	if opened {
		t.Fatal("the card must never be touched for an invalid --digest")
	}
}

func TestSignDigestRequiresThumbprint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunSignDigest(context.Background(), []string{"--digest", validDigestHex()}, &stdout, &stderr, "en", SignDeps{})
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero when --thumbprint is missing")
	}
}

func TestSignDigestRepeatCapAt1000(t *testing.T) {
	sess := &fakeSignSession{}
	var stdout, stderr bytes.Buffer
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", validDigestHex(), "--repeat", "5000"}, &stdout, &stderr, "en", openDeps(sess, nil))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Signed 1000/1000") {
		// repeat is capped at 1000 (F2 §6): the report must reflect the
		// capped count, not the requested 5000.
		t.Fatalf("expected a report over 1000 items (capped), got: %s", stderr.String())
	}
}

func TestSignDigestReportsTimingForRepeat(t *testing.T) {
	sess := &fakeSignSession{}
	var stdout, stderr bytes.Buffer
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", validDigestHex(), "--repeat", "3"}, &stdout, &stderr, "en", openDeps(sess, nil))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must stay silent with --repeat > 1, got: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Signed 3/3") {
		t.Fatalf("expected a timing report, got: %s", stderr.String())
	}
}

func TestSignDigestMarksTestKeySignature(t *testing.T) {
	sess := &fakeSignSession{cert: keysource.Certificate{IsTestKey: true}}
	var stdout, stderr bytes.Buffer
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", validDigestHex()}, &stdout, &stderr, "en", openDeps(sess, nil))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "TEST SIGNATURE") {
		t.Fatalf("expected a visible test-signature warning (SPEC §16.6), got stderr: %s", stderr.String())
	}
}

func TestSignDigestDoesNotWarnForRealCertificate(t *testing.T) {
	sess := &fakeSignSession{cert: keysource.Certificate{IsTestKey: false}}
	var stdout, stderr bytes.Buffer
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", validDigestHex()}, &stdout, &stderr, "en", openDeps(sess, nil))
	if code != 0 {
		t.Fatalf("exit code = %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "TEST SIGNATURE") {
		t.Fatalf("a real certificate must not print the test-signature warning, got: %s", stderr.String())
	}
}

func TestSignDigestPropagatesOpenErrorAsLocalisedCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	openErr := errs.New(errs.CodeCardNotPresent, nil)
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", validDigestHex()}, &stdout, &stderr, "en", openDeps(nil, openErr))
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero when Open fails")
	}
	if !strings.Contains(stderr.String(), "Insert your card into the reader.") {
		t.Fatalf("expected the localised CARD_NOT_PRESENT message, got: %s", stderr.String())
	}
}

func TestSignDigestFailureSurfacesLocalisedCode(t *testing.T) {
	sess := &fakeSignSession{signErr: errs.New(errs.CodeSignFailed, nil)}
	var stdout, stderr bytes.Buffer
	code := RunSignDigest(context.Background(), []string{"--thumbprint", "ABCD", "--digest", validDigestHex()}, &stdout, &stderr, "en", openDeps(sess, nil))
	if code == 0 {
		t.Fatal("exit code = 0, want non-zero when signing fails")
	}
	if !strings.Contains(stderr.String(), "The card failed to produce a signature.") {
		t.Fatalf("expected the localised SIGN_FAILED message, got: %s", stderr.String())
	}
}
