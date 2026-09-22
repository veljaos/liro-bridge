//go:build windows

package pkcs11

import (
	"fmt"
	"runtime"
	"unsafe"
)

// signRaw is C_SignInit and C_Sign, and it is the only part of signing
// that is this platform's rather than this program's. Everything around
// it — which key, which certificate, what a failure means — is in
// sign.go and is the same on both platforms (D-349).
// sign runs C_SignInit and C_Sign over data with CKM_RSA_PKCS.
//
// Two calls for the length, as PKCS#11 requires: the first with a NULL
// pSignature to learn how many bytes the signature needs, the second to fill a
// buffer of that size.
func (s *session) signRaw(key ckULong, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("pkcs11: nothing to sign")
	}

	var p runtime.Pinner
	defer p.Unpin()

	// CK_MECHANISM, packed: mechanism at +0 (CK_ULONG, 4), pParameter at +4
	// (8), ulParameterLen at +12 (4) — the same 16-byte shape and the same
	// reason as CK_ATTRIBUTE's. CKM_RSA_PKCS takes no parameter.
	mech := make([]byte, 16)
	putU32(mech[0:], ckmRSAPKCS)
	putU64(mech[4:], 0)
	putU32(mech[12:], 0)
	p.Pin(&mech[0])

	if rv := ckr(s.m.call(iSignInit, uintptr(s.handle),
		uintptr(unsafe.Pointer(&mech[0])), uintptr(key))); rv != ckrOK {
		return nil, &ckrError{"C_SignInit", rv}
	}

	p.Pin(&data[0])
	var n uint32
	p.Pin(&n)
	if rv := ckr(s.m.call(iSign, uintptr(s.handle),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		0, uintptr(unsafe.Pointer(&n)))); rv != ckrOK {
		return nil, &ckrError{"C_Sign(size)", rv}
	}
	if n == 0 || n > maxSignatureLen {
		return nil, fmt.Errorf("pkcs11: C_Sign wants a %d-byte signature buffer, which this layer will not allocate", n)
	}

	sig := make([]byte, n)
	p.Pin(&sig[0])
	if rv := ckr(s.m.call(iSign, uintptr(s.handle),
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)),
		uintptr(unsafe.Pointer(&sig[0])), uintptr(unsafe.Pointer(&n)))); rv != ckrOK {
		return nil, &ckrError{"C_Sign", rv}
	}
	if int(n) > len(sig) {
		return nil, fmt.Errorf("pkcs11: C_Sign reported %d bytes into a %d-byte buffer", n, len(sig))
	}
	return sig[:n], nil
}
