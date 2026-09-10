package update

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Version is a semantic version, compared by precedence exactly as
// semver 2.0.0 §11 defines it: numerically by major, minor and patch,
// then a pre-release sorting *before* the release it precedes, with
// build metadata ignored entirely.
//
// This is the same rule docs/PROTOCOL.md §4.1 states for
// minimumClientVersion, and it is stated once here rather than twice:
// two comparisons of the same shape in one program is how they come to
// disagree (D-108, D-124, D-138).
type Version struct {
	Major, Minor, Patch int

	// Pre is the pre-release identifier set, already split on ".", or
	// nil for a release. Its presence is what makes 1.0.0-rc.1 sort
	// before 1.0.0.
	Pre []string
}

// ErrNotAVersion is returned for a string that is not a semantic
// version. `dev` is the one this project produces itself: an unstamped
// build (cmd/liro-bridge's `version` default) says so rather than
// pretending to be 0.0.0, which would compare as older than every
// release and offer a developer an "update" over their own build.
var ErrNotAVersion = errors.New("update: not a semantic version")

// ParseVersion parses "1.2.3", "1.2.3-rc.1" or "1.2.3+build.5". A
// leading "v" is accepted because a git tag carries one and a manifest
// does not, and refusing it would make the two spellings of one release
// disagree.
func ParseVersion(s string) (Version, error) {
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return Version{}, fmt.Errorf("%w: %q", ErrNotAVersion, s)
	}

	// Build metadata is not part of precedence (semver §10), so it is
	// dropped before anything else looks at the string.
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}

	var pre []string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		preStr := s[i+1:]
		s = s[:i]
		if preStr == "" {
			return Version{}, fmt.Errorf("%w: %q has an empty pre-release", ErrNotAVersion, s)
		}
		pre = strings.Split(preStr, ".")
		for _, id := range pre {
			if id == "" {
				return Version{}, fmt.Errorf("%w: %q has an empty pre-release identifier", ErrNotAVersion, preStr)
			}
		}
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("%w: %q is not MAJOR.MINOR.PATCH", ErrNotAVersion, s)
	}
	nums := make([]int, 3)
	for i, p := range parts {
		// Leading zeros are forbidden by semver §2 and are rejected
		// rather than tolerated: "01" and "1" would otherwise be two
		// spellings of one release, and only one of them is what a tag
		// says.
		if len(p) > 1 && p[0] == '0' {
			return Version{}, fmt.Errorf("%w: %q has a leading zero", ErrNotAVersion, p)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("%w: %q is not a number", ErrNotAVersion, p)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Pre: pre}, nil
}

// String renders the version back the way it was written, without the
// build metadata precedence ignores.
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	return s
}

// Compare returns -1, 0 or 1 as v sorts before, equal to, or after w.
func (v Version) Compare(w Version) int {
	for _, pair := range [][2]int{{v.Major, w.Major}, {v.Minor, w.Minor}, {v.Patch, w.Patch}} {
		if pair[0] != pair[1] {
			return sign(pair[0] - pair[1])
		}
	}
	switch {
	case len(v.Pre) == 0 && len(w.Pre) == 0:
		return 0
	case len(v.Pre) == 0:
		// A release ranks above a pre-release of the same numbers.
		return 1
	case len(w.Pre) == 0:
		return -1
	}
	for i := 0; i < len(v.Pre) && i < len(w.Pre); i++ {
		if c := comparePreIdentifier(v.Pre[i], w.Pre[i]); c != 0 {
			return c
		}
	}
	return sign(len(v.Pre) - len(w.Pre))
}

// comparePreIdentifier is semver §11's rule for one dot-separated
// pre-release identifier: numeric identifiers compare numerically and
// rank below alphanumeric ones.
func comparePreIdentifier(a, b string) int {
	an, aNum := asNumericIdentifier(a)
	bn, bNum := asNumericIdentifier(b)
	switch {
	case aNum && bNum:
		return sign(an - bn)
	case aNum:
		return -1
	case bNum:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func asNumericIdentifier(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}
