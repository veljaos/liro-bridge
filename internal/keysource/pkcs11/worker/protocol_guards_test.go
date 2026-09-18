package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The guards in this file are written before the entry points they constrain,
// which is D-290's ordering and the reason it was chosen: a test written after
// a backend is a test written around whatever that backend already does.
//
// Each one is a property the protocol has to keep having, not a description of
// what it happens to look like today.

// TestTheProtocolCarriesNoModulePath is F12 §10's rule, enforced where it can
// actually be broken.
//
// "A configured path remains the escape hatch, and a protocol-supplied path
// remains refused." The worker is told which module to load by its command
// line, once, by the parent that spawned it. If a module path could arrive in a
// request instead, then anything that can reach the pipe can choose which
// foreign DLL this process loads — and the pipe is reachable by the agent,
// which is reachable by F7's local HTTP protocol, which is reachable by a web
// page. That is the whole chain the rule exists to cut, and it is cut here
// rather than checked for at the far end.
//
// This is deliberately a check on the *shape of the types* rather than on any
// handler. A handler that ignores a field is one edit away from using it.
//
// # It reads exported fields, and that is the rule rather than a concession
//
// It read every field until the parent's own supervisor arrived in this
// package, and then fired on Worker.modulePath — which is exactly the path F12
// §10 says the worker *is* told, on the command line, by the parent that
// spawned it. Renaming a correctly named field to satisfy a matcher is the
// move D-270 already rejected, and a list of exempt type names is the
// hand-kept list D-158 is about.
//
// The property is the right one and was too wide by accident: the hazard is a
// path that **crosses the pipe**, and encoding/json marshals exported fields
// and nothing else. So a field that cannot cross the wire cannot carry a path
// across it, and checking exported fields is both necessary and sufficient.
// The parent's unexported field is not in the protocol's reach; a field added
// to Request is, whatever it is called.
func TestTheProtocolCarriesNoModulePath(t *testing.T) {
	// Exact names and explicit compounds rather than substring matching: a
	// field called Profile ends in "file" and a field called Disposition
	// contains "so", and a guard that cries wolf gets deleted by the third
	// person who hits it.
	pathish := map[string]bool{
		"path": true, "module": true, "modulepath": true, "library": true,
		"librarypath": true, "dll": true, "dllpath": true, "file": true,
		"filepath": true, "filename": true, "location": true, "lib": true,
		"libpath": true, "sofile": true, "image": true, "imagepath": true,
	}

	forEachStructField(t, func(structName, fieldName, typeName string, pos token.Position) {
		lower := strings.ToLower(fieldName)
		{
			if !ast.IsExported(fieldName) {
				return // cannot cross the wire; see the doc comment
			}
			if pathish[lower] {
				t.Errorf("%s.%s (%s) at %s:%d looks like it carries a module path.\n\n"+
					"F12 §10: a protocol-supplied module path is refused. The worker "+
					"learns which module to load from its command line, from the parent "+
					"that spawned it, once. A path arriving in a request would let "+
					"anything that reaches this pipe choose which foreign DLL this "+
					"process loads.\n\n"+
					"If this field is not that, rename it so the next reader does not "+
					"have to work it out.",
					structName, fieldName, typeName, filepath.Base(pos.Filename), pos.Line)
			}
		}
	})
}

// TestTheWorkerNeverBuffersItsInput is SPEC §6.5.1 clause 2, enforced as an
// import rule because that is where it is decidable.
//
// The clause permits the PIN to cross exactly one process boundary — a pipe the
// agent creates and this process inherits — and bounds the exposure: "one
// write, read immediately, never buffered".
//
// A buffered reader defeats that, silently and at a distance. bufio reads ahead
// by design: asked for a request frame, it may pull the PIN that follows into
// its own buffer, where it sits in this process's heap for as long as the
// reader lives, is never overwritten, and is invisible to every guard in
// pin_test.go because it is not a field, a parameter or a named result — it is
// somebody else's byte slice.
//
// That is why the framing is length-prefixed and read with io.ReadFull: exactly
// four bytes, then exactly that many. Never one byte more. The PIN written
// immediately after a request can then be read by an exact-length read and has
// never been buffered by anything.
//
// The rule is on the whole package because the hazard is not in one function.
func TestTheWorkerNeverBuffersItsInput(t *testing.T) {
	forbidden := map[string]string{
		"bufio": "reads ahead by design, and what it reads ahead into may be the PIN",
		"encoding/gob": "decodes from a stream it is free to buffer, and constructs " +
			"arbitrary values while doing it",
	}

	for _, file := range packageFiles(t) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, imp := range parsed.Imports {
			name := strings.Trim(imp.Path.Value, `"`)
			if why, bad := forbidden[name]; bad {
				t.Errorf("%s imports %q, which %s.\n\n"+
					"SPEC §6.5.1 clause 2 bounds the PIN's exposure to one write, read "+
					"immediately, never buffered. The framing is length-prefixed and read "+
					"with io.ReadFull for exactly this reason.",
					filepath.Base(file), name, why)
			}
		}
	}
}

// TestEveryOperationIsNamedInOneClosedSet makes adding an operation a
// deliberate act with somewhere to read why.
//
// The worker's whole vocabulary is the reason it is safe to have in a release
// binary: it signs nothing, holds no consent, and cannot be driven into
// signing. That argument is only as good as the list of things it will do. A
// new operation added without touching this test is a widening of the contract
// that nobody reviewed.
//
// It is not a check that the set is correct — that is a human's job — but it
// fails when the set changes, which is when a human should look.
func TestEveryOperationIsNamedInOneClosedSet(t *testing.T) {
	// The operations this worker is permitted to have. Changing this list is a
	// change to what a release binary can be asked to do; F12 §2's scrutiny
	// applies to every addition.
	permitted := map[string]bool{
		"OpEnumerate": true, // read certificate objects off the token
		"OpList":      true, // the same, shaped for the agent's listing
		"OpChainFor":  true, // the issuer chain for one thumbprint
		"OpShutdown":  true, // C_Finalize and exit

		// The four the PIN seam added. Together they are the whole of what makes
		// this subcommand able to sign, so each is named with what it costs.
		//
		// "OpLogin":       C_Login on the token holding one thumbprint. Costs a
		//                  PIN attempt, of which a card has three. Carries an
		//                  exchange identifier the parent minted for one
		//                  approved operation, and is refused without one.
		// "OpLoginPIN":    the length of a PIN that follows as raw bytes. It is
		//                  only ever sent in answer to a question, and offered
		//                  outside one it ends the worker rather than being
		//                  refused — see the OpLoginPIN branch in Serve.
		// "OpSignDigest":  C_Sign with the key the login found. This is the one
		//                  that produces a signature, and it needs a session,
		//                  which needs a login, which needs an exchange.
		// "OpCloseSession": C_Logout and close, leaving the module loaded. It
		//                  costs nothing and its absence costs a card left
		//                  authenticated (SPEC §6.5).
		"OpLogin":        true,
		"OpLoginPIN":     true,
		"OpSignDigest":   true,
		"OpCloseSession": true,
	}

	found := map[string]bool{}
	for _, file := range packageFiles(t) {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for _, name := range spec.Names {
				if strings.HasPrefix(name.Name, "Op") && name.Name != "Op" {
					found[name.Name] = true
				}
			}
			return true
		})
	}

	if len(found) == 0 {
		t.Skip("no operations are declared yet; this guard is written before the " +
			"entry points it constrains (D-290) and starts working when they arrive")
	}
	for name := range found {
		if !permitted[name] {
			t.Errorf("the worker declares operation %s, which is not in this test's "+
				"permitted set.\n\n"+
				"Every operation widens what a release binary can be asked to do. F12 §2: "+
				"\"The subcommand must not become a way in.\" Add it here, with a comment "+
				"saying what it does, so the addition is visible to whoever reviews this "+
				"next.", name)
		}
	}
	for name := range permitted {
		if !found[name] {
			t.Logf("permitted operation %s is not declared yet", name)
		}
	}
}

// packageFiles lists this package's non-test Go files. The guards read the
// shipped code, not themselves.
func packageFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, name)
	}
	return out
}

// forEachStructField visits every field of every struct declared in this
// package's non-test files.
func forEachStructField(t *testing.T, visit func(structName, fieldName, typeName string, pos token.Position)) {
	t.Helper()
	fset := token.NewFileSet()
	for _, file := range packageFiles(t) {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				typeName := exprString(field.Type)
				for _, name := range field.Names {
					visit(ts.Name.Name, name.Name, typeName, fset.Position(name.Pos()))
				}
				if len(field.Names) == 0 { // embedded
					visit(ts.Name.Name, typeName, typeName, fset.Position(field.Pos()))
				}
			}
			return true
		})
	}
}

// exprString renders a type expression well enough for a message. It does not
// need to be complete; it needs to name the thing a reader is looking for.
func exprString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprString(t.X)
	case *ast.SelectorExpr:
		return exprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprString(t.Elt)
	case *ast.MapType:
		return "map[" + exprString(t.Key) + "]" + exprString(t.Value)
	default:
		return "?"
	}
}
