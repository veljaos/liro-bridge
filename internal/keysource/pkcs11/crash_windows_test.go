package pkcs11

import (
	"os"
	"syscall"
	"unsafe"
)

// This file makes a probe child die on purpose, so that the parent's handling
// of a dead child can be measured instead of waited for.
//
// # Why this is not the synthetic input D-094 forbids
//
// D-094 is about not manufacturing the human input a test is supposed to be
// evidence about: a test that clicks its own Approve button proves nothing
// about consent, because whether a person approved is the thing under test.
//
// Here the thing under test is the parent's handling of a dead child. The input
// is the child dying, and a child that dies on purpose is really dead: the
// parent reads an exit status from the operating system, from a process that
// genuinely ended, through exactly the code path a vendor module's crash would
// take. Nothing about the parent is simulated. Writing an exit code into the
// parent instead of letting it read one would cross the line, and nothing here
// does that.
//
// The owner ruled this permitted with two conditions, both met below: say which
// termination is used and why, and say plainly that this demonstrates the
// parent's handling and not the module's behaviour.
//
// # It exists only in a test binary
//
// The dispatch is in TestMain (testmain_test.go), not in RunProbe. A release
// binary has no way to reach it, because the code is not in one: every file
// involved ends in _test.go. That is the same reasoning D-222 and D-228 used
// for the no-consent signing paths, except that here the mechanism is the Go
// build rather than a build tag, and it is stronger — a tag can be passed.

// crashSentinelPath is the module path that means "die instead of probing".
//
// It cannot collide with a real path: no Windows path may contain a NUL, and
// this is not a path anyway — the child recognises it before openModule is
// reached. It is deliberately loud rather than plausible, so that a person who
// ever sees it in a log knows it came from a test.
const crashSentinelPath = "!!liro-test-crash-failfast!!"

// crashSentinelThrow asks for the other termination D-272 saw, so that whether
// it is reachable at all is measured rather than assumed.
const crashSentinelThrow = "!!liro-test-crash-cppthrow!!"

var (
	kernel32                   = syscall.NewLazyDLL("kernel32.dll")
	procRaiseFailFastException = kernel32.NewProc("RaiseFailFastException")
	procRaiseException         = kernel32.NewProc("RaiseException")
)

// crashIfAskedForTest ends this process if the path is one of the sentinels.
// It never returns in that case, which is the point.
func crashIfAskedForTest(path string) {
	switch path {
	case crashSentinelPath:
		// D-272 measured NetSeT 1.1.0.0 dying two ways. This is the fail-fast
		// family: the CRT's invalid-parameter handler reaching __fastfail, which
		// D-272 saw as STATUS_INVALID_CRUNTIME_PARAMETER (0xC0000417).
		//
		// RaiseFailFastException is the reachable equivalent from Go. It takes
		// the same path out of the process — straight to the kernel, past the
		// exception dispatcher, past SetUnhandledExceptionFilter and past any
		// vectored handler, which is precisely the property that makes a
		// fail-fast unsurvivable and the reason D-275 exists.
		//
		// And it is bit-for-bit the same code, which was not expected: the
		// prediction was that the process would exit 0xC0000602
		// (STATUS_FAIL_FAST_EXCEPTION) whatever the record said. Measured, the
		// ExceptionCode in the record becomes the process exit status, so the
		// parent sees exactly the 0xC0000417 D-272 read off the real module.
		var rec [4]uintptr
		rec[0] = 0xC0000417 // the code D-272 measured, in the record
		_, _, _ = procRaiseFailFastException.Call(uintptr(unsafe.Pointer(&rec[0])), 0, 0)

		// Unreachable if the call did its job. If it ever is reached, the test
		// must not silently pass by falling through into a normal probe.
		os.Exit(0xBAD)

	case crashSentinelThrow:
		// The other termination: 0xE06D7363 is 0xE0000000 | 'msc', a Microsoft
		// C++ exception raised by throw with nothing catching it. Raising it
		// with RaiseException is the closest a Go process can come.
		//
		// Measured: it does NOT reproduce that termination. Unlike a fail-fast,
		// a raised exception goes through the dispatcher, where Go's own handler
		// meets it first — the child prints a Go runtime crash dump and exits 2.
		// So the parent sees "exit 0x2", not "exit 0xE06D7363", and this
		// sentinel measures the parent against a Go panic rather than against a
		// C++ throw. D-296 records that D-272's second termination is therefore
		// untested rather than letting the fail-fast stand for both.
		_, _, _ = procRaiseException.Call(0xE06D7363, 1 /* EXCEPTION_NONCONTINUABLE */, 0, 0)
		os.Exit(0xBAD)
	}
}
