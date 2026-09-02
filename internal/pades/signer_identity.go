package pades

import (
	"crypto/x509"
	"encoding/asn1"
	"strings"
)

var (
	oidGivenNameStamp    = asn1.ObjectIdentifier{2, 5, 4, 42}
	oidSurnameStamp      = asn1.ObjectIdentifier{2, 5, 4, 4}
	oidSerialNumberStamp = asn1.ObjectIdentifier{2, 5, 4, 5}
)

// signerDisplayName builds the visual stamp's signer-name line from
// givenName and surname alone (SPEC §11.7/F4 §5.1) — never by parsing
// CommonName, which carries a CA-internal number and, for MUP, the
// literal word "Sign" (SPEC §11.7's three worked examples). Falls back
// to CommonName only when both attributes are absent.
//
// This duplicates roughly a dozen lines of internal/trust/classify's
// dn.go (D-019 reaches the identical rule there), rather than importing
// that package: internal/pades/appearance's stamp options carry only
// plain strings, never a certificate (see appearance.Options' doc
// comment), specifically so that this package's own, self-contained
// extraction logic is the *only* code path that ever touches a signer
// certificate's Subject DN on the way to a stamp — one small function
// to audit for "does this ever produce PNORS," instead of a shared
// dependency whose safety this package would otherwise have to trust
// blindly. See docs/decisions.md for the full reasoning.
func signerDisplayName(cert *x509.Certificate) string {
	var given, surname string
	for _, atv := range cert.Subject.Names {
		v, ok := atv.Value.(string)
		if !ok {
			continue
		}
		switch {
		case atv.Type.Equal(oidGivenNameStamp):
			given = v
		case atv.Type.Equal(oidSurnameStamp):
			surname = v
		}
	}
	switch {
	case given != "" && surname != "":
		return given + " " + surname
	case given != "":
		return given
	case surname != "":
		return surname
	default:
		return cert.Subject.CommonName
	}
}

// documentIDFromCertificate returns the ID-card document number
// embedded in cert's Subject DN — the "IDCRS-" prefixed value inside
// serialNumber (SPEC §11.6 Trap 2, Halcom's non-resident identifier) —
// or "" if none is present. Called only when --stamp-show-document-id
// is set (F4 §5.2): "opt-in, never the default."
//
// This function recognises only the IDCRS prefix. It is structurally
// incapable of ever returning a PNORS-prefixed value (the national
// identity number) because PNORS is never checked for anywhere in this
// function — not filtered out, simply never looked for — regardless of
// what else the same multi-valued RDN might also contain (SPEC §11.6
// Trap 1). "The national identity number must be unreachable from this
// package" (F4 §5.2) holds even if this function were called
// unconditionally, not only because the caller happens to gate it on a
// flag.
func documentIDFromCertificate(cert *x509.Certificate) string {
	for _, atv := range cert.Subject.Names {
		if !atv.Type.Equal(oidSerialNumberStamp) {
			continue
		}
		v, ok := atv.Value.(string)
		if !ok {
			continue
		}
		if strings.HasPrefix(v, "IDCRS-") {
			return strings.TrimPrefix(v, "IDCRS-")
		}
	}
	return ""
}
