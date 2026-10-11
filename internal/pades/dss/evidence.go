package dss

// One certificate's revocation evidence, read rather than merely fetched
// ([[D-462]]). Until session 33 an OCSP response was embedded whatever it
// said and a CRL whatever it listed: a revoked certificate's "revoked"
// went into /DSS beside a signature that then claimed B-LT, and the
// parsed CRL was thrown away (D4, D-310). The rules below are written
// after reading how DSS 6.5 reaches PAdES BASELINE-LT (its
// PAdESLevelBaselineLT and the revocation sources under it); DSS is Java
// and LGPL, so nothing of it is used, and where this file does otherwise
// it says why.

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/asn1"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/ocsp"
)

// oidOCSPNoCheck is id-pkix-ocsp-nocheck (RFC 6960 §4.2.2.2.1): a
// delegated responder certificate that carries it is not itself checked
// for revocation — the CA has said a client need not, and asking the
// responder about its own key would be circular.
var oidOCSPNoCheck = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 48, 1, 5}

// fetchable reports whether this package may dial raw: http and https
// only. SPEC §6.8 enumerates this program's outbound connections and an
// OCSP or CRL fetch is one of them over HTTP; DSS also reads ldap://,
// ftp:// and file:// distribution points, and an LDAP reader here would
// be a new kind of connection, which is the owner's decision (D-281's
// option (b)), not this file's.
func fetchable(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	s := strings.ToLower(u.Scheme)
	return s == "http" || s == "https"
}

// hasOCSPNoCheck reports whether c carries id-pkix-ocsp-nocheck.
func hasOCSPNoCheck(c *x509.Certificate) bool {
	for _, e := range c.Extensions {
		if e.Id.Equal(oidOCSPNoCheck) {
			return true
		}
	}
	return false
}

// evidence is what collectOne found for one certificate.
type evidence struct {
	OCSPResponse []byte
	CRL          []byte

	// Revoked is the evidence saying the certificate is revoked, and
	// RevokedAt when. The evidence is still returned — it is the proof —
	// and the caller decides what a revoked certificate means for its
	// document (for the signer's own: nothing is saved, CERT_REVOKED).
	Revoked   bool
	RevokedAt time.Time

	// Stale is evidence whose thisUpdate is not after the time the
	// signature is known to have existed (the timestamp's genTime), even
	// after asking once more. DSS embeds it with a warning, and so does
	// this package: it is the freshest the responder would give, and a
	// validator may still accept it under its own policy. It does not
	// lower the level; it is reported.
	Stale bool

	// Responder is a delegated OCSP responder's certificate — one other
	// than the issuer — which needs evidence of its own unless it carries
	// ocsp-nocheck.
	Responder *x509.Certificate

	TooLarge     bool
	SkippedBytes int64
}

func (e evidence) found() bool { return len(e.OCSPResponse) > 0 || len(e.CRL) > 0 }

// collectOne asks cert's OCSP responders, then its CRL distribution
// points: OCSP first, as DSS does, because a response is a few kilobytes
// about one certificate and a CRL can be every card a state has issued.
// notBefore, when not zero, is the time evidence must be newer than.
func collectOne(ctx context.Context, cert, issuer *x509.Certificate, notBefore time.Time, maxArtefactSize int64, mem *EndpointMemory) evidence {
	if e := fetchOCSP(ctx, cert, issuer, notBefore, maxArtefactSize, mem); e.found() || e.TooLarge {
		// An oversized OCSP response is not followed by a CRL: a
		// responder answering megabytes is anomalous enough on its own
		// (the reasoning the old code gave, kept).
		return e
	}
	return fetchCRL(ctx, cert, issuer, notBefore, maxArtefactSize, mem)
}

// fetchOCSP tries every OCSP responder URL the certificate's AIA names,
// up to ocspAttempts times each.
//
// A response is used only if it parses, its signature verifies — by the
// issuer, or by a responder certificate the issuer signed (what
// ocsp.ParseResponseForCert checks) — it is about this certificate, and
// it says Good or Revoked. Unknown is a responder that does not know the
// certificate, which proves nothing either way: DSS does not accept it,
// and neither does this.
//
// A URL mem already knows did not answer is skipped without a request,
// and one that never answered is recorded before moving on.
func fetchOCSP(ctx context.Context, cert, issuer *x509.Certificate, notBefore time.Time, maxArtefactSize int64, mem *EndpointMemory) evidence {
	if len(cert.OCSPServer) == 0 || issuer == nil {
		return evidence{}
	}
	// SHA-1 in the CertID, no nonce, POST: DSS's own defaults, and the
	// only CertID hash every responder is required to understand
	// (RFC 6960 §4.3, RFC 5019). This hash identifies a certificate to a
	// responder; nothing is signed with it.
	reqDER, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return evidence{}
	}
	for _, raw := range cert.OCSPServer {
		if !fetchable(raw) || mem.silent(raw) {
			continue
		}
		answered := false
		var stale *evidence
		for attempt := 0; attempt < ocspAttempts; attempt++ {
			body, err := postWithTimeout(ctx, raw, "application/ocsp-request", reqDER, ocspTimeout, maxOCSPResponseSize)
			if err != nil {
				continue
			}
			// The endpoint answered. Whether the answer is usable is a
			// separate question — a malformed response is not a reason
			// to stop asking a responder that is plainly up.
			answered = true
			resp, err := ocsp.ParseResponseForCert(body, cert, issuer)
			if err != nil || (resp.Status != ocsp.Good && resp.Status != ocsp.Revoked) {
				continue
			}
			if int64(len(body)) > maxArtefactSize {
				return evidence{TooLarge: true, SkippedBytes: int64(len(body))}
			}
			e := evidence{OCSPResponse: body}
			if resp.Status == ocsp.Revoked {
				e.Revoked, e.RevokedAt = true, resp.RevokedAt
			}
			if resp.Certificate != nil && !bytes.Equal(resp.Certificate.Raw, issuer.Raw) && !hasOCSPNoCheck(resp.Certificate) {
				e.Responder = resp.Certificate
			}
			if !notBefore.IsZero() && !resp.ThisUpdate.After(notBefore) {
				// Older than the signature's own proof of existence:
				// asked once more, as DSS does, in case the responder
				// produces responses on demand and this one was cached.
				e.Stale = true
				if stale == nil {
					stale = &e
					continue
				}
			}
			return e
		}
		if stale != nil {
			return *stale
		}
		if !answered {
			mem.remember(raw)
		}
	}
	return evidence{}
}

// fetchCRL tries every CRL distribution point the certificate names,
// once each.
//
// A CRL is used only if it parses, its issuer is the certificate's
// issuer by name and its signature verifies under the issuer's key —
// x509's CheckSignatureFrom also refuses an issuer whose key usage
// leaves out cRLSign. An indirect CRL (one signed by someone else on the
// issuer's behalf) is refused, as DSS refuses it: it would need the
// CRL issuer's own chain, and no Serbian issuer is known to use one.
func fetchCRL(ctx context.Context, cert, issuer *x509.Certificate, notBefore time.Time, maxArtefactSize int64, mem *EndpointMemory) evidence {
	for _, raw := range cert.CRLDistributionPoints {
		if !fetchable(raw) || mem.silent(raw) {
			continue
		}
		body, err := getWithTimeout(ctx, raw, crlTimeout, maxCRLSize)
		if err != nil {
			mem.remember(raw)
			continue
		}
		rl, err := x509.ParseRevocationList(body)
		if err != nil {
			continue // not a parseable CRL: do not embed it
		}
		if issuer != nil {
			if !bytes.Equal(rl.RawIssuer, cert.RawIssuer) || rl.CheckSignatureFrom(issuer) != nil {
				continue
			}
		}
		if int64(len(body)) > maxArtefactSize {
			return evidence{TooLarge: true, SkippedBytes: int64(len(body))}
		}
		e := evidence{CRL: body}
		for _, rc := range rl.RevokedCertificateEntries {
			if rc.SerialNumber != nil && rc.SerialNumber.Cmp(cert.SerialNumber) == 0 {
				e.Revoked, e.RevokedAt = true, rc.RevocationTime
				break
			}
		}
		// Not asked again when stale: a CRL is issued on a schedule, and
		// the same address an instant later is the same list.
		if !notBefore.IsZero() && !rl.ThisUpdate.After(notBefore) {
			e.Stale = true
		}
		return e
	}
	return evidence{}
}
