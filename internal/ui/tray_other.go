//go:build !windows

package ui

func newTray(TrayOptions) (Tray, error) {
	return nil, ErrUnsupportedPlatform
}
