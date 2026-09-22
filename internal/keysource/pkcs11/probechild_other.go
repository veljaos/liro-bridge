//go:build !windows && !linux

package pkcs11

// describeModule answers that there is no binding here, without pretending to
// try.
//
// The Windows version loads the module and reads C_GetInfo. Writing one
// function for both would mean testing an error that can only ever be non-nil
// on this platform, which staticcheck reports as dead code (SA4023) and which
// discover_other.go's Modules was already split to avoid. Saying it directly is
// honest and shorter, and it keeps the platform difference in the files where
// every other platform difference in this package lives.
//
// The probe subcommand therefore still exists on macOS and Linux and still
// answers with one JSON object — a person who runs it gets "not supported on
// this platform yet" rather than silence or a crash — and the parent turns that
// into the same Failure it would get from any other module it could not use.
// The path is not named in the answer because the parent's Failure already
// carries the candidate it asked about.
func describeModule(_ string) probeResult {
	return probeResult{Err: ErrPlatform.Error()}
}
