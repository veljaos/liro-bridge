//go:build !windows && !linux

package ui

func newTray(TrayOptions) (Tray, error) {
	return nil, ErrUnsupportedPlatform
}
