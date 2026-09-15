package pkcs11

import (
	"os"
	"path/filepath"
	"strings"
)

// Origin says where a candidate module path came from. It is not decoration:
// a configured path is a person's own instruction and is tried first and
// reported when it fails, where a known path that is simply absent is not a
// failure at all.
type Origin int

const (
	// OriginConfigured is a path the person put in their configuration. It is
	// the escape hatch for every installation this project did not anticipate
	// (F11 §3).
	OriginConfigured Origin = iota
	// OriginKnown is a path measured off a real machine, where an issuer's own
	// installer puts its module.
	OriginKnown
)

func (o Origin) String() string {
	if o == OriginConfigured {
		return "configured"
	}
	return "known path"
}

// Candidate is a module this machine might have.
type Candidate struct {
	Path   string
	Vendor string
	Origin Origin
}

// Failure is a candidate that did not turn out to be a usable module, and why.
//
// It exists because F11 §3 is explicit that a module which fails to load is
// not a crash: say which path, say the module did not load, and carry on with
// the sources that did. Every one of these is something to log and continue
// past, never something to stop for.
type Failure struct {
	Candidate Candidate
	Err       error
}

func (f Failure) Error() string { return f.Candidate.Path + ": " + f.Err.Error() }

// Candidates returns the module paths worth trying on this machine, the
// configured one first.
//
// It reads the filesystem and nothing else — no module is loaded here, so
// nothing here can run anybody's DllMain. Use Modules for the half that does.
//
// # Where a configured path may come from, and where it may not
//
// From the person's own configuration, and from nowhere else. A path supplied
// over the protocol is arbitrary code execution wearing a configuration field
// (F11 §3): a calling application that can name a DLL for this agent to load
// has taken the agent over, and no amount of checking the file first changes
// that. Nothing in this package reads a request, and nothing above it may pass
// one through to here.
func Candidates(configured string) []Candidate {
	var out []Candidate
	seen := map[string]bool{}

	add := func(path, vendor string, origin Origin) {
		if path == "" {
			return
		}
		key := strings.ToLower(filepath.Clean(path))
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Candidate{Path: path, Vendor: vendor, Origin: origin})
	}

	// The configured path is tried first and is not required to exist yet: a
	// person who has typed a path and got it slightly wrong deserves to be
	// told so by name, which means it has to reach Modules rather than being
	// filtered out here.
	add(strings.TrimSpace(configured), "configured", OriginConfigured)

	for _, k := range knownModulePaths() {
		if fileExists(k.Path) {
			add(k.Path, k.Vendor, OriginKnown)
		}
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Sources returns one Source per usable module, and every candidate that was
// not one.
//
// One card can appear through more than one of them — this project's own
// machine has NetSeT installed at two paths in two builds five years apart,
// and both see the same card identically. Collapsing that into one row is the
// caller's job and the thumbprint is what makes it possible (F11 §4).
func Sources(configured string) ([]Source, []Failure) {
	usable, failures := Modules(configured)
	out := make([]Source, 0, len(usable))
	for _, c := range usable {
		out = append(out, NewSource(c.Path))
	}
	return out, failures
}
