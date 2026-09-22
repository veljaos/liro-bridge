//go:build !windows && !linux

package pkcs11

import "io"

// knownModulePaths has nothing in it yet on macOS and Linux, and that is a
// statement rather than a placeholder.
//
// SafeSign is the only one of the three Serbian middlewares shipping macOS and
// Linux builds (SPEC §11.11), so the list for those platforms is not the
// Windows list with different separators — it is one vendor's paths, and a
// person holding a MUP or Halcom card gets nothing from it. SPEC §11.11
// already says the agent must tell them which issuers are actually supported
// rather than reporting "no certificates found", and F11 §0.1 records that
// reaching a MUP card on those platforms needs PC/SC directly rather than
// anybody's middleware — a phase of its own, and not this one.
//
// Returning nothing is therefore correct here today: a configured path still
// works, because Candidates puts it first and does not consult this list.
func knownModulePaths() []Candidate { return nil }

// Modules answers that there is no binding here, naming every candidate.
//
// It is written out rather than sharing the Windows loop because on this
// platform the loop has no live branch: openModule can never succeed, so a
// shared version would carry a comparison that is always true — which
// staticcheck reports, correctly, as dead code. Saying it directly is both
// honest and shorter.
//
// A configured path still reaches here and is still named in the failure, so a
// person on macOS or Linux who has configured a module is told that the
// binding does not exist yet rather than that their path is wrong.
func Modules(configured string, _ func(modulePath string) io.Writer) ([]Candidate, []Failure) {
	candidates := Candidates(configured)
	failures := make([]Failure, 0, len(candidates))
	for _, c := range candidates {
		failures = append(failures, Failure{Candidate: c, Err: ErrPlatform})
	}
	return nil, failures
}
