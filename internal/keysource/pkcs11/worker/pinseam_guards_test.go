package worker

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// The guards in this file are about the PIN's path through this package, and
// each one is a property that a syntax tree or a table can decide. What they do
// not establish is in each one's own comment, because a guard whose limits are
// not written down is a guard people believe more than they should (D-296).

// TestNothingRetriesALoginOrASignature is SPEC §6.5.1 clause 5 as a property of
// the table rather than as a sentence in its doc comment.
//
// "Nothing retries a PIN automatically, ever, for any reason. One wrong PIN is
// one attempt. Three block the card, and for a national identity card
// unblocking means a visit to a police station."
//
// retryable is an allow-list (D-259) and that is most of the protection: an
// operation added to the protocol is not retryable until somebody puts it
// there. This is the other half, and it exists because the addition that would
// be worst is the one that looks most reasonable — a login that failed because
// the *worker* died, rather than because the PIN was wrong, is genuinely a
// login that could be retried without costing an attempt, and telling those two
// apart from the parent's side is exactly what cannot be done: the child died,
// so there is nobody left to ask whether C_Login ran.
//
// OpSignDigest and OpCloseSession are here for a different and simpler reason.
// They need a session, a respawned worker has none, and a retry would send them
// to a process that can only refuse — which turns one honest failure into three
// and a confusing message.
//
// # What it does not establish
//
// That nothing else retries. A caller above this package is free to call Open
// twice, and no test in this package can see that. What it establishes is that
// the supervisor does not do it by itself, silently, inside a function whose
// name does not mention PINs.
func TestNothingRetriesALoginOrASignature(t *testing.T) {
	// The positive control first: an allow-list that is empty, or that lost the
	// reads it is for, would pass the absence check below for the wrong reason.
	for _, want := range []Op{OpEnumerate, OpList, OpChainFor} {
		if !retryable[want] {
			t.Fatalf("%q is not retryable, so this table is not the one this test "+
				"is about and its silence about logins means nothing", want)
		}
	}

	mustNot := map[Op]string{
		OpLogin: "a login costs one of three PIN attempts, and a parent cannot " +
			"tell a worker that died before C_Login from one that died after it",
		OpLoginPIN: "a PIN is written once, into a child that is already blocked " +
			"waiting for it; re-sending one is sending it to a process that " +
			"never asked",
		OpSignDigest:   "a respawned worker has no session, so a retry can only be refused",
		OpCloseSession: "a respawned worker has no session to close",
	}
	for op, why := range mustNot {
		if retryable[op] {
			t.Errorf("%q is in retryable, and must never be: %s.\n\n"+
				"SPEC §6.5.1 clause 5: nothing retries a PIN automatically, ever, "+
				"for any reason.", op, why)
		}
	}
}

// TestThePINIsTheOnlyThingWrittenOutsideAFrame is what makes the framing
// argument checkable.
//
// The whole reason this protocol is length-prefixed rather than
// newline-delimited is that a PIN can then be written raw, immediately behind a
// frame that names its length, and read by an exact-length read that buffers
// nothing (SPEC §6.5.1 clause 2, D-297). That argument holds only while the PIN
// is the *only* thing ever written outside a frame. A second raw write anywhere
// on this pipe — a length, a marker, a newline somebody added to make a log
// readable — would put bytes between a frame and the PIN that follows it, and
// the far end would read them as a PIN.
//
// So: every call to a writer's Write method in this package's shipped code must
// be in one of two functions. WriteFrame, which is the framing itself, and
// sendPIN, which is the one place a PIN is written. An allow-list of two
// (D-259), not a deny-list, and deliberately by function name — because the
// point is that a reader can hold both of them in their head at once.
//
// # What it does not establish
//
// That sendPIN writes the PIN once rather than twice, or that what it writes is
// the PIN at all. A syntax tree cannot see either. What it establishes is that
// nobody has quietly added a third writer somewhere else in the package, which
// is the way this property would actually be lost.
func TestThePINIsTheOnlyThingWrittenOutsideAFrame(t *testing.T) {
	permitted := map[string]string{
		"WriteFrame": "the framing itself",
		"sendPIN":    "the one place a PIN is written, in one Write, behind the frame that names its length",
	}

	fset := token.NewFileSet()
	found := map[string]bool{}
	for _, file := range packageFiles(t) {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "Write" {
					return true
				}
				found[fn.Name.Name] = true
				if _, allowed := permitted[fn.Name.Name]; !allowed {
					t.Errorf("%s: %s calls Write on a writer, and only %v may.\n\n"+
						"This protocol is length-prefixed so that a PIN can be written raw "+
						"behind a frame that names its length and read by an exact-length "+
						"read (SPEC §6.5.1 clause 2). A second raw write puts bytes between "+
						"the frame and the PIN, and the far end reads them as one.",
						fset.Position(call.Pos()), fn.Name.Name, keysOf(permitted))
				}
				return true
			})
		}
	}

	// Both halves, because a walker that found nothing would report no
	// violations just as loudly as a package that has none (D-031).
	for name, what := range permitted {
		if !found[name] {
			t.Errorf("%s does not call Write, so this walker is not seeing what it "+
				"thinks it is; it is meant to be %s", name, what)
		}
	}
}

// TestTheExchangeIdentifierIsNotDerivedFromAnything is the owner's first
// condition, at the one place it can be decided statically.
//
// "The parent must only honour a PIN request that corresponds to an operation a
// person has already approved. A worker that can ask at will is a worker that
// can make PIN dialogs appear... Bind the request to the approved operation, not
// to the worker being alive."
//
// What makes that binding hold is that the identifier is unguessable and
// minted per operation. A counter would bind the question to the login in
// flight, which is most of the job — and a child that has seen one value can
// write the next, so the binding would be to the conversation rather than to
// the approval.
//
// # What it does not establish
//
// That Open calls newExchangeID, or that the value reaches the Request. Those
// are behaviour and are measured in parent_test.go. This establishes that the
// mint is crypto/rand and stays crypto/rand, which is the part a later
// simplification would take away without meaning anything by it.
func TestTheExchangeIdentifierIsNotDerivedFromAnything(t *testing.T) {
	fset := token.NewFileSet()
	var declaredIn *ast.File
	callsRandRead := false

	for _, file := range packageFiles(t) {
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "newExchangeID" || fn.Body == nil {
				continue
			}
			declaredIn = parsed
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if ok && pkg.Name == "rand" && sel.Sel.Name == "Read" {
					callsRandRead = true
				}
				return true
			})
		}
	}

	if declaredIn == nil {
		t.Fatal("newExchangeID was not found, so this check cannot see what it is looking for")
	}
	if !callsRandRead {
		t.Error("newExchangeID does not call rand.Read.\n\n" +
			"The exchange identifier binds a PIN question to the operation a person " +
			"approved. Deriving it from a counter, a clock or a process id binds it " +
			"to the conversation instead, and a child that has seen one value can " +
			"write the next.")
	}

	// Which rand it is, since the call above cannot tell them apart by name.
	crypto := false
	for _, imp := range declaredIn.Imports {
		switch strings.Trim(imp.Path.Value, `"`) {
		case "crypto/rand":
			crypto = true
		case "math/rand", "math/rand/v2":
			t.Error("the file declaring newExchangeID imports math/rand, which is " +
				"predictable by design; the identifier must not be guessable")
		}
	}
	if !crypto {
		t.Error("the file declaring newExchangeID does not import crypto/rand")
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
