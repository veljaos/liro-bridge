//go:build !windows

package platform

// noSigningFlag is what every platform but Windows gets. There is no
// installer on those platforms yet (SPEC §19: macOS is phase 12, Linux
// phase 13), so there is nothing for a mark to be read by — and a
// no-op that reports "not held" is the honest answer to "is an
// installer about to be blocked", not a pretence that a mark was made.
type noSigningFlag struct{}

// NewSigningFlag returns the non-Windows no-op.
func NewSigningFlag() SigningFlag { return noSigningFlag{} }

func (noSigningFlag) Begin() error             { return nil }
func (noSigningFlag) End() error               { return nil }
func (noSigningFlag) Clear() error             { return nil }
func (noSigningFlag) Held() (bool, int, error) { return false, 0, nil }
