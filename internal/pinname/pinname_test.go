package pinname

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"
)

func TestNamesCatchesTheCompoundsAndLeavesOrdinaryWordsAlone(t *testing.T) {
	yes := []string{
		"pin", "PIN", "Pin",
		"userPIN", "userPin", "pinBytes", "cachedPin", "PINCode", "card_pin",
		"pinValue", "theUserPINBuffer", "puk", "pukCode", "unblockPUK",
	}
	for _, name := range yes {
		if !Names(name) {
			t.Errorf("Names(%q) = false, want true — components %v", name, Components(name))
		}
	}

	// Ordinary identifiers, and in particular the memory-pinning vocabulary
	// internal/keysource/pkcs11 needs. A rule that fired on these is a rule
	// people rename around.
	no := []string{
		"pinner", "Pinner", "pins", "spinner", "spin", "pinned", "unpin",
		"pinning", "pinnedBuf", "spinLock", "session", "_", "",
	}
	for _, name := range no {
		if Names(name) {
			t.Errorf("Names(%q) = true, want false — components %v", name, Components(name))
		}
	}
}

func TestComponentsSplitsTheWayGoIdentifiersAreWritten(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"userPIN", []string{"user", "pin"}},
		{"PINCode", []string{"pin", "code"}},
		{"pinBytes", []string{"pin", "bytes"}},
		{"card_pin", []string{"card", "pin"}},
		{"HTTPServer", []string{"http", "server"}},
		{"pinner", []string{"pinner"}},
		{"ID", []string{"id"}},
		{"", nil},
	} {
		got := Components(c.in)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("Components(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestCouldCarryAPINAsksWhatTheDeclarationHolds pins the boundary that keeps
// this rule usable: every PIN-named declaration already in this tree is about
// a PIN without being one, and the rule must leave all of them alone while
// still catching anything that could hold the characters themselves.
func TestCouldCarryAPINAsksWhatTheDeclarationHolds(t *testing.T) {
	carries := []string{
		"string", "[]byte", "[]rune", "[8]byte", "[]string", "*string",
		"*[]byte", "any", "interface{}", "[][]byte",
	}
	for _, src := range carries {
		if !CouldCarryAPIN(parseType(t, src)) {
			t.Errorf("CouldCarryAPIN(%s) = false, want true", src)
		}
	}

	// Every one of these is a real declaration's type from this tree, taken
	// from the scan that produced this rule.
	doesNot := []string{
		"bool", "int", "PINPolicy", "signing.PINPolicy", "Code", "JobState",
		"uint32", "error", "func()", "chan int",
	}
	for _, src := range doesNot {
		if CouldCarryAPIN(parseType(t, src)) {
			t.Errorf("CouldCarryAPIN(%s) = true, want false — this is the shape of "+
				"every PIN-named declaration already in the tree", src)
		}
	}
}

func parseType(t *testing.T, src string) ast.Expr {
	t.Helper()
	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parsing type %q: %v", src, err)
	}
	return expr
}

// TestDeclarationCouldCarryAPINReadsTheInitialiserWhenThereIsNoType covers the
// case that a real declaration found the moment internal/keysource/pkcs11
// first declared the PKCS#11 token flags: a constant that names a PIN, has no
// type expression, and is an integer.
func TestDeclarationCouldCarryAPINReadsTheInitialiserWhenThereIsNoType(t *testing.T) {
	for _, c := range []struct {
		src  string // a single var/const spec's source
		want bool
		why  string
	}{
		{"const ckfUserPINLocked = 0x00040000", false, "a PKCS#11 flag; an integer"},
		{"const iInitPIN = 11", false, "an index into CK_FUNCTION_LIST"},
		{"const userPINCounterFlags = ckfUserPINCountLow | ckfUserPINFinalTry", false, "an expression over constants"},
		{"const defaultPin = \"0000\"", true, "a string literal is a PIN's characters"},
		{"var cachedPIN []byte", true, "a byte slice"},
		{"var cachedPIN = readIt()", true, "a call; not seen through, so conservative"},
		{"var pinPolicy signing.PINPolicy", false, "a named enum from another package"},
		{"var perSignaturePIN bool", false, "a bool"},
	} {
		typ, values := parseSpec(t, c.src)
		if got := DeclarationCouldCarryAPIN(typ, values); got != c.want {
			t.Errorf("DeclarationCouldCarryAPIN for %q = %v, want %v — %s", c.src, got, c.want, c.why)
		}
	}

	// A bare name continuing a const block has neither a type nor a value: it
	// repeats or increments the previous expression, iota included.
	if DeclarationCouldCarryAPIN(nil, nil) {
		t.Error("a bare name in a const block was reported as able to carry a PIN; " +
			"it takes the previous constant expression")
	}
}

func parseSpec(t *testing.T, src string) (ast.Expr, []ast.Expr) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "x.go", "package x\n"+src+"\n", 0)
	if err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	gen := file.Decls[0].(*ast.GenDecl)
	spec := gen.Specs[0].(*ast.ValueSpec)
	return spec.Type, spec.Values
}

// TestTheWordBoundaryPatternMissesRealNames measures the claim this package's
// own doc comment makes, rather than leaving it as an assertion: the rule
// internal/keysource/windowscng, internal/keysource/softtoken and
// internal/signing used for three phases does not fire on the names a field
// holding a PIN would plausibly be given.
//
// It is a test rather than a sentence because a sentence cannot go stale
// noisily. If some future Go regexp implementation changed what \b means
// here, this fails and the doc comment gets corrected.
func TestTheWordBoundaryPatternMissesRealNames(t *testing.T) {
	old := regexp.MustCompile(`(?i)\bpin\b`)

	missed := []string{}
	for _, name := range []string{"userPIN", "pinBytes", "cachedPin", "PINCode", "card_pin"} {
		if !old.MatchString(name) && Names(name) {
			missed = append(missed, name)
		}
	}
	if len(missed) != 5 {
		t.Fatalf("the old pattern missed %v; expected it to miss all five, so either "+
			"this package's doc comment is wrong or Names has regressed", missed)
	}

	// And the two it did catch, so the finding is "it fired on almost nothing"
	// rather than "it never fired".
	for _, name := range []string{"pin", "PIN"} {
		if !old.MatchString(name) {
			t.Errorf("the old pattern did not match %q, which it did catch", name)
		}
	}
}
