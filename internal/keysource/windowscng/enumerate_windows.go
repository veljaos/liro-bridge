package windowscng

// Certificate enumeration from the Windows current-user certificate
// store via crypt32.dll (F1 §3.2). This file is the thin, untestable
// layer that actually calls the Windows API; the logic around it
// (provider-to-OnHardware mapping, thumbprint computation) lives in
// provider.go and thumbprint.go, where it is unit-tested without
// Windows.

import (
	"context"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	crypt32DLL = windows.NewLazySystemDLL("crypt32.dll")

	procCertGetCertificateContextProperty = crypt32DLL.NewProc("CertGetCertificateContextProperty")
)

const (
	// certStoreProvSystemW selects a system store by its well-known
	// name (here "MY"), per CertOpenStore's CERT_STORE_PROV_SYSTEM_W.
	certStoreProvSystemW = 10

	// certSystemStoreCurrentUser scopes the store to the current user's
	// profile. F1 §3.2: smart card certificates propagate to the user
	// store, not CERT_SYSTEM_STORE_LOCAL_MACHINE.
	certSystemStoreCurrentUser = 1 << 16

	certKeyProvInfoPropID = 2 // CERT_KEY_PROV_INFO_PROP_ID
	certCloseStoreForce   = 1 // CERT_CLOSE_STORE_FORCE_FLAG

	// cryptENotFound (0x80092004) from CertGetCertificateContextProperty
	// means the certificate has no associated private key — normal for
	// CA certificates and imported public certificates sharing the same
	// store (F1 §3.3). Not an error; the certificate is skipped.
	cryptENotFound = 0x80092004
)

// cryptKeyProvInfo mirrors CRYPT_KEY_PROV_INFO. Field order and pointer
// widths are fixed by the Windows ABI (F1 §3.2).
type cryptKeyProvInfo struct {
	containerName  *uint16
	provName       *uint16
	provType       uint32
	flags          uint32
	provParamCount uint32
	provParam      uintptr
	keySpec        uint32
}

// Enumerate lists every certificate with an associated private key in
// the current user's "MY" store. It contains no X.509 parsing — see the
// package doc comment.
func Enumerate(_ context.Context) ([]Certificate, error) {
	storeName, err := windows.UTF16PtrFromString("MY")
	if err != nil {
		return nil, fmt.Errorf("encoding store name: %w", err)
	}

	store, err := windows.CertOpenStore(
		certStoreProvSystemW,
		0,
		0,
		certSystemStoreCurrentUser,
		uintptr(unsafe.Pointer(storeName)),
	)
	if err != nil {
		return nil, fmt.Errorf("opening MY store: %w", err)
	}
	defer func() { _ = windows.CertCloseStore(store, certCloseStoreForce) }()

	var out []Certificate
	var prev *windows.CertContext
	for {
		ctx, err := windows.CertEnumCertificatesInStore(store, prev)
		if err != nil || ctx == nil {
			break // enumeration exhausted — not an error condition
		}
		prev = ctx

		provInfo, ok, err := keyProvInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("reading key provider info: %w", err)
		}
		if !ok {
			continue // no private key — F1 §3.3
		}

		// The certificate bytes belong to the store and are freed when
		// it closes; copy them before that happens (F1 §3.2 step 4).
		der := make([]byte, ctx.Length)
		copy(der, unsafe.Slice(ctx.EncodedCert, ctx.Length))

		provName := windows.UTF16PtrToString(provInfo.provName)
		container := windows.UTF16PtrToString(provInfo.containerName)

		out = append(out, Certificate{
			Thumbprint:   thumbprint(der),
			DER:          der,
			Provider:     provName,
			OnHardware:   onHardware(provName),
			KeyContainer: container,
		})
	}
	return out, nil
}

// isNoPrivateKey reports whether err is CRYPT_E_NOT_FOUND, which
// CertGetCertificateContextProperty returns when a certificate has no
// associated key-provider property — normal, since CA certificates and
// imported public certificates share the same store (F1 §3.3). Split
// out from keyProvInfo so the mapping itself is testable without a real
// syscall: a windows.Errno is a plain uintptr-based value, constructible
// directly in a test.
func isNoPrivateKey(err error) bool {
	errno, ok := err.(windows.Errno)
	return ok && uint32(errno) == cryptENotFound
}

// keyProvInfo reads CERT_KEY_PROV_INFO_PROP_ID via the two-call
// buffer-sizing pattern (F1 §3.2). ok is false, with no error, when the
// certificate has no private key (CRYPT_E_NOT_FOUND).
func keyProvInfo(ctx *windows.CertContext) (*cryptKeyProvInfo, bool, error) {
	var size uint32
	r0, _, callErr := procCertGetCertificateContextProperty.Call(
		uintptr(unsafe.Pointer(ctx)),
		certKeyProvInfoPropID,
		0,
		uintptr(unsafe.Pointer(&size)),
	)
	if r0 == 0 {
		if isNoPrivateKey(callErr) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("CertGetCertificateContextProperty (size): %w", callErr)
	}

	buf := make([]byte, size)
	r0, _, callErr = procCertGetCertificateContextProperty.Call(
		uintptr(unsafe.Pointer(ctx)),
		certKeyProvInfoPropID,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r0 == 0 {
		return nil, false, fmt.Errorf("CertGetCertificateContextProperty: %w", callErr)
	}
	return (*cryptKeyProvInfo)(unsafe.Pointer(&buf[0])), true, nil
}
