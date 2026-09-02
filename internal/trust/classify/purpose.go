package classify

import "crypto/x509"

// purposeFromKeyUsage decides Purpose from KeyUsage alone.
//
// SPEC §11.4, measured: MUP and Pošta signing certificates carry
// digitalSignature + contentCommitment, but Halcom's signing certificate
// carries contentCommitment ONLY. A filter that requires digitalSignature
// rejects every Halcom certificate — contentCommitment (a.k.a.
// nonRepudiation, x509.KeyUsageContentCommitment) is the only reliable
// signal, and per SPEC §11.5 it is also the only thing that
// distinguishes a signing certificate from an authentication certificate
// sharing the exact same Subject.
func purposeFromKeyUsage(ku x509.KeyUsage) Purpose {
	if ku&x509.KeyUsageContentCommitment != 0 {
		return PurposeSigning
	}
	if ku&(x509.KeyUsageDigitalSignature|x509.KeyUsageKeyEncipherment) != 0 {
		return PurposeAuthentication
	}
	return PurposeUnknown
}
