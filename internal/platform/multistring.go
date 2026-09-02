package platform

import "unicode/utf16"

// parseMultiString splits a Windows double-null-terminated UTF-16
// multi-string ("Reader A\0Reader B\0\0") into individual strings.
//
// windows.UTF16PtrToString stops at the first NUL and would silently
// return only the first reader (F1 §2.3) — this function is the reason
// that shortcut is not used for SCardListReadersW's output.
func parseMultiString(buf []uint16) []string {
	var out []string
	start := 0
	for i, c := range buf {
		if c != 0 {
			continue
		}
		if i > start {
			out = append(out, string(utf16.Decode(buf[start:i])))
		}
		start = i + 1
	}
	if out == nil {
		return []string{}
	}
	return out
}
