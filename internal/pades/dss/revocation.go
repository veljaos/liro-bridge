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

// DefaultMaxArtefactSize is the largest single OCSP response or CRL this
// package will embed into a document's /DSS dictionary (Task 1b). Zero or
// negative passed to CollectRevocation means "use this".
//
// Measured directly, against a real MUP Gradjani CA 4 certificate: MUP's
// OCSP responder (http://ocsp.mup.gov.rs/MUPGradjaniCAocsp, from the
// certificate's own AIA per SPEC §11.9) does not answer at all — TCP
// connections to ocsp.mup.gov.rs on both port 80 and 443 time out after
// 10-20s, repeatedly, while ca.mup.gov.rs (the CRL host on the same
// domain) accepts a connection immediately. This is the responder being
// unreachable at the network level, not a bug in this project's OCSP
// client — see docs/decisions.md. The code therefore falls back to
// MUPGradjaniCA4.crl: the revocation list for every Serbian identity
// card, which measured exactly 30,136,214 bytes. Embedding that CRL
// produced a 30,813,989-byte PDF that Adobe Acrobat could not open at
// all; the same document signed at B-T (no /DSS) was 673,111 bytes and
// opened normally. 5 MB is comfortably above any OCSP response or any
// CRL scoped to something short of "every card the state has issued",
// and comfortably below the point where embedding one makes a document
// unusable.
const DefaultMaxArtefactSize = 5 * 1024 * 1024

// Entry is one certificate's collected revocation evidence: an OCSP
// response if one could be obtained, else a CRL. All three of
// OCSPResponse, CRL and TooLarge false/empty means collection failed for
// this certificate entirely (no responder answered, no CRL could be
// fetched) — a caller-visible different situation from TooLarge, which
// means evidence *was* obtained but exceeded the size cap and was
// deliberately not embedded (Task 1b/1c).
type Entry struct {
	Certificate  *x509.Certificate
	OCSPResponse []byte // DER
	CRL          []byte // DER

	// TooLarge is true when a CRL or OCSP response was actually fetched
	// and validated but exceeded the configured size cap, so it was
	// discarded rather than embedded. SkippedBytes is that artefact's
	// size — needed by a caller reporting why B-LT was not reached
	// (Task 1c: "Revocation data was too large to embed (32 MB)").
	TooLarge     bool
	SkippedBytes int64
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
// maxArtefactSize caps how large a single OCSP response or CRL may be
// before this function refuses to hand it back for embedding (Task 1b).
// Zero or negative means DefaultMaxArtefactSize.
//
// mem, when non-nil, is one batch's memory of endpoints that did not
// answer (see EndpointMemory). An endpoint already recorded there is not
// contacted again; one that produces no usable answer here is recorded
// before this function returns. Nil means every endpoint is tried, which
// is what a single-document signature wants.
//
// A certificate with no OCSP responder and no CRL distribution point,
// or one whose endpoints all fail, is not an error: it simply
// contributes no Entry, and the caller (F3 §7.3) reports the resulting
// level as B-T rather than claiming B-LT for a document some of whose
// certificates have no embedded revocation evidence. The same is true,
// with a more specific reason, when evidence exists but is too large
// (Entry.TooLarge).
func CollectRevocation(ctx context.Context, certs []*x509.Certificate, maxArtefactSize int64, mem *EndpointMemory) []Entry {
	if maxArtefactSize <= 0 {
		maxArtefactSize = DefaultMaxArtefactSize
	}
	out := make([]Entry, len(certs))
	for i, cert := range certs {
		out[i] = Entry{Certificate: cert}
		if i+1 >= len(certs) {
			continue // no issuer available (it was the excluded root): leave empty
		}
		issuer := certs[i+1]

		if resp, skipped := fetchOCSP(ctx, cert, issuer, maxArtefactSize, mem); resp != nil {
			out[i].OCSPResponse = resp
			continue
		} else if skipped > 0 {
			// An oversized OCSP response is not expected in practice —
			// real responses are a few KB — but is handled the same way
			// as an oversized CRL for symmetry. This project does not
			// additionally try a CRL after an oversized OCSP response:
			// a responder returning megabytes of OCSP data is anomalous
			// enough on its own, and trying every remaining fallback
			// after every possible oversized artefact multiplies this
			// function's cases for a scenario that has never been
			// measured to occur.
			out[i].TooLarge = true
			out[i].SkippedBytes = skipped
			continue
		}

		if crl, skipped := fetchCRL(ctx, cert, maxArtefactSize, mem); crl != nil {
			out[i].CRL = crl
		} else if skipped > 0 {
			out[i].TooLarge = true
			out[i].SkippedBytes = skipped
		}
	}
	return out
}

// fetchOCSP tries every OCSP responder URL the certificate's AIA
// extension names, up to ocspAttempts times each, with a ocspTimeout
// per attempt. skipped is non-zero only when a response was obtained and
// validated but exceeded maxArtefactSize (Task 1b) — in which case resp
// is nil and the certificate's OCSP evidence is not embedded.
//
// A URL mem already knows did not answer is skipped without a request,
// and a URL that produces no usable answer after every attempt is
// recorded in mem before moving on: within one batch, that turns 20 s
// per document into 20 s once.
func fetchOCSP(ctx context.Context, cert, issuer *x509.Certificate, maxArtefactSize int64, mem *EndpointMemory) (resp []byte, skipped int64) {
	if len(cert.OCSPServer) == 0 {
		return nil, 0
	}
	reqDER, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return nil, 0
	}
	for _, url := range cert.OCSPServer {
		if mem.silent(url) {
			continue
		}
		answered := false
		for attempt := 0; attempt < ocspAttempts; attempt++ {
			body, err := postWithTimeout(ctx, url, "application/ocsp-request", reqDER, ocspTimeout, maxOCSPResponseSize)
			if err != nil {
				continue
			}
			// The endpoint answered. Whether the answer is usable is a
			// separate question — a malformed response is not a reason
			// to stop asking a responder that is plainly up, and it is
			// not the twenty-second cost this memory exists to remove.
			answered = true
			if _, err := ocsp.ParseResponseForCert(body, cert, issuer); err != nil {
				continue // malformed or not-for-this-cert: try again/next
			}
			if int64(len(body)) > maxArtefactSize {
				return nil, int64(len(body))
			}
			return body, 0
		}
		if !answered {
			mem.remember(url)
		}
	}
	return nil, 0
}

// fetchCRL tries every CRL distribution point the certificate names,
// once each, with crlTimeout. skipped is non-zero only when a CRL was
// downloaded and parsed successfully but exceeded maxArtefactSize (Task
// 1b: MUP Gradjani CA 4's CRL measured 30,136,214 bytes) — in which case
// crl is nil and the certificate's CRL evidence is not embedded.
//
// A distribution point that did not answer is remembered for the rest of
// the batch, exactly as an OCSP responder is: crlTimeout is 30 s, so the
// cost of asking a dead host again is larger here, not smaller.
func fetchCRL(ctx context.Context, cert *x509.Certificate, maxArtefactSize int64, mem *EndpointMemory) (crl []byte, skipped int64) {
	for _, url := range cert.CRLDistributionPoints {
		if mem.silent(url) {
			continue
		}
		body, err := getWithTimeout(ctx, url, crlTimeout, maxCRLSize)
		if err != nil {
			mem.remember(url)
			continue
		}
		if _, err := x509.ParseRevocationList(body); err != nil {
			continue // not a parseable CRL: do not embed it
		}
		if int64(len(body)) > maxArtefactSize {
			return nil, int64(len(body))
		}
		return body, 0
	}
	return nil, 0
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
