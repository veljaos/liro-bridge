//go:build !linux

package pkcs11

// exitHint has nothing to add off Linux; see loaderror_linux.go.
func exitHint(string, int) *LoadError { return nil }
