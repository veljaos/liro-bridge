package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// The four headers every authenticated request carries (F7 §3).
const (
	HeaderAppID     = "X-Liro-App-Id"
	HeaderTimestamp = "X-Liro-Timestamp"
	HeaderNonce     = "X-Liro-Nonce"
	HeaderSignature = "X-Liro-Signature"
)

// EmptyBodySHA256 is sha256hex of no bytes at all — the last line of
// the canonical string for any request with an empty body.
//
// It is a named constant because it is the single thing integrators get
// wrong first: a GET with no body still hashes *something*, and the
// something is this. F7 §3 asks for it to be stated outright in the
// documentation, and docs/PROTOCOL.md does; naming it here means the
// documentation and the code cannot drift apart, because
// TestEmptyBodyHashIsTheDocumentedConstant compares this against a
// freshly computed sha256 of nothing.
const EmptyBodySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// CanonicalString builds the exact text that an authenticated request's
// signature covers. A client must build an identical string or the
// signature will not match.
//
//	METHOD \n PATH \n TIMESTAMP \n NONCE \n sha256hex(BODY)
//
// The separator is a single "\n" and there is nothing else: no trailing
// newline, no spaces around the parts, no query string on the path.
//
//   - method is upper-cased here, so a client that sends "post" still
//     signs "POST". Everything else is used exactly as given.
//   - path is the request path without the query string, exactly as it
//     appears in the request line.
//   - timestamp is the *string* from the X-Liro-Timestamp header, not a
//     re-formatting of the number it parses to: "1757260800" and
//     "01757260800" are the same instant and different canonical
//     strings, and the one that was sent is the one that was signed.
//   - nonce is the string from X-Liro-Nonce, likewise verbatim.
//   - body is the raw request body, hashed before any parsing (F7 §3).
//     An empty body hashes to EmptyBodySHA256.
func CanonicalString(method, path, timestamp, nonce string, body []byte) string {
	sum := sha256.Sum256(body)
	return strings.Join([]string{
		strings.ToUpper(method),
		path,
		timestamp,
		nonce,
		hex.EncodeToString(sum[:]),
	}, "\n")
}

// Sign returns the request signature: HMAC-SHA256 over canonical, keyed
// with the device secret, hex-encoded in lowercase. This is the value
// of the X-Liro-Signature header.
func Sign(deviceSecret []byte, canonical string) string {
	mac := hmac.New(sha256.New, deviceSecret)
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignatureMatches reports whether presented is the correct signature
// for canonical under deviceSecret.
//
// The comparison is constant time. An ordinary string comparison leaks
// how much of a forged signature was correct, which turns a 2^256
// search into 64 searches of 16 — the reason hmac.Equal exists.
//
// A presented value that is not valid hexadecimal, or is not 32 bytes
// once decoded, is simply wrong rather than an error worth
// distinguishing: the caller's answer is the same either way, and F7 §3
// forbids telling it which check failed. Uppercase hexadecimal is
// accepted even though this project only ever produces lowercase —
// leniency in what is read costs nothing here and rejecting it would
// be a rejection nobody could diagnose from the response.
func SignatureMatches(deviceSecret []byte, canonical, presented string) bool {
	want := Sign(deviceSecret, canonical)
	got, err := hex.DecodeString(presented)
	if err != nil {
		return false
	}
	wantBytes, err := hex.DecodeString(want)
	if err != nil {
		// Unreachable: want is this package's own hex encoding.
		return false
	}
	return hmac.Equal(wantBytes, got)
}
