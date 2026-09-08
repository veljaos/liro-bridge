//go:build windows

package main

// The certificate source behind GET /v2/certificates.
//
// It is the same enumeration the agent's own certificate step uses —
// gatherInteractiveCertificates, which is cli.Gather, which is what
// "liro-bridge certs" itself prints — narrowed by the same one rule
// every listing in this program goes through (classify.Info's
// HiddenByDefault, via CertRow.Hidden). Three copies of that rule is
// how a listing and a window come to disagree about what a machine
// holds (D-108, D-149), and a caller reading the list is choosing from
// the same rows the person will be offered.
//
// What crosses the boundary is what internal/trust/classify produced
// and nothing else — see api.Certificate for why the certificate itself
// does not.

import (
	"context"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
)

// gatheredCertificates implements api.CertificateSource over the real
// Windows CNG store and, in a softtoken build, the soft token.
type gatheredCertificates struct{}

// Certificates implements api.CertificateSource.
func (gatheredCertificates) Certificates(ctx context.Context) ([]api.Certificate, error) {
	report, err := gatherInteractiveCertificates(ctx)
	if err != nil {
		return nil, err
	}
	infos := visibleCertificates(report)
	out := make([]api.Certificate, 0, len(infos))
	for _, info := range infos {
		out = append(out, api.Certificate{
			Thumbprint:      info.Thumbprint,
			DisplayName:     info.Subject.DisplayName,
			Issuer:          info.IssuerCN,
			Purpose:         certificatePurpose(info.Purpose),
			Qualified:       info.Qualification == classify.QualificationQualified,
			Usable:          info.Usable,
			NotUsableReason: info.NotUsableReason,
			IsTestKey:       info.IsTestKey,
		})
	}
	return out, nil
}

// certificatePurpose names the role KeyUsage gives a certificate, in the
// protocol's own stable vocabulary.
//
// Every row a listing shows today is a signing certificate — the
// authentication twin every Serbian card carries beside it is hidden by
// the same rule that hides it from the person (D-149). The field is
// carried anyway because it is what SPEC §11.5 requires every row to
// state, and because a caller reading "signing" has been told that
// rather than left to assume it.
func certificatePurpose(p classify.Purpose) string {
	switch p {
	case classify.PurposeSigning:
		return "signing"
	case classify.PurposeAuthentication:
		return "authentication"
	default:
		return "unknown"
	}
}
