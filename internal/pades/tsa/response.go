package tsa

import (
	"encoding/asn1"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Universal BER/DER tag numbers this file matches on.
const (
	tagBoolean         = 1
	tagInteger         = 2
	tagOctetString     = 4
	tagUTF8String      = 12
	tagSequence        = 16
	tagSet             = 17
	tagUTCTime         = 23
	tagGeneralizedTime = 24
)

// classUniversal and classContext are the two BER tag classes this
// package's structures use.
const (
	classUniversal = 0
	classContext   = 2
)

// PKIStatus values (RFC 3161 §2.4.2).
const (
	pkiStatusGranted          = 0
	pkiStatusGrantedWithMods  = 1
	pkiStatusRejection        = 2
	pkiStatusWaiting          = 3
	pkiStatusRevocationWarn   = 4
	pkiStatusRevocationNotify = 5
)

// RejectionError is returned when the TSA answered with a PKIStatus
// other than granted/grantedWithMods (F3 §6.3: this is a deterministic
// rejection, never retried).
type RejectionError struct {
	Status int
	Detail string
}

func (e *RejectionError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("tsa: request rejected, PKIStatus %d", e.Status)
	}
	return fmt.Sprintf("tsa: request rejected, PKIStatus %d: %s", e.Status, e.Detail)
}

// Response is one successful RFC 3161 exchange's parsed result.
type Response struct {
	// TokenDER is the complete TimeStampToken exactly as the TSA sent
	// it — BER or DER, whichever the TSA used (F3 §12.5) — for
	// embedding verbatim into the CMS unsignedAttrs (this project never
	// re-encodes a value it did not generate itself).
	TokenDER []byte

	GenTime            time.Time
	Nonce              *big.Int
	MessageImprintAlg  asn1.ObjectIdentifier
	MessageImprintHash []byte
	SerialNumber       *big.Int
	SignerCertificates [][]byte // raw DER, present when certReq was honoured
}

// parseResponse parses a raw TimeStampResp (F3 §6, BER-tolerant per
// §12.5).
func parseResponse(raw []byte) (*Response, error) {
	root, _, err := parseBERValue(raw)
	if err != nil {
		return nil, fmt.Errorf("tsa: parsing TimeStampResp: %w", err)
	}
	parts, err := root.children()
	if err != nil {
		return nil, fmt.Errorf("tsa: parsing TimeStampResp: %w", err)
	}
	if len(parts) < 1 {
		return nil, fmt.Errorf("tsa: empty TimeStampResp")
	}
	statusParts, err := parts[0].children()
	if err != nil || len(statusParts) < 1 {
		return nil, fmt.Errorf("tsa: malformed PKIStatusInfo")
	}
	status := int(berInt(statusParts[0].content))
	if status != pkiStatusGranted && status != pkiStatusGrantedWithMods {
		return nil, &RejectionError{Status: status, Detail: statusFreeText(statusParts)}
	}
	if len(parts) < 2 {
		return nil, fmt.Errorf("tsa: status granted but no timeStampToken present")
	}

	tstInfo, certs, err := parseTimeStampToken(parts[1])
	if err != nil {
		return nil, fmt.Errorf("tsa: parsing timeStampToken: %w", err)
	}
	tstInfo.TokenDER = parts[1].raw
	tstInfo.SignerCertificates = certs
	return tstInfo, nil
}

// statusFreeText renders PKIFreeText (an optional SEQUENCE OF
// UTF8String following PKIStatus) for a log-only rejection detail.
func statusFreeText(statusParts []berNode) string {
	if len(statusParts) < 2 {
		return ""
	}
	texts, err := statusParts[1].children()
	if err != nil {
		return ""
	}
	var b strings.Builder
	for i, t := range texts {
		if i > 0 {
			b.WriteString("; ")
		}
		b.Write(t.content)
	}
	return b.String()
}

// parseTimeStampToken descends TimeStampToken (a CMS ContentInfo
// wrapping SignedData) to the TSTInfo it carries and the certificates
// its SignedData embeds.
func parseTimeStampToken(token berNode) (*Response, [][]byte, error) {
	ciParts, err := token.children()
	if err != nil || len(ciParts) < 2 {
		return nil, nil, fmt.Errorf("malformed ContentInfo")
	}
	explicitWrapper := ciParts[1]
	if explicitWrapper.class != classContext || explicitWrapper.tag != 0 {
		return nil, nil, fmt.Errorf("ContentInfo.content is not [0] EXPLICIT")
	}
	sdParts, err := explicitWrapper.children()
	if err != nil || len(sdParts) < 1 {
		return nil, nil, fmt.Errorf("malformed explicit wrapper")
	}
	sdChildren, err := sdParts[0].children()
	if err != nil || len(sdChildren) < 3 {
		return nil, nil, fmt.Errorf("malformed SignedData")
	}

	// encapContentInfo is sdChildren[2]: [eContentType, optional
	// [0] EXPLICIT eContent].
	eciParts, err := sdChildren[2].children()
	if err != nil || len(eciParts) < 2 {
		return nil, nil, fmt.Errorf("SignedData has no eContent (TSTInfo)")
	}
	eContentWrapper := eciParts[1]
	if eContentWrapper.class != classContext || eContentWrapper.tag != 0 {
		return nil, nil, fmt.Errorf("eContent is not [0] EXPLICIT")
	}
	eContentParts, err := eContentWrapper.children()
	if err != nil || len(eContentParts) < 1 {
		return nil, nil, fmt.Errorf("malformed eContent wrapper")
	}
	tstInfoDER, err := eContentParts[0].octetStringValue()
	if err != nil {
		return nil, nil, fmt.Errorf("decoding eContent OCTET STRING: %w", err)
	}
	resp, err := parseTSTInfo(tstInfoDER)
	if err != nil {
		return nil, nil, err
	}

	var certs [][]byte
	for _, c := range sdChildren[3:] {
		if c.class == classContext && c.tag == 0 { // certificates
			certChildren, err := c.children()
			if err == nil {
				for _, cc := range certChildren {
					certs = append(certs, cc.raw)
				}
			}
		}
	}
	return resp, certs, nil
}

// parseTSTInfo parses TSTInfo ::= SEQUENCE { version, policy,
// messageImprint, serialNumber, genTime, accuracy OPTIONAL,
// ordering OPTIONAL, nonce OPTIONAL, tsa [0] OPTIONAL,
// extensions [1] OPTIONAL } (RFC 3161 §2.4.2).
func parseTSTInfo(der []byte) (*Response, error) {
	root, _, err := parseBERValue(der)
	if err != nil {
		return nil, fmt.Errorf("parsing TSTInfo: %w", err)
	}
	fields, err := root.children()
	if err != nil || len(fields) < 5 {
		return nil, fmt.Errorf("TSTInfo has %d fields, want at least 5", len(fields))
	}
	miParts, err := fields[2].children()
	if err != nil || len(miParts) < 2 {
		return nil, fmt.Errorf("malformed messageImprint")
	}
	algParts, err := miParts[0].children()
	if err != nil || len(algParts) < 1 {
		return nil, fmt.Errorf("malformed messageImprint.hashAlgorithm")
	}
	var alg asn1.ObjectIdentifier
	if _, err := asn1.Unmarshal(algParts[0].raw, &alg); err != nil {
		return nil, fmt.Errorf("parsing hashAlgorithm OID: %w", err)
	}

	genTime, err := parseGeneralizedOrUTCTime(fields[4])
	if err != nil {
		return nil, fmt.Errorf("parsing genTime: %w", err)
	}

	resp := &Response{
		GenTime:            genTime,
		MessageImprintAlg:  alg,
		MessageImprintHash: miParts[1].content,
		SerialNumber:       berBigInt(fields[3].content),
	}
	for _, f := range fields[5:] {
		if f.class == classUniversal && f.tag == tagInteger {
			resp.Nonce = berBigInt(f.content)
		}
	}
	return resp, nil
}

func parseGeneralizedOrUTCTime(n berNode) (time.Time, error) {
	s := string(n.content)
	if n.tag == tagUTCTime {
		return time.Parse("060102150405Z0700", normalizeTimeString(s))
	}
	return time.Parse("20060102150405Z0700", normalizeTimeString(s))
}

// normalizeTimeString drops sub-second fractions (irrelevant to the
// ±10 minute skew check, F3 §6.5) and turns a bare "Z" into the form
// Go's reference layout above expects.
func normalizeTimeString(s string) string {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		j := i
		for j < len(s) && (s[j] == '.' || (s[j] >= '0' && s[j] <= '9')) {
			j++
		}
		s = s[:i] + s[j:]
	}
	if strings.HasSuffix(s, "Z") {
		return s
	}
	return s + "Z"
}

// berInt decodes a small BER/DER INTEGER's content as a plain int
// (two's complement), for fields (PKIStatus, version) known to be
// small.
func berInt(content []byte) int64 {
	var v int64
	for i, b := range content {
		if i == 0 && b&0x80 != 0 {
			v = -1
		}
		v = v<<8 | int64(b)
	}
	return v
}

// berBigInt decodes a BER/DER INTEGER's content as a big.Int (two's
// complement), for fields (serialNumber, nonce) that may exceed 64
// bits.
func berBigInt(content []byte) *big.Int {
	if len(content) == 0 {
		return big.NewInt(0)
	}
	n := new(big.Int).SetBytes(content)
	if content[0]&0x80 != 0 {
		full := new(big.Int).Lsh(big.NewInt(1), uint(len(content)*8))
		n.Sub(n, full)
	}
	return n
}
