package errs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestAllCodesListsEveryDeclaredCode closes the one gap that would make
// TestEveryErrorCodeHasAMessageInEveryCatalogue (internal/i18n) vacuous.
//
// That test walks AllCodes() and requires a catalogue entry for each. It
// is exactly as strong as AllCodes() is complete, and nothing checked
// that: a new Code constant declared here and forgotten there compiles,
// passes every test in the repository, and reaches a user as its own
// key — "error.cert_revoked" — which is the failure D-104 added the
// catalogue check to prevent in the first place. Three of the codes in
// this package were added after that check existed (INPUT_UNREADABLE,
// OUTPUT_WRITE_FAILED and the two TSA client-certificate ones), so the
// list has already had to be kept in step by hand four times.
//
// This reads the source rather than reflecting over values, for the same
// reason D-025's PIN-field check does: the property is about what is
// *declared*, and a declaration that is never referenced is invisible to
// anything but the syntax tree.
func TestAllCodesListsEveryDeclaredCode(t *testing.T) {
	declared := declaredCodeNames(t)
	listed := listedCodeNames(t)

	inList := make(map[string]bool, len(listed))
	for _, n := range listed {
		inList[n] = true
	}
	var missing []string
	for _, n := range declared {
		if !inList[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("AllCodes() does not list %d declared code(s): %s\n"+
			"Every code needs a message in all three catalogues; a code missing "+
			"from this list is one the catalogue check silently skips.",
			len(missing), strings.Join(missing, ", "))
	}

	isDeclared := make(map[string]bool, len(declared))
	for _, n := range declared {
		isDeclared[n] = true
	}
	for _, n := range listed {
		if !isDeclared[n] {
			t.Errorf("AllCodes() lists %s, which is not a declared Code constant", n)
		}
	}

	seen := map[string]bool{}
	for _, n := range listed {
		if seen[n] {
			t.Errorf("AllCodes() lists %s twice", n)
		}
		seen[n] = true
	}
	if len(declared) == 0 {
		t.Fatal("found no Code constants at all — this test is not reading errs.go")
	}
	t.Logf("%d Code constants declared, %d listed by AllCodes()", len(declared), len(listed))
}

// TestEveryCodeValueIsScreamingSnakeCase is SPEC §7's own rule, which
// nothing checked either: codes are stable machine-readable identifiers,
// and one that arrives in another shape is one an SDK's generated enum
// cannot carry.
func TestEveryCodeValueIsScreamingSnakeCase(t *testing.T) {
	for _, c := range AllCodes() {
		s := string(c)
		if s == "" {
			t.Error("a code has an empty value")
			continue
		}
		for _, r := range s {
			if (r < 'A' || r > 'Z') && r != '_' {
				t.Errorf("code %q is not SCREAMING_SNAKE_CASE", s)
				break
			}
		}
	}
	seen := map[Code]bool{}
	for _, c := range AllCodes() {
		if seen[c] {
			t.Errorf("two constants share the code value %q", c)
		}
		seen[c] = true
	}
}

// declaredCodeNames returns the identifier of every `X Code = "..."`
// constant in errs.go.
func declaredCodeNames(t *testing.T) []string {
	t.Helper()
	f := parseErrsFile(t)
	var out []string
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			id, ok := vs.Type.(*ast.Ident)
			if !ok || id.Name != "Code" {
				continue
			}
			for _, name := range vs.Names {
				out = append(out, name.Name)
			}
		}
	}
	return out
}

// listedCodeNames returns the identifiers AllCodes()'s slice literal
// names, read from the source rather than from the returned values, so
// a duplicate or a stray non-constant expression is visible.
func listedCodeNames(t *testing.T) []string {
	t.Helper()
	f := parseErrsFile(t)
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "AllCodes" {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, el := range lit.Elts {
				if id, ok := el.(*ast.Ident); ok {
					out = append(out, id.Name)
				}
			}
			return false
		})
		return false
	})
	return out
}

func parseErrsFile(t *testing.T) *ast.File {
	t.Helper()
	src, err := os.ReadFile("errs.go")
	if err != nil {
		t.Fatalf("reading errs.go: %v", err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "errs.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing errs.go: %v", err)
	}
	// A sanity check that the file really is the one with the codes in
	// it, so a rename cannot turn this test into one that passes by
	// finding nothing: INTERNAL is SPEC §7's own catch-all and has been
	// there since F0.
	if !strings.Contains(string(src), strconv.Quote("INTERNAL")) {
		t.Fatal("errs.go does not contain INTERNAL — this test is reading the wrong file")
	}
	return f
}
