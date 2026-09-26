//go:build linux

package pkcs11

import "errors"

// describeModule loads the module in this child process and reads
// CK_INFO, which is the check discovery actually relies on: a file with
// "pkcs11" in its name is not a module, and the only thing that settles
// it is asking the module about itself.
//
// It runs in the probe child rather than in the agent because a module
// that kills the process must kill only this one (F12 §2, D-275) — and
// on this platform there is a second reason with the same shape: an
// older vendor module can want symbols from an OpenSSL the distribution
// no longer ships, and dlopen fails with an undefined symbol rather
// than with anything meaningful (F12 §10). That is a Failure with a
// readable reason, and it is readable because the child reports it
// rather than dying with it.
func describeModule(path string) probeResult {
	m, err := openModule(path)
	if err != nil {
		res := probeResult{Err: err.Error()}
		var le *LoadError
		if errors.As(err, &le) {
			res.Load = le
		}
		return res
	}
	defer func() { _ = m.close() }()

	info, err := m.info()
	if err != nil {
		return probeResult{Err: err.Error()}
	}
	return probeResult{
		OK:                 true,
		Manufacturer:       info.Manufacturer,
		LibraryDescription: info.LibraryDescription,
	}
}
