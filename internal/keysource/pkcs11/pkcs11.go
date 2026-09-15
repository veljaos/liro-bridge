// Package pkcs11 signs through an issuer's own PKCS#11 module, as a third
// keysource.Source beside windowscng and softtoken.
//
// Nothing is implemented yet. This file exists so that the package exists, and
// the package exists so that pin_test.go can be written before the backend it
// constrains — see below.
//
// # What governs this package
//
// SPEC §6.5.1 permits something no other part of this program is permitted:
// on a module that does not advertise CKF_PROTECTED_AUTHENTICATION_PATH, the
// PIN is an argument to C_Login and therefore exists in this process's memory.
// That was measured rather than assumed (D-268: the MUP token raises no dialog
// of its own, and a NULL PIN is passed to the card as an empty one and costs
// an attempt), and it was ruled on rather than discovered (D-269).
//
// The permission is narrow and every clause of §6.5.1 is a requirement. The
// one this package is shaped around is the second:
//
//	The PIN exists only for the duration of C_Login, and is overwritten
//	immediately afterwards. It is not left for the garbage collector, not
//	held in a struct field, not captured by a closure that outlives the
//	call, and not merely dropped.
//
// # Why the test is older than the code
//
// pin_test.go was the first commit of this package, before any of the backend
// existed. That ordering is the substance rather than a formality: a test
// written after a backend is a test written around whatever that backend
// already does, and this one has to be a constraint the backend is built to
// satisfy instead. The PIN is a local variable in one function because the
// test makes it impossible for it to be anything else.
//
// # What this package must not become
//
// The PIN does not travel. It is obtained, handed to C_Login, and overwritten
// in one function — so there is no parameter to pass it through, no field to
// keep it in, and no second function that has ever seen it. If a future change
// finds that shape inconvenient, the inconvenience is the rule working.
package pkcs11
