// Command verifypdf runs this project's independent verifier over a
// signed PDF and prints what it found (SPEC §16.4, F3 §8).
//
// It is the verifier the specification requires: a second, from-scratch
// implementation that shares no code with the signing path — its own
// byte-level /ByteRange scanner, its own BER/DER reader, its own copy of
// RFC 5652 §5.4's re-tagging step. A bug in a shared helper would pass
// both ways, which is the whole reason it exists (D-044).
//
// It existed only inside tests until now, which meant that verifying a
// document somebody actually signed — on a real card, from an installed
// binary — meant writing a throwaway program to do it. This is that
// program, kept.
//
//	go run ./scripts/verifypdf <file.pdf>
//
// Exit status is 1 if any signature in the document fails any check, so
// it can be used as a check rather than only read.
package main

import (
	"context"
	"crypto/sha1" //nolint:gosec // certificate thumbprints, not a security claim
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/pades/verify"
	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: verifypdf <file.pdf>")
		os.Exit(2)
	}
	pdfBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "verifypdf:", err)
		os.Exit(1)
	}

	slots, err := verify.FindSignatures(pdfBytes)
	if err != nil {
		fmt.Fprintln(os.Stderr, "verifypdf:", err)
		os.Exit(1)
	}
	fmt.Printf("%s\n%d bytes, %d signature slot(s)\n\n", os.Args[1], len(pdfBytes), len(slots))

	// The Trusted List, so that SignerChainTrusted is an answer rather
	// than a default. Result's own documentation is explicit that the
	// field is "false, not merely unset, when CheckChainTrust was never
	// called" — so a verifier that never calls it reports every
	// signature in the country as untrusted, which is worse than saying
	// nothing. SPEC §11.1 makes the Trusted List the only thing that
	// decides qualification, and this is where it gets consulted.
	list := loadTrustedList()

	allOK := true
	for i, slot := range slots {
		r := verify.VerifySignature(pdfBytes, slot)
		if list != nil {
			at := time.Now()
			if r.HasTimestamp && r.TimestampGenTime != "" {
				// PAdES takes the time from the timestamp, never from a
				// clock here (SPEC §12.3).
				if t, err := time.Parse(time.RFC3339, r.TimestampGenTime); err == nil {
					at = t
				}
			}
			verify.CheckChainTrust(r, list, at)
		}
		fmt.Printf("signature %d\n", i+1)
		fmt.Printf("  /ByteRange                 %v\n", r.ByteRange)
		fmt.Printf("  ByteRangeDigestOK          %t   (SHA-256 recomputed over the signed span, against messageDigest)\n", r.ByteRangeDigestOK)
		fmt.Printf("  SignatureOK                %t   (RSA over the signed attributes re-tagged as SET OF)\n", r.SignatureOK)
		fmt.Printf("  SigningCertificateOK       %t   (signingCertificateV2 certHash against the signer used)\n", r.SigningCertificateOK)
		fmt.Printf("  SignerChainTrusted         %t\n", r.SignerChainTrusted)
		printCert(os.Stdout, "  signer", r.SignerCertificate, false)
		fmt.Printf("  HasTimestamp               %t\n", r.HasTimestamp)
		if r.HasTimestamp {
			fmt.Printf("  TimestampOK                %t   (token parses; messageImprint against the RSA signature)\n", r.TimestampOK)
			fmt.Printf("  TimestampGenTime           %s\n", r.TimestampGenTime)
			if r.TimestampSerial != nil {
				fmt.Printf("  TimestampSerial            %x\n", r.TimestampSerial)
			}
			fmt.Printf("  TimestampChainTrusted      %t\n", r.TimestampChainTrusted)
			// A timestamp unit is a machine, so its subject names nobody.
			printCert(os.Stdout, "  timestamp", r.TimestampCertificate, true)
		}
		if len(r.Errors) == 0 {
			fmt.Printf("  Errors                     none\n")
		}
		for _, e := range r.Errors {
			fmt.Printf("  ERROR                      %s\n", e)
		}
		// A document timestamp slot (/SubFilter /ETSI.RFC3161) carries
		// its own messageDigest over the embedded TSTInfo rather than
		// over this document's /ByteRange, so its ByteRangeDigestOK is
		// expected false — the same fact internal/pades/verify's own
		// real-fixture test records.
		if !r.ByteRangeDigestOK || !r.SignatureOK || !r.SigningCertificateOK {
			allOK = false
		}
		fmt.Println()
	}
	if !allOK {
		fmt.Println("at least one signature did not pass every check")
		os.Exit(1)
	}
	fmt.Println("every signature passed every check this verifier makes")
}

// loadTrustedList reads the agent's own cached Trusted List, falling
// back to the seed embedded in the binary. It never fetches: this is a
// verifier, and a check that quietly reaches the network is a check
// that behaves differently on a machine that has none.
func loadTrustedList() *tsl.List {
	cache := filepath.Join(platform.ConfigDir(runtime.GOOS, platform.OSEnv), "tsl-cache.xml")
	store, err := tsl.NewFileStore(cache, tsl.DefaultURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verifypdf: no Trusted List available (%v); chain trust not evaluated\n", err)
		return nil
	}
	list, prov, err := store.Current(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "verifypdf: no Trusted List available (%v); chain trust not evaluated\n", err)
		return nil
	}
	fmt.Printf("Trusted List: sequence %d, issued %s, from %s\n\n",
		prov.Sequence, prov.IssuedAt.Format("2006-01-02"), prov.Source)
	return list
}

// subjectWithheld is what stands where a person's name would be, and it
// says why rather than leaving a blank somebody will read as "empty".
const subjectWithheld = "(withheld: on an identity card this carries the holder's name and their JMBG)"

// printCert names a certificate. subjectIsNotAPerson is true only where
// the subject is a machine — a timestamp unit, a CA — and false for any
// certificate that might belong to somebody.
//
// **The signer's subject is never printed** (D59). On a Serbian identity
// card its common name carries the holder's name *and their national
// identity number*, and this program writes to standard output, which is
// where terminals, transcripts, CI logs and anything pasted into an issue
// come from. D-480 took certificate names out of every message the agent
// writes; this verifier was not changed with it, and D-487 is what that
// omission cost once a third party's number had reached a public
// repository — which is the same way this one was found (D-506).
//
// Nothing is lost for the purpose SPEC §16.4 gives this tool. It answers
// whether the cryptography holds, and the thumbprint below identifies the
// certificate exactly — it is the same value the audit log and the
// agent's own warnings use, so a signature can still be tied to a
// certificate, a log line and an audit entry without naming a person.
// Whoever needs the subject itself can read it out of the PDF with a
// tool whose output they have chosen to look at.
func printCert(w io.Writer, label string, c *x509.Certificate, subjectIsNotAPerson bool) {
	if c == nil {
		fmt.Fprintf(w, "%s certificate         (none)\n", label)
		return
	}
	if subjectIsNotAPerson {
		// subject-ok: a timestamp unit is a machine, and its name is the
		// one thing that says which authority stamped the document.
		fmt.Fprintf(w, "%s subject            %s\n", label, c.Subject.CommonName)
	} else {
		fmt.Fprintf(w, "%s subject            %s\n", label, subjectWithheld)
	}
	fmt.Fprintf(w, "%s thumbprint         %s\n", label, thumbprintOf(c))
	fmt.Fprintf(w, "%s issuer             %s\n", label, c.Issuer.CommonName)
	fmt.Fprintf(w, "%s serial             %X\n", label, c.SerialNumber)
	fmt.Fprintf(w, "%s validity           %s .. %s\n", label,
		c.NotBefore.Format("2006-01-02"), c.NotAfter.Format("2006-01-02"))
}

// thumbprintOf is the SHA-1 of the certificate's bytes, uppercase hex —
// the form the audit log, Windows' certificate store and the agent's own
// warnings all name a certificate by.
//
// Written out here rather than taken from internal/pades/dss on purpose:
// this command is the second implementation SPEC §16.4 requires, and it
// shares no code with the signing path (D-044). Three lines of duplicate
// hashing is the price of that, and it is the right price.
func thumbprintOf(c *x509.Certificate) string {
	sum := sha1.Sum(c.Raw) //nolint:gosec // an identifier, not a security claim
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
