//go:build linux

package i18n

// platformKeys are the messages whose default sentence is false on Linux,
// each mapped to the key T reads instead (D-358). The error codes a caller
// receives do not change; only what a person reads does.
//
//   - SMART_CARD_SERVICE_DOWN is pcscd, and the remedy is a command, not
//     a restart of Windows' service.
//   - An empty certificate list is explained from the PKCS#11 modules' own
//     slots (cli.CardSlots), because nothing else on Linux lists readers —
//     this program does not talk PC/SC (F12 §9, SPEC §1.1). Until open item
//     A20, NO_READER's sentence said "no card reader was found" whenever no
//     card gave a usable certificate, and a person with a MUP card in a
//     plugged-in reader was told so. Now:
//     NO_READER is "no installed card program sees a reader", which is true
//     both when none is installed and when one is and no reader is attached,
//     and it keeps F12 §9's driver question; CARD_NOT_PRESENT is "no card";
//     and an empty list with a card in the reader says no signing
//     certificate was found on any card. Those two sentences where a person
//     may hold a card nothing here reads end with which issuers this program
//     supports on Linux — SPEC §11.11's requirement. That paragraph says
//     what was measured of this program (Pošta through SafeSign, D-361; MUP
//     read by neither SafeSign nor OpenSC, A20) and that Halcom is untried,
//     and says nothing about what other vendors ship: that belongs in the
//     guide, by the owner's ruling.
//   - `certs` cannot say "none attached", for the same reason.
var platformKeys = map[string]string{
	"error.smart_card_service_down": "error.smart_card_service_down_linux",
	"error.no_reader":               "error.no_reader_linux",
	"error.card_not_present":        "error.card_not_present_linux",
	"consent.no_certificate_found":  "consent.no_certificate_found_linux",
	"certs.readers_none":            "certs.readers_none_linux",
}
