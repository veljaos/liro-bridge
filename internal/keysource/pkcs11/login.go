//go:build windows || linux

package pkcs11

import "fmt"

// SPEC §6.5.1 clause 2's arithmetic, which is the whole of the login
// sequence that is not the PIN itself.
//
// **The PIN does not appear in this file, and that is the shape the
// guard asked for rather than one chosen for tidiness.** The first
// attempt at sharing this logic put the C_Login call behind a
// `callLogin(pin []byte, n int)` primitive so that one `login` could
// serve both platforms — and `pin_test.go` refused it, correctly: a
// function parameter is a way for the PIN to travel to a second
// function, and the clause permits it in exactly one, as a local
// variable, in the function that obtains it, passes it to C_Login and
// overwrites it. The guard was written before the backend it constrains
// (D-269) and it caught the first thing that tried to widen it (D-349).
//
// So what is shared is what can be shared without the PIN crossing a
// boundary: the token's declared limits, the clamp, and what the screen
// is told. Those are numbers. The twenty lines that hold the characters
// are per platform, because the call at the end of them is, and they
// are covered on both by the same AST guard.

// pinBounds is what this layer will accept from a token, after
// §6.5.1's seventh clause has been applied to what the token declares.
type pinBounds struct {
	// min is the fewest bytes the token will accept.
	min int

	// max is the most this layer will collect: the token's own
	// maximum, clamped to what this layer will allocate for.
	max int
}

// maxPlausiblePINDeclaration is the largest ulMaxPinLen this layer
// treats as a *declaration* rather than as evidence that the structure
// it was read out of was misread.
//
// **The distinction is F11's central finding pointed at a field
// instead of at a layout** (D-353). Three of four hand-written
// CK_ATTRIBUTE layouts returned CKR_OK and garbage, and a wrong layout
// does not announce itself — except that some of the garbage is
// absurd, and an absurd number is then the only warning anybody gets.
// A token declaring a PIN of a million characters is not a permissive
// token; it is a structure read at the wrong offset. Clamping it would
// swallow the one signal.
//
// 255 because that is what a conforming software token declares
// (SoftHSM), and it is already an order of magnitude above every card
// this project has measured — MUP 8, Pošta 15.
const maxPlausiblePINDeclaration = 255

// pinBoundsFor applies clause 7 to a token's declaration.
//
// **A token may declare a maximum larger than this layer will allocate
// for, and that is not a reason to refuse the token** (D-349). It was,
// until this ran against a conforming module for the first time:
// SoftHSM declares ulMaxPinLen 255, which is not absurd — it is a
// software token being permissive — and refusing it outright made the
// whole PIN seam unexercisable on a platform with no card in it.
//
// So a *plausible* declared maximum is clamped, and the clamp is the effective
// maximum from there on: the buffer, the length check, and what the
// screen is told. No human PIN exceeds MaxPINLength — the two cards
// this project has measured declare 8 and 15 — so clamping cannot
// refuse a PIN somebody would type.
//
// A token whose *minimum* exceeds it is still refused, and that is the
// case the old check was really about: a token needing more than this
// layer will hold is one it cannot serve, and pretending otherwise
// would send a truncated PIN to a card and spend one of three attempts
// on it.
func pinBoundsFor(ti tokenInfo) (pinBounds, error) {
	minLen, maxLen := int(ti.MinPINLen), int(ti.MaxPINLen)
	if maxLen <= 0 {
		return pinBounds{}, fmt.Errorf("pkcs11: this token declares a maximum PIN length of %d", maxLen)
	}
	if minLen < 0 || minLen > maxLen {
		return pinBounds{}, fmt.Errorf("pkcs11: this token declares a minimum PIN length of %d against a maximum of %d", minLen, maxLen)
	}
	if maxLen > maxPlausiblePINDeclaration {
		return pinBounds{}, fmt.Errorf("pkcs11: this token declares a maximum PIN length of %d, which is not a PIN length; "+
			"the structure it was read from is more likely to be misread than the token to be unusual", maxLen)
	}
	if minLen > MaxPINLength {
		return pinBounds{}, fmt.Errorf("pkcs11: this token requires a PIN of at least %d characters, which is more than this layer will hold (%d)", minLen, MaxPINLength)
	}
	if maxLen > MaxPINLength {
		maxLen = MaxPINLength
	}
	return pinBounds{min: minLen, max: maxLen}, nil
}

// check reports whether a length the screen produced is one this token
// will accept. It takes a length and not a PIN.
func (b pinBounds) check(n int) error {
	if n < b.min || n > b.max {
		return &PINLengthError{Got: n, Min: b.min, Max: b.max}
	}
	return nil
}
