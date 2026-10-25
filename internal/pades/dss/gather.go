package dss

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// What B-LT needs, gathered (F3 §7, [[D-462]]). CollectRevocation asks
// about one chain in the order it was given; Gather asks about every
// certificate a validator will need evidence for, which is what DSS 6.5
// does on its way to BASELINE-LT:
//
//   - the signer's certificate and each issuer up to a trust anchor;
//   - the signature timestamp's signer and its issuers — B-LT is the
//     validation data for the signature *and its timestamp*, and a
//     timestamp whose authority cannot be checked proves no time;
//   - each delegated OCSP responder's certificate, unless it carries
//     ocsp-nocheck.
//
// An issuer is looked for among the certificates already at hand, then in
// the trust store, then at the certificate's AIA caIssuers address, and is
// taken only if it signed the certificate. A certificate that is
// self-signed, or is itself in the trust store, is an anchor: a validator
// trusts it because of where it is listed, not because of a CRL, and DSS
// asks nothing about one either.

// Timestamp is the signature timestamp, as far as validation data goes.
type Timestamp struct {
	GenTime time.Time
	// Certificates are the certificates the token carried (DER), the
	// authority's own among them when the TSA honoured certReq.
	Certificates [][]byte
}

// GatherRequest is everything Gather is told.
type GatherRequest struct {
	Signer *x509.Certificate
	// Chain is the signer's issuers as signed into the CMS, nearest
	// first; it may be empty.
	Chain []*x509.Certificate
	// Timestamp is nil for a signature with none (there is then no B-LT
	// to reach, and Gather is not called).
	Timestamp *Timestamp
	// TrustStore is the Trusted List's CA certificates: issuers to use,
	// and anchors to stop at.
	TrustStore []*x509.Certificate
	// FetchIssuer fetches an AIA caIssuers URL; nil means never.
	FetchIssuer     func(ctx context.Context, url string) ([]byte, error)
	MaxArtefactSize int64
	Memory          *EndpointMemory
}

// Material is what Gather found, ready for ApplyMaterial.
type Material struct {
	Certs []*x509.Certificate
	OCSPs [][]byte
	CRLs  [][]byte

	// Complete is true when every certificate that needed evidence got
	// it. False means B-T, whatever else was found.
	Complete bool

	// TooLarge and LargestSkippedBytes are CollectRevocation's, for the
	// message that says why B-LT was not reached (Task 1c).
	TooLarge            bool
	LargestSkippedBytes int64

	// Revoked lists the certificates the evidence itself says are
	// revoked, each with RevokedRoles saying whose chain it is in.
	Revoked []RevokedCertificate

	// Stale counts evidence no newer than the timestamp after asking
	// once more; Missing is the certificates that needed evidence and got
	// none, for the log.
	Stale   int
	Missing []*x509.Certificate
}

// Role says which part of the signature a certificate belongs to.
type Role int

const (
	RoleSigner    Role = iota // the signer's certificate or one of its issuers
	RoleTimestamp             // the timestamp authority's, or one of its issuers
	RoleResponder             // a delegated OCSP responder's
)

// Thumbprint is how a message names a certificate: its SHA-1, uppercase
// hex, as everywhere in the agent. Never the subject, which for a
// signer is a person's name and, on a MUP card, their JMBG (SPEC
// §18.3).
func Thumbprint(c *x509.Certificate) string {
	sum := sha1.Sum(c.Raw) //nolint:gosec // an identifier, as in dss.go
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

// String names the role in the log.
func (r Role) String() string {
	switch r {
	case RoleSigner:
		return "signer"
	case RoleTimestamp:
		return "timestamp"
	case RoleResponder:
		return "responder"
	}
	return fmt.Sprintf("Role(%d)", int(r))
}

// RevokedCertificate is one certificate the evidence says is revoked.
type RevokedCertificate struct {
	Certificate *x509.Certificate
	Role        Role
	RevokedAt   time.Time
}

type gatherItem struct {
	cert   *x509.Certificate
	issuer *x509.Certificate // known already, or nil to look for
	role   Role
}

// Gather fetches the validation data for one timestamped signature.
func Gather(ctx context.Context, req GatherRequest) Material {
	max := req.MaxArtefactSize
	if max <= 0 {
		max = DefaultMaxArtefactSize
	}
	var notBefore time.Time
	var tokenCerts []*x509.Certificate
	if req.Timestamp != nil {
		notBefore = req.Timestamp.GenTime
		for _, der := range req.Timestamp.Certificates {
			if c, err := x509.ParseCertificate(der); err == nil {
				tokenCerts = append(tokenCerts, c)
			}
		}
	}

	g := &gatherer{
		req:        req,
		known:      append(append(append([]*x509.Certificate{req.Signer}, req.Chain...), tokenCerts...), req.TrustStore...),
		seenCert:   map[string]bool{},
		seenOCSP:   map[[32]byte]bool{},
		seenCRL:    map[[32]byte]bool{},
		done:       map[string]bool{},
		notBefore:  notBefore,
		maxSize:    max,
		material:   Material{Complete: true},
		anchorRaws: map[string]bool{},
	}
	for _, c := range req.TrustStore {
		g.anchorRaws[string(c.Raw)] = true
	}

	queue := []gatherItem{{cert: req.Signer, role: RoleSigner}}
	for _, c := range req.Chain {
		queue = append(queue, gatherItem{cert: c, role: RoleSigner})
	}
	if tsaCert := timestampSigner(tokenCerts); tsaCert != nil {
		queue = append(queue, gatherItem{cert: tsaCert, role: RoleTimestamp})
	} else if req.Timestamp != nil {
		// Without the authority's certificate its chain cannot be
		// checked, and B-LT is the timestamp's validation data too.
		slog.Warn("pades/dss: the timestamp carries no certificate of its authority, so its chain cannot be given validation data")
		g.material.Complete = false
	}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		queue = append(queue, g.visit(ctx, item)...)
	}
	return g.material
}

type gatherer struct {
	req        GatherRequest
	known      []*x509.Certificate
	anchorRaws map[string]bool
	seenCert   map[string]bool
	seenOCSP   map[[32]byte]bool
	seenCRL    map[[32]byte]bool
	done       map[string]bool
	notBefore  time.Time
	maxSize    int64
	material   Material
}

func (g *gatherer) addCert(c *x509.Certificate) {
	if c == nil || g.seenCert[string(c.Raw)] {
		return
	}
	g.seenCert[string(c.Raw)] = true
	g.material.Certs = append(g.material.Certs, c)
}

func (g *gatherer) isAnchor(c *x509.Certificate) bool {
	return g.anchorRaws[string(c.Raw)] || (bytes.Equal(c.RawIssuer, c.RawSubject) && c.CheckSignatureFrom(c) == nil)
}

// visit embeds item's certificate, finds its issuer and its evidence, and
// returns whatever else now needs visiting.
func (g *gatherer) visit(ctx context.Context, item gatherItem) []gatherItem {
	c := item.cert
	if g.done[string(c.Raw)] {
		return nil
	}
	g.done[string(c.Raw)] = true
	g.addCert(c)
	if g.isAnchor(c) {
		return nil
	}
	issuer := item.issuer
	if issuer == nil {
		issuer = g.findIssuer(ctx, c)
	}
	if issuer == nil {
		slog.Warn("pades/dss: no issuer found for a certificate that needs validation data", "thumbprint", Thumbprint(c), "role", item.role.String())
		g.material.Complete = false
		g.material.Missing = append(g.material.Missing, c)
		return nil
	}
	next := []gatherItem{{cert: issuer, role: item.role}}

	e := collectOne(ctx, c, issuer, g.notBefore, g.maxSize, g.req.Memory)
	switch {
	case len(e.OCSPResponse) > 0:
		g.addOCSP(e.OCSPResponse)
	case len(e.CRL) > 0:
		g.addCRL(e.CRL)
	case e.TooLarge:
		slog.Warn("pades/dss: the evidence for a certificate is larger than the cap, so it was left out",
			"thumbprint", Thumbprint(c), "role", item.role.String(), "bytes", e.SkippedBytes, "cap", g.maxSize)
		g.material.Complete = false
		g.material.TooLarge = true
		if e.SkippedBytes > g.material.LargestSkippedBytes {
			g.material.LargestSkippedBytes = e.SkippedBytes
		}
		return next
	default:
		slog.Warn("pades/dss: no OCSP response or CRL could be had for a certificate",
			"thumbprint", Thumbprint(c), "role", item.role.String(),
			"ocsp", c.OCSPServer, "crl", c.CRLDistributionPoints)
		g.material.Complete = false
		g.material.Missing = append(g.material.Missing, c)
		return next
	}
	if e.Revoked {
		g.material.Revoked = append(g.material.Revoked, RevokedCertificate{Certificate: c, Role: item.role, RevokedAt: e.RevokedAt})
	}
	if e.Stale {
		g.material.Stale++
	}
	if e.Responder != nil {
		// The responder's own certificate is signed by the CA it answers
		// for: that is how ocsp.ParseResponseForCert accepted it.
		next = append(next, gatherItem{cert: e.Responder, issuer: issuer, role: RoleResponder})
	}
	return next
}

func (g *gatherer) addOCSP(der []byte) {
	h := sha256.Sum256(der)
	if !g.seenOCSP[h] {
		g.seenOCSP[h] = true
		g.material.OCSPs = append(g.material.OCSPs, der)
	}
}

func (g *gatherer) addCRL(der []byte) {
	h := sha256.Sum256(der)
	if !g.seenCRL[h] {
		g.seenCRL[h] = true
		g.material.CRLs = append(g.material.CRLs, der)
	}
}

// findIssuer looks among what is at hand, then at AIA, and takes a
// candidate only if it signed c.
func (g *gatherer) findIssuer(ctx context.Context, c *x509.Certificate) *x509.Certificate {
	for _, cand := range g.known {
		if signedBy(c, cand) {
			return cand
		}
	}
	if g.req.FetchIssuer == nil {
		return nil
	}
	for _, u := range c.IssuingCertificateURL {
		if !fetchable(u) {
			continue
		}
		der, err := g.req.FetchIssuer(ctx, u)
		if err != nil {
			slog.Info("pades/dss: AIA caIssuers fetch failed", "url", u, "error", err)
			continue
		}
		cand, err := x509.ParseCertificate(der)
		if err != nil {
			continue // Halcom's AIA names a .crl (F3 §11.8)
		}
		if signedBy(c, cand) {
			g.known = append(g.known, cand)
			return cand
		}
	}
	return nil
}

func signedBy(c, issuer *x509.Certificate) bool {
	return bytes.Equal(c.RawIssuer, issuer.RawSubject) && c.CheckSignatureFrom(issuer) == nil
}

// timestampSigner picks the authority's own certificate out of the
// token's: the one whose extended key usage is time stamping (RFC 3161
// §2.3 requires it, and critical).
func timestampSigner(certs []*x509.Certificate) *x509.Certificate {
	for _, c := range certs {
		for _, u := range c.ExtKeyUsage {
			if u == x509.ExtKeyUsageTimeStamping {
				return c
			}
		}
	}
	return nil
}
