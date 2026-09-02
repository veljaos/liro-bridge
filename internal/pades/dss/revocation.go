// Package dss embeds revocation evidence into a signed document's
// /DSS dictionary, producing PAdES B-LT from B-T (F3 §7).
package dss

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/crypto/ocsp"
)

// F3 §7.2's explicit numbers.
const (
	ocspTimeout  = 10 * time.Second
	ocspAttempts = 2
	crlTimeout   = 30 * time.Second // CRLs are large
	crlAttempts  = 1

	maxOCSPResponseSize = 64 * 1024
	maxCRLSize          = 64 * 1024 * 1024
)

// Entry is one certificate's collected revocation evidence: an OCSP
// response if one could be obtained, else a CRL. Both nil means
// collection failed for this certificate entirely.
type Entry struct {
	Certificate  *x509.Certificate
	OCSPResponse []byte // DER
	CRL          []byte // DER
}

// CollectRevocation fetches revocation evidence for every certificate
// in certs (F3 §7.2: read AIA from the certificate being checked, not
// its issuer — MUP's OCSP URL appears only on the end-entity
// certificate). certs must be ordered signer-first with each
// certificate's issuer at the next index (as
// internal/pades/cms.CompleteChain returns); the last certificate's
// issuer is therefore unknown here — it was excluded as the root — so
// it is skipped rather than guessed at.
//
// A certificate with no OCSP responder and no CRL distribution point,
// or one whose endpoints all fail, is not an error: it simply
// contributes no Entry, and the caller (F3 §7.3) reports the resulting
// level as B-T rather than claiming B-LT for a document some of whose
// certificates have no embedded revocation evidence.
func CollectRevocation(ctx context.Context, certs []*x509.Certificate) []Entry {
	out := make([]Entry, len(certs))
	for i, cert := range certs {
		out[i] = Entry{Certificate: cert}
		if i+1 >= len(certs) {
			continue // no issuer available (it was the excluded root): leave empty
		}
		issuer := certs[i+1]
		if resp := fetchOCSP(ctx, cert, issuer); resp != nil {
			out[i].OCSPResponse = resp
		} else if crl := fetchCRL(ctx, cert); crl != nil {
			out[i].CRL = crl
		}
	}
	return out
}

// fetchOCSP tries every OCSP responder URL the certificate's AIA
// extension names, up to ocspAttempts times each, with a ocspTimeout
// per attempt.
func fetchOCSP(ctx context.Context, cert, issuer *x509.Certificate) []byte {
	if len(cert.OCSPServer) == 0 {
		return nil
	}
	reqDER, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return nil
	}
	for _, url := range cert.OCSPServer {
		for attempt := 0; attempt < ocspAttempts; attempt++ {
			body, err := postWithTimeout(ctx, url, "application/ocsp-request", reqDER, ocspTimeout, maxOCSPResponseSize)
			if err != nil {
				continue
			}
			if _, err := ocsp.ParseResponseForCert(body, cert, issuer); err != nil {
				continue // malformed or not-for-this-cert: try again/next
			}
			return body
		}
	}
	return nil
}

// fetchCRL tries every CRL distribution point the certificate names,
// once each, with crlTimeout.
func fetchCRL(ctx context.Context, cert *x509.Certificate) []byte {
	for _, url := range cert.CRLDistributionPoints {
		body, err := getWithTimeout(ctx, url, crlTimeout, maxCRLSize)
		if err != nil {
			continue
		}
		if _, err := x509.ParseRevocationList(body); err != nil {
			continue // not a parseable CRL: do not embed it
		}
		return body
	}
	return nil
}

func postWithTimeout(ctx context.Context, url, contentType string, body []byte, timeout time.Duration, maxResp int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return doAndRead(req, maxResp)
}

func getWithTimeout(ctx context.Context, url string, timeout time.Duration, maxResp int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return doAndRead(req, maxResp)
}

func doAndRead(req *http.Request, maxResp int64) ([]byte, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dss: %s: HTTP %d", req.URL, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxResp))
}
