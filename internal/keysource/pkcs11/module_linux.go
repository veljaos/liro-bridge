//go:build linux

package pkcs11

/*
#cgo pkg-config: p11-kit-1
#cgo LDFLAGS: -ldl

#include <dlfcn.h>
#include <stdlib.h>
#include "pkcs11_linux.h"
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// The Linux PKCS#11 module layer.
//
// **cgo against the platform's own `pkcs11.h`, not a second hand-written
// layout.** F11 measured, on Windows, that three of four candidate
// `CK_ATTRIBUTE` layouts returned `CKR_OK` **with a zero length** — so a
// wrong layout does not announce itself, and the way to find out is not
// to try. Here `CK_ULONG` is eight bytes and the structures are
// natively aligned rather than packed to one, which makes every offset
// in `module_windows.go` wrong here rather than merely unavailable. The
// compiler computes them instead, and `TestTheLayoutIsTheCompilers`
// asserts it agrees with what Go thinks it is calling.
//
// **`RTLD_LOCAL`, never `RTLD_GLOBAL`** (F12 §2). A vendor module brings
// its own OpenSSL, and making those symbols visible to the rest of the
// process is how two OpenSSLs in one address space becomes one crash
// nobody can read.

// The PKCS#11 constants this layer names, **taken from the platform's
// own header rather than written down again**. That is the same reason
// this file uses cgo at all: F11 measured, on Windows, that three of
// four hand-written candidate layouts returned CKR_OK with a zero
// length, so a value this layer gets wrong does not announce itself.
const (
	ckaClass      = ckULong(C.CKA_CLASS)
	ckaID         = ckULong(C.CKA_ID)
	ckaValue      = ckULong(C.CKA_VALUE)
	ckaLabel      = ckULong(C.CKA_LABEL)
	ckoPrivateKey = ckULong(C.CKO_PRIVATE_KEY)
)

// findBatch is how many object handles C_FindObjects is asked for at a
// time. The same number as the Windows layer, for the same reason: a
// token holds a handful of objects and a batch larger than that buys
// nothing.
const findBatch = 32

// ckULong is what PKCS#11 calls CK_ULONG.
//
// **Eight bytes here and four on Windows** — Linux is LP64, Windows x64
// is LLP64 — which is F12 §1's second reason cgo is unavoidable on this
// platform and why the two module layers are separate files. It is an
// alias rather than a defined type so that sign.go, source.go and
// login.go can name it once and compile for both (D-349).
type ckULong = uint64

// module is one loaded PKCS#11 module, initialised.
//
// Not safe for concurrent use, for the same reason as the Windows one:
// the card is a single serial device, its driver serialises anyway, and
// concurrent access to smart-card APIs is a known source of
// driver-level failure (D-027).
type module struct {
	path   string
	handle unsafe.Pointer
	list   C.CK_FUNCTION_LIST_PTR
}

func openModule(path string) (*module, error) { return openModuleWith(path, false) }

// openModuleLocking is openModule with CKF_OS_LOCKING_OK, which F12 §2
// requires of the worker because it holds C_Initialize open across many
// requests.
func openModuleLocking(path string) (*module, error) { return openModuleWith(path, true) }

func openModuleWith(path string, osLocking bool) (*module, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	// RTLD_NOW so that a module wanting a symbol the distribution no
	// longer ships fails here, with a reason, rather than at the first
	// call into it — which F12 §10 names as the Linux failure mode
	// Windows does not have.
	handle := C.dlopen(cpath, C.RTLD_NOW|C.RTLD_LOCAL)
	if handle == nil {
		dlerr := C.GoString(C.dlerror())
		if le := classifyLoadFailure(path, dlerr); le != nil {
			return nil, fmt.Errorf("pkcs11: %s could not be loaded: %w", path, le)
		}
		return nil, fmt.Errorf("pkcs11: %s could not be loaded: %s", path, dlerr)
	}

	var list C.CK_FUNCTION_LIST_PTR
	if rv := C.liro_get_function_list(handle, &list); rv != C.CKR_OK {
		C.dlclose(handle)
		return nil, fmt.Errorf("pkcs11: %s has no usable C_GetFunctionList: %w", path, &ckrError{"C_GetFunctionList", ckr(rv)})
	}
	if list == nil {
		C.dlclose(handle)
		return nil, fmt.Errorf("pkcs11: %s returned a nil function list", path)
	}

	m := &module{path: path, handle: handle, list: list}
	if rv := ckr(C.liro_initialize(list, cbool(osLocking))); rv != ckrOK &&
		rv != ckrCryptokiAlreadyInitialized {
		C.dlclose(handle)
		return nil, fmt.Errorf("pkcs11: %s would not initialise: %w", path, &ckrError{"C_Initialize", rv})
	}
	return m, nil
}

func cbool(b bool) C.CK_BBOOL {
	if b {
		return C.CK_BBOOL(1)
	}
	return C.CK_BBOOL(0)
}

// close finalises and unloads the module.
func (m *module) close() error {
	if m.list != nil {
		C.liro_finalize(m.list)
		m.list = nil
	}
	if m.handle != nil {
		C.dlclose(m.handle)
		m.handle = nil
	}
	return nil
}

// moduleInfo is CK_INFO, decoded.
type moduleInfo struct {
	CryptokiVersion    string
	Manufacturer       string
	LibraryDescription string
	LibraryVersion     string
}

func (m *module) info() (moduleInfo, error) {
	var ci C.CK_INFO
	if rv := ckr(C.liro_get_info(m.list, &ci)); rv != ckrOK {
		return moduleInfo{}, &ckrError{"C_GetInfo", rv}
	}
	return moduleInfo{
		CryptokiVersion:    fmt.Sprintf("%d.%d", ci.cryptokiVersion.major, ci.cryptokiVersion.minor),
		Manufacturer:       padded(C.GoBytes(unsafe.Pointer(&ci.manufacturerID[0]), 32)),
		LibraryDescription: padded(C.GoBytes(unsafe.Pointer(&ci.libraryDescription[0]), 32)),
		LibraryVersion:     fmt.Sprintf("%d.%d", ci.libraryVersion.major, ci.libraryVersion.minor),
	}, nil
}

// tokenInfo is the part of CK_TOKEN_INFO this layer uses.
type tokenInfo struct {
	Label        string
	Manufacturer string
	Model        string
	SerialNumber string
	Flags        uint64
	MinPINLen    uint64
	MaxPINLen    uint64
}

// HasProtectedAuthenticationPath reports whether the module or the
// reader collects the PIN itself, which is the arrangement SPEC §6.5
// describes and §6.5.1 requires this layer to use wherever it is
// offered.
func (t tokenInfo) HasProtectedAuthenticationPath() bool {
	return t.Flags&uint64(C.CKF_PROTECTED_AUTHENTICATION_PATH) != 0
}

// LoginRequired reports whether the token needs a login before its
// private objects can be used.
func (t tokenInfo) LoginRequired() bool { return t.Flags&uint64(C.CKF_LOGIN_REQUIRED) != 0 }

// slots returns the slot identifiers this module offers.
//
// Two calls, which is PKCS#11's own convention: the first with a nil
// buffer asks how many there are, the second reads them. A module that
// grew a slot between the two is handled by taking the smaller count,
// rather than by reading past the buffer.
func (m *module) slots(tokenPresent bool) ([]ckULong, error) {
	var n C.CK_ULONG
	if rv := ckr(C.liro_get_slot_list(m.list, cbool(tokenPresent), nil, &n)); rv != ckrOK {
		return nil, &ckrError{"C_GetSlotList(size)", rv}
	}
	if n == 0 {
		return nil, nil
	}
	ids := make([]C.CK_SLOT_ID, int(n))
	if rv := ckr(C.liro_get_slot_list(m.list, cbool(tokenPresent), &ids[0], &n)); rv != ckrOK {
		return nil, &ckrError{"C_GetSlotList", rv}
	}
	if int(n) < len(ids) {
		ids = ids[:int(n)]
	}
	out := make([]ckULong, len(ids))
	for i, id := range ids {
		out[i] = ckULong(id)
	}
	return out, nil
}

func (m *module) tokenInfo(slot ckULong) (tokenInfo, error) {
	var ti C.CK_TOKEN_INFO
	if rv := ckr(C.liro_get_token_info(m.list, C.CK_SLOT_ID(slot), &ti)); rv != ckrOK {
		return tokenInfo{}, &ckrError{"C_GetTokenInfo", rv}
	}
	return tokenInfo{
		Label:        padded(C.GoBytes(unsafe.Pointer(&ti.label[0]), 32)),
		Manufacturer: padded(C.GoBytes(unsafe.Pointer(&ti.manufacturerID[0]), 32)),
		Model:        padded(C.GoBytes(unsafe.Pointer(&ti.model[0]), 16)),
		SerialNumber: padded(C.GoBytes(unsafe.Pointer(&ti.serialNumber[0]), 16)),
		Flags:        uint64(ti.flags),
		MinPINLen:    uint64(ti.ulMinPinLen),
		MaxPINLen:    uint64(ti.ulMaxPinLen),
	}, nil
}

func (m *module) mechanisms(slot ckULong) ([]ckULong, error) {
	var n C.CK_ULONG
	if rv := ckr(C.liro_get_mechanism_list(m.list, C.CK_SLOT_ID(slot), nil, &n)); rv != ckrOK {
		return nil, &ckrError{"C_GetMechanismList(size)", rv}
	}
	if n == 0 {
		return nil, nil
	}
	types := make([]C.CK_MECHANISM_TYPE, int(n))
	if rv := ckr(C.liro_get_mechanism_list(m.list, C.CK_SLOT_ID(slot), &types[0], &n)); rv != ckrOK {
		return nil, &ckrError{"C_GetMechanismList", rv}
	}
	if int(n) < len(types) {
		types = types[:int(n)]
	}
	out := make([]ckULong, len(types))
	for i, t := range types {
		out[i] = ckULong(t)
	}
	return out, nil
}

// session is one open PKCS#11 session on one slot.
type session struct {
	m      *module
	handle C.CK_SESSION_HANDLE
}

func (m *module) openSession(slot ckULong) (*session, error) {
	var h C.CK_SESSION_HANDLE
	if rv := ckr(C.liro_open_session(m.list, C.CK_SLOT_ID(slot), &h)); rv != ckrOK {
		return nil, &ckrError{"C_OpenSession", rv}
	}
	return &session{m: m, handle: h}, nil
}

func (s *session) close() error {
	if s.m == nil || s.m.list == nil {
		return nil
	}
	rv := ckr(C.liro_close_session(s.m.list, s.handle))
	s.m = nil
	if rv != ckrOK {
		return &ckrError{"C_CloseSession", rv}
	}
	return nil
}

// attribute is one CK_ATTRIBUTE, with its value in Go memory.
type attribute struct {
	typ   ckULong
	value []byte
}

// findObjects returns every object matching attrs.
//
// The template is built in C memory rather than marshalled by hand. That
// is the whole reason this file uses cgo: the Windows file computes
// offsets because it must, and F11 measured what a wrong guess looks
// like — CKR_OK and a zero length.
func (s *session) findObjects(attrs []attribute) ([]ckULong, error) {
	var tmpl *C.CK_ATTRIBUTE
	if len(attrs) > 0 {
		raw := C.calloc(C.size_t(len(attrs)), C.liro_sizeof_ck_attribute())
		if raw == nil {
			return nil, fmt.Errorf("pkcs11: no memory for a %d-attribute template", len(attrs))
		}
		defer C.free(raw)
		tmpl = (*C.CK_ATTRIBUTE)(raw)
		slice := unsafe.Slice(tmpl, len(attrs))
		for i, a := range attrs {
			slice[i]._type = C.CK_ATTRIBUTE_TYPE(a.typ)
			if len(a.value) > 0 {
				v := C.CBytes(a.value)
				defer C.free(v)
				slice[i].pValue = v
				slice[i].ulValueLen = C.CK_ULONG(len(a.value))
			}
		}
	}

	if rv := ckr(C.liro_find_init(s.m.list, s.handle, tmpl, C.CK_ULONG(len(attrs)))); rv != ckrOK {
		return nil, &ckrError{"C_FindObjectsInit", rv}
	}
	defer C.liro_find_final(s.m.list, s.handle)

	var out []ckULong
	batch := make([]C.CK_OBJECT_HANDLE, findBatch)
	for {
		var n C.CK_ULONG
		if rv := ckr(C.liro_find(s.m.list, s.handle, &batch[0], C.CK_ULONG(len(batch)), &n)); rv != ckrOK {
			return nil, &ckrError{"C_FindObjects", rv}
		}
		if n == 0 {
			return out, nil
		}
		for _, h := range batch[:int(n)] {
			out = append(out, ckULong(h))
		}
		if int(n) < len(batch) {
			return out, nil
		}
	}
}

// attributeValue reads one attribute of one object.
//
// Two calls again: the first asks the length with a nil buffer, the
// second reads it. **The length is checked rather than trusted** — a
// module that answers CKR_OK with the unavailable-length sentinel is
// exactly what F11 measured a wrong layout doing, and reading that many
// bytes would be the defect rather than the diagnosis.
func (s *session) attributeValue(obj ckULong, typ ckULong) ([]byte, error) {
	var n C.CK_ULONG = C.CK_ULONG(^C.CK_ULONG(0))
	if rv := ckr(C.liro_get_attribute(s.m.list, s.handle, C.CK_OBJECT_HANDLE(obj),
		C.CK_ATTRIBUTE_TYPE(typ), nil, &n)); rv != ckrOK {
		return nil, &ckrError{"C_GetAttributeValue(size)", rv}
	}
	if n == C.CK_ULONG(^C.CK_ULONG(0)) {
		return nil, fmt.Errorf("pkcs11: attribute 0x%08x is unavailable on this object", typ)
	}
	if n == 0 {
		return nil, nil
	}
	if n > 1<<20 {
		return nil, fmt.Errorf("pkcs11: attribute 0x%08x claims %d bytes, which is not a certificate", typ, n)
	}
	buf := make([]byte, int(n))
	if rv := ckr(C.liro_get_attribute(s.m.list, s.handle, C.CK_OBJECT_HANDLE(obj),
		C.CK_ATTRIBUTE_TYPE(typ), unsafe.Pointer(&buf[0]), &n)); rv != ckrOK {
		return nil, &ckrError{"C_GetAttributeValue", rv}
	}
	return buf[:int(n)], nil
}

// certificateObjects returns every X.509 certificate object in this
// session's token.
func (s *session) certificateObjects() ([]ckULong, error) {
	return s.findObjects([]attribute{
		{typ: ckULong(C.CKA_CLASS), value: ckULongBytes(ckULong(C.CKO_CERTIFICATE))},
		{typ: ckULong(C.CKA_CERTIFICATE_TYPE), value: ckULongBytes(ckULong(C.CKC_X_509))},
	})
}

// ckULongBytes encodes a CK_ULONG the way the platform lays one out.
//
// Eight bytes here against Windows' four, which is the difference F12 §1
// names; the size is read from the compiler rather than written as a
// literal, so a platform where it is neither does not silently truncate.
func ckULongBytes(v ckULong) []byte {
	n := int(C.liro_sizeof_ck_ulong())
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte(v >> (8 * i)) // little-endian, which is what x86-64 and aarch64 are
	}
	return b
}

func padded(b []byte) string { return strings.TrimRight(string(b), " \x00") }

// signRaw is C_SignInit and C_Sign over data with CKM_RSA_PKCS.
//
// Two calls for the length, as PKCS#11 requires: the first with a NULL
// pSignature to learn how many bytes the signature needs, the second to
// fill a buffer of that size. Everything around it — which key, which
// certificate, what a failure means — is in sign.go.
func (s *session) signRaw(key ckULong, data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("pkcs11: nothing to sign")
	}
	if rv := ckr(C.liro_sign_init(s.m.list, s.handle, C.CK_OBJECT_HANDLE(key))); rv != ckrOK {
		return nil, &ckrError{"C_SignInit", rv}
	}

	var n C.CK_ULONG
	if rv := ckr(C.liro_sign(s.m.list, s.handle,
		(*C.CK_BYTE)(unsafe.Pointer(&data[0])), C.CK_ULONG(len(data)),
		nil, &n)); rv != ckrOK {
		return nil, &ckrError{"C_Sign(size)", rv}
	}
	if n == 0 || n > maxSignatureLen {
		return nil, fmt.Errorf("pkcs11: C_Sign wants a %d-byte signature buffer, which this layer will not allocate", n)
	}

	sig := make([]byte, int(n))
	if rv := ckr(C.liro_sign(s.m.list, s.handle,
		(*C.CK_BYTE)(unsafe.Pointer(&data[0])), C.CK_ULONG(len(data)),
		(*C.CK_BYTE)(unsafe.Pointer(&sig[0])), &n)); rv != ckrOK {
		return nil, &ckrError{"C_Sign", rv}
	}
	if int(n) > len(sig) {
		return nil, fmt.Errorf("pkcs11: C_Sign reported %d bytes into a %d-byte buffer", n, len(sig))
	}
	return sig[:int(n)], nil
}
