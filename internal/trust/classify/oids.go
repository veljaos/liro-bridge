package classify

import (
	"crypto/x509"
	"encoding/asn1"
)

// Policy and qcStatements OIDs, all taken from real certificates
// (SPEC §11.2/§11.3), never from vendor documentation: the Halcom policy
// document states 1.3.6.1.4.1.5939.11.2.6, but the actual certificate
// carries 1.3.6.1.4.1.5939.10.1.6 — a different branch entirely. These
// are supporting, display-only evidence; qualification is decided
// against the Trusted List (F1 §5.4), never from this table.
const (
	// oidQCPnQSCD is present on every end-entity signing certificate
	// examined from all three issuers: qualified, natural person, on a
	// QSCD (SPEC §11.2).
	oidQCPnQSCD = "0.4.0.194112.1.2"

	oidPolicyMUP    = "1.3.6.1.4.1.33589.1.1.0"
	oidPolicyPosta  = "1.3.6.1.4.1.15672.10.142.1.0"
	oidPolicyHalcom = "1.3.6.1.4.1.5939.10.1.6"
)

// qcStatements OIDs (extension 1.3.6.1.5.5.7.1.3), SPEC §11.2.
var (
	oidQCStatementsExtension = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}

	oidQcCompliance = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}
	oidQcSSCD       = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}
	oidQcPDS        = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 5} // Halcom only
	oidQcTypeESign  = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 1}
	oidQcTypeESeal  = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 6, 2}

	// oidPKIXQCSyntaxV2 marks a qcStatement whose Info carries
	// SemanticsInformation (SPEC §11.2). F1 asks only that its presence
	// be recognised as natural-person evidence, not that the nested
	// semantics identifier be decoded — the statement OID itself already
	// only appears in these certificates for that purpose.
	oidPKIXQCSyntaxV2 = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 11, 2}
)

// qcStatement mirrors the QCStatement ASN.1 structure from RFC 3739:
// the qcStatements extension value is a SEQUENCE OF this. It is not
// parsed by crypto/x509, so it is decoded here by hand (F1 §5.4).
type qcStatement struct {
	StatementID asn1.ObjectIdentifier
	Info        asn1.RawValue `asn1:"optional"`
}

// qcStatementIDs returns the statement OIDs present in cert's
// qcStatements extension, or nil if the extension is absent or
// unparseable. A parse failure is not an error for classification
// purposes: it degrades to "no supporting evidence", never to a crash.
func qcStatementIDs(cert *x509.Certificate) []asn1.ObjectIdentifier {
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidQCStatementsExtension) {
			continue
		}
		var statements []qcStatement
		if _, err := asn1.Unmarshal(ext.Value, &statements); err != nil {
			return nil
		}
		ids := make([]asn1.ObjectIdentifier, 0, len(statements))
		for _, s := range statements {
			ids = append(ids, s.StatementID)
		}
		return ids
	}
	return nil
}

func containsOID(ids []asn1.ObjectIdentifier, want asn1.ObjectIdentifier) bool {
	for _, id := range ids {
		if id.Equal(want) {
			return true
		}
	}
	return false
}

// hasEIDASPolicyOID reports whether cert carries the QCP-n-qscd policy
// OID common to all three issuers (SPEC §11.2), for display purposes.
func hasEIDASPolicyOID(cert *x509.Certificate) bool {
	for _, p := range cert.Policies {
		if p.String() == oidQCPnQSCD {
			return true
		}
	}
	return false
}

// hasQcSSCD reports whether cert's qcStatements assert QcSSCD
// (SPEC §11.2), one of the two OnQSCD signals (F1 §5.1).
func hasQcSSCD(cert *x509.Certificate) bool {
	return containsOID(qcStatementIDs(cert), oidQcSSCD)
}

// hasQcCompliance reports whether cert's qcStatements assert
// QcCompliance (SPEC §11.2), present on all three issuers' certificates.
func hasQcCompliance(cert *x509.Certificate) bool {
	return containsOID(qcStatementIDs(cert), oidQcCompliance)
}

// hasQcPDS reports whether cert's qcStatements carry a QcPDS statement.
// SPEC §11.2 measured this on Halcom certificates only — MUP and Pošta
// omit it — so it doubles as issuer-recognition evidence.
func hasQcPDS(cert *x509.Certificate) bool {
	return containsOID(qcStatementIDs(cert), oidQcPDS)
}

// isESign reports whether the certificate's qcStatements assert QcType
// esign — a natural person's signature, the common case (SPEC §11.2).
func isESign(cert *x509.Certificate) bool {
	return containsOID(qcStatementIDs(cert), oidQcTypeESign)
}

// hasNaturalPersonSyntax reports whether cert's qcStatements include the
// PKIXQCSyntax-v2 statement (SPEC §11.2), display-only evidence that the
// certificate identifies a natural person.
func hasNaturalPersonSyntax(cert *x509.Certificate) bool {
	return containsOID(qcStatementIDs(cert), oidPKIXQCSyntaxV2)
}

// IsESeal reports whether the certificate's qcStatements assert QcType
// eseal — a legal person's seal, not a natural person's signature
// (SPEC §11.2). The UI for eseal certificates arrives in a later phase;
// this exists so eseal certificates are recognised rather than silently
// treated as ordinary signing certificates.
func IsESeal(cert *x509.Certificate) bool {
	return containsOID(qcStatementIDs(cert), oidQcTypeESeal)
}

// issuerPolicyLabel returns a human-readable issuer name recognised from
// the certificate's own policy OIDs (SPEC §11.3), for display only —
// never used to decide qualification, and never populated from vendor
// documentation (Halcom's published policy document names a different,
// wrong OID branch). Empty if none of the three known OIDs is present.
func issuerPolicyLabel(cert *x509.Certificate) string {
	for _, p := range cert.Policies {
		switch p.String() {
		case oidPolicyMUP:
			return "MUP"
		case oidPolicyPosta:
			return "Posta Srbije"
		case oidPolicyHalcom:
			return "Halcom"
		}
	}
	return ""
}
