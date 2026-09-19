// Command cngprobe lists what the Windows certificate store shows this
// program through CNG, and nothing else. It is p11probe's counterpart: one
// reads the card through a PKCS#11 module, this reads it through Windows.
//
// It exists because F11 §4 step 3 — one certificate to one row across two
// backends — cannot be planned without knowing whether both backends see the
// same card at all. That is a question about this machine, not about the code,
// so it is measured rather than assumed.
//
// It has already answered it once (D-310): run against a Pošta card, this
// printed AF5063BB74378BD503AB46DD08AEAD205BA2AA54, which is byte-for-byte
// what aetpkss1.dll reported through the worker half an hour earlier. The
// identity `dedupe` asserts in a comment is therefore measured rather than
// argued, and this is the tool that measures it again on the next card.
//
// It opens no session, asks for no PIN, and writes nothing anywhere.
package main

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"github.com/veljaos/liro-bridge/internal/keysource/windowscng"
)

func main() {
	fmt.Printf("cngprobe  %s\n\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	start := time.Now()
	certs, err := windowscng.Enumerate(context.Background())
	took := time.Since(start)
	if err != nil {
		fmt.Printf("Enumerate failed after %v: %v\n", took.Round(time.Millisecond), err)
		os.Exit(1)
	}
	fmt.Printf("%d certificate(s) in %v\n\n", len(certs), took.Round(time.Millisecond))
	for i, c := range certs {
		fmt.Printf("[%d] %s\n", i, c.Thumbprint)
		fmt.Printf("     provider    %q\n", c.Provider)
		fmt.Printf("     onHardware  %v\n", c.OnHardware)
		fmt.Printf("     container   %q\n", c.KeyContainer)
		// Parsing here rather than in windowscng is deliberate: that package
		// keeps X.509 logic out by design (F1 §3.1), and a probe is not a
		// reason to weaken it.
		parsed, perr := x509.ParseCertificate(c.DER)
		if perr != nil {
			fmt.Printf("     (%d bytes, did not parse: %v)\n\n", len(c.DER), perr)
			continue
		}
		fmt.Printf("     subject     %s\n", parsed.Subject)
		fmt.Printf("     issuer      %s\n", parsed.Issuer.CommonName)
		fmt.Printf("     serial      %X\n", parsed.SerialNumber)
		fmt.Printf("     valid       %s .. %s\n",
			parsed.NotBefore.Format("2006-01-02"), parsed.NotAfter.Format("2006-01-02"))
		fmt.Printf("     keyUsage    %d\n\n", parsed.KeyUsage)
	}
}
