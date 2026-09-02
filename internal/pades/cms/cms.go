package cms

import (
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"math/big"
)

// bigOne is the CMSVersion value (1) used for both SignedData.version
// and SignerInfo.version, correct whenever sid is issuerAndSerialNumber
// and there are no attribute certificates (RFC 5652 §5.1/§5.3) — the
// only case this project ever builds.
var bigOne = big.NewInt(1)

// Builder assembles one RFC 5652 SignedData over a pre-computed
// document digest (F3 §5.1), matching SPEC §12.3 exactly:
//
//	SignedData
//	  version: 1
//	  digestAlgorithms: { SHA-256 }
//	  encapContentInfo: id-data, no content (detached)
//	  certificates: signer + chain
//	  signerInfos:
//	    sid: issuerAndSerialNumber
//	    digestAlgorithm: SHA-256
//	    signedAttrs: contentType, messageDigest, signingCertificateV2
//	    signatureAlgorithm: sha256WithRSAEncryption
//	    unsignedAttrs: signatureTimeStampToken (added later, see
//	    AddUnsignedAttribute)
//
// Usage:
//
//	b := cms.NewBuilder(signerCert, chain, messageDigest)
//	digest := b.SignedAttrsDigest()
//	sig, err := session.SignDigest(ctx, keysource.DigestSHA256, digest[:])
//	b.SetSignature(sig)
//	// optionally: b.AddUnsignedAttribute(timestampTokenOID, tokenDER)
//	der, err := b.Finish()
type Builder struct {
	signerCert    *x509.Certificate
	chain         []*x509.Certificate
	signedAttrs   []byte // the [0] IMPLICIT-tagged blob, as it appears in SignerInfo
	signature     []byte
	unsignedAttrs [][]byte // pre-built Attribute SEQUENCEs
	err           error
}

// NewBuilder starts a SignedData over messageDigest (SHA-256 of the
// /ByteRange span). signerCert and chain (issuer(s), signer excluded,
// see F3 §5.4) become the CMS certificates set.
//
// No signingTime attribute is ever added — PAdES takes the time from
// the RFC 3161 timestamp, not the signer's clock. All three fixtures
// this specification was written against omit it, and adding one is a
// conformance error, not a harmless extra (RFC 5652 §11.1 defines
// signingTime as informational only; PAdES/CAdES baseline profiles
// (ETSI EN 319 122) rely exclusively on the signature timestamp for
// time, which is why F3 §5.2 forbids it outright here).
func NewBuilder(signerCert *x509.Certificate, chain []*x509.Certificate, messageDigest []byte) *Builder {
	contentTypeAttr := buildAttribute(oidContentType, derOID(oidData))
	messageDigestAttr := buildAttribute(oidMessageDigest, derOctetString(messageDigest))
	signingCertV2Attr := buildAttribute(oidSigningCertificateV2, buildSigningCertificateV2(signerCert))

	signedAttrs := derImplicitSetOf(0, [][]byte{contentTypeAttr, messageDigestAttr, signingCertV2Attr})

	return &Builder{
		signerCert:  signerCert,
		chain:       chain,
		signedAttrs: signedAttrs,
	}
}

// SignedAttrsDigest returns the digest a signing session must sign: not
// SHA-256 of the raw signedAttrs bytes as they sit in SignerInfo, but of
// those same bytes with the leading tag byte replaced by 0x31.
//
// RFC 5652 §5.4: "the IMPLICIT [0] tag in the signedAttrs field is not
// used for the DER encoding, rather an EXPLICIT SET OF tag is used...
// the identifier octets, the tag, and the length octets... are not part
// of the contents octets... [but] a separate encoding of the
// SignedAttributes value MUST be generated using DER, with the tag
// changed to SET OF." Only the leading tag byte differs (0xA0 -> 0x31);
// the length and contents are identical. A verifier that skips this
// re-tagging step and hashes the implicit [0] form as-is will happily
// accept the result, which is exactly why the independent verifier
// (internal/pades/verify) is written from this RFC directly, not from
// this code (F3 §8).
func (b *Builder) SignedAttrsDigest() [32]byte {
	retagged := append([]byte{}, b.signedAttrs...)
	retagged[0] = 0x31
	return sha256.Sum256(retagged)
}

// SetSignature records the RSA signature produced over
// SignedAttrsDigest().
func (b *Builder) SetSignature(sig []byte) { b.signature = sig }

// AddUnsignedAttribute adds an attribute to SignerInfo's unsignedAttrs
// (F3 §6: used for signatureTimeStampToken once a timestamp token
// exists). valueDER is the attribute's already-DER-encoded value (a
// single complete TLV) — for signatureTimeStampToken, the TSA's
// response TimeStampToken bytes verbatim, never re-encoded.
func (b *Builder) AddUnsignedAttribute(oid []int, valueDER []byte) {
	b.unsignedAttrs = append(b.unsignedAttrs, buildAttribute(oid, valueDER))
}

// Finish assembles the final SignedData DER. It fails if SetSignature
// was never called.
func (b *Builder) Finish() ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	if len(b.signature) == 0 {
		return nil, fmt.Errorf("cms: Finish called before SetSignature")
	}

	sidBytes := derSequence(b.signerCert.RawIssuer, derInteger(b.signerCert.SerialNumber))
	digestAlgBytes := algorithmIdentifier(oidSHA256)
	sigAlgBytes := algorithmIdentifier(oidSHA256WithRSA)

	signerInfoParts := [][]byte{
		derInteger(bigOne),
		sidBytes,
		digestAlgBytes,
		b.signedAttrs,
		sigAlgBytes,
		derOctetString(b.signature),
	}
	if len(b.unsignedAttrs) > 0 {
		signerInfoParts = append(signerInfoParts, derImplicitSetOf(1, b.unsignedAttrs))
	}
	signerInfo := derSequence(signerInfoParts...)

	certs := make([][]byte, 0, 1+len(b.chain))
	certs = append(certs, b.signerCert.Raw)
	for _, c := range b.chain {
		certs = append(certs, c.Raw)
	}
	certificatesField := derImplicitSetOf(0, certs)

	digestAlgorithms := derSetOf([][]byte{algorithmIdentifier(oidSHA256)})
	encapContentInfo := derSequence(derOID(oidData)) // detached: no eContent

	signedData := derSequence(
		derInteger(bigOne),
		digestAlgorithms,
		encapContentInfo,
		certificatesField,
		derSetOf([][]byte{signerInfo}),
	)

	contentInfo := derSequence(derOID(oidSignedData), derExplicit(0, signedData))
	return contentInfo, nil
}

// buildAttribute builds "Attribute ::= SEQUENCE { attrType OID,
// attrValues SET OF AttributeValue }" with exactly one value, which is
// every attribute this package ever builds.
func buildAttribute(oid []int, valueDER []byte) []byte {
	return derSequence(derOID(oid), derSetOf([][]byte{valueDER}))
}

// buildSigningCertificateV2 builds the SigningCertificateV2 attribute
// value (RFC 5035): a SEQUENCE OF ESSCertIDv2 containing exactly one
// entry describing signerCert, binding the signature to it so it cannot
// be replayed against a different certificate holding the same key
// (F3 §5.3). hashAlgorithm is omitted: RFC 5035 §4 gives it a DEFAULT of
// {algorithm id-sha256}, which we always use, and DER requires a
// default-valued field to be omitted, never encoded explicitly.
func buildSigningCertificateV2(cert *x509.Certificate) []byte {
	certHash := sha256.Sum256(cert.Raw)
	generalName := derExplicit(4, cert.RawIssuer) // [4] directoryName
	generalNames := derSequence(generalName)
	issuerSerial := derSequence(generalNames, derInteger(cert.SerialNumber))
	essCertIDv2 := derSequence(derOctetString(certHash[:]), issuerSerial)
	return derSequence(derSequence(essCertIDv2))
}
