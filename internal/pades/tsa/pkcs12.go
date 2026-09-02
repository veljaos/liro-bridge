package tsa

import (
	"crypto/tls"
	"fmt"

	"software.sslmate.com/src/go-pkcs12"
)

// LoadPKCS12ClientCert decodes a PKCS#12 file for TLS client
// certificate authentication — F3 §6.2's second Pošta test endpoint,
// which exercises the client-certificate auth path production TSAs
// actually require (the first endpoint's HTTP Basic auth is the easy
// case). This reuses software.sslmate.com/src/go-pkcs12 (D-032), whose
// justification now extends beyond the soft token and gentestkeys to
// this client, since a PFX is exactly the file format F3 §6.2 specifies
// — see [[D-040]].
func LoadPKCS12ClientCert(p12 []byte, password string) (tls.Certificate, error) {
	key, cert, err := pkcs12.Decode(p12, password)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("tsa: decoding PKCS#12: %w", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
		Leaf:        cert,
	}, nil
}
