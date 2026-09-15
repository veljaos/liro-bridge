package pkcs11

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// The struct layouts below are measured on Windows x64 rather than assumed,
// and they are the reason this file is Windows-only rather than merely
// Windows-first:
//
//   - CK_ULONG is 4 bytes. Windows is LLP64, and the Windows PKCS#11 headers
//     type CK_ULONG as unsigned long int. On F13's Linux it will be 8, so
//     every offset here is wrong there rather than merely unavailable.
//   - Structs are packed to one byte (#pragma pack(1) in the Windows header),
//     so nothing is padded to its natural alignment.
//   - CK_FUNCTION_LIST therefore begins with a two-byte CK_VERSION and its
//     function pointers start at offset +2, every one of them unaligned.
//     amd64 loads those happily; a platform that trapped unaligned loads would
//     need byte-wise reads.
//
// Confirmed indirectly and soundly: CK_INFO's libraryDescription parses
// cleanly at offset 34+4, and would begin with four NUL bytes at 34+8.
const (
	ckULongSize = 4  // CK_ULONG on Windows x64
	ckPtrSize   = 8  // CK_VOID_PTR
	ckAttrSize  = 16 // CK_ATTRIBUTE: type at +0 (4), pValue at +4 (8), ulValueLen at +12 (4)

	ckInfoSize      = 72  // CK_INFO
	ckTokenInfoSize = 160 // CK_TOKEN_INFO

	ckFunctionListBase = 2 // past the CK_VERSION
)

// Index into CK_FUNCTION_LIST, counting the function pointers only. PKCS#11
// v2.40 §3.6 fixes this order and it is normative.
//
// Written with iota rather than as a const block with one explicit value:
// an explicit first value followed by bare names repeats rather than
// increments, which in this project's own earlier PKCS#11 work made every
// index 12 and turned C_FindObjectsInit into C_OpenSession — producing
// CKR_ARGUMENTS_BAD from two modules and a process crash from a third, all of
// which read like facts about the modules.
//
// It is deliberately NOT confirmed by comparing index 3 against the exported
// C_GetFunctionList: SafeSign's export is a jmp rel32 thunk and does not
// match, where three other modules do. moduleInfo's strings are the check that
// actually holds.
const (
	iInitialize = iota
	iFinalize
	iGetInfo
	iGetFunctionList
	iGetSlotList
	iGetSlotInfo
	iGetTokenInfo
	iGetMechanismList
	iGetMechanismInfo
	iInitToken
	iInitPIN
	iSetPIN
	iOpenSession
	iCloseSession
	iCloseAllSessions
	iGetSessionInfo
	iGetOperationState
	iSetOperationState
	iLogin
	iLogout
	iCreateObject
	iCopyObject
	iDestroyObject
	iGetObjectSize
	iGetAttributeValue
	iSetAttributeValue
	iFindObjectsInit
	iFindObjects
	iFindObjectsFinal
	iEncryptInit
	iEncrypt
	iEncryptUpdate
	iEncryptFinal
	iDecryptInit
	iDecrypt
	iDecryptUpdate
	iDecryptFinal
	iDigestInit
	iDigest
	iDigestUpdate
	iDigestKey
	iDigestFinal
	iSignInit
	iSign
)

// The eleven names between C_FindObjectsFinal and C_SignInit are declared
// rather than skipped with a count. This layer calls none of them — it does
// not encrypt, decrypt or digest, and PKCS#11's own digesting is the mechanism
// F11 §2.1 forbids using — but writing `iSignInit = iFindObjectsFinal + 14`
// would be the same arithmetic with the working shown once and then trusted,
// and this const block's own comment records what that cost the last time:
// every index came out 12, C_FindObjectsInit became C_OpenSession, and two
// modules answered CKR_ARGUMENTS_BAD while a third crashed the process. Naming
// each one keeps iota doing the counting.

// CK_TOKEN_INFO flags.
const (
	ckfRNG                         = 0x00000001
	ckfWriteProtected              = 0x00000002
	ckfLoginRequired               = 0x00000004
	ckfUserPINInitialized          = 0x00000008
	ckfProtectedAuthenticationPath = 0x00000100
	ckfTokenInitialized            = 0x00000400
	ckfUserPINCountLow             = 0x00010000
	ckfUserPINFinalTry             = 0x00020000
	ckfUserPINLocked               = 0x00040000
)

// Session flags, object classes and the attribute types this layer reads.
const (
	ckfSerialSession = 0x00000004

	ckoCertificate = 0x00000001
	ckoPublicKey   = 0x00000002
	ckoPrivateKey  = 0x00000003

	ckaClass    = 0x00000000
	ckaLabel    = 0x00000003
	ckaValue    = 0x00000011
	ckaID       = 0x00000102
	ckaCertType = 0x00000080

	ckcX509 = 0x00000000
)

// Mechanisms. F11 §2.1's distinction is the one that matters here and getting
// it wrong is invisible: the signature verifies against nothing while every
// layer reports success.
//
//   - ckmRSAPKCS signs a pre-built DigestInfo, which is what SignDigest is
//     handed. This is the one to use.
//   - ckmSHA256RSAPKCS expects the data and hashes it itself, which would sign
//     a hash of a hash.
//
// The last two are here so that this layer can recognise what a module offers
// and never ask for it: SPEC §18.8 forbids producing SHA-1 or MD5 anywhere,
// whatever a module is willing to do.
const (
	ckmRSAPKCS       = 0x00000001
	ckmSHA256RSAPKCS = 0x00000040
	ckmMD5RSAPKCS    = 0x00000005
	ckmSHA1RSAPKCS   = 0x00000006
)

// findBatch is how many object handles C_FindObjects is asked for at a time.
// A Serbian card carries two certificates; the batch is larger than that so
// that one call normally suffices, and small enough to be an ordinary
// allocation if a module ever reports many.
const findBatch = 32

// module is one loaded PKCS#11 module, initialised.
//
// Not safe for concurrent use. The card is a single serial device and its
// driver serialises anyway, so parallelism here buys no throughput and
// concurrent access to smart card APIs is a known source of driver-level
// failures (D-027). The orchestration layer owns serialisation, as it does for
// every other keysource.
type module struct {
	path   string
	handle syscall.Handle
	list   uintptr
	pinner runtime.Pinner
}

// moduleInfo is CK_INFO, decoded.
type moduleInfo struct {
	CryptokiVersion    string
	Manufacturer       string
	LibraryDescription string
	LibraryVersion     string
}

// tokenInfo is the part of CK_TOKEN_INFO this layer uses.
type tokenInfo struct {
	Label        string
	Manufacturer string
	Model        string
	SerialNumber string
	Flags        uint32
	MinPINLen    uint32
	MaxPINLen    uint32
}

// HasProtectedAuthenticationPath reports whether the module or the reader
// collects the PIN itself, which is the arrangement SPEC §6.5 describes and
// §6.5.1 requires this layer to use wherever it is offered.
func (t tokenInfo) HasProtectedAuthenticationPath() bool {
	return t.Flags&ckfProtectedAuthenticationPath != 0
}

// LoginRequired reports whether the token needs a login before its private
// objects can be used.
func (t tokenInfo) LoginRequired() bool { return t.Flags&ckfLoginRequired != 0 }

// openModule loads a PKCS#11 module and initialises it.
//
// The path comes from configuration or from discovery and from nowhere else: a
// calling application naming a DLL for the agent to load is arbitrary code
// execution wearing a configuration field (F11 §3). Nothing in this package
// takes a path from the protocol, and nothing above it may pass one through.
func openModule(path string) (*module, error) {
	if path == "" {
		return nil, fmt.Errorf("pkcs11: no module path")
	}
	handle, err := syscall.LoadLibrary(path)
	if err != nil {
		return nil, fmt.Errorf("pkcs11: loading %s: %w", path, err)
	}
	m := &module{path: path, handle: handle}

	getList, err := syscall.GetProcAddress(handle, "C_GetFunctionList")
	if err != nil {
		_ = syscall.FreeLibrary(handle)
		// A file with a promising name that does not export this is not a
		// PKCS#11 module — IAIK's pkcs11wrapper_64.dll is a Java JNI wrapper
		// that consumes modules rather than being one, and it sits in a
		// directory discovery will look in. Discovery must test for the entry
		// point rather than for the name.
		return nil, fmt.Errorf("pkcs11: %s exports no C_GetFunctionList, so it is not a module: %w", path, err)
	}

	var listPtr uintptr
	m.pinner.Pin(&listPtr)
	if rv := ckr(callN(getList, uintptr(unsafe.Pointer(&listPtr)))); rv != ckrOK {
		m.pinner.Unpin()
		_ = syscall.FreeLibrary(handle)
		return nil, &ckrError{"C_GetFunctionList", rv}
	}
	m.list = listPtr

	// C_GetFunctionList is the only function callable before C_Initialize;
	// measured, TrustEdgeID answers C_GetInfo with CKR_CRYPTOKI_NOT_INITIALIZED
	// before it, which is the module being right.
	if rv := ckr(m.call(iInitialize, 0)); rv != ckrOK {
		m.pinner.Unpin()
		_ = syscall.FreeLibrary(handle)
		return nil, &ckrError{"C_Initialize", rv}
	}
	return m, nil
}

// close finalises the module and unloads it.
func (m *module) close() error {
	if m.list != 0 {
		_ = m.call(iFinalize, 0)
		m.list = 0
	}
	m.pinner.Unpin()
	if m.handle != 0 {
		err := syscall.FreeLibrary(m.handle)
		m.handle = 0
		return err
	}
	return nil
}

// fn reads the i'th function pointer out of CK_FUNCTION_LIST.
func (m *module) fn(i int) uintptr {
	return *(*uintptr)(unsafe.Pointer(m.list + ckFunctionListBase + uintptr(i)*ckPtrSize))
}

func (m *module) call(i int, args ...uintptr) uint32 { return callN(m.fn(i), args...) }

func callN(fn uintptr, args ...uintptr) uint32 {
	r1, _, _ := syscall.SyscallN(fn, args...)
	return uint32(r1)
}

// info returns CK_INFO. It doubles as the behavioural confirmation that the
// index map above is right: a module answering with recognisable strings is
// reading from the function pointer this layer believes is C_GetInfo.
func (m *module) info() (moduleInfo, error) {
	var buf [ckInfoSize]byte
	var p runtime.Pinner
	p.Pin(&buf[0])
	defer p.Unpin()

	if rv := ckr(m.call(iGetInfo, uintptr(unsafe.Pointer(&buf[0])))); rv != ckrOK {
		return moduleInfo{}, &ckrError{"C_GetInfo", rv}
	}
	return moduleInfo{
		CryptokiVersion:    version(buf[0:2]),
		Manufacturer:       padded(buf[2:34]),
		LibraryDescription: padded(buf[38:70]),
		LibraryVersion:     version(buf[70:72]),
	}, nil
}

// slots returns the slot identifiers, optionally only those with a token in
// them.
func (m *module) slots(tokenPresent bool) ([]uint32, error) {
	present := uintptr(0)
	if tokenPresent {
		present = 1
	}
	var count uint32
	var p runtime.Pinner
	p.Pin(&count)
	defer p.Unpin()

	if rv := ckr(m.call(iGetSlotList, present, 0, uintptr(unsafe.Pointer(&count)))); rv != ckrOK {
		return nil, &ckrError{"C_GetSlotList", rv}
	}
	if count == 0 {
		return nil, nil
	}
	ids := make([]uint32, count)
	p.Pin(&ids[0])
	if rv := ckr(m.call(iGetSlotList, present,
		uintptr(unsafe.Pointer(&ids[0])), uintptr(unsafe.Pointer(&count)))); rv != ckrOK {
		return nil, &ckrError{"C_GetSlotList", rv}
	}
	return ids[:count], nil
}

// tokenInfo returns CK_TOKEN_INFO for one slot.
//
// A slot with a token present is not necessarily a card this module can use:
// SafeSign answers C_GetSlotList(tokenPresent=TRUE) with one slot and then
// CKR_TOKEN_NOT_RECOGNIZED here for a MUP card. That is "not mine" rather than
// an error, and the caller decides which (F11 §4).
func (m *module) tokenInfo(slot uint32) (tokenInfo, error) {
	var buf [ckTokenInfoSize]byte
	var p runtime.Pinner
	p.Pin(&buf[0])
	defer p.Unpin()

	if rv := ckr(m.call(iGetTokenInfo, uintptr(slot), uintptr(unsafe.Pointer(&buf[0])))); rv != ckrOK {
		return tokenInfo{}, &ckrError{"C_GetTokenInfo", rv}
	}
	return tokenInfo{
		Label:        padded(buf[0:32]),
		Manufacturer: padded(buf[32:64]),
		Model:        padded(buf[64:80]),
		SerialNumber: padded(buf[80:96]),
		Flags:        u32(buf[96:]),
		MaxPINLen:    u32(buf[116:]),
		MinPINLen:    u32(buf[120:]),
	}, nil
}

// mechanisms returns the mechanism types the token supports.
//
// F11 §2.1 turns on this list: CKM_RSA_PKCS signs a pre-computed DigestInfo,
// which is what SignDigest has, and CKM_SHA256_RSA_PKCS hashes the data
// itself, which would be wrong here. Reading the list costs nothing and needs
// no login.
func (m *module) mechanisms(slot uint32) ([]uint32, error) {
	var count uint32
	var p runtime.Pinner
	p.Pin(&count)
	defer p.Unpin()

	if rv := ckr(m.call(iGetMechanismList, uintptr(slot), 0,
		uintptr(unsafe.Pointer(&count)))); rv != ckrOK {
		return nil, &ckrError{"C_GetMechanismList", rv}
	}
	if count == 0 {
		return nil, nil
	}
	out := make([]uint32, count)
	p.Pin(&out[0])
	if rv := ckr(m.call(iGetMechanismList, uintptr(slot),
		uintptr(unsafe.Pointer(&out[0])), uintptr(unsafe.Pointer(&count)))); rv != ckrOK {
		return nil, &ckrError{"C_GetMechanismList", rv}
	}
	return out[:count], nil
}

// session is one read-only public session on one slot.
//
// Read-only and public is all this layer opens today: everything above
// C_Login is reading, and the login step is deliberately not built yet
// (D-269 — SafeSign has not been asked whether its token offers a protected
// authentication path, and the answer decides whether the fallback is needed
// at all).
type session struct {
	m      *module
	handle uint32
}

func (m *module) openSession(slot uint32) (*session, error) {
	var h uint32
	var p runtime.Pinner
	p.Pin(&h)
	defer p.Unpin()

	if rv := ckr(m.call(iOpenSession, uintptr(slot), ckfSerialSession, 0, 0,
		uintptr(unsafe.Pointer(&h)))); rv != ckrOK {
		return nil, &ckrError{"C_OpenSession", rv}
	}
	return &session{m: m, handle: h}, nil
}

func (s *session) close() error {
	if s.handle == 0 {
		return nil
	}
	rv := ckr(s.m.call(iCloseSession, uintptr(s.handle)))
	s.handle = 0
	if rv != ckrOK {
		return &ckrError{"C_CloseSession", rv}
	}
	return nil
}

// attribute is one CK_ATTRIBUTE as this layer builds them.
type attribute struct {
	typ   uint32
	value []byte
}

// marshalTemplate lays attributes out as an array of packed 16-byte
// CK_ATTRIBUTE structures and pins every buffer an entry points at.
//
// Go cannot express #pragma pack(1) — its natural layout for the same three
// fields is 24 bytes — so the template is written byte by byte. The caller
// must keep the returned pinner alive for the whole call.
//
// This layout is fixed by construction and is never probed at runtime. Three
// wrong layouts return CKR_OK with a zero length, which is a silent wrong
// answer rather than a failure; and Nexus's personal64.dll does not return at
// all when handed a template of the wrong shape — it takes the process down
// with an access violation, reproduced twice, taking any batch in flight with
// it.
func marshalTemplate(attrs []attribute, p *runtime.Pinner) ([]byte, uintptr) {
	if len(attrs) == 0 {
		return nil, 0
	}
	buf := make([]byte, len(attrs)*ckAttrSize)
	p.Pin(&buf[0])
	for i, a := range attrs {
		base := i * ckAttrSize
		putU32(buf[base:], a.typ) // type at +0, 4 bytes
		var ptr uintptr
		if len(a.value) > 0 {
			p.Pin(&a.value[0])
			ptr = uintptr(unsafe.Pointer(&a.value[0]))
		}
		putU64(buf[base+4:], uint64(ptr))           // pValue at +4, 8 bytes
		putU32(buf[base+12:], uint32(len(a.value))) // ulValueLen at +12, 4 bytes
	}
	return buf, uintptr(unsafe.Pointer(&buf[0]))
}

// findObjects returns the handles of every object matching the template.
func (s *session) findObjects(attrs []attribute) ([]uint32, error) {
	var p runtime.Pinner
	defer p.Unpin()
	_, tmpl := marshalTemplate(attrs, &p)

	if rv := ckr(s.m.call(iFindObjectsInit, uintptr(s.handle), tmpl, uintptr(len(attrs)))); rv != ckrOK {
		return nil, &ckrError{"C_FindObjectsInit", rv}
	}
	defer s.m.call(iFindObjectsFinal, uintptr(s.handle))

	var out []uint32
	for {
		handles := make([]uint32, findBatch)
		var found uint32
		var q runtime.Pinner
		q.Pin(&handles[0])
		q.Pin(&found)
		rv := ckr(s.m.call(iFindObjects, uintptr(s.handle),
			uintptr(unsafe.Pointer(&handles[0])), uintptr(findBatch),
			uintptr(unsafe.Pointer(&found))))
		q.Unpin()
		if rv != ckrOK {
			return nil, &ckrError{"C_FindObjects", rv}
		}
		if found == 0 {
			return out, nil
		}
		out = append(out, handles[:found]...)
		if found < findBatch {
			return out, nil
		}
	}
}

// attributeValue reads one attribute of one object.
//
// Two calls, as PKCS#11 requires: the first with a NULL pValue to learn the
// length, the second to fill a buffer of that size. A zero length from the
// first call is a real answer for an empty attribute and is returned as such.
func (s *session) attributeValue(obj uint32, typ uint32) ([]byte, error) {
	var p runtime.Pinner
	defer p.Unpin()

	sizing, tmpl := marshalTemplate([]attribute{{typ: typ}}, &p)
	if rv := ckr(s.m.call(iGetAttributeValue, uintptr(s.handle), uintptr(obj), tmpl, 1)); rv != ckrOK {
		return nil, &ckrError{"C_GetAttributeValue(size)", rv}
	}
	n := u32(sizing[12:])
	// CK_UNAVAILABLE_INFORMATION is ~0 and means the attribute is absent or
	// sensitive; it is not a length.
	if n == 0 || n == ^uint32(0) {
		return nil, nil
	}

	var q runtime.Pinner
	defer q.Unpin()
	value := make([]byte, n)
	_, tmpl2 := marshalTemplate([]attribute{{typ: typ, value: value}}, &q)
	if rv := ckr(s.m.call(iGetAttributeValue, uintptr(s.handle), uintptr(obj), tmpl2, 1)); rv != ckrOK {
		return nil, &ckrError{"C_GetAttributeValue", rv}
	}
	return value, nil
}

// certificateObjects returns the handles of every X.509 certificate on the
// token. It needs no login: certificates are public objects.
func (s *session) certificateObjects() ([]uint32, error) {
	return s.findObjects([]attribute{
		{typ: ckaClass, value: u32Bytes(ckoCertificate)},
		{typ: ckaCertType, value: u32Bytes(ckcX509)},
	})
}

// ---------------------------------------------------------------- decoding

func u32(b []byte) uint32 { return *(*uint32)(unsafe.Pointer(&b[0])) }

func putU32(b []byte, v uint32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

func putU64(b []byte, v uint64) {
	for i := 0; i < 8; i++ {
		b[i] = byte(v >> (8 * i))
	}
}

// u32Bytes renders a CK_ULONG as the bytes a template entry points at.
func u32Bytes(v uint32) []byte {
	b := make([]byte, ckULongSize)
	putU32(b, v)
	return b
}

// padded reads a fixed-width, space-padded PKCS#11 character field.
func padded(b []byte) string { return strings.TrimRight(string(b), " \x00") }

func version(b []byte) string { return fmt.Sprintf("%d.%d", b[0], b[1]) }
