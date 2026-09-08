package api

// GET /v2/certificates: which certificates this machine can sign with.
//
// It exists because /v2/sign requires certificateThumbprint and nothing
// told a caller where to get one. An application that builds its own
// CMS has to name the signer certificate it built it around, and an SDK
// that means to offer the person a choice has to have something to
// offer — neither was possible from inside the protocol.
//
// What it carries is bounded by two rules, and the second is the one
// that decides the shape.
//
// **Nothing the classification layer strips.** A Serbian qualified
// certificate carries the holder's national identity number in its
// Subject DN and their email address in the DN or the SAN (SPEC §11.6).
// internal/trust/classify scrubs both before anything is displayed or
// logged, and this endpoint reports what that layer produced, never the
// certificate. There is no field here they could travel in.
//
// **Not the DER, and not a PEM.** A caller building a CMS needs the
// signer certificate itself — for issuerAndSerialNumber, for
// signingCertificateV2, and for the certificates set (SPEC §12.3) — so
// the obvious thing to do is put it here. It is not here, and the
// reason is what the first rule is about: the DER *is* the national
// identity number and the email address, in full, wrapped in ASN.1.
// Handing it over on a listing turns "which certificates exist" into
// "here is the identity document of the person at this machine", and
// this listing is answered without any window, at any moment a paired
// application chooses.
//
// That leaves the CMS path with a real hole, stated rather than
// glossed: a caller that has never seen the certificate still cannot
// build a CMS around it. The answer is not to widen this endpoint, it
// is that the certificate should travel with something the person
// approved — the signing operation itself. Recorded for whoever takes
// it up; nothing here pretends the gap is closed.
//
// What is hidden is what is hidden everywhere else, through the same
// one rule (classify.Info.HiddenByDefault, D-108/D-149): the
// Windows-internal artefacts a machine issues itself, and the
// authentication certificate every Serbian card carries beside its
// signing one — which shows the person their own name a second time
// and is not a choice anybody can make.

import (
	"context"
	"errors"
	"net/http"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// Certificate is one certificate this machine can sign with, as
// GET /v2/certificates reports it.
//
// Every field is already through internal/trust/classify: DisplayName
// is givenName + surname (SPEC §11.7, never parsed out of CN), and
// nothing in this struct can hold a national identity number or an
// email address because no field carries a Subject DN at all — the same
// allow-list guarantee audit.Entry gets its safety from (D-084).
type Certificate struct {
	// Thumbprint is the SHA-1 thumbprint, uppercase hex: the value
	// /v2/sign's certificateThumbprint takes.
	Thumbprint string

	// DisplayName is the signer's name as the agent's own window shows
	// it.
	DisplayName string

	// Issuer is the issuing CA's common name.
	Issuer string

	// Purpose is the role KeyUsage gives this certificate — "signing",
	// "authentication" or "unknown" (SPEC §11.4/§11.5).
	Purpose string

	// Qualified reports whether the certificate's issuer is a Trusted
	// List service that was granted at the time of asking (SPEC §11.1).
	Qualified bool

	// Usable reports whether it can sign right now.
	Usable bool

	// NotUsableReason says why not, and is empty when it can. It is one
	// of the codes in §7 — CARD_NOT_PRESENT, CERT_EXPIRED,
	// CERT_NOT_USABLE.
	NotUsableReason errs.Code

	// IsTestKey marks a soft-token certificate. SPEC §16.6 requires a
	// test signature to be visibly marked everywhere it appears, and a
	// caller that could not tell one apart would be the one place it
	// was not.
	IsTestKey bool
}

// CertificateSource enumerates what this machine can sign with.
//
// It is an interface for the same reason Signer is: internal/api must
// not import internal/cli (SPEC §4.2 rule 4), and cmd/liro-bridge is
// the one place that knows a certificate comes from Windows CNG.
type CertificateSource interface {
	// Certificates returns the certificates a person would be offered,
	// in the order they would be offered in, with everything a listing
	// hides already left out.
	Certificates(ctx context.Context) ([]Certificate, error)
}

// certificatesResponse is the body of GET /v2/certificates.
type certificatesResponse struct {
	Certificates []certificateBody `json:"certificates"`
}

type certificateBody struct {
	Thumbprint      string `json:"thumbprint"`
	DisplayName     string `json:"displayName"`
	Issuer          string `json:"issuer"`
	Purpose         string `json:"purpose"`
	Qualified       bool   `json:"qualified"`
	Usable          bool   `json:"usable"`
	NotUsableReason string `json:"notUsableReason,omitempty"`
	IsTestKey       bool   `json:"isTestKey"`
}

// handleCertificates answers GET /v2/certificates.
func (s *Server) handleCertificates(w http.ResponseWriter, r *http.Request) {
	if e := requireGET(r); e != nil {
		fail(w, e)
		return
	}
	// No content type is required, because there is no content — the
	// rule a real client cannot obey on a GET (see handleHealth). The
	// four X-Liro-* headers are what keep a page out.
	//
	// The signature covers the hash of no bytes at all
	// (EmptyBodySHA256).
	if _, e := s.auth.Authenticate(r, nil); e != nil {
		fail(w, e)
		return
	}
	// Checked after authentication, exactly as the whole-document
	// path's own switch is (D-195): whether this machine will say who
	// is at it is a fact about how it is set up, and an unpaired caller
	// has no business learning it.
	if !s.certificateListing() {
		fail(w, errs.New(errs.CodeCertificateListingDisabled,
			errors.New("the certificate listing is switched off")))
		return
	}
	if s.certificates == nil {
		// A build with no certificate source wired serves pairing,
		// health and signing and nothing else. That is this agent's own
		// misconfiguration, not the caller's request.
		fail(w, errs.New(errs.CodeInternal, errors.New("no certificate source is wired")))
		return
	}

	certs, err := s.certificates.Certificates(r.Context())
	if err != nil {
		fail(w, errs.New(errs.CodeInternal, err))
		return
	}
	out := certificatesResponse{Certificates: make([]certificateBody, 0, len(certs))}
	for _, c := range certs {
		out.Certificates = append(out.Certificates, certificateBody{
			Thumbprint:      c.Thumbprint,
			DisplayName:     c.DisplayName,
			Issuer:          c.Issuer,
			Purpose:         c.Purpose,
			Qualified:       c.Qualified,
			Usable:          c.Usable,
			NotUsableReason: string(c.NotUsableReason),
			IsTestKey:       c.IsTestKey,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
