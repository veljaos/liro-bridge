package pkcs11

import (
	"crypto/sha1"
	"crypto/x509"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// These tests need a real PKCS#11 module and, for most of them, a real card.
// They opt in through an environment variable rather than by looking in the
// places a module is usually installed, so that they cannot quietly start
// depending on one developer's machine — the pattern D-038 established for the
// real PDF fixtures and D-021 for real certificates.
//
//	LIRO_PKCS11_MODULE="C:\Program Files\TrustEdgeID\netsetpkcs11_x64.dll" go test ./internal/keysource/pkcs11/
//
// Every one of them is read-only. Nothing here logs in, so nothing here can
// spend a PIN attempt — which is a property of the package as it stands rather
// than of the tests: there is no login step yet (D-269).
func moduleForTest(t *testing.T) *module {
	t.Helper()
	path := os.Getenv("LIRO_PKCS11_MODULE")
	if path == "" {
		t.Skip("LIRO_PKCS11_MODULE is not set; skipping the real-module tests")
	}
	m, err := openModule(path)
	if err != nil {
		t.Fatalf("openModule(%s): %v", path, err)
	}
	t.Cleanup(func() {
		if err := m.close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return m
}

// TestTheModuleAnswersWithRecognisableStrings is the behavioural confirmation
// that the CK_FUNCTION_LIST index map is right, and it is deliberately not a
// comparison of index 3 against the exported C_GetFunctionList: SafeSign's
// export is a jmp rel32 thunk and does not match, where three other modules
// do.
func TestTheModuleAnswersWithRecognisableStrings(t *testing.T) {
	m := moduleForTest(t)
	info, err := m.info()
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if info.Manufacturer == "" || info.LibraryDescription == "" {
		t.Fatalf("C_GetInfo returned empty strings (%+v); the layout or the index map is wrong", info)
	}
	if !strings.Contains(info.CryptokiVersion, ".") {
		t.Errorf("cryptokiVersion %q does not look like a version", info.CryptokiVersion)
	}
	t.Logf("cryptoki=%s manufacturer=%q library=%q %s",
		info.CryptokiVersion, info.Manufacturer, info.LibraryDescription, info.LibraryVersion)
}

func TestTheTokenReportsItsFlagsAndPINLengths(t *testing.T) {
	m := moduleForTest(t)
	slots, err := m.slots(true)
	if err != nil {
		t.Fatalf("slots: %v", err)
	}
	if len(slots) == 0 {
		t.Skip("no slot reports a token present; is the card in the reader?")
	}
	for _, slot := range slots {
		ti, err := m.tokenInfo(slot)
		if err != nil {
			// A slot with a token present is not necessarily a card this
			// module can use. SafeSign answers CKR_TOKEN_NOT_RECOGNIZED here
			// for a MUP card, which is "not mine" rather than a failure.
			if rv, ok := asCKR(err); ok && rv == ckrTokenNotRecognized {
				t.Logf("slot %d: %s — not this module's card", slot, rv)
				continue
			}
			t.Fatalf("tokenInfo(%d): %v", slot, err)
		}
		if ti.Label == "" {
			t.Errorf("slot %d: empty label; the CK_TOKEN_INFO offsets are wrong", slot)
		}
		// SPEC §6.5.1's first clause turns on this flag, and the whole shape of
		// the login step turns on it, so the test says it out loud.
		t.Logf("slot %d: label=%q serial=%q flags=0x%X protectedAuthPath=%v loginRequired=%v minPin=%d maxPin=%d",
			slot, ti.Label, ti.SerialNumber, ti.Flags,
			ti.HasProtectedAuthenticationPath(), ti.LoginRequired(), ti.MinPINLen, ti.MaxPINLen)

		if ti.LoginRequired() && !ti.HasProtectedAuthenticationPath() {
			if ti.MinPINLen == 0 || ti.MaxPINLen == 0 || ti.MaxPINLen < ti.MinPINLen {
				t.Errorf("slot %d: this token needs a PIN from us and reports an unusable "+
					"length range (%d..%d); SPEC §6.5.1 requires this layer to enforce it",
					slot, ti.MinPINLen, ti.MaxPINLen)
			}
		}
	}
}

// TestTheMechanismListAnswersSection21 reads what the token offers. F11 §2.1
// turns on the difference: CKM_RSA_PKCS signs a pre-computed DigestInfo, which
// is what SignDigest has, and CKM_SHA256_RSA_PKCS hashes the data itself,
// which would be wrong here. Reading the list needs no login.
func TestTheMechanismListAnswersSection21(t *testing.T) {
	m := moduleForTest(t)
	slots, err := m.slots(true)
	if err != nil || len(slots) == 0 {
		t.Skip("no slot with a token present")
	}
	for _, slot := range slots {
		mechs, err := m.mechanisms(slot)
		if err != nil {
			if rv, ok := asCKR(err); ok && rv == ckrTokenNotRecognized {
				continue
			}
			t.Fatalf("mechanisms(%d): %v", slot, err)
		}
		has := func(want uint32) bool {
			for _, got := range mechs {
				if got == want {
					return true
				}
			}
			return false
		}
		t.Logf("slot %d: %d mechanisms; CKM_RSA_PKCS=%v CKM_SHA256_RSA_PKCS=%v",
			slot, len(mechs), has(ckmRSAPKCS), has(ckmSHA256RSAPKCS))

		// SPEC §18.8 forbids this program producing SHA-1 or MD5 whatever a
		// module offers. Offering them is not a defect in the module; using one
		// would be a defect here, so the test records rather than fails.
		for _, forbidden := range []struct {
			id   uint32
			name string
		}{
			{ckmMD5RSAPKCS, "CKM_MD5_RSA_PKCS"},
			{ckmSHA1RSAPKCS, "CKM_SHA1_RSA_PKCS"},
		} {
			if has(forbidden.id) {
				t.Logf("slot %d: offers %s, which SPEC §18.8 forbids this program producing",
					slot, forbidden.name)
			}
		}
		if !has(ckmRSAPKCS) {
			t.Errorf("slot %d offers no CKM_RSA_PKCS, which is the mechanism SignDigest needs", slot)
		}
	}
}

// TestCertificatesComeOffTheCardAndParse is the acceptance check for the whole
// binding: the packed CK_ATTRIBUTE marshalling, the two-call
// C_GetAttributeValue, and the find loop. A certificate that crypto/x509
// parses is evidence the bytes are right, where a call returning CKR_OK is
// not — three wrong template layouts return CKR_OK with a zero length.
func TestCertificatesComeOffTheCardAndParse(t *testing.T) {
	m := moduleForTest(t)
	slots, err := m.slots(true)
	if err != nil || len(slots) == 0 {
		t.Skip("no slot with a token present")
	}

	parsed := 0
	for _, slot := range slots {
		if _, err := m.tokenInfo(slot); err != nil {
			continue // not this module's card
		}
		s, err := m.openSession(slot)
		if err != nil {
			t.Fatalf("openSession(%d): %v", slot, err)
		}
		defer func() {
			if err := s.close(); err != nil {
				t.Errorf("session close: %v", err)
			}
		}()

		objects, err := s.certificateObjects()
		if err != nil {
			t.Fatalf("certificateObjects: %v", err)
		}
		for _, obj := range objects {
			der, err := s.attributeValue(obj, ckaValue)
			if err != nil {
				t.Fatalf("CKA_VALUE of object %d: %v", obj, err)
			}
			if len(der) == 0 {
				t.Errorf("object %d: CKA_VALUE is empty — the template layout is wrong, "+
					"and a wrong layout returns CKR_OK with a zero length rather than failing", obj)
				continue
			}
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				t.Errorf("object %d: %d bytes of CKA_VALUE do not parse as a certificate: %v",
					obj, len(der), err)
				continue
			}
			parsed++

			label, err := s.attributeValue(obj, ckaLabel)
			if err != nil {
				t.Errorf("CKA_LABEL of object %d: %v", obj, err)
			}
			sum := sha1.Sum(der)
			t.Logf("object %d: label=%q subject=%q serial=%X sha1=%s keyUsage=%d",
				obj, string(label), cert.Subject.CommonName, cert.SerialNumber.Bytes(),
				strings.ToUpper(hex.EncodeToString(sum[:])), cert.KeyUsage)
		}
	}
	if parsed == 0 {
		t.Fatal("no certificate was read and parsed; this test would pass for the wrong reason")
	}
}
