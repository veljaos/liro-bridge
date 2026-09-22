package ui

import (
	"errors"
	"unicode/utf8"
)

// This file is the PIN dialog's platform-neutral half, and it carries no build
// constraint on purpose.
//
// The sentinel below is neutral for the reason window.go's own two are: a
// caller above this layer branches on one value whatever it was compiled for,
// and a sentinel that exists on one platform is one a cross-platform caller
// has to spell twice. The encoding is neutral because it is ordinary Go with
// no syscall in it, so it is tested everywhere — including the Linux runner,
// which is the only place this project's CI runs the race detector (D-012,
// D-112).

// ErrPINTooLong is returned by CollectPIN when what was typed fits the edit
// control and does not fit the token's own buffer.
//
// # It exists because those two limits are counted in different units
//
// EM_SETLIMITTEXT bounds the control in **characters**. PKCS#11's
// ulMaxPinLen — the number this program sizes the buffer from, and the
// ulPinLen it passes to C_Login — is in **bytes**. For the two Serbian cards
// this project has measured the limits are 8 (MUP) and 15 (Pošta), so fifteen
// Cyrillic characters are fifteen the control accepts and thirty bytes the
// token will not take.
//
// Refusing is right, and SPEC §6.5.1's seventh clause is why: half a PIN is a
// wrong PIN, and a wrong PIN is one of three attempts. Truncating would spend
// one.
//
// # What this actually fixes
//
// Reporting it as a **cancellation** was the defect. CollectPIN answered this
// case with ok=false and no error, which is the same answer it gives for a
// person pressing Cancel — so a person who typed something and pressed OK
// would have been told nothing at all, and every layer above would have
// recorded a cancellation nobody performed. A cancellation is not a failure
// and must not be reported as one (D-145); the converse holds just as
// strongly, and it is the direction that loses information.
//
// It was found by writing the first caller this dialog has ever had. Nothing
// had called CollectPIN, so nothing had ever had to tell the two apart.
//
// It is `var x error = …` rather than an inferred type for D-270's reason,
// which login_windows.go and this file's own class names already record: the
// PIN guard asks what a declaration can hold rather than what it is called,
// and a var with a call for an initialiser and no type expression is one it
// cannot see through. Naming the type answers its question truthfully — an
// error cannot hold a PIN's characters — where renaming a correct name to
// dodge a matcher is the tail wagging the dog.
var ErrPINTooLong error = errors.New("ui: what was typed is longer than this token's own maximum, so it was not accepted")

// encodePINInto writes runes into dst as UTF-8 and reports how many bytes it
// wrote, or -1 if they do not fit.
//
// # Nothing is written unless all of it fits
//
// The length is totalled first and the encoding only then. Writing as far as
// the buffer allows and reporting failure afterwards would leave a prefix of
// what was typed in the caller's buffer on a path the caller is being told
// produced nothing — the caller does overwrite it (every one of them wipes on
// every path out), but "it is wiped anyway" is a reason to be careless that
// this clause has no room for.
//
// # Why it is a function rather than a loop inside accept
//
// So that the one decision in this dialog that can be got wrong without a
// window — how many bytes a string of characters needs, and whether that fits
// — is tested on every platform rather than only where a message loop can run.
// The rest of accept is Win32 and cannot be.
func encodePINInto(dst []byte, runes []rune) int {
	total := 0
	for _, r := range runes {
		n := utf8.RuneLen(r)
		if n < 0 {
			// utf16.Decode replaces an unpaired surrogate with U+FFFD, which
			// is encodable, so this is unreachable through the dialog. It is
			// refused rather than skipped because a character silently dropped
			// from a PIN is a wrong PIN, which costs an attempt.
			return -1
		}
		total += n
	}
	if total > len(dst) {
		return -1
	}
	at := 0
	for _, r := range runes {
		at += utf8.EncodeRune(dst[at:], r)
	}
	return at
}

// PINPrompt is the text the dialog shows, already localised by the caller.
//
// This package has no i18n dependency — SPEC §4.2 rule 4 keeps internal/ui,
// internal/api and internal/cli independent of each other, and pickFolder and
// ShowRuntimeMissingMessage already take their strings the same way. The three
// locales are the caller's, which is where the catalogue is.
type PINPrompt struct {
	// Title is the window's caption.
	Title string

	// Heading names this program, and it is the whole of §6.5.1's sixth
	// clause. It is drawn first, in the heavier face, above everything else.
	Heading string

	// Subject says which card and which certificate is being asked about.
	Subject string

	// Label is the edit control's own label.
	Label string

	// Hint says how many characters this token accepts, so a person is told
	// the rule rather than discovering it by being refused.
	Hint string

	// OK and Cancel are the two buttons.
	OK     string
	Cancel string
}
