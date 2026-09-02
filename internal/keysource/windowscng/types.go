// Package windowscng enumerates signing certificates from the Windows
// current-user certificate store via CNG (F1 §3).
//
// This package deliberately does not parse X.509 content: it only finds
// certificates and describes how the associated key is stored. Subject,
// issuer, validity and qualification are decided in internal/trust/classify
// from the raw DER this package returns — keeping this package free of
// X.509 logic is what keeps it a Windows-API package rather than a
// Windows-API package with certificate logic spilled into it (F1 §3.1).
package windowscng

// Certificate is one candidate signing certificate found on this machine.
type Certificate struct {
	// Thumbprint is the SHA-1 hash of the DER certificate, uppercase hex.
	// It is the identifier used everywhere in the agent and in the API.
	Thumbprint string

	// DER is the raw certificate.
	DER []byte

	// Provider is the CNG provider name, e.g.
	// "Microsoft Smart Card Key Storage Provider".
	Provider string

	// OnHardware is true when the key lives on a smart card or token,
	// deduced from the provider (F1 §3.4).
	OnHardware bool

	// KeyContainer is the CNG key container name. Needed later for
	// opening a signing session; carried now so nothing has to
	// re-enumerate.
	KeyContainer string
}
