package windowscng

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pinPattern matches "pin" as a whole word, case-insensitively, so it
// catches PIN, Pin, pin but not unrelated identifiers.
var pinPattern = regexp.MustCompile(`(?i)\bpin\b`)

// TestNoPINFieldOrParameterExistsAnywhere is the failing test SPEC §6.5
// and F2 §2.3 require: the agent must never collect, transport or store
// a PIN, so no struct field and no function parameter in this package
// may be named after one. This walks the AST rather than grepping text,
// so a doc comment explaining *why* there is no PIN field (as this
// package's own comments do) can say the word "PIN" without tripping
// the check — only real declarations count.
func TestNoPINFieldOrParameterExistsAnywhere(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch decl := n.(type) {
			case *ast.Field: // struct fields and function parameters/results
				for _, id := range decl.Names {
					if pinPattern.MatchString(id.Name) {
						t.Errorf("%s: field/parameter %q must not exist — the agent never handles a PIN (SPEC §6.5)", name, id.Name)
					}
				}
			}
			return true
		})
	}
}
