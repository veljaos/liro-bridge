package cms

import "encoding/asn1"

// Object identifiers used throughout this package, all from SPEC §12.3
// and F3 §5.1/§5.3.
var (
	oidData       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}

	oidContentType          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningCertificateV2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}

	// oidSignatureTimeStampToken (RFC 3161 / RFC 5035) is the unsigned
	// attribute a signature timestamp token is attached under (F3 §6).
	// Exported as OIDSignatureTimeStampToken for callers orchestrating a
	// full sign (internal/pades) that need to pass it to
	// AddUnsignedAttribute themselves.
	oidSignatureTimeStampToken = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}

	oidSHA256        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA256WithRSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
)

// OIDSignatureTimeStampToken is oidSignatureTimeStampToken, exported for
// signing-side callers outside this package (e.g. internal/pades's
// orchestration) that need to pass it to Builder.AddUnsignedAttribute
// themselves. Not for internal/pades/verify, which defines its own copy
// independently (F3 §8).
var OIDSignatureTimeStampToken = oidSignatureTimeStampToken
