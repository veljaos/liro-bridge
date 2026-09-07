package pades

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// updateGolden regenerates testdata/golden/minimal-signed-bb.pdf instead
// of comparing against it — deliberate opt-in only, per this file's own
// TestGoldenFile doc comment.
var updateGolden = flag.Bool("update", false, "regenerate the golden file instead of comparing against it")

// goldenPath is the stored, byte-exact reference F3 §4.5/§16.2 requires:
// "sign with a fixed test key and a fixed timestamp, compare the output
// byte for byte against a stored reference." This catches an accidental
// structural change to the signer's output that still happens to
// verify — the class of bug otherwise invisible until an external
// validator somewhere rejects it.
const goldenPath = "../../testdata/golden/minimal-signed-bb.pdf"

// goldenKeyPEM is a fixed RSA-2048 key used only to produce the golden
// file's byte-exact reference signature. It carries no security value
// and signs nothing outside this one deterministic test fixture.
//
// It is embedded here, rather than generated at test time from a seeded
// math/rand source, because that was tried first and found not to
// work: crypto/rsa.GenerateKey draws additional randomness during
// primality search beyond what a seeded io.Reader alone determines
// (confirmed empirically — the same rand.NewSource(42) reader produced
// a different key on every run), so it cannot produce the fixed key a
// byte-exact golden file needs. x509.CreateCertificate, given this
// already-fixed key, *is* fully reproducible from a seeded reader
// (confirmed the same way) — the self-signing step below still uses
// one, rather than embedding a second fixed artifact for it.
const goldenKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA42Xhwq/827aeilxLNi8cYG9QSZJTHC+MrOClEElUZ4IYoxOl
Nqk9oqfxVA2mC05i178Tb5wd9z27YaZCu48togSEHlLw825M+SWZyQ2VxZ6Y7LcA
/bUAKGnhQSFreSJ0gkH2/WDSXer59OX//wfq/jQ1nbJ/IEOxhcGlNKS15JbrLXhk
D8Bqyz7H9tbfztjISRRiKxeGLktsiM+X0P0fgyZG7JzLLU7O2Rbus+98RDAIxtIJ
sPAyJ+FJlXPbsdQMp5VwZb5PZwIi801VQjar7XA+kXDNqkB6P8w7/zcx+YnKTqtB
Y6hwfLxpfkjiL0ntbZ7jY7eyQChs1zR1rnLOIQIDAQABAoIBABsCb6+QeDv/o7Ki
9kMEEv6IUjS+5tS++BpTn3+A+j/GJdd+3p2QuhOvF3zIlzuaDqb6GNylojCK+k4F
scD153FqUGgKqXh8ljN0qiDFlo/Pv/HD5d/8pv1l4B28krejZkvPen8LiEkj/xb9
16uK3PhfKqwltrBWIgiVYOJRGxLAufCNBQncTCyDv4JV5Dy5OJA1TiVJAXRqcvqW
H/7zZCpwg5aFNa5rZyYrRdex9E/7Agqfon9rCetVEmw0s9d6psS6E8wEjtrNlQGr
77RXWNw+4AgJ9WEj8sDz4+wB7lyjsRW300V/JNa73DC1KTSD75ouRzhYkrHYUEa/
nE1qZs0CgYEA9ZGkvz6hnWW7vEknanT3HuLKw7h1OJflFhQrTsaFXpOrQO7jV0XK
B2gGf0GCSv+drLCr9+Mfg4y2Jh7wEQuyVeen3a11wTDDJliShmfh3iQnx784joG6
bnINZY2J87Dj9uMwjTxElfDAoPjUxyx015BGjahW2RUkwis1jfpdaV0CgYEA7Q6k
/76zaLHabOXNTQcn4/k21kuGxu0mf9vti+OoHkYsO9FROjufyx1Jjz3Qa5Yz9QO8
BY1F/KBZNoMEFPeomYJPrIylsgneAmze7AOxbnPHDaCA1LEd69+uN6tbZF1Qv40G
5I2F7hG4SQ6+sAx8/nH/c/nBUhzECv7f3cuft5UCgYBkkA4dWzKn2D93LaX8jIWe
mlVarTEjyeBAmGXbzqRTRLm+z5U96hB/0/PFLTiEKgWR8I+b5eDD6F23YrgA4v9W
+pTdzOkKAkQIcgEfFW+Dnt7Dh+VLRojoLcCas8moh+ny8rqxO9sCZCMeSIgqQGRg
2m5qGGPoZiY1dahqyfpy6QKBgQDA+3MHX8/eIyuWC14envyycmdZ/RIzT0xQOlIf
1609OBM6fySJK5DiYW1I1yGc9CJIDEo8ms2m40K9RdtE1njCv3rtFXKuhanef5La
wAbpzAb36Pn4LFgXdXj2iOFVy0G5Lq210iB9tp83mnFSEFiRK2yylVfz6McPzH2i
qenUIQKBgQDq7CAGJjeFJUYfUh5/4CkiSNrsRfUv/7wcPnldIQGNlW84l+TuZg3p
TTn9LPqMeLXTb9elcdoI3wLTTUG/JRGP8buY4pN7s0NXcl/TFedQwMKvPF+Opi2d
DIbLDoToGvMrsyRDt7khI3HfyjneNQNbNKjvwL/HpNjZm44LCHXCeg==
-----END RSA PRIVATE KEY-----`

// deterministicSession is a fake keysource.Session whose certificate and
// RSA key are fixed, so signing the same document under the same
// options always produces byte-identical output.
type deterministicSession struct {
	cert *x509.Certificate
	key  *rsa.PrivateKey
}

func newDeterministicSession(t testing.TB) *deterministicSession {
	t.Helper()
	block, _ := pem.Decode([]byte(goldenKeyPEM))
	if block == nil {
		t.Fatal("decoding goldenKeyPEM failed")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("ParsePKCS1PrivateKey: %v", err)
	}
	src := rand.New(rand.NewSource(42))
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Liro Bridge Golden Test Key"},
		NotBefore:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2036, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	der, err := x509.CreateCertificate(src, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return &deterministicSession{cert: cert, key: key}
}

func (s *deterministicSession) SignDigest(_ context.Context, _ keysource.DigestAlgorithm, digest []byte) ([]byte, error) {
	// RSA PKCS#1v1.5 signing is deterministic for a fixed key and
	// digest (F2 §6's own reasoning, D-037) — no randomness needed here
	// at all, unlike key generation above.
	return rsa.SignPKCS1v15(nil, s.key, crypto.SHA256, digest)
}
func (s *deterministicSession) Certificate() keysource.Certificate {
	return keysource.Certificate{Thumbprint: "GOLDEN", DER: s.cert.Raw}
}
func (s *deterministicSession) Chain() [][]byte { return nil }
func (s *deterministicSession) Close() error    { return nil }

func goldenMinimalPDF() []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := map[int]int{}
	write := func(num int, body string) {
		offsets[num] = buf.Len()
		buf.WriteString(itoaGolden(num) + " 0 obj\n" + body + "\nendobj\n")
	}
	write(1, "<< /Type /Catalog /Pages 2 0 R >>")
	write(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	write(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>")
	content := "BT /F1 12 Tf 72 712 Td (Hello) Tj ET"
	offsets[4] = buf.Len()
	buf.WriteString("4 0 obj\n<< /Length " + itoaGolden(len(content)) + " >>\nstream\n" + content + "\nendstream\nendobj\n")
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 5\n0000000000 65535 f \n")
	for i := 1; i <= 4; i++ {
		buf.WriteString(padGolden(offsets[i]) + " 00000 n \n")
	}
	buf.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n" + itoaGolden(xrefStart) + "\n%%EOF")
	return buf.Bytes()
}

func itoaGolden(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func padGolden(n int) string {
	s := itoaGolden(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// TestGoldenFile is F3 §4.5/§16.2's byte-exact test: a fixed key, a
// fixed signing date, no TSA (kept fully offline and deterministic —
// this project's own timestamp tests already prove the TSA path works
// against a real service; this test's job is structural byte-exactness,
// which a live network call cannot provide), compared byte for byte
// against testdata/golden/minimal-signed-bb.pdf.
//
// Regenerate the reference deliberately (never to make a red test pass
// without understanding why) with:
//
//	go test ./internal/pades/... -run TestGoldenFile -update
func TestGoldenFile(t *testing.T) {
	sess := newDeterministicSession(t)
	fixedDate := time.Date(2026, 3, 24, 15, 55, 52, 0, time.FixedZone("CET", 3600))

	result, err := SignDocument(context.Background(), goldenMinimalPDF(), sess, Options{
		ReservedBytes: 4096, // smaller than the 32768 default: no chain, no timestamp, so the CMS is tiny; keeps the golden file small
		Now:           fixedDate,
	})
	if err != nil {
		t.Fatalf("SignDocument: %v", err)
	}

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("creating golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, result.Bytes, 0o644); err != nil {
			t.Fatalf("writing golden file: %v", err)
		}
		t.Logf("wrote golden file: %s (%d bytes)", goldenPath, len(result.Bytes))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden file (run with -update to create it): %v", err)
	}
	if !bytes.Equal(result.Bytes, want) {
		t.Fatalf("output does not match the golden file byte-for-byte (got %d bytes, want %d bytes) — if this change is deliberate, re-run with -update", len(result.Bytes), len(want))
	}
}
