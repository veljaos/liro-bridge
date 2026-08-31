// Command checkdeps enforces the dependency rules from SPEC §4.2. It is
// hand-written rather than a third-party import linter, per F0 §6.2: the
// rule set is small and must not silently stop working when a dependency
// updates.
package main

import (
	"fmt"
	"sort"
	"strings"
)

// modulePath is the module declared in go.mod (F0 §1.1). It is fixed by
// the spec, so hardcoding it here keeps this checker dependency-free.
const modulePath = "github.com/veljaos/liro-bridge"

// Violation names one forbidden import: pkg (module-relative, e.g.
// "internal/pades/cms") depends on forbidden (also module-relative).
type Violation struct {
	Package     string
	Forbidden   string
	Explanation string
}

const (
	rule1 = "the PDF engine must be usable without the HTTP server existing (SPEC §4.2 rule 1)"
	rule2 = "key sources sign hashes; they do not know what a PDF is (SPEC §4.2 rule 2)"
	rule3 = "trust evaluation (TSL, OCSP, chain building) is pure and independently testable (SPEC §4.2 rule 3)"
	rule4 = "internal/api, internal/ui and internal/cli are three independent front doors onto the same core (SPEC §4.2 rule 4)"
	rule5 = "nothing in internal/ may import cmd/ (SPEC §4.2 rule 5)"
)

// isUnder reports whether pkg is prefix itself or a subpackage of prefix.
func isUnder(pkg, prefix string) bool {
	return pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
}

// checkPackage evaluates one package's module-relative dependency list
// against every rule and returns every violation found.
func checkPackage(pkg string, deps []string) []Violation {
	var out []Violation

	switch {
	case isUnder(pkg, "internal/pades"):
		for _, d := range deps {
			if isUnder(d, "internal/api") {
				out = append(out, Violation{pkg, d, rule1})
			}
		}
	case isUnder(pkg, "internal/keysource"):
		for _, d := range deps {
			if isUnder(d, "internal/pades") {
				out = append(out, Violation{pkg, d, rule2})
			}
		}
	case isUnder(pkg, "internal/trust"):
		for _, d := range deps {
			if isUnder(d, "internal") && !isUnder(d, "internal/trust") {
				out = append(out, Violation{pkg, d, rule3})
			}
		}
	case isUnder(pkg, "internal/api"):
		for _, d := range deps {
			if isUnder(d, "internal/ui") || isUnder(d, "internal/cli") {
				out = append(out, Violation{pkg, d, rule4})
			}
		}
	case isUnder(pkg, "internal/ui"):
		for _, d := range deps {
			if isUnder(d, "internal/api") || isUnder(d, "internal/cli") {
				out = append(out, Violation{pkg, d, rule4})
			}
		}
	case isUnder(pkg, "internal/cli"):
		for _, d := range deps {
			if isUnder(d, "internal/api") || isUnder(d, "internal/ui") {
				out = append(out, Violation{pkg, d, rule4})
			}
		}
	}

	// Rule 5 applies to every internal/ package, regardless of category.
	if isUnder(pkg, "internal") {
		for _, d := range deps {
			if isUnder(d, "cmd") {
				out = append(out, Violation{pkg, d, rule5})
			}
		}
	}

	return out
}

// checkGraph evaluates every package in the graph (module-relative import
// path -> module-relative dependency import paths) and returns every
// violation, sorted for deterministic output.
func checkGraph(graph map[string][]string) []Violation {
	var out []Violation
	for pkg, deps := range graph {
		out = append(out, checkPackage(pkg, deps)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].Forbidden < out[j].Forbidden
	})
	return out
}

// relative strips the module path prefix from an absolute import path. It
// returns the path unchanged, and false, if the path is not in this module
// (e.g. a standard library or third-party package).
func relative(importPath string) (string, bool) {
	prefix := modulePath + "/"
	if !strings.HasPrefix(importPath, prefix) {
		return "", false
	}
	return strings.TrimPrefix(importPath, prefix), true
}

func formatViolations(violations []Violation) string {
	var b strings.Builder
	for _, v := range violations {
		fmt.Fprintf(&b, "FORBIDDEN IMPORT\n  %s imports %s\n  Rule: %s\n\n", v.Package, v.Forbidden, v.Explanation)
	}
	return b.String()
}
