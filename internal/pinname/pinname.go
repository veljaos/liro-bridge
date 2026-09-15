// Package pinname answers one question: does a Go identifier name a PIN?
//
// It exists so that the four packages enforcing SPEC §6.5 and §6.5.1 share one
// answer rather than four. Three of them had a copy each before this package
// existed, and the copies agreed — which is the only reason it was not already
// a defect, and exactly the shape this project has had to remove for a
// classification rule (D-108), for a question asked twice (D-124) and for a
// margin (D-138).
//
// # Why a word boundary is the wrong instrument
//
// The rule those three packages used was the regular expression
// `(?i)\bpin\b`. A word boundary sits between a word character and a non-word
// character, and a Go identifier is one word, so the only boundaries available
// are its two ends. The pattern therefore matches an identifier that is
// exactly "pin", "PIN" or "Pin", and misses every compound:
//
//	userPIN      not matched
//	pinBytes     not matched
//	cachedPin    not matched
//	PINCode      not matched
//	card_pin     not matched
//
// Those are the names a field holding a PIN would actually be given. The ones
// it did catch are the names nobody writes, because a bare "pin" as a struct
// field is unusual in Go. So for three phases the guard on the most important
// paragraph in the specification fired on almost nothing. Measured, not
// inferred: see TestTheWordBoundaryPatternMissesRealNames.
//
// Names splits an identifier into its camelCase and underscore components
// instead, and matches a component whole. That catches every compound above
// and still leaves ordinary words alone.
package pinname

import (
	"go/ast"
	"strings"
	"unicode"
)

// words are the identifier components that mean a PIN or a PUK.
//
// "pins" is deliberately absent. internal/keysource/pkcs11 pins Go memory that
// crosses into a foreign module — runtime.Pinner, for the reason D-101 records
// — so "pins" and "pinner" are near-certain to appear there meaning something
// else. A rule that fired on memory-pinning code is a rule people rename
// around, and SPEC §6.5.1 forbids holding even one PIN, so a plural is not the
// shape the defect would take.
var words = map[string]bool{"pin": true, "puk": true}

// Names reports whether identifier names a PIN or a PUK.
//
// The blank identifier never does: an ignored parameter holds nothing.
func Names(identifier string) bool {
	if identifier == "_" {
		return false
	}
	for _, w := range Components(identifier) {
		if words[w] {
			return true
		}
	}
	return false
}

// CouldCarryAPIN reports whether a declared type could hold a PIN's
// characters. A nil expression — a var whose type is inferred — is treated as
// though it could.
//
// # Why the name alone is not the question
//
// Measured over this whole tree when the stronger matcher first ran: every
// PIN-named declaration in it is about a PIN without being one. The policy a
// card enforces (signing.PINPolicy, SPEC §12.9's own vocabulary), the error
// codes (errs.CodePINIncorrect), a job state (jobs.JobAwaitingPIN), whether a
// batch asks per signature (a bool), the token flags a PKCS#11 module reports
// — and, in internal/ui, "pin" used as a verb for pinning Go memory
// (pinPtr, pinUTF16, handlerPinCount).
//
// Not one of them holds a PIN, and renaming them to satisfy a name-matching
// rule would make correct code worse and harder to trace back to the
// specification that named it. SPEC §6.5 forbids the agent handling a PIN,
// which is about the material rather than about the vocabulary — so the check
// asks what a declaration can carry, and lets a policy enum keep its name.
//
// # What this deliberately does not catch
//
// A PIN behind a named type — a field declared as some secretBytes rather than
// as []byte — is not reported, because a named type could be anything and
// guessing would put the false positives straight back. That case is caught by
// reading the code, and it is a far less likely accident than the ones above,
// which were already in the tree.
func CouldCarryAPIN(expr ast.Expr) bool {
	switch t := expr.(type) {
	case nil:
		return true // an inferred type; be conservative
	case *ast.Ident:
		return t.Name == "string" || t.Name == "any"
	case *ast.InterfaceType:
		return true // holds anything, including a PIN
	case *ast.StarExpr:
		return CouldCarryAPIN(t.X)
	case *ast.ParenExpr:
		return CouldCarryAPIN(t.X)
	case *ast.ArrayType:
		// []byte, []rune, [8]byte, []string, and slices of those.
		if id, ok := t.Elt.(*ast.Ident); ok {
			return id.Name == "byte" || id.Name == "rune" || id.Name == "string"
		}
		return CouldCarryAPIN(t.Elt)
	default:
		// A selector (signing.PINPolicy), a map, a channel, a function, a
		// named type. None of these is how a PIN would plausibly be held by
		// accident.
		return false
	}
}

// Components splits a Go identifier into its lower-cased camelCase and
// underscore-separated parts: "userPIN" becomes {"user", "pin"} and "PINCode"
// becomes {"pin", "code"}.
//
// It is exported so that a test can show what it did, which is what makes a
// failure readable rather than merely correct.
func Components(identifier string) []string {
	var out []string
	r := []rune(identifier)
	start := 0
	flush := func(end int) {
		if end > start {
			out = append(out, strings.ToLower(string(r[start:end])))
		}
	}
	for i := 1; i < len(r); i++ {
		switch {
		case r[i] == '_':
			flush(i)
			start = i + 1
		case unicode.IsUpper(r[i]) && !unicode.IsUpper(r[i-1]):
			// lower or digit, then upper: "userPIN" -> "user" | "PIN"
			flush(i)
			start = i
		case unicode.IsUpper(r[i-1]) && unicode.IsUpper(r[i]) &&
			i+1 < len(r) && unicode.IsLower(r[i+1]):
			// a run of upper, then Upper+lower: "PINCode" -> "PIN" | "Code"
			flush(i)
			start = i
		}
	}
	flush(len(r))
	return out
}
