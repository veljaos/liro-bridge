//go:build linux

package i18n

// platformKeys are the messages whose default sentence is false on Linux,
// each mapped to the key T reads instead (D-358). The error codes a caller
// receives do not change; only what a person reads does.
//
//   - SMART_CARD_SERVICE_DOWN is pcscd, and the remedy is a command, not
//     a restart of Windows' service.
//   - NO_READER cannot say "no card reader detected", and `certs` cannot
//     say "none attached", because nothing on Linux lists readers: this
//     program does not talk PC/SC (F12 §9, SPEC §1.1), so the list is
//     always empty. What is known is that no card gave a certificate, and
//     F12 §9's second support question — a reader that needs its maker's
//     driver — belongs in the same sentence.
var platformKeys = map[string]string{
	"error.smart_card_service_down": "error.smart_card_service_down_linux",
	"error.no_reader":               "error.no_reader_linux",
	"certs.readers_none":            "certs.readers_none_linux",
}
