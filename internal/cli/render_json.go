package cli

import (
	"encoding/json"
	"encoding/pem"
	"io"
	"time"

	"github.com/veljaos/liro-bridge/internal/trust/classify"
	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

// jsonReport is the --json shape (F1 §6.1): the same data as the
// human-readable report, machine-readable, for scripting and testing.
// Field names are stable and independent of locale — this is data, not
// display text.
type jsonReport struct {
	Readers      []jsonReader `json:"readers"`
	Certificates []jsonCert   `json:"certificates"`
	TrustedList  jsonTSL      `json:"trustedList"`
}

type jsonReader struct {
	Name        string `json:"name"`
	CardPresent bool   `json:"cardPresent"`
}

type jsonCert struct {
	Thumbprint      string `json:"thumbprint"`
	DisplayName     string `json:"displayName"`
	IssuerCN        string `json:"issuerCN"`
	Purpose         string `json:"purpose"`
	Qualification   string `json:"qualification"`
	OnQSCD          bool   `json:"onQSCD"`
	OnHardware      bool   `json:"onHardware"`
	Usable          bool   `json:"usable"`
	NotUsableReason string `json:"notUsableReason,omitempty"`
	NotBefore       string `json:"notBefore"`
	NotAfter        string `json:"notAfter"`
	Hidden          bool   `json:"hidden"`

	// IsTestKey marks a soft-token certificate (F2 §3.1, SPEC §16.6) —
	// always false for a real, hardware-backed certificate.
	IsTestKey bool `json:"isTestKey"`

	// PEM is the PEM-encoded certificate (F2 §6.1): the external
	// OpenSSL verification recipe reads the public key from here, via
	// `certs --json | jq -r '.certificates[0].pem' | openssl x509
	// -pubkey -noout`.
	PEM string `json:"pem"`
}

type jsonTSL struct {
	Source   string `json:"source"`
	Sequence int    `json:"sequence"`
	IssuedAt string `json:"issuedAt"`
	AgeDays  int    `json:"ageDays"`
	Stale    bool   `json:"stale"`
}

// RenderJSON writes report as JSON, including rows hidden from the
// default text view (each carries its own "hidden" flag) — --all only
// affects RenderText's filtering, not what --json reports (F1 §6.1: the
// same data, machine-readable).
func RenderJSON(w io.Writer, report Report, now time.Time) error {
	out := jsonReport{
		Readers:      make([]jsonReader, 0, len(report.Readers)),
		Certificates: make([]jsonCert, 0, len(report.Certificates)),
	}
	for _, r := range report.Readers {
		out.Readers = append(out.Readers, jsonReader{Name: r.Name, CardPresent: r.CardPresent})
	}
	for _, row := range report.Certificates {
		out.Certificates = append(out.Certificates, jsonCert{
			Thumbprint:      row.Info.Thumbprint,
			DisplayName:     row.Info.Subject.DisplayName,
			IssuerCN:        row.Info.IssuerCN,
			Purpose:         purposeString(row.Info.Purpose),
			Qualification:   qualificationString(row.Info.Qualification),
			OnQSCD:          row.Info.OnQSCD,
			OnHardware:      row.OnHardware,
			Usable:          row.Info.Usable,
			NotUsableReason: string(row.Info.NotUsableReason),
			NotBefore:       row.Info.NotBefore.Format(time.RFC3339),
			NotAfter:        row.Info.NotAfter.Format(time.RFC3339),
			Hidden:          row.Hidden(),
			IsTestKey:       row.Info.IsTestKey,
			PEM:             certPEM(row.DER),
		})
	}
	age := now.Sub(report.TSL.IssuedAt)
	out.TrustedList = jsonTSL{
		Source:   sourceString(report.TSL.Source),
		Sequence: report.TSL.Sequence,
		IssuedAt: report.TSL.IssuedAt.Format(time.RFC3339),
		AgeDays:  int(age.Hours() / 24),
		Stale:    age > staleWarningAge,
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func purposeString(p classify.Purpose) string {
	switch p {
	case classify.PurposeSigning:
		return "signing"
	case classify.PurposeAuthentication:
		return "authentication"
	default:
		return "unknown"
	}
}

func qualificationString(q classify.Qualification) string {
	switch q {
	case classify.QualificationQualified:
		return "qualified"
	case classify.QualificationNotQualified:
		return "notQualified"
	default:
		return "unknown"
	}
}

func sourceString(s tsl.SourceKind) string { return s.String() }

// certPEM PEM-encodes der, or returns "" when der is empty (a row built
// without DER, which does not happen for real rows but keeps this
// total rather than panicking on a zero-value CertRow in a test).
func certPEM(der []byte) string {
	if len(der) == 0 {
		return ""
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
