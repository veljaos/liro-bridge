package verify

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"math/big"
	"strings"
	"time"
)

// verifyTimestamp parses tokenDER (a TimeStampToken: RFC 5652
// SignedData wrapping a TSTInfo, BER-tolerant per F3 §12.5) and checks
// that its messageImprint matches SHA-256 of signatureBytes — a
// signature timestamp token covers the signature value it accompanies,
// not the whole document. It populates r's Timestamp* fields.
func verifyTimestamp(r *Result, tokenDER, signatureBytes []byte) {
	root, _, err := readDER(tokenDER)
	if err != nil {
		r.fail("parsing timestamp token: %v", err)
		return
	}
	ciParts, err := root.sequence()
	if err != nil || len(ciParts) < 2 {
		r.fail("timestamp token is not a well-formed ContentInfo")
		return
	}
	if ciParts[1].class != classContext || ciParts[1].tag != 0 {
		r.fail("timestamp token's content is not [0] EXPLICIT")
		return
	}
	sdWrapper, err := ciParts[1].sequence()
	if err != nil || len(sdWrapper) != 1 {
		r.fail("malformed timestamp token SignedData wrapper")
		return
	}
	sdParts, err := sdWrapper[0].sequence()
	if err != nil || len(sdParts) < 3 {
		r.fail("malformed timestamp token SignedData")
		return
	}
	eciParts, err := sdParts[2].sequence()
	if err != nil || len(eciParts) < 2 {
		r.fail("timestamp token has no eContent (TSTInfo)")
		return
	}
	if eciParts[1].class != classContext || eciParts[1].tag != 0 {
		r.fail("timestamp token eContent is not [0] EXPLICIT")
		return
	}
	eContentWrapper, err := eciParts[1].sequence()
	if err != nil || len(eContentWrapper) != 1 {
		r.fail("malformed timestamp token eContent wrapper")
		return
	}
	tstInfoDER, err := eContentWrapper[0].octetStringValue()
	if err != nil {
		r.fail("decoding TSTInfo OCTET STRING: %v", err)
		return
	}

	tstRoot, _, err := readDER(tstInfoDER)
	if err != nil {
		r.fail("parsing TSTInfo: %v", err)
		return
	}
	fields, err := tstRoot.sequence()
	if err != nil || len(fields) < 5 {
		r.fail("TSTInfo has too few fields")
		return
	}
	miParts, err := fields[2].sequence()
	if err != nil || len(miParts) < 2 {
		r.fail("malformed TSTInfo.messageImprint")
		return
	}
	algParts, err := miParts[0].sequence()
	if err != nil || len(algParts) < 1 {
		r.fail("malformed messageImprint.hashAlgorithm")
		return
	}
	if !isSHA256OID(algParts[0].raw) {
		r.fail("timestamp messageImprint uses a hash algorithm other than SHA-256")
		return
	}
	imprint := miParts[1].content

	want := sha256.Sum256(signatureBytes)
	if !bytes.Equal(imprint, want[:]) {
		r.fail("timestamp messageImprint does not match SHA-256 of the signature it covers")
		return
	}

	r.TimestampSerial = new(big.Int).SetBytes(fields[3].content)
	genTime, err := parseGeneralizedTime(fields[4].content)
	if err != nil {
		r.fail("parsing TSTInfo.genTime: %v", err)
		return
	}
	r.TimestampGenTime = genTime.UTC().Format(time.RFC3339)

	// Best-effort: locate the TSA's own certificate among the token's
	// embedded certificates, for a caller that wants to check it
	// against the Trusted List separately. Its absence does not fail
	// this check — certReq is what makes it present, and F3 §8 lists
	// chain validation as its own, separate step.
	for _, f := range sdParts[3:] {
		if f.class == classContext && f.tag == 0 {
			certs, err := f.sequence()
			if err != nil {
				continue
			}
			for _, c := range certs {
				if cert, err := x509.ParseCertificate(c.raw); err == nil {
					r.TimestampCertificate = cert
				}
			}
		}
	}

	r.TimestampOK = true
}

func isSHA256OID(raw []byte) bool {
	// A minimal, independent OID comparison against 2.16.840.1.101.3.4.2.1
	// (id-sha256), avoiding a shared helper with any other package's OID
	// table.
	want := []byte{0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01}
	return bytes.Equal(raw, want)
}

// parseGeneralizedTime parses a GeneralizedTime's content bytes
// ("YYYYMMDDHHMMSS[.ffffff]Z", RFC 3161 requires UTC with a trailing Z).
func parseGeneralizedTime(content []byte) (time.Time, error) {
	s := string(content)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		j := i
		for j < len(s) && (s[j] == '.' || (s[j] >= '0' && s[j] <= '9')) {
			j++
		}
		s = s[:i] + s[j:]
	}
	if !strings.HasSuffix(s, "Z") {
		s += "Z"
	}
	return time.Parse("20060102150405Z0700", s)
}
