package verify

import (
	"encoding/asn1"
	"fmt"
	"math/big"
)

const (
	classUniversal = 0
	classContext   = 2

	tagInteger = 2
	tagSet     = 17
)

// parsedCMS holds everything VerifySignature needs out of a SignedData,
// extracted by walking the DER/BER tree directly rather than through
// any structure internal/pades/cms defines.
type parsedCMS struct {
	certificates      [][]byte // raw DER, as embedded
	issuer            []byte   // signerInfo.sid.issuer, raw DER Name
	serialNumber      *big.Int
	signedAttrsRaw    []byte // the implicit [0] TLV, tag byte included
	messageDigest     []byte
	signingCertHash   []byte
	signature         []byte
	timestampTokenDER []byte // raw signatureTimeStampToken attribute value, if present
}

func parseCMS(der []byte) (*parsedCMS, error) {
	root, _, err := readDER(der)
	if err != nil {
		return nil, fmt.Errorf("reading ContentInfo: %w", err)
	}
	ciParts, err := root.sequence()
	if err != nil || len(ciParts) < 2 {
		return nil, fmt.Errorf("malformed ContentInfo")
	}
	var oid asn1.ObjectIdentifier
	if _, err := asn1.Unmarshal(ciParts[0].raw, &oid); err != nil {
		return nil, fmt.Errorf("parsing contentType: %w", err)
	}
	if !oid.Equal(oidSignedData) {
		return nil, fmt.Errorf("ContentInfo.contentType is %v, not signedData", oid)
	}
	if ciParts[1].class != classContext || ciParts[1].tag != 0 {
		return nil, fmt.Errorf("ContentInfo.content is not [0] EXPLICIT")
	}
	sdWrapper, err := ciParts[1].sequence()
	if err != nil || len(sdWrapper) != 1 {
		return nil, fmt.Errorf("malformed [0] EXPLICIT content")
	}
	sdParts, err := sdWrapper[0].sequence()
	if err != nil || len(sdParts) < 3 {
		return nil, fmt.Errorf("malformed SignedData")
	}

	out := &parsedCMS{}
	var signerInfoSet *derNode
	for _, f := range sdParts[3:] {
		switch {
		case f.class == classContext && f.tag == 0: // certificates
			certs, err := f.sequence()
			if err == nil {
				for _, c := range certs {
					out.certificates = append(out.certificates, c.raw)
				}
			}
		case f.class == classUniversal && f.tag == tagSet:
			node := f
			signerInfoSet = &node
		}
	}
	if signerInfoSet == nil {
		return nil, fmt.Errorf("SignedData has no signerInfos")
	}
	signerInfos, err := signerInfoSet.sequence()
	if err != nil || len(signerInfos) != 1 {
		return nil, fmt.Errorf("SignedData has %d signerInfos, want exactly 1", len(signerInfos))
	}
	if err := parseSignerInfo(signerInfos[0], out); err != nil {
		return nil, err
	}
	return out, nil
}

func parseSignerInfo(n derNode, out *parsedCMS) error {
	fields, err := n.sequence()
	if err != nil || len(fields) < 6 {
		return fmt.Errorf("malformed SignerInfo")
	}
	// version, sid, digestAlgorithm, signedAttrs, signatureAlgorithm, signature, [unsignedAttrs]
	sidParts, err := fields[1].sequence()
	if err != nil || len(sidParts) != 2 {
		return fmt.Errorf("malformed SignerIdentifier: only issuerAndSerialNumber is supported")
	}
	out.issuer = sidParts[0].raw
	out.serialNumber = new(big.Int).SetBytes(sidParts[1].content)
	if len(sidParts[1].content) > 0 && sidParts[1].content[0]&0x80 != 0 {
		// Serial numbers are never negative in practice; guard anyway.
		return fmt.Errorf("negative serial number is not supported")
	}

	if fields[3].class != classContext || fields[3].tag != 0 {
		return fmt.Errorf("signedAttrs is not tagged as implicit [0]")
	}
	out.signedAttrsRaw = fields[3].raw
	attrs, err := fields[3].sequence()
	if err != nil {
		return fmt.Errorf("malformed signedAttrs: %w", err)
	}
	for _, a := range attrs {
		aParts, err := a.sequence()
		if err != nil || len(aParts) != 2 {
			continue
		}
		var attrOID asn1.ObjectIdentifier
		if _, err := asn1.Unmarshal(aParts[0].raw, &attrOID); err != nil {
			continue
		}
		values, err := aParts[1].sequence() // SET OF, one value
		if err != nil || len(values) != 1 {
			continue
		}
		switch {
		case attrOID.Equal(oidMessageDigest):
			out.messageDigest, _ = values[0].octetStringValue()
		case attrOID.Equal(oidSigningCertificateV2):
			out.signingCertHash = extractESSCertIDv2Hash(values[0])
		}
	}

	sigField := fields[5]
	if sigField.class != classUniversal || sigField.tag != 4 {
		return fmt.Errorf("signature field is not an OCTET STRING")
	}
	out.signature, _ = sigField.octetStringValue()

	if len(fields) > 6 && fields[6].class == classContext && fields[6].tag == 1 {
		uAttrs, err := fields[6].sequence()
		if err == nil {
			for _, a := range uAttrs {
				aParts, err := a.sequence()
				if err != nil || len(aParts) != 2 {
					continue
				}
				var attrOID asn1.ObjectIdentifier
				if _, err := asn1.Unmarshal(aParts[0].raw, &attrOID); err != nil {
					continue
				}
				if !attrOID.Equal(oidSignatureTimeStampToken) {
					continue
				}
				values, err := aParts[1].sequence()
				if err == nil && len(values) == 1 {
					out.timestampTokenDER = values[0].raw
				}
			}
		}
	}
	return nil
}

// extractESSCertIDv2Hash reads SigningCertificateV2 ::= SEQUENCE {
// certs SEQUENCE OF ESSCertIDv2 }, ESSCertIDv2 ::= SEQUENCE {
// hashAlgorithm AlgorithmIdentifier DEFAULT {sha256}, certHash
// OCTET STRING, issuerSerial IssuerSerial OPTIONAL }, returning the
// first entry's certHash. hashAlgorithm is not re-checked here: this
// project's own signer always omits it (its DER-mandated default is
// SHA-256, see internal/pades/cms's own comment on the same point,
// reached independently by both from RFC 5035 §4), and a present-but-
// non-default hashAlgorithm would need SHA-256 vs. something else
// distinguished — out of scope for what F3 §8 asks this check to prove.
func extractESSCertIDv2Hash(scv2 derNode) []byte {
	outer, err := scv2.sequence()
	if err != nil || len(outer) < 1 {
		return nil
	}
	certs, err := outer[0].sequence()
	if err != nil || len(certs) < 1 {
		return nil
	}
	fields, err := certs[0].sequence()
	if err != nil || len(fields) < 1 {
		return nil
	}
	// First field is either certHash (OCTET STRING, if hashAlgorithm
	// was omitted per its DEFAULT) or hashAlgorithm (SEQUENCE) followed
	// by certHash.
	if fields[0].class == classUniversal && fields[0].tag == 4 {
		v, _ := fields[0].octetStringValue()
		return v
	}
	if len(fields) >= 2 {
		v, _ := fields[1].octetStringValue()
		return v
	}
	return nil
}
