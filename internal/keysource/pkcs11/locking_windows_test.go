package pkcs11

import (
	"os"
	"testing"
)

// TestWhichModulesAcceptOSLocking is the behavioural confirmation of the
// CK_C_INITIALIZE_ARGS layout, and the measurement of which real modules will
// take CKF_OS_LOCKING_OK.
//
// # Why it has to be behavioural
//
// There is no PKCS#11 header on this machine, so the 44-byte layout is derived
// from the two facts the rest of module_windows.go rests on — CK_ULONG is 4
// bytes, structs are packed to one — rather than read. marshalTemplate's
// comment is the warning about exactly that: "Three wrong layouts return CKR_OK
// with a zero length, which is a silent wrong answer rather than a failure; and
// Nexus's personal64.dll does not return at all when handed a template of the
// wrong shape — it takes the process down with an access violation."
//
// C_Initialize is a kinder case than a template — it returns a code rather than
// a length — but "returns CKR_OK having read a field from the wrong offset" is
// still available, and the only thing that distinguishes a right layout from a
// lucky one is asking several different implementations.
//
// # What each answer means
//
//   - CKR_OK: the module accepted the args and will use OS locking.
//   - CKR_CANT_LOCK: the module read the flags, understood them, and cannot
//     comply. This is a *good* answer for the layout — it proves the flags were
//     read from offset 32 and not from padding — and an ordinary answer for the
//     worker, which then goes single-threaded.
//   - CKR_ARGUMENTS_BAD: the module rejected the structure. That is the layout
//     being wrong, or the module refusing something it is entitled to refuse.
//
// It reports rather than asserting which, because which of those a given
// vendor's module gives is a fact about that vendor and not about this code.
// What it asserts is that no module is left initialised and that nothing
// crashes.
func TestWhichModulesAcceptOSLocking(t *testing.T) {
	path := os.Getenv("LIRO_PKCS11_MODULE")
	if path == "" {
		t.Skip("LIRO_PKCS11_MODULE is not set; skipping the real-module tests")
	}

	// The control: the form this package has used since F11, which is known to
	// work against all four modules on this machine. If it fails, the failure
	// is not about locking and the treatment below would be measuring the wrong
	// thing.
	plain, plainErr := openModule(path)
	if plainErr != nil {
		t.Fatalf("C_Initialize(NULL) failed for %s: %v\n\n"+
			"This is the form used since F11. Its failing means something other "+
			"than OS locking is wrong, and the treatment would measure that instead.",
			path, plainErr)
	}
	if err := plain.close(); err != nil {
		t.Errorf("close after the control: %v", err)
	}
	t.Logf("control  C_Initialize(NULL)             -> CKR_OK")

	// The negative control, and the reason this test means anything.
	//
	// CKR_OK from the treatment below is consistent with two different worlds:
	// the layout is right and the module accepted OS locking, or the module
	// ignores the args structure entirely and would have said CKR_OK to
	// anything. Four modules all saying CKR_OK does not separate those — it is
	// four repetitions of a check that could not fail.
	//
	// So: ask a question whose answer is known in advance. PKCS#11 v2.40 §5.4
	// requires a non-NULL pReserved to be refused with CKR_ARGUMENTS_BAD. A
	// module that refuses it has read a field at offset 36 of the structure this
	// code built, which is the layout being confirmed by the module rather than
	// by arithmetic.
	readsArgs := negativeControl(t, path)

	// The treatment: identical but for the args structure.
	locked, lockErr := openModuleLocking(path)
	if lockErr == nil {
		if readsArgs {
			t.Logf("treatment C_Initialize(CKF_OS_LOCKING_OK) -> CKR_OK   " +
				"(this module read the structure and accepted OS locking)")
		} else {
			t.Logf("treatment C_Initialize(CKF_OS_LOCKING_OK) -> CKR_OK   " +
				"BUT the negative control shows this module does not read the args " +
				"structure at all, so this CKR_OK is not a promise of anything. It is " +
				"not evidence about the layout and not evidence that OS locking is in " +
				"use. Recorded as uninformative rather than as agreement.")
		}
		if err := locked.close(); err != nil {
			t.Errorf("close after the treatment: %v", err)
		}
		return
	}

	rv, ok := asCKR(lockErr)
	if !ok {
		t.Fatalf("C_Initialize with OS locking failed with something that is not a "+
			"PKCS#11 return code: %v", lockErr)
	}

	switch rv {
	case ckrCantLock:
		t.Logf("treatment C_Initialize(CKF_OS_LOCKING_OK) -> CKR_CANT_LOCK   "+
			"(the flags were read: %s will not use OS locking, and the worker goes "+
			"single-threaded)", path)
	case ckrArgumentsBad:
		t.Errorf("treatment C_Initialize(CKF_OS_LOCKING_OK) -> CKR_ARGUMENTS_BAD\n\n" +
			"The module rejected the structure. Either the 44-byte layout is wrong " +
			"— it is derived rather than read, because there is no PKCS#11 header on " +
			"this machine — or this module refuses an argument it is entitled to " +
			"refuse. Both need a person; neither should be shipped past.")
	default:
		t.Errorf("treatment C_Initialize(CKF_OS_LOCKING_OK) -> %v\n\n"+
			"Not one of the three answers this call is documented to give. Worth "+
			"understanding before the worker relies on it.", rv)
	}
}

// negativeControl asks the module a question whose answer PKCS#11 v2.40 §5.4
// fixes in advance: a non-NULL pReserved must be refused with
// CKR_ARGUMENTS_BAD.
//
// It is the only part of this test that can distinguish a correct
// CK_C_INITIALIZE_ARGS layout from a module that never looks at one. Without
// it, CKR_OK from every module on the machine would be equally consistent with
// the structure being read correctly and with it being ignored.
//
// pReserved is set to 1 rather than to a real address. The specification
// requires the module to refuse it without dereferencing, and a value that is
// not a mappable address means a module which dereferences anyway fails loudly
// here rather than reading something of this process's.
// It returns whether this module demonstrably reads the structure. That is not
// pass-or-fail: a module that ignores pReserved is a fact about that vendor,
// not a defect here, and measured on this machine two of the four do. What it
// changes is what the treatment's CKR_OK is allowed to mean.
func negativeControl(t *testing.T, path string) (readsArgs bool) {
	t.Helper()

	m, err := openModuleProbingLayout(path, 1)
	if err == nil {
		_ = m.close()
		t.Logf("negative control C_Initialize(pReserved!=NULL) -> CKR_OK   " +
			"(PKCS#11 v2.40 §5.4 requires CKR_ARGUMENTS_BAD, so this module does " +
			"not read the arguments structure at all: it confirms nothing about " +
			"the 44-byte layout and promises nothing about OS locking)")
		return false
	}

	rv, ok := asCKR(err)
	if !ok {
		t.Errorf("negative control: non-PKCS#11 failure: %v", err)
		return false
	}
	if rv == ckrArgumentsBad {
		t.Logf("negative control C_Initialize(pReserved!=NULL) -> CKR_ARGUMENTS_BAD   " +
			"(the module read offset 36, so the layout is confirmed by the module)")
		return true
	}
	t.Logf("negative control C_Initialize(pReserved!=NULL) -> %v   "+
		"(a refusal, but not the documented one; the layout is supported rather "+
		"than confirmed by this module)", rv)
	return true
}
