package verify

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/trust/tsl"
)

func buildTestCAAndLeaf(t *testing.T, caName string) (ca, leaf *x509.Certificate) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: caName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(CA): %v", err)
	}
	ca, err = x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("ParseCertificate(CA): %v", err)
	}

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "Chain Test Leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate(leaf): %v", err)
	}
	leaf, err = x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("ParseCertificate(leaf): %v", err)
	}
	return ca, leaf
}

func TestChainTrustedGrantedService(t *testing.T) {
	ca, leaf := buildTestCAAndLeaf(t, "Chain Test CA")
	list := &tsl.List{
		Providers: []tsl.Provider{{
			Name: "Test Provider",
			Services: []tsl.Service{{
				Type:        "http://example.rs/TrstSvc/Svctype/CA/QC",
				Certificate: ca.Raw,
				Status:      "http://example.rs/TrstSvc/Svcstatus/granted",
				StatusStart: time.Now().Add(-24 * time.Hour),
			}},
		}},
	}
	if !ChainTrusted(leaf, list, time.Now(), caQCTypeSuffix) {
		t.Fatal("ChainTrusted = false, want true for a granted matching CA/QC service")
	}
}

func TestChainTrustedNoMatch(t *testing.T) {
	_, leaf := buildTestCAAndLeaf(t, "Chain Test CA")
	other, _ := buildTestCAAndLeaf(t, "Different CA")
	list := &tsl.List{
		Providers: []tsl.Provider{{
			Services: []tsl.Service{{
				Type:        "http://example.rs/TrstSvc/Svctype/CA/QC",
				Certificate: other.Raw,
				Status:      "http://example.rs/TrstSvc/Svcstatus/granted",
			}},
		}},
	}
	if ChainTrusted(leaf, list, time.Now(), caQCTypeSuffix) {
		t.Fatal("ChainTrusted = true, want false: no service matches leaf's issuer")
	}
}

func TestChainTrustedNilList(t *testing.T) {
	_, leaf := buildTestCAAndLeaf(t, "Chain Test CA")
	if ChainTrusted(leaf, nil, time.Now(), caQCTypeSuffix) {
		t.Fatal("ChainTrusted = true with a nil list, want false")
	}
}

func TestChainTrustedWrongServiceType(t *testing.T) {
	ca, leaf := buildTestCAAndLeaf(t, "Chain Test CA")
	list := &tsl.List{
		Providers: []tsl.Provider{{
			Services: []tsl.Service{{
				Type:        "http://example.rs/TrstSvc/Svctype/TSA/QTST", // wrong type
				Certificate: ca.Raw,
				Status:      "http://example.rs/TrstSvc/Svcstatus/granted",
			}},
		}},
	}
	if ChainTrusted(leaf, list, time.Now(), caQCTypeSuffix) {
		t.Fatal("ChainTrusted = true, want false: service is TSA/QTST, not CA/QC")
	}
}

func TestCheckChainTrustSetsBothFields(t *testing.T) {
	ca, leaf := buildTestCAAndLeaf(t, "Chain Test CA")
	tsaCA, tsaCert := buildTestCAAndLeaf(t, "TSA CA")
	list := &tsl.List{
		Providers: []tsl.Provider{{
			Services: []tsl.Service{
				{
					Type:        "http://example.rs/TrstSvc/Svctype/CA/QC",
					Certificate: ca.Raw,
					Status:      "http://example.rs/TrstSvc/Svcstatus/granted",
				},
				{
					Type:        "http://example.rs/TrstSvc/Svctype/TSA/QTST",
					Certificate: tsaCA.Raw,
					Status:      "http://example.rs/TrstSvc/Svcstatus/granted",
				},
			},
		}},
	}
	r := &Result{SignerCertificate: leaf, HasTimestamp: true, TimestampCertificate: tsaCert}
	CheckChainTrust(r, list, time.Now())
	if !r.SignerChainTrusted {
		t.Error("SignerChainTrusted = false, want true")
	}
	if !r.TimestampChainTrusted {
		t.Error("TimestampChainTrusted = false, want true")
	}
}
