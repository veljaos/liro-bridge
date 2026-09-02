package classify

import (
	"bytes"
	"crypto/x509"
	"time"

	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// qcWithQSCDQualifier is the TSL qualifier URI suffix meaning the
// service's certificates are backed by a QSCD (F1 §4.3/§5.1).
const qcWithQSCDQualifier = "QCWithQSCD"

// qualify decides Qualification and the TSL-derived half of OnQSCD for
// cert against list at reference time now.
//
// SPEC §11.1: a certificate is qualified if its chain terminates at a
// service that was granted in the TSL at the time of signing — the
// Trusted List is the primary test, never an OID or qcStatements
// heuristic (those are supporting evidence only, folded in by the
// caller). F1 §5.4: in this phase, "chain" means only what's already
// available — cert's own Issuer bytes compared against each CA/QC
// service's certificate Subject bytes. AIA fetching and full chain
// completion are out of scope until F3.
func qualify(cert *x509.Certificate, list *tsl.List, now time.Time) (Qualification, bool) {
	if list == nil {
		return QualificationUnknown, false
	}

	for _, p := range list.Providers {
		for _, svc := range p.Services {
			if !svc.IsCA() || len(svc.Certificate) == 0 {
				continue
			}
			issuerCert, err := x509.ParseCertificate(svc.Certificate)
			if err != nil {
				continue // a malformed service certificate is not this certificate's issuer
			}
			if !bytes.Equal(cert.RawIssuer, issuerCert.RawSubject) {
				continue
			}
			if !tsl.StatusIsGranted(svc.StatusAt(now)) {
				continue // matched the issuer, but that service was not granted at the reference time
			}
			return QualificationQualified, svc.HasQualifier(qcWithQSCDQualifier)
		}
	}
	return QualificationNotQualified, false
}
