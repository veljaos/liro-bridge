//go:build !windows && !linux && !(darwin && cgo)

package ui

func newTray(TrayOptions) (Tray, error) {
	return nil, ErrUnsupportedPlatform
}
