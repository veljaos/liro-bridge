//go:build !windows

package platform

import "context"

// unsupportedSmartCardService is used on macOS and Linux, which arrive in
// phases 12 and 13 (F1 §2.4). It reports no readers rather than an error,
// since "no smart card support on this platform yet" is not the same
// failure as "reader hardware absent".
type unsupportedSmartCardService struct{}

func (unsupportedSmartCardService) Readers(context.Context) ([]ReaderState, error) {
	return []ReaderState{}, nil
}

func (s unsupportedSmartCardService) AnyCardPresent(ctx context.Context) (bool, error) {
	return anyCardPresent(ctx, s)
}

// NewSmartCardService returns the stub SmartCardService for platforms
// without a reader-detection implementation yet.
func NewSmartCardService() SmartCardService {
	return unsupportedSmartCardService{}
}
