//go:build !linux

package pkcs11

import "context"

// Slots answers nothing off Linux. On Windows SCardListReaders already says
// whether a reader and a card are there (SPEC §11.10), and F12 leaves Windows'
// behaviour as it was; a nil survey is how the layers above know to use that.
func (l *LiveModule) Slots(context.Context) (*SlotSurvey, error) { return nil, nil }
