package pkcs11

import "errors"

// errNothingRecognisable is a file that loaded and answered C_GetInfo with
// nothing a PKCS#11 module would say.
//
// It lives beside the only code that can produce it. It was briefly in
// probe.go, on the reasoning that "both the parent and the child reach it" —
// which was false: the parent stopped needing it the moment Modules began
// reading the child's reported string instead of deciding for itself.
var errNothingRecognisable = errors.New("C_GetInfo returned nothing recognisable")

// describeModule loads one module and says what it is, or why it is not one.
//
// It is split per platform rather than shared, for the reason discover_other.go
// already records about Modules: on a platform with no binding, openModule can
// never succeed, so a shared version carries an `if err != nil` that is always
// true — and staticcheck reports that, correctly, as dead code (SA4023). The
// first version of RunProbe did exactly that, and the Ubuntu lint job caught it
// where two local runs did not, because both of those linted the Windows view.
//
// The split is not a concession to a linter. The analyzer was pointing at
// something real: a function written as though it were platform-neutral over a
// step that is not.
func describeModule(path string) probeResult {
	m, err := openModule(path)
	if err != nil {
		return probeResult{Err: err.Error()}
	}

	info, infoErr := m.info()
	_ = m.close()

	switch {
	case infoErr != nil:
		return probeResult{Err: infoErr.Error()}
	case info.LibraryDescription == "" && info.Manufacturer == "":
		return probeResult{Err: errNothingRecognisable.Error()}
	default:
		return probeResult{
			OK:                 true,
			Manufacturer:       info.Manufacturer,
			LibraryDescription: info.LibraryDescription,
		}
	}
}
