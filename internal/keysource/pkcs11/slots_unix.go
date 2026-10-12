//go:build linux

package pkcs11

import "context"

// Slot flags from PKCS#11 v2.40 §3.2, CK_SLOT_INFO.
const (
	ckfTokenPresent    = 0x1
	ckfRemovableDevice = 0x2
)

// Slots is the module's SlotSurvey. It opens no session and reads nothing
// off a card: C_GetSlotList, C_GetSlotInfo, and C_GetTokenInfo for a slot
// that says a card is in it.
func (l *LiveModule) Slots(ctx context.Context) (*SlotSurvey, error) {
	if l.module == nil {
		return nil, ErrModuleClosed
	}
	return survey(ctx, l.module)
}

func survey(ctx context.Context, m *module) (*SlotSurvey, error) {
	slots, err := m.slots(false)
	if err != nil {
		return nil, err
	}
	var out SlotSurvey
	for _, slot := range slots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		flags, err := m.slotFlags(slot)
		if err != nil {
			return nil, err
		}
		if flags&ckfRemovableDevice == 0 {
			continue
		}
		out.ReaderSlots++
		if flags&ckfTokenPresent == 0 {
			continue
		}
		if _, err := m.tokenInfo(slot); err != nil {
			rv, _ := asCKR(err)
			switch rv {
			case ckrTokenNotRecognized:
				out.CardsPresent++
				out.CardsUnrecognised++
			case ckrTokenNotPresent:
				// Taken out between the two calls: not a card.
			default:
				return nil, err
			}
			continue
		}
		out.CardsPresent++
	}
	return &out, nil
}
