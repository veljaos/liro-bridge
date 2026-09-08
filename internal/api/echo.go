package api

// POST /v2/echo: the canonical string this agent built from the request
// it just received.
//
// Every way authentication can fail answers 401 AUTH_FAILED with no
// detail, and that is right — D-175 settled it, and it stands: a code
// that says "your nonce was reused" tells whoever is probing that the
// timestamp and the signature were both fine. The cost is that a first
// integration is a hunt. Writing the first client against this protocol
// produced four failures in a row, each giving the same answer: a wrong
// field name, a missing origin, a body altered between hashing and
// sending, and a Content-Type .NET cannot put on a GET.
//
// This endpoint is the other half of D-176's answer. The agent's own log
// already names which check refused a request; this names the string the
// agent hashed. An integrator prints their own beside it and the
// difference is visible in one line.
//
// It reveals nothing. Every part of what it returns is something the
// caller supplied moments earlier and still has in front of it: the
// method, the path, the timestamp, the nonce and the body. The
// signature is not returned and cannot be derived from what is — that
// needs the device secret, which is the one thing this does not touch.
// And a request only reaches this handler once it has already
// authenticated, so the string it echoes is the string of a request
// that was already accepted; a caller that could not authenticate
// learns nothing here it could not learn from any other endpoint.
//
// It is not a debugging back door into other requests: it answers about
// the one request that carried it, and nothing else.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
)

// maxEchoBody bounds what this endpoint will hash. It is generous
// enough for a realistic body — a /v2/sign request naming five hundred
// digests, which is what an integrator most wants to compare — and
// nowhere near either signing endpoint's own limit, because nothing
// here holds a document.
const maxEchoBody = 64 << 10

// echoResponse is what POST /v2/echo answers with.
//
// Two fields, because two are what a mismatch is diagnosed from. The
// body hash is in the canonical string already; it is repeated on its
// own because the commonest single mistake is a body that changed
// between being hashed and being sent, and comparing one 64-character
// value is easier than finding it inside a five-line string.
type echoResponse struct {
	CanonicalString string `json:"canonicalString"`
	BodySHA256      string `json:"bodySha256"`
}

// handleEcho answers POST /v2/echo.
func (s *Server) handleEcho(w http.ResponseWriter, r *http.Request) {
	if e := requirePOST(r); e != nil {
		fail(w, e)
		return
	}
	raw, e := readBody(r, maxEchoBody)
	if e != nil {
		fail(w, e)
		return
	}
	// Authenticated exactly like every other endpoint, and before
	// anything is echoed: an unauthenticated caller gets AUTH_FAILED
	// and no string at all.
	//
	// The body is never parsed. It does not have to be valid JSON —
	// only declared as JSON, like every other body — because the whole
	// point is to hash the exact bytes the caller sent, whatever they
	// are.
	if _, e := s.auth.Authenticate(r, raw); e != nil {
		fail(w, e)
		return
	}
	sum := sha256.Sum256(raw)
	writeJSON(w, http.StatusOK, echoResponse{
		CanonicalString: CanonicalString(r.Method, requestPath(r),
			r.Header.Get(HeaderTimestamp), r.Header.Get(HeaderNonce), raw),
		BodySHA256: hex.EncodeToString(sum[:]),
	})
}
