package windowscng

import "log/slog"

// Provider names observed on a real machine with TrustEdgeID (MUP, PKS)
// and Nexus Personal (Halcom) middleware installed — F1 §3.5. Every
// Serbian-issuer certificate surfaces through Microsoft's own KSP; no
// vendor-specific provider was found.
const (
	providerSmartCardKSP  = "Microsoft Smart Card Key Storage Provider"
	providerBaseSmartCard = "Microsoft Base Smart Card Crypto Provider" // legacy CSP path
	providerSoftwareKSP   = "Microsoft Software Key Storage Provider"
)

// onHardware deduces whether a key lives on a smart card or token from its
// CNG/CSP provider name (F1 §3.4). An unrecognised provider is logged at
// info level and treated as software: if a CA ever ships its own KSP, the
// name should be visible in a user's log rather than silently guessed at.
func onHardware(provider string) bool {
	switch provider {
	case providerSmartCardKSP, providerBaseSmartCard:
		return true
	case providerSoftwareKSP:
		return false
	default:
		slog.Info("windowscng: certificate uses an unrecognised provider, treating as software-backed", "provider", provider)
		return false
	}
}
