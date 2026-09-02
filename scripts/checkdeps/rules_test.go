package main

import (
	"strings"
	"testing"
)

func TestPadesImportingAPIIsForbidden(t *testing.T) {
	graph := map[string][]string{
		"internal/pades/cms": {"internal/api", "internal/errs"},
	}
	violations := checkGraph(graph)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(violations), violations)
	}
	if violations[0].Package != "internal/pades/cms" || violations[0].Forbidden != "internal/api" {
		t.Fatalf("unexpected violation: %+v", violations[0])
	}
}

func TestKeysourceImportingPadesIsForbidden(t *testing.T) {
	graph := map[string][]string{
		"internal/keysource/windowscng": {"internal/pades/cms"},
	}
	violations := checkGraph(graph)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(violations), violations)
	}
}

func TestTrustImportingAnyInternalIsForbidden(t *testing.T) {
	graph := map[string][]string{
		"internal/trust/tsl": {"internal/config", "internal/trust/classify"},
	}
	violations := checkGraph(graph)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(violations), violations)
	}
	if violations[0].Forbidden != "internal/config" {
		t.Fatalf("flagged the wrong import: %+v", violations[0])
	}
}

func TestTrustImportingItsOwnSubpackagesIsAllowed(t *testing.T) {
	graph := map[string][]string{
		"internal/trust/classify": {"internal/trust/tsl"},
	}
	if v := checkGraph(graph); len(v) != 0 {
		t.Fatalf("trust importing its own subpackage was flagged: %+v", v)
	}
}

func TestTrustImportingErrsIsAllowed(t *testing.T) {
	graph := map[string][]string{
		"internal/trust/classify": {"internal/errs"},
	}
	if v := checkGraph(graph); len(v) != 0 {
		t.Fatalf("trust importing internal/errs was flagged: %+v", v)
	}
}

// TestTrustImportingAnyOtherInternalPackageIsForbidden proves the errs
// exception is narrow: every other internal/ package is still forbidden
// from internal/trust, not just internal/config (SPEC §4.2 rule 3).
func TestTrustImportingAnyOtherInternalPackageIsForbidden(t *testing.T) {
	graph := map[string][]string{
		"internal/trust/classify/a": {"internal/config"},
		"internal/trust/classify/b": {"internal/keysource"},
		"internal/trust/classify/c": {"internal/platform"},
		"internal/trust/classify/d": {"internal/pades"},
	}
	violations := checkGraph(graph)
	if len(violations) != 4 {
		t.Fatalf("got %d violations, want 4: %+v", len(violations), violations)
	}
	for _, v := range violations {
		if v.Explanation != rule3 {
			t.Errorf("wrong rule cited for %+v", v)
		}
	}
}

func TestAPIUICLITriangleIsForbiddenInEveryDirection(t *testing.T) {
	graph := map[string][]string{
		"internal/api/server": {"internal/ui"},
		"internal/ui/tray":    {"internal/cli"},
		"internal/cli":        {"internal/api"},
	}
	violations := checkGraph(graph)
	if len(violations) != 3 {
		t.Fatalf("got %d violations, want 3: %+v", len(violations), violations)
	}
}

func TestInternalImportingCmdIsForbidden(t *testing.T) {
	graph := map[string][]string{
		"internal/signing": {"cmd/liro-bridge"},
	}
	violations := checkGraph(graph)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(violations), violations)
	}
	if violations[0].Explanation != rule5 {
		t.Fatalf("wrong rule cited: %+v", violations[0])
	}
}

func TestCleanGraphProducesNoViolations(t *testing.T) {
	graph := map[string][]string{
		"internal/pades/cms": {"internal/errs"},
		"internal/keysource": {"internal/errs"},
		"internal/trust/tsl": {"internal/trust/classify", "internal/errs"},
		"internal/api":       {"internal/signing", "internal/errs"},
		"internal/ui":        {"internal/signing", "internal/errs"},
		"internal/cli":       {"internal/signing", "internal/errs"},
		"cmd/liro-bridge":    {"internal/api", "internal/ui", "internal/cli", "internal/config"},
	}
	if v := checkGraph(graph); len(v) != 0 {
		t.Fatalf("clean graph produced violations: %+v", v)
	}
}

func TestFormatViolationsNamesPackageImportAndSection(t *testing.T) {
	out := formatViolations([]Violation{{
		Package:     "internal/pades/cms",
		Forbidden:   "internal/api",
		Explanation: rule1,
	}})
	for _, want := range []string{"internal/pades/cms", "internal/api", "SPEC §4.2 rule 1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}

func TestRelativeStripsModulePrefix(t *testing.T) {
	rel, ok := relative(modulePath + "/internal/pades")
	if !ok || rel != "internal/pades" {
		t.Fatalf("relative() = %q, %v", rel, ok)
	}
	if _, ok := relative("encoding/json"); ok {
		t.Fatalf("relative() should reject a non-module import path")
	}
}
