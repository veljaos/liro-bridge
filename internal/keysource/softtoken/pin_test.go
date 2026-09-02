//go:build softtoken

package softtoken

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

var pinPattern = regexp.MustCompile(`(?i)\bpin\b`)

// TestNoPINFieldOrParameterExistsAnywhere mirrors the identical guard in
// internal/keysource/windowscng: the soft token has no PIN either (F2
// §3.2 — the PKCS#12 password protects a file on disk, not a device,
// and is deliberately not named "pin" anywhere in this package).
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
			if field, ok := n.(*ast.Field); ok {
				for _, id := range field.Names {
					if pinPattern.MatchString(id.Name) {
						t.Errorf("%s: field/parameter %q must not exist — the agent never handles a PIN (SPEC §6.5)", name, id.Name)
					}
				}
			}
			return true
		})
	}
}
