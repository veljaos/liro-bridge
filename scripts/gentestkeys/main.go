// Command gentestkeys generates the soft token's test key material (F2
// §3.3): a self-signed RSA-2048 certificate with contentCommitment set
// in its KeyUsage — so internal/trust/classify treats it as a signing
// certificate, exercising the whole path exactly as a real card's
// certificate would — and its PKCS#12 encoding.
//
// The private key is never committed, even as a test key: SPEC §16.6/F2
// §3.3 requires the output directory to be gitignored, and a key that
// starts as "just for testing" has a way of ending up copied into
// something real eventually.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"software.sslmate.com/src/go-pkcs12"
)

// defaultPassword protects only a gitignored local test file, never a
// production secret — fixed and documented, the same posture as SPEC
// §12.7's published test-TSA credentials.
const defaultPassword = "liro-softtoken-test"

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: gentestkeys <output-dir> [password]")
		os.Exit(2)
	}
	outDir := os.Args[1]
	password := defaultPassword
	if len(os.Args) == 3 {
		password = os.Args[2]
	}
	must(os.MkdirAll(outDir, 0o755))

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)

	tmpl := &x509.Certificate{
		SerialNumber: randSerial(),
		Subject:      pkix.Name{CommonName: "Liro Bridge Soft Token (TEST - never a real signature)"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		// contentCommitment, and nothing else: F2 §3.3 requires this so
		// classify.purposeFromKeyUsage (SPEC §11.4/D-015) treats the
		// soft token exactly like a real signing certificate, never
		// like Halcom's authentication certificate.
		KeyUsage:              x509.KeyUsageContentCommitment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	must(err)
	cert, err := x509.ParseCertificate(der)
	must(err)

	p12, err := pkcs12.Modern.Encode(key, cert, nil, password)
	must(err)

	p12Path := filepath.Join(outDir, "test.p12")
	must(os.WriteFile(p12Path, p12, 0o600))

	// A plain PEM certificate alongside, for quick local use with
	// openssl without loading the PKCS#12 (the OpenSSL recipe in the
	// README uses "certs --json" instead, but this is a fast sanity
	// check while developing).
	certPath := filepath.Join(outDir, "test-cert.pem")
	must(os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644))

	fmt.Printf("wrote %s\n", p12Path)
	fmt.Printf("wrote %s\n", certPath)
	fmt.Println()
	fmt.Println("Set these before running liro-bridge sign-digest against the soft token:")
	fmt.Printf("  LIRO_SOFTTOKEN_P12=%s\n", p12Path)
	fmt.Printf("  LIRO_SOFTTOKEN_PASSWORD=%s\n", password)
}

func randSerial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	must(err)
	return n
}
