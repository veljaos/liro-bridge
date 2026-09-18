package pkcs11

import (
	"errors"
	"fmt"
	"runtime"
)

// This file is the PIN seam, and it carries no build constraint on purpose.
//
// Everything here is ordinary Go with no syscall in it, so it compiles and is
// tested on every platform — including the Linux runner, which is the only
// place this project's CI runs the race detector (D-012, D-112). The half that
// actually calls C_Login is Windows-only and lives in login_windows.go.
//
// It also means the type a caller has to satisfy exists on every platform, so
// wiring written for F12 or F13 does not have to be written twice.

// ErrNoPINEntry is returned when a token needs a PIN and nothing was wired up
// to collect one. It is a sentinel so the layer above can tell it from a card
// refusing a PIN, which is an entirely different thing to tell a person.
var ErrNoPINEntry error = errors.New("pkcs11: this token has no protected authentication path and no PIN entry was configured")

// ErrPINCancelled is what a PINEntry returns when the person closed the screen
// rather than answering it. A cancellation is not a failure and must not be
// reported as one — the same distinction the folder chooser already draws, and
// the same one D-145 had to make for a window closed before its first payload.
var ErrPINCancelled error = errors.New("pkcs11: the person cancelled the PIN screen")

// PINLengthError is a PIN refused by this layer for its length, before it
// reached the card.
//
// It exists because of what it prevents, which was measured and was not free.
// D-268 took one authorised C_Login with a NULL PIN on a MUP token: the module
// passed it to the card as an empty PIN, the card rejected it as a wrong PIN,
// and one of three attempts was gone. The token had declared minPin=4 the
// whole time, and the module range-checked nothing — a declared limit
// describes what the card accepts, not what the module enforces. SPEC §6.5.1's
// seventh clause exists for exactly that, and this type is where it bites: a
// PIN that cannot possibly be right never reaches the card, so it cannot cost
// an attempt.
type PINLengthError struct{ Got, Min, Max int }

func (e *PINLengthError) Error() string {
	return fmt.Sprintf("pkcs11: a PIN of %d characters cannot be right for this token, which accepts %d to %d; it was not sent to the card",
		e.Got, e.Min, e.Max)
}

// PINRequest is what a PIN screen needs in order to obey SPEC §6.5.1's sixth
// clause — a person must be able to tell they are giving their PIN to Liro
// Bridge, and for which card.
//
// It carries no PIN and never will; it is the question, not the answer.
type PINRequest struct {
	// TokenLabel and TokenSerial are the card's own, as CK_TOKEN_INFO reports
	// them — "Savka Odžić 200100123" on the Pošta card this was measured on.
	TokenLabel  string
	TokenSerial string

	// CertificateLabel is the certificate about to be signed with, so that a
	// person with two cards can tell which one is being asked about.
	CertificateLabel string

	// ModulePath is the middleware this is going through. A machine can have
	// one card visible through two modules at two versions (D-271, D-272), and
	// when something is wrong this is the only thing that tells them apart.
	ModulePath string

	// MinLength is the fewest characters this token will accept. The most is
	// len(dst) — one fact in one place, since a second copy could disagree
	// with the buffer it describes.
	MinLength int
}

// PINEntry collects the token's PIN into dst and reports how many bytes it
// wrote.
//
// dst is exactly the token's own ulMaxPinLen bytes long, and is the buffer the
// PIN will be passed to C_Login in. Write the PIN into it, return the length,
// and write it nowhere else.
//
// # Why a callback that fills a caller's buffer, rather than a function that returns a PIN
//
// SPEC §6.5.1's second clause: the PIN exists only for the duration of
// C_Login, and is overwritten immediately afterwards — not left for the
// garbage collector, not held in a struct field, not captured by a closure
// that outlives the call, and not merely dropped.
//
// A function returning a PIN cannot satisfy that. If it returned a string the
// value could never be overwritten at all, because a Go string is immutable —
// which is the same fact that decided the PIN screen is a native window rather
// than a page (D-277). If it returned a []byte, the buffer would be the
// callee's, and this layer would be overwriting a copy while the original
// stayed wherever it was made. Filling a buffer the caller allocated, pins,
// and wipes in the same function that calls C_Login is the only arrangement in
// which there is exactly one copy and this layer owns it.
//
// The shape is not a preference. pin_test.go refuses a struct field, a
// function parameter or a named result that is named after a PIN and could
// hold one, and it was written before this backend existed precisely so the
// backend would have to be built to satisfy it (D-269). There is no
// login(session, pin []byte) to write, so there is no second function that has
// ever seen it.
//
// An implementation must not retry, must not log what it collected, and must
// return ErrPINCancelled — never a PIN of length zero — when the person
// cancelled. Nothing above this retries either (clause 5): one wrong PIN is
// one of three attempts, and the third means a visit to a police station.
type PINEntry func(dst []byte, req PINRequest) (n int, err error)

// Wipe overwrites b and keeps it alive across the write.
//
// It is exported because there are two buffers now and one rule. SPEC §6.5.1
// clause 2 permits the PIN to cross exactly one process boundary, so it exists
// in two places this program owns: the child's, which login allocates, pins and
// passes to C_Login, and the parent's, which the PIN screen fills and the pipe
// is written from. Both are overwritten, and a wipe written twice is the one
// function where a second copy would be worst — it is the only thing standing
// between the clause and a buffer nobody overwrote.
//
// Everything about how it works is in wipe's own comment below, and the
// KeepAlive there is not decoration.
func Wipe(b []byte) { wipe(b) }

// MaxPINLength bounds the buffer a token's own ulMaxPinLen is allowed to ask
// for.
//
// A card PIN is a handful of characters — the two this project has measured
// declare 8 (MUP) and 15 (Pošta) — and a token reporting something far larger
// is a token this layer should refuse rather than allocate for. 64 is well
// clear of anything a person types and small enough that a wrong value cannot
// become an allocation worth noticing.
//
// It is exported for the same reason Wipe is: the parent allocates its buffer
// from a number the child sent it, and both ends refusing the same number is
// one rule rather than two that could disagree.
//
// It lives here, unconstrained, so that the parent — which is not
// platform-specific — reads the same constant the login does.
const MaxPINLength = 64

// wipe overwrites b and keeps it alive across the write.
//
// runtime.KeepAlive is not decoration: with no use after the loop the stores
// have no reader, and a compiler is entitled to notice that. Pinning at the
// call site puts the buffer at a fixed address as well, which is what lets
// TestWipeZeroesTheBytesAtThePinnedAddress read the result back through that
// address rather than through the slice header — a loop that was elided and a
// loop that ran look identical through the slice.
func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
	runtime.KeepAlive(b)
}
