package consent

import (
	"sort"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/trust/classify"
)

// Role is the certificate's function, shown next to its name (F5
// §5.2): SPEC §11.5 — two certificates on one card can share a subject
// byte-for-byte, and the role plus thumbprint tail are sometimes the
// only difference.
type Role string

const (
	RoleSigning        Role = "signing"
	RoleAuthentication Role = "authentication"
	RoleUnknown        Role = "unknown"
)

func roleFromPurpose(p classify.Purpose) Role {
	switch p {
	case classify.PurposeSigning:
		return RoleSigning
	case classify.PurposeAuthentication:
		return RoleAuthentication
	default:
		return RoleUnknown
	}
}

// CertificateOption is one row in the consent screen's certificate list
// (F5 §5.2). Every field the row needs to distinguish two
// identical-subject certificates is present directly — the page never
// has to re-derive anything from a raw certificate.
type CertificateOption struct {
	Thumbprint string

	// DisplayName is the signer's name (givenName + surname, SPEC
	// §11.7) — never localised (SPEC §9.3): a Cyrillic name stays
	// Cyrillic in an English interface.
	DisplayName string

	Role   Role
	Issuer string

	// ThumbprintTail is the last 8 characters of the SHA-1 thumbprint
	// (F5 §5.2 — "not optional; it is sometimes the only difference").
	ThumbprintTail string

	Qualified bool

	// Usable mirrors classify.Info.Usable. A signing certificate that
	// cannot be used right now — an absent card, an expired certificate
	// — is still included and shown disabled with its reason: that is a
	// real choice temporarily unavailable, and hiding it is what would
	// make the card look broken.
	//
	// A certificate that is not for signing at all no longer reaches
	// here: the caller drops it (classify.Info.HiddenByDefault),
	// reversing F1 §6.1 and F5 §5.2. See docs/decisions.md.
	Usable bool

	// DisabledReason is the errs.Code explaining Usable == false; empty
	// when Usable is true.
	DisabledReason errs.Code

	// IsTestKey marks a soft-token certificate (SPEC §16.6): must be
	// loudly, unmissably marked wherever it appears.
	IsTestKey bool
}

// thumbprintTail returns the last n characters of a thumbprint, or the
// whole string if it is shorter than n — a thumbprint this short would
// itself be a bug elsewhere, not something this display helper should
// panic over.
func thumbprintTail(thumbprint string, n int) string {
	if len(thumbprint) <= n {
		return thumbprint
	}
	return thumbprint[len(thumbprint)-n:]
}

// BuildCertificateOptions converts every classify.Info the caller knows
// about into a CertificateOption row. Nothing is filtered out here —
// the caller passes exactly the certificates it wants offered at all
// (classify.Info.HiddenByDefault is the one rule that decides which
// those are, and applying it is the caller's business, not this
// function's).
//
// Usable certificates sort to the top, unusable ones follow (Task 4,
// F5 first-real-run review): a real choice belongs above a row the
// user cannot act on, but that row still stays visible with its
// reason rather than being pushed out of sight entirely. The sort is
// stable, so within each group the caller's own order (e.g. Gather's
// enumeration order) is preserved.
func BuildCertificateOptions(certs []classify.Info) []CertificateOption {
	out := make([]CertificateOption, 0, len(certs))
	for _, c := range certs {
		out = append(out, CertificateOption{
			Thumbprint:     c.Thumbprint,
			DisplayName:    c.Subject.DisplayName,
			Role:           roleFromPurpose(c.Purpose),
			Issuer:         c.IssuerCN,
			ThumbprintTail: thumbprintTail(c.Thumbprint, 8),
			Qualified:      c.Qualification == classify.QualificationQualified,
			Usable:         c.Usable,
			DisabledReason: c.NotUsableReason,
			IsTestKey:      c.IsTestKey,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Usable && !out[j].Usable
	})
	return out
}
