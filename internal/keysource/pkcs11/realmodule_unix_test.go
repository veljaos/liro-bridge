//go:build linux

package pkcs11

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"os"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

// The Linux PKCS#11 layer against a real module.
//
// **SoftHSM, not a fake**: a real PKCS#11 implementation with a real
// PIN, real C_Login attempt counting and real CK_ULONG widths, and no
// hardware. F12 §5 asks for exactly this, because the layer it exercises
// had compiled on this platform and never run — and "it compiles" is
// what F11 measured three of four wrong struct layouts also do.
//
// It skips, saying why, where the environment has not been set up. What
// it needs is in `docs/f12-linux-session-5.md`: softhsm2 installed, a
// token initialised, and LIRO_SOFTHSM_PIN naming its PIN.
func softhsmModule(t *testing.T) (path, pin string) {
	t.Helper()
	pin = os.Getenv("LIRO_SOFTHSM_PIN")
	if pin == "" {
		t.Skip("LIRO_SOFTHSM_PIN is not set, so there is no token to talk to and this layer is untested here")
	}
	for _, p := range []string{
		"/usr/lib/softhsm/libsofthsm2.so",
		"/usr/lib64/softhsm/libsofthsm2.so",
		"/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so",
	} {
		if _, err := os.Stat(p); err == nil {
			return p, pin
		}
	}
	t.Skip("softhsm2 is not installed here")
	return "", ""
}

// TestTheLinuxLayerReachesARealToken walks the whole path this layer
// exists for, in the order the agent walks it, and ends at a signature
// verified against the certificate's own public key.
//
// **The verification is the test.** SPEC §16.4 and F12's own rules say
// never to conclude from a call returning bytes: F11 measured three of
// four candidate struct layouts returning CKR_OK, and a wrong layout
// here would produce a signature that verifies against nothing while
// every layer reports success.
func TestTheLinuxLayerReachesARealToken(t *testing.T) {
	path, pin := softhsmModule(t)

	m, err := openModule(path)
	if err != nil {
		t.Fatalf("openModule: %v", err)
	}
	defer func() { _ = m.close() }()

	info, err := m.info()
	if err != nil {
		t.Fatalf("C_GetInfo: %v", err)
	}
	t.Logf("module: %q %q cryptoki %s library %s",
		info.Manufacturer, info.LibraryDescription, info.CryptokiVersion, info.LibraryVersion)
	if info.Manufacturer == "" {
		t.Error("the module named no manufacturer, which is what a misread CK_INFO looks like")
	}

	slots, err := m.slots(true)
	if err != nil {
		t.Fatalf("C_GetSlotList: %v", err)
	}
	if len(slots) == 0 {
		t.Fatal("no slot has a token in it")
	}

	slot := slots[0]
	ti, err := m.tokenInfo(slot)
	if err != nil {
		t.Fatalf("C_GetTokenInfo: %v", err)
	}
	t.Logf("token: label=%q serial=%q minPIN=%d maxPIN=%d protectedPath=%v loginRequired=%v",
		ti.Label, ti.SerialNumber, ti.MinPINLen, ti.MaxPINLen,
		ti.HasProtectedAuthenticationPath(), ti.LoginRequired())

	// The flags are read rather than assumed, because clause 1 turns on
	// one of them and this is the third module this project has asked.
	if ti.HasProtectedAuthenticationPath() {
		t.Fatal("this token claims a protected authentication path, which would make the rest of this test measure nothing")
	}
	if !ti.LoginRequired() {
		t.Error("this token says no login is required, so the PIN path below proves nothing about a card")
	}
	if ti.MinPINLen == 0 || ti.MaxPINLen == 0 {
		t.Errorf("the token declares min/max PIN %d/%d, which is what a misread CK_TOKEN_INFO looks like",
			ti.MinPINLen, ti.MaxPINLen)
	}

	sess, err := m.openSession(slot)
	if err != nil {
		t.Fatalf("C_OpenSession: %v", err)
	}
	defer func() { _ = sess.close() }()

	// The certificate first: public, needs no login, and is what says
	// which key to use.
	certs, err := sess.certificateObjects()
	if err != nil {
		t.Fatalf("finding certificates: %v", err)
	}
	if len(certs) == 0 {
		t.Fatal("the token holds no certificate, so there is nothing to sign with")
	}
	der, err := sess.attributeValue(certs[0], ckaValue)
	if err != nil {
		t.Fatalf("reading CKA_VALUE: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("the bytes CKA_VALUE returned are not a certificate, which is what a wrong "+
			"CK_ATTRIBUTE layout produces: %v", err)
	}
	t.Logf("certificate: %s", cert.Subject)

	// The login, through the same seam the dialog fills: a callback
	// writing into a caller-owned buffer, never a function returning a
	// []byte (SPEC §6.5.1 clause 2).
	var sawRequest PINRequest
	var sawBufferLen int
	entry := func(dst []byte, req PINRequest) (int, error) {
		sawRequest, sawBufferLen = req, len(dst)
		return copy(dst, pin), nil
	}
	if err := sess.login(ti, entry, PINRequest{
		TokenLabel: ti.Label, TokenSerial: ti.SerialNumber, ModulePath: path,
	}); err != nil {
		t.Fatalf("C_Login: %v", err)
	}
	defer sess.logout()

	if sawRequest.MinLength != int(ti.MinPINLen) {
		t.Errorf("the screen was told the minimum is %d, the token says %d",
			sawRequest.MinLength, ti.MinPINLen)
	}
	// D-349: a token may declare more than this layer will allocate
	// for, and the buffer is the clamp rather than the declaration.
	wantBuf := int(ti.MaxPINLen)
	if wantBuf > MaxPINLength {
		wantBuf = MaxPINLength
	}
	if sawBufferLen != wantBuf {
		t.Errorf("the screen was given a %d-byte buffer, want %d (the token declares %d)",
			sawBufferLen, wantBuf, ti.MaxPINLen)
	}

	// The private key, which was invisible until the login.
	key, err := sess.privateKeyFor(certs[0])
	if err != nil {
		t.Fatalf("privateKeyFor: %v", err)
	}

	digest := sha256.Sum256([]byte("F12 §5: the Linux PKCS#11 layer, run rather than compiled"))
	prefixed, err := digestInfo(keysource.DigestSHA256, digest[:])
	if err != nil {
		t.Fatalf("digestInfo: %v", err)
	}
	sig, err := sess.signRaw(key, prefixed)
	if err != nil {
		t.Fatalf("C_Sign: %v", err)
	}
	if len(sig) == 0 {
		t.Fatal("C_Sign returned no bytes")
	}

	// **The independent check.** Not this package's, not PKCS#11's:
	// crypto/rsa, against the public key in the certificate the token
	// handed over.
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("the certificate carries a %T, not an RSA public key", cert.PublicKey)
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("the signature does not verify against the certificate's own public key — "+
			"which is exactly what a wrong struct layout produces while every call returns CKR_OK: %v", err)
	}
	t.Logf("a %d-byte signature verified against the certificate's public key", len(sig))
}

// TestAWrongPINIsOneAttemptAndNothingRetries is clause 5 against a
// module that actually counts.
//
// SoftHSM locks a token after a number of wrong user PINs exactly as a
// card does, so this deliberately makes **one** wrong attempt and
// asserts what came back. A layer that retried would spend three here
// and lock the token, and the test would find out by the next one
// failing — which is why this runs before nothing and is written to be
// read in that order.
func TestAWrongPINIsOneAttemptAndNothingRetries(t *testing.T) {
	path, pin := softhsmModule(t)

	m, err := openModule(path)
	if err != nil {
		t.Fatalf("openModule: %v", err)
	}
	defer func() { _ = m.close() }()
	slots, err := m.slots(true)
	if err != nil || len(slots) == 0 {
		t.Fatalf("slots: %v", err)
	}
	ti, err := m.tokenInfo(slots[0])
	if err != nil {
		t.Fatalf("tokenInfo: %v", err)
	}
	sess, err := m.openSession(slots[0])
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	defer func() { _ = sess.close() }()

	calls := 0
	wrong := pin[:len(pin)-1] + "0"
	if wrong == pin {
		wrong = pin[:len(pin)-1] + "1"
	}
	err = sess.login(ti, func(dst []byte, _ PINRequest) (int, error) {
		calls++
		return copy(dst, wrong), nil
	}, PINRequest{})
	if err == nil {
		t.Fatal("a wrong PIN was accepted")
	}
	if calls != 1 {
		t.Fatalf("the PIN screen was asked %d times for one login; clause 5 says nothing retries, ever", calls)
	}
	var ce *ckrError
	if !errors.As(err, &ce) {
		t.Fatalf("a wrong PIN produced %T (%v), not the module's own return value", err, err)
	}
	t.Logf("one wrong PIN, one attempt, %v", err)

	// And the right PIN still works afterwards, which is how this test
	// says it did not quietly spend the token's attempts.
	sess2, err := m.openSession(slots[0])
	if err != nil {
		t.Fatalf("openSession: %v", err)
	}
	defer func() { _ = sess2.close() }()
	if err := sess2.login(ti, func(dst []byte, _ PINRequest) (int, error) {
		return copy(dst, pin), nil
	}, PINRequest{}); err != nil {
		t.Fatalf("the correct PIN was refused after one wrong attempt: %v", err)
	}
	sess2.logout()
}

// TestTheLayoutIsTheCompilers is the guard against the thing F11
// measured: three of four hand-written CK_ATTRIBUTE layouts returned
// CKR_OK with a zero length, so a wrong layout does not announce
// itself. This file uses cgo precisely so that nobody writes a fourth,
// and this asserts the sizes Go is compiling against are the header's.
func TestTheLayoutIsTheCompilers(t *testing.T) {
	if got := len(ckULongBytes(1)); got != 8 {
		t.Errorf("a CK_ULONG encodes to %d bytes here; LP64 says 8", got)
	}
	// Round-trip the encoding the templates use, because a wrong
	// endianness is the other way a template silently matches nothing.
	b := ckULongBytes(0x0102030405060708)
	var back ckULong
	for i := len(b) - 1; i >= 0; i-- {
		back = back<<8 | ckULong(b[i])
	}
	if back != 0x0102030405060708 {
		t.Errorf("a CK_ULONG did not survive its own encoding: got %#x", back)
	}
}

// TestAnUnrecognisedSlotBesideTheTokenDoesNotHideIt is the listing the agent
// actually does, through Source rather than through the module directly.
//
// SoftHSM always presents one uninitialised slot after the ones that hold
// tokens, and on it C_GetTokenInfo answers while C_OpenSession returns
// CKR_TOKEN_NOT_RECOGNIZED. The installed agent listed **no certificates at
// all** from a token sitting in the slot beside it (D-355): one slot that was
// not this module's card failed the whole module. A reader holding a card
// another vendor's module does not know is the same shape on hardware.
//
// The precondition is asserted rather than assumed, so that the day SoftHSM
// stops doing this the test says so instead of passing for no reason.
func TestAnUnrecognisedSlotBesideTheTokenDoesNotHideIt(t *testing.T) {
	path, _ := softhsmModule(t)

	m, err := openModule(path)
	if err != nil {
		t.Fatalf("openModule: %v", err)
	}
	slots, err := m.slots(true)
	if err != nil {
		_ = m.close()
		t.Fatalf("C_GetSlotList: %v", err)
	}
	unrecognised := 0
	for _, slot := range slots {
		sess, err := m.openSession(slot)
		if err != nil {
			if isNotThisModulesToken(err) {
				unrecognised++
			}
			continue
		}
		_ = sess.close()
	}
	_ = m.close()
	if unrecognised == 0 {
		t.Skip("this SoftHSM presents no slot whose session is refused as not recognised, so the case is not here to test")
	}

	certs, err := NewSource(path).Enumerate(t.Context())
	if err != nil {
		t.Fatalf("Enumerate failed with %d unrecognised slot(s) beside the token: %v", unrecognised, err)
	}
	if len(certs) == 0 {
		t.Fatal("Enumerate returned no certificates although the token carries one")
	}

	// And the sign path finds its way past the same slot to the key.
	want := keysource.Thumbprint(certs[0].Thumbprint)
	sess, err := NewSource(path).WithPINEntry(func(_ []byte, _ PINRequest) (int, error) {
		return 0, ErrPINCancelled
	}).Open(t.Context(), want)
	if sess != nil {
		_ = sess.Close()
	}
	if err != nil && !errors.Is(err, ErrPINCancelled) {
		t.Fatalf("Open stopped before reaching the token's PIN: %v", err)
	}
}

// TestTheSlotSurveyReadsARealModulesFlags is the C half of open item A20's
// survey against a real module. SoftHSM's token slot says a token is present
// and is not a slot a card goes into — the shape of gnome-keyring's and
// p11-kit-trust's slots on every Ubuntu desktop — so the survey must count no
// reader. The first assertion is what makes the second mean something: a
// wrapper that read nothing would return zero flags and a survey of zeros.
func TestTheSlotSurveyReadsARealModulesFlags(t *testing.T) {
	path, _ := softhsmModule(t)
	m, err := openModule(path)
	if err != nil {
		t.Fatalf("openModule: %v", err)
	}
	defer func() { _ = m.close() }()

	present, err := m.slots(true)
	if err != nil || len(present) == 0 {
		t.Fatalf("no slot with a token (%v); the token this test needs is not there", err)
	}
	flags, err := m.slotFlags(present[0])
	if err != nil {
		t.Fatalf("slotFlags: %v", err)
	}
	if flags&ckfTokenPresent == 0 {
		t.Fatalf("slot %d is listed as holding a token and its flags (%#x) do not say so: C_GetSlotInfo was not read", present[0], flags)
	}
	if flags&ckfRemovableDevice != 0 {
		t.Errorf("SoftHSM's slot is flagged removable (%#x); the survey would count it as a reader", flags)
	}

	s, err := survey(context.Background(), m)
	if err != nil {
		t.Fatalf("survey: %v", err)
	}
	if s.ReaderSlots != 0 || s.CardsPresent != 0 {
		t.Errorf("survey of a module with no reader slot = %+v, want nothing counted", *s)
	}
}
