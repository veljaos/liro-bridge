package windowscng

// The real CNG signing plumbing (F2 §2): acquiring an NCRYPT_KEY_HANDLE
// for a certificate already in the current user's store, signing a
// digest with it, and releasing it correctly. This file is the thin,
// untestable layer that actually calls crypt32.dll/ncrypt.dll; the
// logic around it (digest validation, error mapping, fCallerFree
// bookkeeping) lives in session_core.go and errors.go, where it is
// unit-tested without Windows — mirroring enumerate_windows.go's split.

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/errs"
)

var (
	ncryptDLL = windows.NewLazySystemDLL("ncrypt.dll")

	procNCryptSignHash    = ncryptDLL.NewProc("NCryptSignHash")
	procNCryptFreeObject  = ncryptDLL.NewProc("NCryptFreeObject")
	procNCryptSetProperty = ncryptDLL.NewProc("NCryptSetProperty")
)

const (
	// cryptAcquireOnlyNCryptKeyFlag forces the modern CNG path.  Without
	// it a legacy CSP handle may come back, which signs through a
	// different API entirely (F2 §2.1).
	cryptAcquireOnlyNCryptKeyFlag = 0x00040000

	// certNCryptKeySpec is CERT_NCRYPT_KEY_SPEC. Anything else means a
	// legacy handle came back despite the flag above (F2 §2.1).
	certNCryptKeySpec = 0xFFFFFFFF

	// certEncodingType is X509_ASN_ENCODING | PKCS_7_ASN_ENCODING, the
	// encoding CertFindCertificateInStore expects.
	certEncodingType = 0x00000001 | 0x00010000

	// certFindHash is CERT_FIND_HASH / CERT_FIND_SHA1_HASH:
	// (CERT_COMPARE_SHA1_HASH << CERT_COMPARE_SHIFT).
	certFindHash = 1 << 16

	// bcryptPadPKCS1 is BCRYPT_PAD_PKCS1.
	bcryptPadPKCS1 = 0x00000002

	// nCryptWindowHandleProperty is NCRYPT_WINDOW_HANDLE_PROPERTY
	// (L"HWND Handle"). Set on the key handle so the OS PIN dialog is
	// parented to the agent's window (F2 §2.3) — zero in this phase,
	// since there is no window yet (see F5).
	nCryptWindowHandleProperty = "HWND Handle"
)

// cryptHashBlob mirrors CRYPT_HASH_BLOB.
type cryptHashBlob struct {
	cbData uint32
	pbData *byte
}

// bcryptPKCS1PaddingInfo mirrors BCRYPT_PKCS1_PADDING_INFO.
type bcryptPKCS1PaddingInfo struct {
	pszAlgID *uint16
}

// realConn implements ncryptConn over the real Windows APIs.
type realConn struct{}

func newConn() ncryptConn { return realConn{} }

// findAndAcquire implements ncryptConn.findAndAcquire (F2 §2.1).
func (realConn) findAndAcquire(thumbprintHex string) ([]byte, ncryptKeyHandle, bool, error) {
	hash, err := hex.DecodeString(thumbprintHex)
	if err != nil || len(hash) == 0 {
		return nil, 0, false, errs.New(errs.CodeCertNotFound, fmt.Errorf("invalid thumbprint %q: %w", thumbprintHex, err))
	}

	storeName, err := windows.UTF16PtrFromString("MY")
	if err != nil {
		return nil, 0, false, fmt.Errorf("encoding store name: %w", err)
	}
	store, err := windows.CertOpenStore(
		certStoreProvSystemW,
		0,
		0,
		certSystemStoreCurrentUser,
		uintptr(unsafe.Pointer(storeName)),
	)
	if err != nil {
		return nil, 0, false, fmt.Errorf("opening MY store: %w", err)
	}
	defer func() { _ = windows.CertCloseStore(store, certCloseStoreForce) }()

	blob := cryptHashBlob{cbData: uint32(len(hash)), pbData: &hash[0]}
	certCtx, err := windows.CertFindCertificateInStore(store, certEncodingType, 0, certFindHash, unsafe.Pointer(&blob), nil)
	if err != nil {
		// Any failure to find the certificate itself — most commonly
		// CRYPT_E_NOT_FOUND, which is not in F2 §2.4's table (that table
		// covers signing-operation failures, not lookup failures) — means
		// exactly one thing to a caller: no certificate with this
		// thumbprint exists in the store. mapStatus's generic
		// SIGN_FAILED fallback would be misleading here.
		return nil, 0, false, errs.New(errs.CodeCertNotFound, err)
	}
	defer func() { _ = windows.CertFreeCertificateContext(certCtx) }()

	der := make([]byte, certCtx.Length)
	copy(der, unsafe.Slice(certCtx.EncodedCert, certCtx.Length))

	var hKey windows.Handle
	var keySpec uint32
	var callerFree bool
	err = windows.CryptAcquireCertificatePrivateKey(certCtx, cryptAcquireOnlyNCryptKeyFlag, nil, &hKey, &keySpec, &callerFree)
	if err != nil {
		if errno, ok := err.(windows.Errno); ok { //nolint:errorlint // matching the identical pattern in enumerate_windows.go
			return nil, 0, false, mapStatus("CryptAcquireCertificatePrivateKey", uint32(errno))
		}
		return nil, 0, false, fmt.Errorf("CryptAcquireCertificatePrivateKey: %w", err)
	}

	if keySpec != certNCryptKeySpec {
		// CRYPT_ACQUIRE_ONLY_NCRYPT_KEY_FLAG should make this
		// unreachable, but F2 §2.1 requires treating it as an error
		// rather than assuming, and logging the value.
		slog.Warn("windowscng: CryptAcquireCertificatePrivateKey returned an unexpected key spec",
			"keySpec", fmt.Sprintf("0x%08X", keySpec), "want", "CERT_NCRYPT_KEY_SPEC")
		if callerFree {
			_, _, _ = procNCryptFreeObject.Call(uintptr(hKey))
		}
		return nil, 0, false, errs.WithDetails(errs.CodeSignFailed,
			fmt.Errorf("unexpected key spec 0x%08X, want CERT_NCRYPT_KEY_SPEC", keySpec),
			map[string]any{"keySpec": fmt.Sprintf("0x%08X", keySpec)})
	}

	return der, ncryptKeyHandle(hKey), callerFree, nil
}

// setWindowHandle implements ncryptConn.setWindowHandle (F2 §2.3).
func (realConn) setWindowHandle(key ncryptKeyHandle, hwnd uintptr) error {
	propName, err := windows.UTF16PtrFromString(nCryptWindowHandleProperty)
	if err != nil {
		return fmt.Errorf("encoding property name: %w", err)
	}
	r0, _, _ := procNCryptSetProperty.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(propName)),
		uintptr(unsafe.Pointer(&hwnd)),
		unsafe.Sizeof(hwnd),
		0,
	)
	runtime.KeepAlive(propName)
	if r0 != 0 {
		return fmt.Errorf("NCryptSetProperty(%s): status 0x%08X", nCryptWindowHandleProperty, uint32(r0))
	}
	return nil
}

// signHash implements ncryptConn.signHash (F2 §2.2): the two-call
// pattern, once to learn the signature size, once to fill it. The
// algorithm identifier pointer is kept alive across both calls via
// runtime.KeepAlive — letting the Go string it came from be collected
// mid-call is the exact mistake F2 §2.2 warns about.
func (realConn) signHash(key ncryptKeyHandle, digest []byte) ([]byte, error) {
	algID, err := windows.UTF16PtrFromString("SHA256") // BCRYPT_SHA256_ALGORITHM
	if err != nil {
		return nil, fmt.Errorf("encoding algorithm id: %w", err)
	}
	padding := bcryptPKCS1PaddingInfo{pszAlgID: algID}

	var size uint32
	r0, _, _ := procNCryptSignHash.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(&padding)),
		uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)),
		0, 0, uintptr(unsafe.Pointer(&size)),
		bcryptPadPKCS1,
	)
	runtime.KeepAlive(algID)
	if r0 != 0 {
		return nil, mapStatus("NCryptSignHash (size)", uint32(r0))
	}

	sig := make([]byte, size)
	r0, _, _ = procNCryptSignHash.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(&padding)),
		uintptr(unsafe.Pointer(&digest[0])), uintptr(len(digest)),
		uintptr(unsafe.Pointer(&sig[0])), uintptr(size), uintptr(unsafe.Pointer(&size)),
		bcryptPadPKCS1,
	)
	runtime.KeepAlive(algID)
	if r0 != 0 {
		return nil, mapStatus("NCryptSignHash", uint32(r0))
	}
	return sig[:size], nil
}

// freeKey implements ncryptConn.freeKey.
func (realConn) freeKey(key ncryptKeyHandle) error {
	r0, _, _ := procNCryptFreeObject.Call(uintptr(key))
	if r0 != 0 {
		return mapStatus("NCryptFreeObject", uint32(r0))
	}
	return nil
}
