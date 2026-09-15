//go:build windows

package pkcs11

import (
	"fmt"
	"runtime"
	"unsafe"
)

// ckuUser is CKU_USER, the only user type this layer ever logs in as. CKU_SO
// is the security officer and belongs to card administration, which this
// program does not do.
const ckuUser = 1

// maxSanePINLen bounds the buffer a token's own ulMaxPinLen is allowed to ask
// for. A card PIN is a handful of characters — the two this project has
// measured declare 8 (MUP) and 15 (Pošta) — and a token reporting something
// far larger is a token this layer should refuse rather than allocate for.
// 64 is well clear of anything a person types and small enough that a wrong
// value cannot become an allocation worth noticing.
const maxSanePINLen = 64

// The two sentinels below are declared `var x error = …` rather than letting
// the type be inferred, and the explicit type is load-bearing rather than a
// style choice.
//
// Both are named after a PIN and neither holds one, which is the case D-270
// measured across this whole tree: every PIN-named declaration in it is *about*
// a PIN without *being* one. pin_test.go asks what a declaration can carry
// rather than what it is called, precisely so that such names can stay — but a
// var with no type expression and a function call for an initialiser is one
// the checker cannot see through, and it is right to be conservative there.
// Naming the type answers its question truthfully: an `error` cannot hold a
// PIN's characters.
//
// The alternative was renaming them to something that dodges the matcher, and
// D-270 rejected exactly that: renaming correct code to satisfy a
// name-matching test is the tail wagging the dog, and it is how a guard
// becomes something people work around rather than something that protects
// anything.

// login authenticates this session as the user.
//
// It is one function on purpose, and the PIN is a local variable in it, and
// both of those are SPEC §6.5.1 clause 2 rather than style. Everything that
// touches the PIN happens between the make and the deferred wipe, in a body
// short enough to read in one go, and nothing carries it out.
//
// Clause 1 first: where the token advertises a protected authentication path
// the module or the reader collects the PIN and this program passes NULL, so
// the whole of the rest never runs. Measured, that branch is currently taken
// by nothing — neither NetSeT build on a MUP card (D-268) nor SafeSign on a
// Pošta card (D-273) offers one — but it is checked per token every time,
// because the flag is a property of a token through a module rather than of an
// issuer and a reader with a pinpad answers differently.
//
// Clause 5, nothing retries: there is no loop here and no caller that calls
// this twice. One wrong PIN is one of three attempts, and for a national
// identity card the third means a visit to a police station.
func (s *session) login(ti tokenInfo, entry PINEntry, req PINRequest) error {
	if ti.HasProtectedAuthenticationPath() {
		if rv := ckr(s.m.call(iLogin, uintptr(s.handle), ckuUser, 0, 0)); rv != ckrOK {
			return &ckrError{"C_Login", rv}
		}
		return nil
	}
	if entry == nil {
		return ErrNoPINEntry
	}

	minLen, maxLen := int(ti.MinPINLen), int(ti.MaxPINLen)
	if maxLen <= 0 || maxLen > maxSanePINLen {
		return fmt.Errorf("pkcs11: this token declares a maximum PIN length of %d, which this layer will not allocate for", maxLen)
	}
	if minLen < 0 || minLen > maxLen {
		return fmt.Errorf("pkcs11: this token declares a minimum PIN length of %d against a maximum of %d", minLen, maxLen)
	}

	// The one place SPEC §6.5.1 permits a PIN to be. It is pinned because its
	// address crosses into a foreign module, and runtime.Pinner rather than
	// runtime.KeepAlive for D-101's measured reason: KeepAlive stops memory
	// being collected and says nothing about it being *copied*, and a stack
	// that grows moves the frame it was on.
	pin := make([]byte, maxLen)
	var p runtime.Pinner
	p.Pin(&pin[0])
	defer func() {
		wipe(pin)
		p.Unpin()
	}()

	req.MinLength = minLen
	n, err := entry(pin, req)
	if err != nil {
		return err
	}
	if n < minLen || n > maxLen {
		return &PINLengthError{Got: n, Min: minLen, Max: maxLen}
	}

	if rv := ckr(s.m.call(iLogin, uintptr(s.handle), ckuUser,
		uintptr(unsafe.Pointer(&pin[0])), uintptr(n))); rv != ckrOK {
		return &ckrError{"C_Login", rv}
	}
	return nil
}

// logout ends the authenticated state. A failure is not worth returning: the
// session is being closed either way, and closing it logs out regardless.
func (s *session) logout() {
	if s.handle != 0 {
		_ = s.m.call(iLogout, uintptr(s.handle))
	}
}
