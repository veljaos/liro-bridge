package platform

import "context"

// ReaderState describes one smart card reader attached to the machine.
type ReaderState struct {
	// Name is the reader name as reported by the OS, e.g.
	// "Generic Smart Card Reader Interface 0".
	Name string

	// CardPresent is true when a card is inserted and powered.
	CardPresent bool

	// ATR is the card's Answer To Reset, empty when no card is present.
	// Useful for identifying the card type; do not rely on it for
	// security decisions.
	ATR []byte
}

// SmartCardService reports on attached readers.
//
// Presence must always be read from here, never inferred from whether a
// certificate enumerates: SPEC §11.10 measured that certificates remain
// listed in the Windows store after the card is physically removed.
type SmartCardService interface {
	// Readers lists every attached reader and whether it holds a card.
	// Returns an empty slice — not an error — when no reader is attached.
	Readers(ctx context.Context) ([]ReaderState, error)

	// AnyCardPresent is a convenience wrapper: true if at least one
	// reader holds a card.
	AnyCardPresent(ctx context.Context) (bool, error)
}

// anyCardPresent implements SmartCardService.AnyCardPresent in terms of
// Readers, shared by every platform implementation.
func anyCardPresent(ctx context.Context, svc SmartCardService) (bool, error) {
	readers, err := svc.Readers(ctx)
	if err != nil {
		return false, err
	}
	for _, r := range readers {
		if r.CardPresent {
			return true, nil
		}
	}
	return false, nil
}
