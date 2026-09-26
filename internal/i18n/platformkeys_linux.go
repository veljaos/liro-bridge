//go:build linux

package i18n

// platformKeys are the messages whose default sentence is false on Linux,
// each mapped to the key T reads instead (D-358). The error codes a caller
// receives do not change; only what a person reads does.
//
//   - SMART_CARD_SERVICE_DOWN is pcscd, and the remedy is a command, not
//     a restart of Windows' service.
//   - NO_READER's Linux sentence begins with the reader, by the owner's
//     ruling (D-361): a sentence about a certificate on a card says
//     something about a card that may not be in the machine. **Its limit,
//     recorded rather than hidden:** nothing on Linux lists readers — this
//     program does not talk PC/SC (F12 §9, SPEC §1.1) — so "no reader was
//     found" is reached whenever pcscd answers and no card gave a usable
//     certificate, reader attached or not. It carries F12 §9's second
//     support question, a reader that needs its maker's driver.
//   - `certs` cannot say "none attached", for the same reason.
var platformKeys = map[string]string{
	"error.smart_card_service_down": "error.smart_card_service_down_linux",
	"error.no_reader":               "error.no_reader_linux",
	"certs.readers_none":            "certs.readers_none_linux",
}
