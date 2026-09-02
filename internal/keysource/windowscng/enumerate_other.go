//go:build !windows

package windowscng

import "context"

// Enumerate returns no certificates on platforms other than Windows.
// CNG is a Windows-only API; macOS and Linux get their own key sources
// in phases 12 and 13.
func Enumerate(_ context.Context) ([]Certificate, error) {
	return nil, nil
}
