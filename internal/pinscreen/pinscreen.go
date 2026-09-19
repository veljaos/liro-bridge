// Package pinscreen is the PIN entry: the one place a pkcs11.PINEntry is made
// out of the native dialog internal/ui draws and the wording internal/i18n
// holds.
//
// # What was missing, and why it was missing
//
// Both halves have existed since F11 and nothing joined them. D-279 §7 said so
// at the time — "the two halves exist and are not joined. pkcs11.PINEntry is a
// func type and ui.CollectPIN fills a buffer; joining them is a dozen lines
// that belong with F11 §4's wiring" — and left the seven pindialog.* catalogue
// keys with no reader, recording that they were deliberately not deleted
// because a reader was coming.
//
// So this package closes the D-247 shape: a feature fully implemented, fully
// tested, and never invoked. It is written now rather than with F11 §4's
// wiring because F12 §2's worker needs it first — the worker's parent asks a
// PINEntry for a PIN and there was nothing to give it.
//
// # Why it is its own package and not somewhere that already exists
//
// Three places were available and each is wrong for its own reason.
//
//   - internal/ui. That package states in pindialog_windows.go and beside
//     pickFolder that it has no i18n dependency and takes already-localised
//     strings from its caller. That is a design property rather than an
//     accident, and adding a catalogue to it to save a package would undo it.
//
//   - internal/keysource/pkcs11. It would make the key source layer import the
//     window layer, which is the architecture upside down — and nothing
//     mechanical stops it. scripts/checkdeps' rule 4 binds api, ui and cli to
//     each other and says nothing about this direction, so it is a judgement
//     and is written down here for that reason.
//
//   - cmd/liro-bridge. It is a main package, so scripts/p11worker cannot
//     import from it, and the thing that would then exist twice is SPEC
//     §6.5.1 clause 6's own mapping — which screen says what, for which card.
//     One rule in two places is what D-108, D-124 and D-138 each had to
//     remove once.
//
// So: one small package both readers reach, on internal/pinname's precedent —
// which exists for exactly this reason, to give four packages one answer
// rather than four.
//
// # The split between this file and the platform ones
//
// Everything about *what the screen says* is here, with no syscall in it, so
// it is tested on every platform — including the Linux runner, which is the
// only place this project's CI runs the race detector (D-012, D-112). Only the
// call that puts a window on a screen is platform-specific.
//
// That is also the half F12 §5 will reuse: the Linux PIN dialog is a different
// window drawing the same sentences, and the sentences are the part SPEC
// §6.5.1's sixth clause is about.
package pinscreen

import (
	"fmt"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/keysource/pkcs11"
)

// Prompt is the text one PIN screen shows, already localised.
//
// It mirrors ui.PINPrompt field for field and is a separate type on purpose:
// ui.PINPrompt is Windows-only, because the window that renders it is, and the
// wording is not. A neutral type is what lets Text be tested where there is no
// window, and what stops F12's GTK dialog needing a second copy of the
// sentences.
type Prompt struct {
	// Title is the window's caption.
	Title string

	// Heading names this program. It is the whole of SPEC §6.5.1's sixth
	// clause — "a person must be able to tell they are giving their PIN to
	// Liro Bridge rather than to the card or to Windows" — and it matters more
	// here than it would elsewhere precisely because the window it is drawn on
	// deliberately looks like a system dialog (SPEC §10.2, D-277, D-278).
	Heading string

	// Subject says which card is being asked about.
	Subject string

	// Label is the input's own label.
	Label string

	// Hint states the token's own limits, so a person is told the rule rather
	// than discovering it by being refused.
	Hint string

	// OK and Cancel are the two buttons. There is no third: SPEC §6.5.1 clause
	// 5 forbids a retry, so there is nothing for a "try again" to do.
	OK     string
	Cancel string
}

// Text builds the prompt for one request, in one locale.
//
// maxLen is the token's own ulMaxPinLen, which is also the length of the
// buffer the PIN will be written into — pkcs11.PINRequest carries the minimum
// and deliberately not the maximum, because the buffer *is* the maximum and a
// number that could disagree with the buffer it describes is a number worth
// not having.
//
// # The limits are the token's and are never this program's
//
// SPEC §6.5.1's seventh clause, as D-276 amended it: "the limits are the
// token's, read when they are needed: measured, a MUP token declares 4 and 8
// where a Pošta token declares 5 and 15, and both live in one person's drawer.
// A screen built around either pair is wrong for the other card." So the hint
// is formatted from the request rather than from anything written down here,
// and TestTheHintCarriesTheTokensOwnLimits is what keeps it that way.
//
// # What is on the screen and what is not
//
// The card's label, and nothing else about where the PIN is going. Not the
// serial, which is not a thing a person reads. Not the module path, which is
// the only thing that tells two sightings of one card apart (D-271, D-272) and
// is therefore diagnostic rather than something to put in front of somebody
// about to type a secret.
//
// Not the certificate label either, and that one is a decision rather than an
// omission. On the Pošta card this project signs with, the token label and the
// certificate label are the same string — "Savka Odžić 200100123" — so showing
// both would show a person their own name twice, which is exactly the defect
// D-149 removed from the certificate list. It is used only when the token has
// no label of its own to show.
func Text(cat *i18n.Catalogue, req pkcs11.PINRequest, maxLen int) Prompt {
	card := req.TokenLabel
	if card == "" {
		card = req.CertificateLabel
	}
	return Prompt{
		Title:   cat.T("pindialog.title"),
		Heading: cat.T("pindialog.heading"),
		Subject: fmt.Sprintf(cat.T("pindialog.subject"), card),
		Label:   cat.T("pindialog.label"),
		Hint:    fmt.Sprintf(cat.T("pindialog.hint"), req.MinLength, maxLen),
		OK:      cat.T("pindialog.ok"),
		Cancel:  cat.T("pindialog.cancel"),
	}
}
