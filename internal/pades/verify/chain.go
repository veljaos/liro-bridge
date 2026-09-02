package verify

import (
	"bytes"
	"crypto/x509"
	"strings"
	"time"

	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// caQCTypeSuffix and tsaQTSTTypeSuffix are the ServiceTypeIdentifier
// suffixes for a qualified-certificate CA and a qualified timestamping
// service (ETSI TS 119 612), matched independently of
// internal/trust/tsl.Service.IsCA (which only recognises the CA/QC
// suffix) since this package also needs the TSA/QTST one.
const (
	caQCTypeSuffix    = "/Svctype/CA/QC"
	tsaQTSTTypeSuffix = "/Svctype/TSA/QTST"
)

// ChainTrusted implements F3 §8 point 6 for one certificate: it reports
// whether cert's issuer matches a service of the given type
// (caQCTypeSuffix or tsaQTSTTypeSuffix) that was granted in list at
// time at. This is deliberately independent of
// internal/trust/classify's identically-purposed issuer-matching logic
// (D-013) — that code is reached from this project's certificate-listing
// and signing side, never from verification, and F3 §8 asks for the
// verifier to share no code with the signing pipeline. Using
// internal/trust/tsl's own List/Service types directly is not the kind
// of sharing that rule is about: tsl is F1's independently developed
// and tested Trusted List client, a trust data source, not part of the
// CMS/ByteRange signing path.
func ChainTrusted(cert *x509.Certificate, list *tsl.List, at time.Time, typeSuffix string) bool {
	if list == nil || cert == nil {
		return false
	}
	for _, p := range list.Providers {
		for _, svc := range p.Services {
			if !strings.HasSuffix(svc.Type, typeSuffix) {
				continue
			}
			if len(svc.Certificate) == 0 {
				continue
			}
			svcCert, err := x509.ParseCertificate(svc.Certificate)
			if err != nil {
				continue
			}
			if !bytes.Equal(svcCert.RawSubject, cert.RawIssuer) {
				continue
			}
			if tsl.StatusIsGranted(svc.StatusAt(at)) {
				return true
			}
		}
	}
	return false
}

// CheckChainTrust runs ChainTrusted for both the signer certificate
// (against the CA/QC service type) and, if present, the timestamp
// certificate (against the TSA/QTST service type), setting r's
// SignerChainTrusted and TimestampChainTrusted fields. It is a separate
// call, not folded into VerifySignature, so a caller with no Trusted
// List on hand (every test-key signature this project's own CI
// produces) can still get the other five checks without either field
// defaulting to a misleading true.
func CheckChainTrust(r *Result, list *tsl.List, at time.Time) {
	if r.SignerCertificate != nil {
		r.SignerChainTrusted = ChainTrusted(r.SignerCertificate, list, at, caQCTypeSuffix)
	}
	if r.TimestampCertificate != nil {
		r.TimestampChainTrusted = ChainTrusted(r.TimestampCertificate, list, at, tsaQTSTTypeSuffix)
	}
}
