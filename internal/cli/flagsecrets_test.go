package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// No flag on any command takes a secret, and this reads the source to
// say so.
//
// On Windows a process's full command line is readable by every other
// process on the machine: Task Manager's "Command line" column,
// Get-CimInstance Win32_Process, any process lister. A password passed
// as an argument is therefore a password published to the machine for
// as long as the process runs, and it is in the shell history and the
// scheduled-task definition afterwards. D-091 already moved the TSA
// credentials into configuration; the flags stayed beside them until
// this phase removed them.
//
// Reading the syntax tree rather than reflecting over a parsed FlagSet
// is this project's own method for a property about what is *declared*
// (D-025's "no PIN field anywhere", D-158's AllCodes, D-185's "nothing
// binds anywhere but loopback"): a flag registered in a file no test
// happens to exercise is invisible to anything but the source.
//
// Every .go file in both directories is read, whatever build constraint
// it carries and whatever this test itself was compiled under, so the
// property holds for every build of every command rather than only for
// the one being tested. That matters here: the signing path with no
// consent window is behind the "softtoken" tag, and its flags are still
// flags.

// commandSourceDirs are every directory in this repository that
// registers a command-line flag.
var commandSourceDirs = []string{".", filepath.Join("..", "..", "cmd", "liro-bridge")}

// secretWords are the words that make a flag name a place a secret
// would go. Each is here because its value would be a credential, never
// because it merely sounds sensitive:
//
//   - pin: SPEC §6.5 is the standing answer. The card caches the PIN in
//     its own state independently of which process is talking to it, so
//     the PIN is not an access-control boundary between applications —
//     a PIN on a command line buys no security and spends a great deal.
//     Nobody has asked for one; somebody will.
//   - password, passwd, passphrase, secret, credential, apikey/api-key:
//     the value is the credential itself.
//
// Deliberately not here: "user". A username is not a secret, and a list
// that caught it would catch --stamp-show-document-id and every future
// flag with "use" in it. --tsa-user was removed with the other two for
// a different reason — it is half of a credential pair whose other half
// now comes from configuration, so a flag for it alone would configure
// nothing — and removedFlags below is what keeps it removed.
var secretWords = []string{"pin", "password", "passwd", "passphrase", "secret", "credential", "apikey", "api-key"}

// removedFlags are the three flags this phase deleted, by name. The
// secret-word rule above would catch two of them and not --tsa-user, so
// this pins all three directly.
var removedFlags = []string{"tsa-user", "tsa-password", "tsa-client-cert-password"}

func TestNoFlagOnAnyCommandAcceptsASecret(t *testing.T) {
	names := declaredFlagNames(t)
	if len(names) == 0 {
		t.Fatal("no flags were found at all, so this test cannot fail for the right reason")
	}

	for name, where := range names {
		lower := strings.ToLower(name)
		for _, word := range secretWords {
			if strings.Contains(lower, word) {
				t.Errorf("%s registers --%s, whose value would be a secret on a command line every process on the machine can read", where, name)
			}
		}
	}

	for _, gone := range removedFlags {
		if where, ok := names[gone]; ok {
			t.Errorf("%s registers --%s again; TSA credentials come from configuration (D-091)", where, gone)
		}
	}
}

// TestTheFlagSecretRuleWouldActuallyFire proves the rule above is
// capable of failing: a check that scans for words nothing could match
// passes forever.
func TestTheFlagSecretRuleWouldActuallyFire(t *testing.T) {
	for _, name := range []string{"pin", "card-pin", "tsa-password", "tsa-client-cert-password", "api-key"} {
		matched := false
		for _, word := range secretWords {
			if strings.Contains(strings.ToLower(name), word) {
				matched = true
			}
		}
		if !matched {
			t.Errorf("--%s would not be caught by the secret-word rule", name)
		}
	}
	for _, name := range []string{"tsa", "tsa-client-cert", "thumbprint", "stamp-show-document-id", "resign", "all"} {
		for _, word := range secretWords {
			if strings.Contains(strings.ToLower(name), word) {
				t.Errorf("--%s would be wrongly caught by the secret-word %q", name, word)
			}
		}
	}
}

// declaredFlagNames returns every flag name registered anywhere in
// commandSourceDirs, mapped to the file and line that registers it.
func declaredFlagNames(t *testing.T) map[string]string {
	t.Helper()
	names := map[string]string{}
	fset := token.NewFileSet()
	for _, dir := range commandSourceDirs {
		// Every .go file in the directory, whatever build constraint it
		// carries and whatever this test itself was compiled under.
		// Parsing the files rather than the package is deliberate: a
		// flag registered in a file this build excludes is still a flag
		// some build of some command accepts.
		paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("listing %s: %v", dir, err)
		}
		if len(paths) == 0 {
			t.Fatalf("no Go files found in %s", dir)
		}
		for _, path := range paths {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parsing %s: %v", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !flagRegistrar(sel.Sel.Name) {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				name, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				names[name] = fset.Position(lit.Pos()).String()
				return true
			})
		}
	}
	return names
}

// flagRegistrar names the methods on a *flag.FlagSet (and the
// package-level functions of the same names) whose first argument is a
// flag name. Var and Func are included even though this project uses
// neither today: a rule that only covers the shapes already present is
// a rule the next shape walks straight past.
func flagRegistrar(method string) bool {
	switch method {
	case "String", "StringVar", "Bool", "BoolVar", "Int", "IntVar", "Int64", "Int64Var",
		"Uint", "UintVar", "Uint64", "Uint64Var", "Float64", "Float64Var",
		"Duration", "DurationVar", "TextVar", "Var", "Func", "BoolFunc":
		return true
	}
	return false
}
