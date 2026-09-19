// Command crlcheck asks MUP's own revocation list whether one certificate is
// on it, asks MUP's OCSP responder whether it is answering at all, and
// compares what it finds against the snapshots of previous runs.
//
// It is a developer tool. Nothing that ships imports it and it touches no
// card: the serial is a public number this project's log already carries
// (D-209, D-271 — 20F048A768F56F099E, the certificate whose SHA-1 is
// B3D1ECCE) and a CRL is a public document. **No reader, no card, no PIN, and
// no attempt is ever spent.**
//
// # Why it exists, and why the run before matters more than the run after
//
// errs.CodeCertRevoked has existed since F1 and nothing in this program can
// produce it (D-310): internal/trust/revocation does not exist and
// classify.computeUsable has no revocation input. So "produce CERT_REVOKED
// against real hardware" is not available. What is available is measuring the
// *data* such a check would depend on — a fact about the issuer rather than
// about this program — and that measurement has a deadline.
//
// The owner's ID card is replaced on 2026-09-22, which cancels the old
// certificate; the old certificate expires on 2026-09-24. So "revoked and not
// yet expired" lasts about 48 hours, MUP publishes one list a day, and at most
// three published CRLs can ever carry that entry. If publication lags more
// than two days it never appears on one at all — which would itself be the
// finding, and is not obtainable any other way.
//
// Finding the serial on a list afterwards would prove nothing on its own: it
// is equally consistent with this tool being right and with it reporting
// "listed" for everything. **The run before is the control**, it expires, and
// it is committed under baseline/ for exactly that reason (D-296's first
// question, asked of a measurement rather than of a test).
//
// # Running it
//
//	go run ./scripts/crlcheck            # from the repository root
//	go run ./scripts/crlcheck -skip-ocsp # the CRL only; ~10s instead of ~30s
//
// The OCSP probe costs two 10-second timeouts because that is the measurement:
// D-076 established that no TCP handshake completes, and this asks again
// rather than quoting a seventeen-day-old result.
package main

import (
	"context"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"time"
)

// Defaults are SPEC §11.9's, which took them from a real MUP certificate's own
// AIA and CRL distribution point rather than from any vendor document. The
// ca.mup.gov.rs address redirects to crl.mup.gov.rs; both are printed so a
// later change of either is visible rather than silent.
const (
	defaultCRLURL   = "http://ca.mup.gov.rs/MUPGradjaniCA4.crl"
	defaultOCSPHost = "ocsp.mup.gov.rs"
	defaultSerial   = "20F048A768F56F099E"
	defaultDir      = "scripts/crlcheck/baseline"
)

// Exit codes are distinct because the difference between them is the whole
// point: a tool that cannot tell "not revoked" from "could not tell" is the
// kind of instrument D-296 and D-304 are about.
const (
	exitNotRevoked  = 0
	exitCannotTell  = 1
	exitRevoked     = 2
	exitControlFail = 3
)

func main() {
	var (
		crlURL   = flag.String("url", defaultCRLURL, "CRL distribution point to fetch")
		ocspHost = flag.String("ocsp", defaultOCSPHost, "OCSP host to probe for reachability")
		serial   = flag.String("serial", defaultSerial, "certificate serial number, hexadecimal")
		dir      = flag.String("dir", defaultDir, "directory of snapshots to compare against and write into")
		skipOCSP = flag.Bool("skip-ocsp", false, "do not probe the OCSP responder (saves two 10s timeouts)")
	)
	flag.Parse()

	fmt.Printf("crlcheck  %s\n", time.Now().Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  serial   %s\n", *serial)
	fmt.Println()

	want, ok := new(big.Int).SetString(*serial, 16)
	if !ok {
		fmt.Printf("  the serial %q is not hexadecimal; nothing below would mean anything\n", *serial)
		os.Exit(exitControlFail)
	}

	if !*skipOCSP {
		probeOCSP(*ocspHost)
	}

	list, body, err := fetchCRL(*crlURL)
	if err != nil {
		fmt.Printf("  CRL      %v\n", err)
		os.Exit(exitCannotTell)
	}
	describe(list, body)

	// The control for the search itself. Without it, "not found" is equally
	// consistent with the certificate being live and with the comparison being
	// broken — and reporting an absence is exactly the direction in which an
	// instrument's own failure is invisible (D-296).
	if n := len(list.RevokedCertificateEntries); n > 0 {
		probe := list.RevokedCertificateEntries[n/2].SerialNumber
		if find(list, probe) == nil {
			fmt.Println("  CONTROL FAILED: the search cannot find a serial taken out of the")
			fmt.Println("                  list itself. Nothing below would mean anything.")
			os.Exit(exitControlFail)
		}
		fmt.Printf("  control  ok: a serial taken out of the middle of the list is found\n")
		fmt.Printf("           by the same search that reports our serial's absence\n\n")
	} else {
		fmt.Println("  CONTROL  the list is empty, so the search was never exercised.")
		fmt.Println("           A \"not revoked\" below would mean nothing.")
		fmt.Println()
		os.Exit(exitControlFail)
	}

	// Compare before writing, so this run is measured against the previous one
	// rather than against itself.
	compare(*dir, list)

	entry := find(list, want)
	if err := write(*dir, list, *serial, entry, len(body)); err != nil {
		// Not fatal to the reading, but fatal to the next run's control, so it
		// is said loudly rather than logged quietly.
		fmt.Printf("  !! THE SNAPSHOT WAS NOT WRITTEN: %v\n", err)
		fmt.Printf("  !! this run is still valid; the next one has no baseline to use.\n\n")
	}

	if entry != nil {
		fmt.Printf("  RESULT   *** THIS CERTIFICATE IS ON THE REVOCATION LIST ***\n")
		fmt.Printf("           revoked at  %s\n", entry.RevocationTime.UTC().Format(time.RFC3339))
		if entry.ReasonCode != 0 {
			fmt.Printf("           reason code %d\n", entry.ReasonCode)
		}
		fmt.Printf("\n           Note what this is and is not: it is a fact about the issuer's\n")
		fmt.Printf("           published list. This program still cannot produce CERT_REVOKED\n")
		fmt.Printf("           (D-310) and nothing about that has changed.\n")
		os.Exit(exitRevoked)
	}
	fmt.Printf("  RESULT   not on the list. As of thisUpdate %s this\n", list.ThisUpdate.UTC().Format(time.RFC3339))
	fmt.Printf("           certificate is not revoked, according to this list.\n")
	os.Exit(exitNotRevoked)
}

// probeOCSP asks whether the responder accepts a TCP connection at all. It
// deliberately does not send an OCSP request: D-076 established that no
// handshake completes, so the question is reachability and the answer needs no
// protocol. Both ports are tried because D-076 tried both.
func probeOCSP(host string) {
	for _, port := range []string{"80", "443"} {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 10*time.Second)
		if err != nil {
			fmt.Printf("  OCSP %s:%-3s  no connection after %v: %v\n",
				host, port, time.Since(start).Round(time.Millisecond), err)
			continue
		}
		fmt.Printf("  OCSP %s:%-3s  CONNECTED in %v  <- this is new; D-076 said it does not\n",
			host, port, time.Since(start).Round(time.Millisecond))
		_ = conn.Close()
	}
	fmt.Println()
}

// fetchCRL downloads and parses the list. The five-minute budget is this
// tool's own and is not the agent's: internal/pades/dss uses crlTimeout of 30s
// and meets it comfortably (measured, D-310 §6). A tool that is measuring
// should not fail for a reason the thing it measures would not.
func fetchCRL(url string) (*x509.RevocationList, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("could not build the request: %w", err)
	}
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("could not be fetched: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("could not be read: %w", err)
	}
	fmt.Printf("  CRL      %s\n", url)
	fmt.Printf("           HTTP %d, %d bytes in %v\n", resp.StatusCode, len(body), time.Since(start).Round(time.Millisecond))
	if final := resp.Request.URL.String(); final != url {
		fmt.Printf("           served from %s\n", final)
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		// Worth printing beside thisUpdate: the gap between them is how long
		// the issuer takes to publish a list it has already signed, which is
		// the lower bound on any revocation's visible latency.
		fmt.Printf("           Last-Modified %s\n", lm)
	}
	list, err := x509.ParseRevocationList(body)
	if err != nil {
		return nil, nil, fmt.Errorf("does not parse as a revocation list: %w", err)
	}
	return list, body, nil
}

func describe(l *x509.RevocationList, body []byte) {
	fmt.Printf("           issuer      %s\n", l.Issuer.CommonName)
	fmt.Printf("           thisUpdate  %s\n", l.ThisUpdate.UTC().Format(time.RFC3339))
	fmt.Printf("           nextUpdate  %s  (in %v)\n",
		l.NextUpdate.UTC().Format(time.RFC3339), time.Until(l.NextUpdate).Round(time.Minute))
	fmt.Printf("           entries     %d\n", len(l.RevokedCertificateEntries))
	if time.Now().After(l.NextUpdate) {
		fmt.Printf("           !! this list is past its own nextUpdate: it is stale, and a\n")
		fmt.Printf("           !! certificate revoked since could not appear on it.\n")
	}
	_ = body
	fmt.Println()
}

func find(l *x509.RevocationList, want *big.Int) *x509.RevocationListEntry {
	for i := range l.RevokedCertificateEntries {
		if l.RevokedCertificateEntries[i].SerialNumber.Cmp(want) == 0 {
			return &l.RevokedCertificateEntries[i]
		}
	}
	return nil
}
