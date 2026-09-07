package api

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// The worked example docs/PROTOCOL.md hands an integrator, pinned here
// so the two cannot drift. Every value is fixed: the same inputs must
// produce the same signature on any machine, in any year, or an
// integrator checking their own implementation against the document
// has nothing to check against.
const (
	exampleSecretBase64 = "bGlyby1icmlkZ2UtZXhhbXBsZS1zZWNyZXQtMzJieXQ="
	exampleMethod       = "POST"
	examplePath         = "/v2/sign"
	exampleTimestamp    = "1757260800"
	exampleNonce        = "9f2c1e0b7a3d4c58"
	exampleBody         = `{"certificateThumbprint":"7758D4","digestAlgorithm":"SHA256","digests":["47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU="]}`

	exampleBodyHash  = "7f249dfc3164e80275772db6207b973f95c984b57356a6489cea975c5a706acc"
	exampleSignature = "e976ad93b0f66044ecaf6d831b3f9794efe3276186a51f1260fef875b469f238"
)

func TestCanonicalStringShape(t *testing.T) {
	got := CanonicalString("post", "/v2/sign", "1757260800", "abc", []byte("hello"))

	sum := sha256.Sum256([]byte("hello"))
	want := "POST\n/v2/sign\n1757260800\nabc\n" + hex.EncodeToString(sum[:])
	if got != want {
		t.Fatalf("CanonicalString =\n%q\nwant\n%q", got, want)
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatal("the canonical string has a trailing newline; it must not")
	}
	if n := strings.Count(got, "\n"); n != 4 {
		t.Fatalf("the canonical string has %d newlines, want exactly 4", n)
	}
}

// The one constant every integrator gets wrong first (F7 §3): a request
// with no body still hashes something, and this is the something.
func TestEmptyBodyHashIsTheDocumentedConstant(t *testing.T) {
	sum := sha256.Sum256(nil)
	if got := hex.EncodeToString(sum[:]); got != EmptyBodySHA256 {
		t.Fatalf("EmptyBodySHA256 is %q, but sha256 of no bytes is %q", EmptyBodySHA256, got)
	}
	got := CanonicalString("GET", "/v2/health", "1", "n", nil)
	if !strings.HasSuffix(got, "\n"+EmptyBodySHA256) {
		t.Fatalf("an empty body did not hash to EmptyBodySHA256:\n%q", got)
	}
	// nil and an empty slice must be the same thing: a client sending
	// no body and a client sending zero bytes are the same request.
	if CanonicalString("GET", "/v2/health", "1", "n", []byte{}) != got {
		t.Fatal("nil and an empty body produced different canonical strings")
	}
}

// The timestamp and nonce go in exactly as the headers carried them.
// Re-formatting the timestamp as a number would sign something the
// client did not send.
func TestCanonicalStringUsesTheHeaderValuesVerbatim(t *testing.T) {
	a := CanonicalString("GET", "/p", "1757260800", "n", nil)
	b := CanonicalString("GET", "/p", "01757260800", "n", nil)
	if a == b {
		t.Fatal("two different timestamp strings for the same instant produced " +
			"the same canonical string; the header's own text is what is signed")
	}
}

func TestCanonicalStringUpperCasesTheMethodAndNothingElse(t *testing.T) {
	if CanonicalString("post", "/V2/Sign", "1", "n", nil) !=
		CanonicalString("POST", "/V2/Sign", "1", "n", nil) {
		t.Fatal("the method is not upper-cased")
	}
	if CanonicalString("POST", "/v2/sign", "1", "n", nil) ==
		CanonicalString("POST", "/V2/SIGN", "1", "n", nil) {
		t.Fatal("the path was upper-cased; only the method is")
	}
}

// The worked example from the documentation, computed here rather than
// asserted from a literal, so that this test states the recipe and the
// two constants above are what an integrator compares against.
func TestWorkedExampleMatchesTheDocumentedValues(t *testing.T) {
	secret, err := DecodeDeviceSecret(exampleSecretBase64)
	if err != nil {
		t.Fatalf("the example device secret is not valid base64: %v", err)
	}
	if len(secret) != DeviceSecretLength {
		t.Fatalf("the example device secret is %d bytes, want %d", len(secret), DeviceSecretLength)
	}

	sum := sha256.Sum256([]byte(exampleBody))
	bodyHash := hex.EncodeToString(sum[:])
	if bodyHash != exampleBodyHash {
		t.Fatalf("the documented body hash is %s, computed %s", exampleBodyHash, bodyHash)
	}

	canonical := CanonicalString(exampleMethod, examplePath, exampleTimestamp, exampleNonce, []byte(exampleBody))
	wantCanonical := strings.Join([]string{exampleMethod, examplePath, exampleTimestamp, exampleNonce, bodyHash}, "\n")
	if canonical != wantCanonical {
		t.Fatalf("canonical string:\n%q\nwant\n%q", canonical, wantCanonical)
	}

	sig := Sign(secret, canonical)
	if sig != exampleSignature {
		t.Fatalf("the documented signature is\n  %s\nbut Sign produces\n  %s\n"+
			"(canonical string was %q)", exampleSignature, sig, canonical)
	}
	if sig != strings.ToLower(sig) {
		t.Fatal("the signature is not lowercase hex")
	}
	if !SignatureMatches(secret, canonical, sig) {
		t.Fatal("SignatureMatches rejected Sign's own output")
	}
}

func TestSignatureMatchesRejectsWhatItShould(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	canonical := CanonicalString("POST", "/v2/sign", "1757260800", "n", []byte("{}"))
	good := Sign(secret, canonical)

	cases := []struct {
		name      string
		secret    []byte
		canonical string
		presented string
	}{
		{"a different secret", []byte("fedcba9876543210fedcba9876543210"), canonical, good},
		{"a different canonical string", secret, canonical + "x", good},
		{"one flipped hex digit", secret, canonical, flipLastHexDigit(good)},
		{"not hexadecimal at all", secret, canonical, "not-a-signature"},
		{"empty", secret, canonical, ""},
		{"truncated", secret, canonical, good[:len(good)-2]},
		{"too long", secret, canonical, good + "00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if SignatureMatches(tc.secret, tc.canonical, tc.presented) {
				t.Fatal("SignatureMatches accepted it")
			}
		})
	}

	// Uppercase hexadecimal is accepted: this project only ever
	// produces lowercase, and rejecting the other spelling would be a
	// rejection nobody could diagnose from the response.
	if !SignatureMatches(secret, canonical, strings.ToUpper(good)) {
		t.Fatal("SignatureMatches rejected the same signature in uppercase hex")
	}
}

func flipLastHexDigit(s string) string {
	if s == "" {
		return s
	}
	last := s[len(s)-1]
	if last == '0' {
		return s[:len(s)-1] + "1"
	}
	return s[:len(s)-1] + "0"
}
