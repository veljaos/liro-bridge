package ui

import (
	"bytes"
	"encoding/binary"
)

// icoPNGEntry returns the PNG stored in the .ico entry whose width is
// nearest want, or nil. It is the directory walk decodeICO does on
// Linux, without the decoding: macOS wants the PNG's own bytes for an
// NSImage (F13's status-bar item), and every entry of this project's
// icon is a PNG (scripts/genicon). A width byte of 0 means 256.
func icoPNGEntry(data []byte, want int) []byte {
	const headerLen, entryLen = 6, 16
	if len(data) < headerLen || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	var best []byte
	bestDist := -1
	for i := 0; i < count; i++ {
		off := headerLen + i*entryLen
		if off+entryLen > len(data) {
			break
		}
		width := int(data[off])
		if width == 0 {
			width = 256
		}
		size := int(binary.LittleEndian.Uint32(data[off+8 : off+12]))
		at := int(binary.LittleEndian.Uint32(data[off+12 : off+16]))
		if at < 0 || size < 0 || at+size > len(data) {
			continue
		}
		blob := data[at : at+size]
		if !bytes.HasPrefix(blob, []byte("\x89PNG\r\n\x1a\n")) {
			continue
		}
		dist := width - want
		if dist < 0 {
			dist = -dist
		}
		if bestDist < 0 || dist < bestDist {
			best, bestDist = blob, dist
		}
	}
	return best
}
