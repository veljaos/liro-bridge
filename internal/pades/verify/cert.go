package verify

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"math/big"
)

// findCertificate locates, among certificates (raw DER, as embedded in
// the CMS), the one matching issuer (a raw DER Name) and serial —
// SignerInfo's issuerAndSerialNumber, the only SignerIdentifier form
// this project's signer ever produces (F3 §5.1).
func findCertificate(certificates [][]byte, issuer []byte, serial *big.Int) (*x509.Certificate, error) {
	for _, der := range certificates {
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			continue
		}
		if bytes.Equal(cert.RawIssuer, issuer) && cert.SerialNumber.Cmp(serial) == 0 {
			return cert, nil
		}
	}
	return nil, fmt.Errorf("no embedded certificate matches the signer's issuerAndSerialNumber")
}
