//go:build linux

package pkcs11

/*
#cgo pkg-config: p11-kit-1
#include "pkcs11_linux.h"
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// login authenticates this session as the user.
//
// **It is one function on purpose, the PIN is a local variable in it,
// and it is per platform for exactly that reason** — SPEC §6.5.1 clause
// 2 permits the characters in one function, the one that obtains them,
// passes them to C_Login and overwrites them, and the C_Login call is
// what differs between platforms. An earlier attempt to share this body
// by passing the PIN to a per-platform primitive was refused by
// pin_test.go, which is the guard doing its job (D-349). What *is*
// shared is login.go's arithmetic, which never sees a PIN.
//
// Clause 1 first: where the token advertises a protected authentication
// path the module or the reader collects the PIN and this program
// passes NULL, so the whole of the rest never runs. Measured, that
// branch is taken by nothing this project has met — neither NetSeT
// build on a MUP card (D-268), nor SafeSign on a Pošta card (D-273),
// nor SoftHSM (D-349) — but it is checked per token every time, because
// the flag is a property of a token through a module rather than of an
// issuer, and a reader with a pinpad answers differently.
//
// Clause 5, nothing retries: there is no loop here and no caller that
// calls this twice. One wrong PIN is one of three attempts, and for a
// national identity card the third means a visit to a police station.
func (s *session) login(ti tokenInfo, entry PINEntry, req PINRequest) error {
	if ti.HasProtectedAuthenticationPath() {
		if rv := ckr(C.liro_login(s.m.list, s.handle, nil, 0)); rv != ckrOK {
			return &ckrError{"C_Login", rv}
		}
		return nil
	}
	if entry == nil {
		return ErrNoPINEntry
	}
	bounds, err := pinBoundsFor(ti)
	if err != nil {
		return err
	}

	// The one place SPEC §6.5.1 permits a PIN to be. Pinned because its
	// address crosses into a foreign module — and on this platform that
	// is not a formality: cgo will not pass a Go pointer into C unless
	// the memory stays put, and D-101's reason holds either way, since
	// a stack that grows moves the frame it was on.
	pin := make([]byte, bounds.max)
	var p runtime.Pinner
	p.Pin(&pin[0])
	defer func() {
		wipe(pin)
		p.Unpin()
	}()

	req.MinLength = bounds.min
	n, err := entry(pin, req)
	if err != nil {
		return err
	}
	if err := bounds.check(n); err != nil {
		return err
	}

	if rv := ckr(C.liro_login(s.m.list, s.handle,
		(*C.CK_UTF8CHAR)(unsafe.Pointer(&pin[0])), C.CK_ULONG(n))); rv != ckrOK {
		return &ckrError{"C_Login", rv}
	}
	return nil
}

// logout ends the authenticated state. A failure is not worth
// returning: the session is being closed either way, and closing it
// logs out regardless.
func (s *session) logout() {
	if s.m != nil && s.m.list != nil {
		C.liro_logout(s.m.list, s.handle)
	}
}
