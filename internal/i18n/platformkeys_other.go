//go:build !linux

package i18n

// platformKeys is empty where every default sentence is true. See
// platformkeys_linux.go.
var platformKeys = map[string]string{}
