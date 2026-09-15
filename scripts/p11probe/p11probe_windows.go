// Command p11probe reads a PKCS#11 module and the token in it, and — only
// when explicitly asked — takes the one C_Login measurement F11 §5 turns on.
//
// It is a developer tool. Nothing that ships imports it, it writes no file,
// and it changes nothing on the machine or the card except in the one case
// below.
//
//	go run ./scripts/p11probe --module "C:\Program Files\TrustEdgeID\netsetpkcs11_x64.dll"
//
// With no --login that is entirely read-only: LoadLibrary, C_GetFunctionList,
// C_Initialize, C_GetSlotList, C_GetTokenInfo, a read-only public session,
// C_GetSessionInfo. It spends nothing and can be run at will.
//
// # --login is not read-only and can cost a PIN attempt
//
// --login takes C_Login(session, CKU_USER, NULL, 0) exactly once. On the MUP
// token that returned CKR_PIN_INCORRECT and consumed one of the card's three
// attempts (D-268). Three block the card, and for a national identity card
// unblocking means a visit to the police. The flag exists because the
// measurement had to be taken; it is not a thing to run to see what happens.
//
// Two guards, so that this is a property of the program rather than of
// whoever is running it: it refuses to call C_Login at all unless the token's
// three user-PIN flags are clear beforehand, and it reads them again
// immediately afterwards, so whether an attempt was consumed is measured
// rather than inferred. There is no retry anywhere in it and no code path
// that could take a second attempt.
//
// # Why it is written the way it is
//
// Standard library only — not even golang.org/x/sys — so that a successful
// run is evidence about the toolchain rather than about a dependency
// (F11 §1). It builds and runs with CGO_ENABLED=0, which is what established
// that this phase needs no C toolchain.
//
// The struct layouts are measured rather than assumed, on Windows x64: CK_ULONG
// is 4 bytes (LLP64) and will be 8 on F13's Linux, structs are packed to one
// byte, and CK_FUNCTION_LIST's function pointers therefore start at offset +2,
// unaligned, immediately after the two-byte CK_VERSION.
//
// go vet's unsafeptr fires throughout, as it does for the COM interop this
// project hand-writes; D-080 disabled it project-wide for that reason.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------- constants

// Index into CK_FUNCTION_LIST, counting the function pointers only; the
// two-byte CK_VERSION is handled by fnAt's own +2. PKCS#11 v2.40 §3.6 fixes
// this order and it is normative. Written out with iota rather than as a bare
// const block: the handover records that an explicit first value followed by
// bare names repeats rather than increments, which made every index 12 and
// turned C_FindObjectsInit into C_OpenSession.
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
)

// CK_TOKEN_INFO flags.
const (
	ckfRNG                         = 0x00000001
	ckfWriteProtected              = 0x00000002
	ckfLoginRequired               = 0x00000004
	ckfUserPINInitialized          = 0x00000008
	ckfRestoreKeyNotNeeded         = 0x00000020
	ckfClockOnToken                = 0x00000040
	ckfProtectedAuthenticationPath = 0x00000100
	ckfDualCryptoOperations        = 0x00000200
	ckfTokenInitialized            = 0x00000400
	ckfSecondaryAuthentication     = 0x00000800
	ckfUserPINCountLow             = 0x00010000
	ckfUserPINFinalTry             = 0x00020000
	ckfUserPINLocked               = 0x00040000
	ckfUserPINToBeChanged          = 0x00080000
	ckfSOPINCountLow               = 0x00100000
	ckfSOPINFinalTry               = 0x00200000
	ckfSOPINLocked                 = 0x00400000
	ckfSOPINToBeChanged            = 0x00800000
)

// The three the owner named: whether an attempt was consumed is measured from
// these rather than inferred.
const userPINCounterFlags = ckfUserPINCountLow | ckfUserPINFinalTry | ckfUserPINLocked

const (
	ckfRWSession     = 0x00000002
	ckfSerialSession = 0x00000004
)

const ckuUser = 1

var tokenFlagNames = []struct {
	bit  uint32
	name string
}{
	{ckfRNG, "RNG"},
	{ckfWriteProtected, "WRITE_PROTECTED"},
	{ckfLoginRequired, "LOGIN_REQUIRED"},
	{ckfUserPINInitialized, "USER_PIN_INITIALIZED"},
	{ckfRestoreKeyNotNeeded, "RESTORE_KEY_NOT_NEEDED"},
	{ckfClockOnToken, "CLOCK_ON_TOKEN"},
	{ckfProtectedAuthenticationPath, "PROTECTED_AUTHENTICATION_PATH"},
	{ckfDualCryptoOperations, "DUAL_CRYPTO_OPERATIONS"},
	{ckfTokenInitialized, "TOKEN_INITIALIZED"},
	{ckfSecondaryAuthentication, "SECONDARY_AUTHENTICATION"},
	{ckfUserPINCountLow, "USER_PIN_COUNT_LOW"},
	{ckfUserPINFinalTry, "USER_PIN_FINAL_TRY"},
	{ckfUserPINLocked, "USER_PIN_LOCKED"},
	{ckfUserPINToBeChanged, "USER_PIN_TO_BE_CHANGED"},
	{ckfSOPINCountLow, "SO_PIN_COUNT_LOW"},
	{ckfSOPINFinalTry, "SO_PIN_FINAL_TRY"},
	{ckfSOPINLocked, "SO_PIN_LOCKED"},
	{ckfSOPINToBeChanged, "SO_PIN_TO_BE_CHANGED"},
}

// CKR return values, PKCS#11 v2.40 §A. A table that is wrong is worse than no
// table, because it does not look like a gap — so every return is printed as
// raw hex as well as by name, and an unknown value says so rather than
// printing a plausible neighbour.
var ckrNames = map[uint32]string{
	0x0000: "CKR_OK",
	0x0001: "CKR_CANCEL",
	0x0002: "CKR_HOST_MEMORY",
	0x0003: "CKR_SLOT_ID_INVALID",
	0x0005: "CKR_GENERAL_ERROR",
	0x0006: "CKR_FUNCTION_FAILED",
	0x0007: "CKR_ARGUMENTS_BAD",
	0x0008: "CKR_NO_EVENT",
	0x0009: "CKR_NEED_TO_CREATE_THREADS",
	0x000A: "CKR_CANT_LOCK",
	0x0010: "CKR_ATTRIBUTE_READ_ONLY",
	0x0011: "CKR_ATTRIBUTE_SENSITIVE",
	0x0012: "CKR_ATTRIBUTE_TYPE_INVALID",
	0x0013: "CKR_ATTRIBUTE_VALUE_INVALID",
	0x001B: "CKR_ACTION_PROHIBITED",
	0x0020: "CKR_DATA_INVALID",
	0x0021: "CKR_DATA_LEN_RANGE",
	0x0030: "CKR_DEVICE_ERROR",
	0x0031: "CKR_DEVICE_MEMORY",
	0x0032: "CKR_DEVICE_REMOVED",
	0x0040: "CKR_ENCRYPTED_DATA_INVALID",
	0x0041: "CKR_ENCRYPTED_DATA_LEN_RANGE",
	0x0050: "CKR_FUNCTION_CANCELED",
	0x0051: "CKR_FUNCTION_NOT_PARALLEL",
	0x0054: "CKR_FUNCTION_NOT_SUPPORTED",
	0x0060: "CKR_KEY_HANDLE_INVALID",
	0x0062: "CKR_KEY_SIZE_RANGE",
	0x0063: "CKR_KEY_TYPE_INCONSISTENT",
	0x0064: "CKR_KEY_NOT_NEEDED",
	0x0065: "CKR_KEY_CHANGED",
	0x0066: "CKR_KEY_NEEDED",
	0x0067: "CKR_KEY_INDIGESTIBLE",
	0x0068: "CKR_KEY_FUNCTION_NOT_PERMITTED",
	0x0069: "CKR_KEY_NOT_WRAPPABLE",
	0x006A: "CKR_KEY_UNEXTRACTABLE",
	0x0070: "CKR_MECHANISM_INVALID",
	0x0071: "CKR_MECHANISM_PARAM_INVALID",
	0x0082: "CKR_OBJECT_HANDLE_INVALID",
	0x0090: "CKR_OPERATION_ACTIVE",
	0x0091: "CKR_OPERATION_NOT_INITIALIZED",
	0x00A0: "CKR_PIN_INCORRECT",
	0x00A1: "CKR_PIN_INVALID",
	0x00A2: "CKR_PIN_LEN_RANGE",
	0x00A3: "CKR_PIN_EXPIRED",
	0x00A4: "CKR_PIN_LOCKED",
	0x00B0: "CKR_SESSION_CLOSED",
	0x00B1: "CKR_SESSION_COUNT",
	0x00B3: "CKR_SESSION_HANDLE_INVALID",
	0x00B4: "CKR_SESSION_PARALLEL_NOT_SUPPORTED",
	0x00B5: "CKR_SESSION_READ_ONLY",
	0x00B6: "CKR_SESSION_EXISTS",
	0x00B7: "CKR_SESSION_READ_ONLY_EXISTS",
	0x00B8: "CKR_SESSION_READ_WRITE_SO_EXISTS",
	0x00C0: "CKR_SIGNATURE_INVALID",
	0x00C1: "CKR_SIGNATURE_LEN_RANGE",
	0x00D0: "CKR_TEMPLATE_INCOMPLETE",
	0x00D1: "CKR_TEMPLATE_INCONSISTENT",
	0x00E0: "CKR_TOKEN_NOT_PRESENT",
	0x00E1: "CKR_TOKEN_NOT_RECOGNIZED",
	0x00E2: "CKR_TOKEN_WRITE_PROTECTED",
	0x00F0: "CKR_UNWRAPPING_KEY_HANDLE_INVALID",
	0x00F1: "CKR_UNWRAPPING_KEY_SIZE_RANGE",
	0x00F2: "CKR_UNWRAPPING_KEY_TYPE_INCONSISTENT",
	0x0100: "CKR_USER_ALREADY_LOGGED_IN",
	0x0101: "CKR_USER_NOT_LOGGED_IN",
	0x0102: "CKR_USER_PIN_NOT_INITIALIZED",
	0x0103: "CKR_USER_TYPE_INVALID",
	0x0104: "CKR_USER_ANOTHER_ALREADY_LOGGED_IN",
	0x0105: "CKR_USER_TOO_MANY_TYPES",
	0x0110: "CKR_WRAPPED_KEY_INVALID",
	0x0112: "CKR_WRAPPED_KEY_LEN_RANGE",
	0x0113: "CKR_WRAPPING_KEY_HANDLE_INVALID",
	0x0114: "CKR_WRAPPING_KEY_SIZE_RANGE",
	0x0115: "CKR_WRAPPING_KEY_TYPE_INCONSISTENT",
	0x0120: "CKR_RANDOM_SEED_NOT_SUPPORTED",
	0x0121: "CKR_RANDOM_NO_RNG",
	0x0130: "CKR_DOMAIN_PARAMS_INVALID",
	0x0150: "CKR_BUFFER_TOO_SMALL",
	0x0160: "CKR_SAVED_STATE_INVALID",
	0x0170: "CKR_INFORMATION_SENSITIVE",
	0x0180: "CKR_STATE_UNSAVEABLE",
	0x0190: "CKR_CRYPTOKI_NOT_INITIALIZED",
	0x0191: "CKR_CRYPTOKI_ALREADY_INITIALIZED",
	0x01A0: "CKR_MUTEX_BAD",
	0x01A1: "CKR_MUTEX_NOT_LOCKED",
	0x0200: "CKR_FUNCTION_REJECTED",
}

var sessionStateNames = map[uint32]string{
	0: "CKS_RO_PUBLIC_SESSION",
	1: "CKS_RO_USER_FUNCTIONS",
	2: "CKS_RW_PUBLIC_SESSION",
	3: "CKS_RW_USER_FUNCTIONS",
	4: "CKS_RW_SO_FUNCTIONS",
}

// ---------------------------------------------------------------- plumbing

// pins keeps every Go buffer whose raw address crosses into the module fixed
// where it was put. runtime.KeepAlive is not enough: it stops memory being
// collected and says nothing about it being copied, which is what happens when
// a goroutine's stack grows between an address being taken and the call that
// uses it. This project has paid for that lesson once already (D-101).
type pins struct{ p runtime.Pinner }

func (x *pins) buf(n int) ([]byte, uintptr) {
	b := make([]byte, n)
	x.p.Pin(&b[0])
	return b, uintptr(unsafe.Pointer(&b[0]))
}

func (x *pins) release() { x.p.Unpin() }

// fnAt reads the i'th function pointer out of CK_FUNCTION_LIST. Structs are
// packed to one byte, so the pointers begin at +2, immediately after the
// CK_VERSION, and every one of them is unaligned. amd64 loads them happily; a
// platform that trapped unaligned loads would need byte-wise reads.
func fnAt(list uintptr, i int) uintptr {
	return *(*uintptr)(unsafe.Pointer(list + 2 + uintptr(i)*8))
}

func call(fn uintptr, args ...uintptr) uint32 {
	r1, _, _ := syscall.SyscallN(fn, args...)
	return uint32(r1)
}

func ckr(rv uint32) string {
	if name, ok := ckrNames[rv]; ok {
		return fmt.Sprintf("%s (0x%X)", name, rv)
	}
	if rv&0x80000000 != 0 {
		return fmt.Sprintf("CKR_VENDOR_DEFINED+0x%X (0x%X)", rv&^uint32(0x80000000), rv)
	}
	return fmt.Sprintf("UNKNOWN-CKR (0x%X) -- not in this probe's table", rv)
}

func flagNames(f uint32) string {
	var out []string
	var known uint32
	for _, fl := range tokenFlagNames {
		if f&fl.bit != 0 {
			out = append(out, fl.name)
			known |= fl.bit
		}
	}
	if rest := f &^ known; rest != 0 {
		out = append(out, fmt.Sprintf("unrecognised:0x%X", rest))
	}
	if len(out) == 0 {
		return "(none)"
	}
	return strings.Join(out, " ")
}

// str reads a fixed-width, space-padded PKCS#11 character field.
func str(b []byte, off, n int) string {
	return strings.TrimRight(string(b[off:off+n]), " \x00")
}

func u32(b []byte, off int) uint32 { return *(*uint32)(unsafe.Pointer(&b[off])) }

func ver(b []byte, off int) string { return fmt.Sprintf("%d.%d", b[off], b[off+1]) }

// say prints a line. os.Stdout is unbuffered in Go, so there is nothing to
// flush: every line is on the terminal before the next statement runs, which
// is what matters when the next statement is a C_Login that may block on a
// dialog.
func say(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

// ------------------------------------------------- watching for a dialog
//
// Whether a dialog appears is the whole measurement, and an answer that lives
// only in somebody noticing before they close the window is an answer that
// gets missed (D-258). This records it instead: every 250ms it names every
// visible top-level window belonging to this process, and the foreground
// window when it is not one of ours. It observes only -- nothing here touches
// the cursor, the keyboard or the foreground (D-094).

var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	pEnumWindows           = user32.NewProc("EnumWindows")
	pGetWindowThreadProcID = user32.NewProc("GetWindowThreadProcessId")
	pIsWindowVisible       = user32.NewProc("IsWindowVisible")
	pGetClassNameW         = user32.NewProc("GetClassNameW")
	pGetWindowTextW        = user32.NewProc("GetWindowTextW")
	pGetForegroundWindow   = user32.NewProc("GetForegroundWindow")
	pGetCurrentProcessId   = kernel32.NewProc("GetCurrentProcessId")
)

func wtext(proc *syscall.LazyProc, hwnd uintptr) string {
	var buf [512]uint16
	n, _, _ := proc.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func pidOf(hwnd uintptr) uint32 {
	var pid uint32
	_, _, _ = pGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func describe(hwnd uintptr) string {
	return fmt.Sprintf("hwnd=0x%X pid=%d class=%q title=%q",
		hwnd, pidOf(hwnd), wtext(pGetClassNameW, hwnd), wtext(pGetWindowTextW, hwnd))
}

// watchForWindows runs until stop is closed. It returns whatever it saw.
func watchForWindows(stop <-chan struct{}, seen chan<- string) {
	self, _, _ := pGetCurrentProcessId.Call()
	me := uint32(self)
	known := map[uintptr]bool{}
	var lastFg uintptr

	// The callback is created once, at start, never per call.
	var found []uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if pidOf(hwnd) == me {
			if vis, _, _ := pIsWindowVisible.Call(hwnd); vis != 0 {
				found = append(found, hwnd)
			}
		}
		return 1 // keep enumerating
	})

	for {
		select {
		case <-stop:
			return
		default:
		}
		found = found[:0]
		_, _, _ = pEnumWindows.Call(cb, 0)
		for _, h := range found {
			if !known[h] {
				known[h] = true
				seen <- "WINDOW APPEARED IN THIS PROCESS: " + describe(h)
			}
		}
		if fg, _, _ := pGetForegroundWindow.Call(); fg != 0 && fg != lastFg {
			lastFg = fg
			if pidOf(fg) != me {
				seen <- "foreground moved to another process: " + describe(fg)
			} else {
				seen <- "foreground moved to THIS process: " + describe(fg)
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "p11probe: "+format+"\n", a...)
	os.Exit(1)
}

// ---------------------------------------------------------------- the probe

func main() {
	module := flag.String("module", "", "full path to the PKCS#11 module")
	login := flag.Bool("login", false,
		"take the one C_Login(session, CKU_USER, NULL, 0) measurement (F11 §5)")
	slotWanted := flag.Int("slot", -1, "slot index to use (default: the first with a token)")
	watchTest := flag.Bool("watch-test", false,
		"run only the window watcher for a few seconds; touches no card and no module")
	flag.Parse()

	// A check of the instrument before it is relied on: run only the watcher,
	// touching no card and no module at all, and show that it reports what is
	// on screen. A watcher that silently sees nothing would make "no dialog
	// appeared" mean nothing.
	if *watchTest {
		say("watcher self-test: 4 seconds, no module loaded, no card touched.")
		stop := make(chan struct{})
		seen := make(chan string, 256)
		watching := make(chan struct{})
		go func() { watchForWindows(stop, seen); close(watching) }()
		go func() {
			for s := range seen {
				say("  [watch] %s", s)
			}
		}()
		time.Sleep(4 * time.Second)
		close(stop)
		<-watching
		time.Sleep(100 * time.Millisecond)
		say("watcher self-test done.")
		return
	}

	if *module == "" {
		die("--module is required")
	}

	var pin pins
	defer pin.release()

	say("module   %s", *module)

	// LoadLibrary + GetProcAddress + SyscallN. No cgo, no C toolchain.
	h, err := syscall.LoadLibrary(*module)
	if err != nil {
		die("LoadLibrary: %v", err)
	}
	defer func() { _ = syscall.FreeLibrary(h) }()

	getList, err := syscall.GetProcAddress(h, "C_GetFunctionList")
	if err != nil {
		die("GetProcAddress C_GetFunctionList: %v (not a PKCS#11 module?)", err)
	}

	listBuf, listPtr := pin.buf(8)
	if rv := call(getList, listPtr); rv != 0 {
		die("C_GetFunctionList: %s", ckr(rv))
	}
	list := *(*uintptr)(unsafe.Pointer(&listBuf[0]))
	say("list     0x%X   (version %d.%d)", list,
		*(*byte)(unsafe.Pointer(list)), *(*byte)(unsafe.Pointer(list + 1)))

	// C_GetFunctionList is the only function callable before C_Initialize;
	// measured here, TrustEdgeID answers C_GetInfo with
	// CKR_CRYPTOKI_NOT_INITIALIZED, which is the module being right.
	if rv := call(fnAt(list, iInitialize), 0); rv != 0 {
		die("C_Initialize: %s", ckr(rv))
	}
	defer call(fnAt(list, iFinalize), 0)

	// Confirm the index map behaviourally rather than by comparing list[3]
	// against the exported C_GetFunctionList: SafeSign's export is a jmp rel32
	// thunk and does not match. C_GetInfo returning recognisable strings is the
	// check that actually holds.
	infoBuf, infoPtr := pin.buf(72)
	if rv := call(fnAt(list, iGetInfo), infoPtr); rv != 0 {
		die("C_GetInfo: %s", ckr(rv))
	}
	say("C_GetInfo cryptoki=%s manufacturer=%q library=%q %s",
		ver(infoBuf, 0), str(infoBuf, 2, 32), str(infoBuf, 38, 32), ver(infoBuf, 70))
	if str(infoBuf, 38, 32) == "" {
		die("libraryDescription is empty at offset 38 -- the layout or the index map is wrong; stopping")
	}

	// C_GetSlotList(tokenPresent=TRUE, NULL, &count), then with a buffer.
	countBuf, countPtr := pin.buf(4)
	if rv := call(fnAt(list, iGetSlotList), 1, 0, countPtr); rv != 0 {
		die("C_GetSlotList(count): %s", ckr(rv))
	}
	n := int(u32(countBuf, 0))
	say("slots with a token present: %d", n)
	if n == 0 {
		die("no slot reports a token present -- is the card in the reader?")
	}
	slotsBuf, slotsPtr := pin.buf(n * 4)
	if rv := call(fnAt(list, iGetSlotList), 1, slotsPtr, countPtr); rv != 0 {
		die("C_GetSlotList(list): %s", ckr(rv))
	}

	tokBuf, tokPtr := pin.buf(160)
	readToken := func(slotID uint32) (uint32, bool) {
		for i := range tokBuf {
			tokBuf[i] = 0
		}
		rv := call(fnAt(list, iGetTokenInfo), uintptr(slotID), tokPtr)
		if rv != 0 {
			say("    C_GetTokenInfo: %s", ckr(rv))
			return 0, false
		}
		return u32(tokBuf, 96), true
	}

	chosen := -1
	var chosenSlotID uint32
	for i := 0; i < n; i++ {
		slotID := u32(slotsBuf, i*4)
		say("")
		say("slot[%d] id=%d", i, slotID)
		f, ok := readToken(slotID)
		if !ok {
			continue
		}
		say("    label        %q", str(tokBuf, 0, 32))
		say("    manufacturer %q", str(tokBuf, 32, 32))
		say("    model        %q", str(tokBuf, 64, 16))
		say("    serial       %q", str(tokBuf, 80, 16))
		say("    minPin=%d maxPin=%d", u32(tokBuf, 120), u32(tokBuf, 116))
		say("    flags        0x%X  %s", f, flagNames(f))
		say("    PROTECTED_AUTHENTICATION_PATH: %v", f&ckfProtectedAuthenticationPath != 0)
		if chosen < 0 && (*slotWanted < 0 || *slotWanted == i) {
			chosen, chosenSlotID = i, slotID
		}
	}
	if chosen < 0 {
		die("no usable slot")
	}
	say("")
	say("using slot[%d] id=%d", chosen, chosenSlotID)

	// A read-only public session: the minimum this measurement needs.
	sessBuf, sessPtr := pin.buf(4)
	if rv := call(fnAt(list, iOpenSession), uintptr(chosenSlotID), ckfSerialSession, 0, 0, sessPtr); rv != 0 {
		die("C_OpenSession: %s", ckr(rv))
	}
	session := u32(sessBuf, 0)
	defer call(fnAt(list, iCloseSession), uintptr(session))
	say("session  handle=%d (read-only, public)", session)

	siBuf, siPtr := pin.buf(16)
	if rv := call(fnAt(list, iGetSessionInfo), uintptr(session), siPtr); rv == 0 {
		st := u32(siBuf, 4)
		name := sessionStateNames[st]
		if name == "" {
			name = "unknown"
		}
		say("         state=%d (%s) flags=0x%X deviceError=%d",
			st, name, u32(siBuf, 8), u32(siBuf, 12))
	} else {
		say("         C_GetSessionInfo: %s", ckr(rv))
	}

	// ---- the PIN counter, immediately before ----
	say("")
	before, ok := readToken(chosenSlotID)
	if !ok {
		die("could not read the token's flags before the measurement; stopping")
	}
	say("PIN COUNTER BEFORE  flags=0x%X", before)
	say("  USER_PIN_COUNT_LOW   %v", before&ckfUserPINCountLow != 0)
	say("  USER_PIN_FINAL_TRY   %v", before&ckfUserPINFinalTry != 0)
	say("  USER_PIN_LOCKED      %v", before&ckfUserPINLocked != 0)

	if !*login {
		say("")
		say("--login not given: no C_Login was attempted. Nothing was spent.")
		return
	}

	// The owner's condition 2, as a property of this program rather than of
	// anyone's discipline: if the counter is not full, stop.
	if before&userPINCounterFlags != 0 {
		die("the PIN counter is NOT full before the measurement (flags 0x%X: %s).\n"+
			"  Stopping without calling C_Login. This needs the owner.", before, flagNames(before))
	}
	say("  -> full counter. Proceeding.")

	say("")
	say("=====================================================================")
	say(" Calling C_Login(session=%d, CKU_USER, pPin=NULL, ulPinLen=0) ONCE.", session)
	say(" If a PIN dialog appears, press CANCEL. Its appearance is the answer;")
	say(" there is no need to type a PIN, and cancelling spends nothing.")
	say("=====================================================================")

	// Record anything that appears, so the answer does not depend on either of
	// us noticing it before it is closed.
	stop := make(chan struct{})
	seen := make(chan string, 256)
	watching := make(chan struct{})
	drained := make(chan struct{})
	go func() { watchForWindows(stop, seen); close(watching) }()
	go func() {
		for s := range seen {
			say("  [watch] %s", s)
		}
		close(drained)
	}()

	rv := call(fnAt(list, iLogin), uintptr(session), ckuUser, 0, 0)

	close(stop)
	<-watching // the watcher has stopped, so nothing more will be sent
	close(seen)
	<-drained // everything it saw has been printed

	say("")
	say("C_Login RETURNED  %s", ckr(rv))

	// ---- the PIN counter, immediately after ----
	after, ok := readToken(chosenSlotID)
	if !ok {
		say("PIN COUNTER AFTER   could not be read -- report this, it matters")
		return
	}
	say("PIN COUNTER AFTER   flags=0x%X", after)
	say("  USER_PIN_COUNT_LOW   %v", after&ckfUserPINCountLow != 0)
	say("  USER_PIN_FINAL_TRY   %v", after&ckfUserPINFinalTry != 0)
	say("  USER_PIN_LOCKED      %v", after&ckfUserPINLocked != 0)
	say("")
	if before == after {
		say("VERDICT: the token's flags are IDENTICAL before and after (0x%X).", before)
		say("         No attempt was consumed, as far as the flags can say.")
	} else {
		say("VERDICT: the token's flags CHANGED: 0x%X -> 0x%X", before, after)
		say("         was: %s", flagNames(before))
		say("         now: %s", flagNames(after))
		say("         AN ATTEMPT WAS PROBABLY CONSUMED. Stop and tell the owner.")
	}

	if rv == 0 {
		say("")
		say("C_Login returned CKR_OK -- logging out again so the session is left as found.")
		if lrv := call(fnAt(list, iLogout), uintptr(session)); lrv != 0 {
			say("C_Logout: %s", ckr(lrv))
		}
	}
}
