package platform

// Windows smart card reader detection via winscard.dll, loaded with
// NewLazySystemDLL rather than linked (F1 §2.3): the agent must not
// depend on a C toolchain (SPEC §8.6, D-012) and must not be vulnerable
// to DLL search-path hijacking, which NewLazySystemDLL avoids by loading
// only from the system directory.
//
// The retry, mapping and Readers/AnyCardPresent logic lives in
// scard_core.go, where it is unit-testable behind the scardConn
// interface. This file is the thin, untestable layer that actually calls
// the DLL.

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winscardDLL = windows.NewLazySystemDLL("winscard.dll")

	procSCardEstablishContext = winscardDLL.NewProc("SCardEstablishContext")
	procSCardReleaseContext   = winscardDLL.NewProc("SCardReleaseContext")
	procSCardListReadersW     = winscardDLL.NewProc("SCardListReadersW")
	procSCardGetStatusChangeW = winscardDLL.NewProc("SCardGetStatusChangeW")
)

const (
	scardScopeUser = 0

	// SCARD_STATE_UNAWARE: the caller has no assumption about the
	// reader's state. Required input for a first status query (F1 §2.3).
	scardStateUnaware = 0x00000000
	// SCARD_STATE_PRESENT: a card is present and powered in the reader.
	scardStatePresent = 0x00000020

	// statusChangeNoTimeout tells SCardGetStatusChangeW to return
	// immediately with the current state rather than block waiting for a
	// change. Passing INFINITE here would hang the agent (F1 §2.3).
	statusChangeNoTimeout = 0
)

// scardReaderStateW mirrors the Win32 SCARD_READERSTATEW structure
// byte-for-byte. Field order and the 36-byte ATR buffer size
// (MAX_ATR_SIZE plus the padding winscard.h reserves) are fixed by the
// Windows ABI and must not be reordered.
type scardReaderStateW struct {
	szReader       *uint16
	pvUserData     uintptr
	dwCurrentState uint32
	dwEventState   uint32
	cbAtr          uint32
	rgbAtr         [36]byte
}

// winscardConn implements scardConn over the real winscard.dll.
type winscardConn struct{}

func (winscardConn) establishContext() (scardContext, error) {
	var ctx uintptr
	r0, _, _ := procSCardEstablishContext.Call(scardScopeUser, 0, 0, uintptr(unsafe.Pointer(&ctx)))
	if err := mapReturnCode("SCardEstablishContext", uint32(r0)); err != nil {
		return 0, err
	}
	return scardContext(ctx), nil
}

func (winscardConn) releaseContext(c scardContext) error {
	r0, _, _ := procSCardReleaseContext.Call(uintptr(c))
	return mapReturnCode("SCardReleaseContext", uint32(r0))
}

// listReaders calls SCardListReadersW twice: once with a nil buffer to
// learn the required length, once to fill it. This is the two-call
// pattern F1 §2.3 asks for, as an alternative to SCARD_AUTOALLOCATE.
func (winscardConn) listReaders(c scardContext) ([]string, error) {
	var needed uint32
	r0, _, _ := procSCardListReadersW.Call(uintptr(c), 0, 0, uintptr(unsafe.Pointer(&needed)))
	if err := mapReturnCode("SCardListReadersW", uint32(r0)); err != nil {
		return nil, err
	}
	if needed == 0 {
		return []string{}, nil
	}

	buf := make([]uint16, needed)
	r0, _, _ = procSCardListReadersW.Call(uintptr(c), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&needed)))
	if err := mapReturnCode("SCardListReadersW", uint32(r0)); err != nil {
		return nil, err
	}
	// parseMultiString scans for the double-NUL terminator itself; it
	// does not rely on "needed" being exact.
	return parseMultiString(buf), nil
}

// statusChange calls SCardGetStatusChangeW with a zero timeout, so it
// reads the current state and returns immediately rather than waiting
// for a change (F1 §2.3).
func (winscardConn) statusChange(c scardContext, names []string) ([]ReaderState, error) {
	states := make([]scardReaderStateW, len(names))
	ptrs := make([]*uint16, len(names))
	for i, name := range names {
		p, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return nil, fmt.Errorf("encoding reader name for SCardGetStatusChangeW: %w", err)
		}
		ptrs[i] = p
		states[i].szReader = p
		states[i].dwCurrentState = scardStateUnaware
	}

	r0, _, _ := procSCardGetStatusChangeW.Call(
		uintptr(c),
		statusChangeNoTimeout,
		uintptr(unsafe.Pointer(&states[0])),
		uintptr(len(states)),
	)
	// The reader-name pointers must stay alive until the call returns;
	// the compiler has no way to know the DLL still holds them.
	runtime.KeepAlive(ptrs)
	if err := mapReturnCode("SCardGetStatusChangeW", uint32(r0)); err != nil {
		return nil, err
	}

	out := make([]ReaderState, len(names))
	for i, name := range names {
		present := states[i].dwEventState&scardStatePresent != 0

		atrLen := states[i].cbAtr
		if atrLen > uint32(len(states[i].rgbAtr)) {
			atrLen = uint32(len(states[i].rgbAtr))
		}
		var atr []byte
		if present && atrLen > 0 {
			atr = append([]byte(nil), states[i].rgbAtr[:atrLen]...)
		}
		out[i] = ReaderState{Name: name, CardPresent: present, ATR: atr}
	}
	return out, nil
}

// NewSmartCardService returns the Windows SmartCardService backed by
// winscard.dll.
func NewSmartCardService() SmartCardService {
	return &pcscService{conn: winscardConn{}}
}
