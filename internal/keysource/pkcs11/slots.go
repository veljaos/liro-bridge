package pkcs11

// SlotSurvey is what one module says about the card readers it reaches and the
// cards in them, without reading any card: counts, never a reader's name or a
// token's label.
//
// It exists because Linux has no other witness. This program does not talk
// PC/SC (F12 §9, SPEC §1.1), so an empty certificate list used to be explained
// as "no reader was found" with the reader plugged in — measured with a MUP
// card in a passed-through reader, which SafeSign and OpenSC both reported as
// present and unrecognised while the window said there was no reader (open
// item A20). The modules had the answer; enumerate threw it away as "not
// mine", which is right for one module's listing and wrong for the machine's.
//
// The counts are per module and are summed above, where several modules can
// report the same reader and the same card: nothing may read them as a number
// of devices, only as "none" or "some".
type SlotSurvey struct {
	// ReaderSlots are slots flagged CKF_REMOVABLE_DEVICE — a slot a card goes
	// into. Measured: SafeSign and OpenSC flag a reader's slot so; the
	// gnome-keyring and p11-kit-trust modules every Ubuntu desktop registers
	// flag none of theirs. **SafeSign also offers four placeholder slots named
	// "UNAVAILABLE 1" to "4", removable and empty, with or without a reader** —
	// so a reader slot is evidence that a card program is installed, and is not
	// evidence that a reader is attached.
	ReaderSlots int `json:"readerSlots"`
	// CardsPresent are reader slots flagged CKF_TOKEN_PRESENT.
	CardsPresent int `json:"cardsPresent"`
	// CardsUnrecognised are present cards whose C_GetTokenInfo answered
	// CKR_TOKEN_NOT_RECOGNIZED: a card this module sees and cannot read.
	CardsUnrecognised int `json:"cardsUnrecognised"`
}
