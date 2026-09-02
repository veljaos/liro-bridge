package cms

import (
	"bytes"
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// AIAFetcher fetches the bytes at an AIA caIssuers URL. HTTPAIAFetcher
// is the real implementation; tests substitute a fake to avoid network
// access.
type AIAFetcher func(ctx context.Context, url string) ([]byte, error)

// aiaTimeout is F3 §5.4's explicit number: AIA fetching is best-effort,
// not required for a signature to succeed, so it gets one attempt and a
// short timeout rather than the retry policy a required network call
// (like the TSA) gets.
const aiaTimeout = 10 * time.Second

// aiaMaxResponseSize bounds an AIA response read: certificates are a
// few KB; anything wildly larger is not a certificate.
const aiaMaxResponseSize = 1 << 20

// HTTPAIAFetcher fetches url with a single attempt and a 10-second
// timeout (F3 §5.4).
func HTTPAIAFetcher(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, aiaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("cms: building AIA request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cms: fetching AIA %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cms: AIA %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, aiaMaxResponseSize))
}

// CompleteChain implements F3 §5.4: when a signing session supplies no
// chain, complete it from the trust store first, then from each
// certificate's own AIA caIssuers, stopping before the root (F3 §5.4:
// "include the signer and every issuer up to but not including the
// root, when available"). trustStore is the caller-supplied set of
// candidate issuer certificates — in production, the CA/QC service
// certificates from the F1 Trusted List; this package has no dependency
// on internal/trust itself, keeping chain completion testable with a
// plain slice.
//
// Fetched AIA content is validated by attempting to parse it as a
// certificate before use (F3 §5.4/§11.8's Halcom AIA defect: its
// caIssuers points at a .crl file where RFC 5280 requires a
// certificate). A failure to complete the chain — no match in the trust
// store, AIA unreachable, or AIA content that is not a certificate — is
// logged and the chain built so far is returned rather than treated as
// an error: a missing chain degrades the signature level (F3 §7.3), it
// never fails the signature outright.
//
// Note on MUP: SPEC §11.8 states MUP-signed documents embed only the
// signer certificate, so a fresh MUP signature needs completion here.
// Whether MUP's Trusted List CA/QC service certificate is itself
// self-signed (a root, correctly excluded) or an intermediate under a
// separate, unlisted root could not be verified in this environment —
// see [[D-038]] — so this function implements the literal rule stated
// above for every issuer, MUP included, rather than special-casing it.
func CompleteChain(ctx context.Context, cert *x509.Certificate, trustStore []*x509.Certificate, fetch AIAFetcher) []*x509.Certificate {
	var chain []*x509.Certificate
	seen := map[string]bool{string(cert.Raw): true}
	current := cert

	for depth := 0; depth < 5; depth++ {
		if isSelfSigned(current) {
			break
		}
		issuer := findIssuerInStore(current, trustStore)
		if issuer == nil && fetch != nil {
			issuer = fetchIssuerViaAIA(ctx, current, fetch)
		}
		if issuer == nil {
			break
		}
		if seen[string(issuer.Raw)] {
			break // cycle guard
		}
		if isSelfSigned(issuer) {
			break // the root: excluded per F3 §5.4
		}
		chain = append(chain, issuer)
		seen[string(issuer.Raw)] = true
		current = issuer
	}
	return chain
}

func isSelfSigned(c *x509.Certificate) bool {
	return bytes.Equal(c.RawIssuer, c.RawSubject)
}

func findIssuerInStore(cert *x509.Certificate, store []*x509.Certificate) *x509.Certificate {
	for _, c := range store {
		if bytes.Equal(c.RawSubject, cert.RawIssuer) {
			return c
		}
	}
	return nil
}

func fetchIssuerViaAIA(ctx context.Context, cert *x509.Certificate, fetch AIAFetcher) *x509.Certificate {
	for _, url := range cert.IssuingCertificateURL {
		data, err := fetch(ctx, url)
		if err != nil {
			slog.Warn("pades/cms: AIA caIssuers fetch failed", "url", url, "error", err)
			continue
		}
		parsed, err := x509.ParseCertificate(data)
		if err != nil {
			// F3 §11.8's Halcom AIA defect: caIssuers points at a .crl
			// file where RFC 5280 requires a certificate. Verifying the
			// content actually parses as one — rather than trusting the
			// AIA field's stated purpose — is what catches this.
			slog.Warn("pades/cms: AIA caIssuers did not return a parseable certificate", "url", url, "error", err)
			continue
		}
		return parsed
	}
	return nil
}
