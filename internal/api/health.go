package api

import (
	"errors"
	"net/http"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// healthResponse is GET /v2/health (F7 §4.3).
//
// Three fields, and the reason there are only three is stated in F7
// §4.3 itself: it reveals nothing else. No certificates, no pairings,
// no user name, no document counts, no paths — nothing that would let
// an unauthenticated caller learn anything about the person at this
// machine. What it does say is what an SDK needs before it can even
// try: that an agent is here, which protocol it speaks, and whether
// this SDK is too old for it.
type healthResponse struct {
	AgentVersion         string `json:"agentVersion"`
	ProtocolVersion      int    `json:"protocolVersion"`
	MinimumClientVersion string `json:"minimumClientVersion"`
}

// handleHealth answers GET /v2/health. It needs no authentication: an
// SDK has to be able to ask whether the agent is there before it has
// paired with it, and the answer says nothing that is worth
// authenticating.
//
// It does not require a content type, because it has no content. That
// looks like a hole in the "a browser cannot reach this" rule and is
// not: a page can make this exact request, and cannot read a word of
// the answer, because no Access-Control-Allow-Origin header is ever
// sent. What it would learn is that something is listening on a
// loopback port — which it can learn from the connection succeeding.
//
// Requiring one here was tried and is what the real-client run
// refused: .NET's HttpClient puts Content-Type on the *content*, so a
// GET with no body cannot carry one at all, and every request from
// what will be F9's own SDK was answered REQUEST_INVALID. A rule that
// cannot be obeyed by the client this protocol exists for is not a
// rule, it is a defect (F7's own "a protocol that passes its tests and
// refuses a real request from a real program").
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if e := requireGET(r); e != nil {
		fail(w, e)
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{
		AgentVersion:         s.agentVersion,
		ProtocolVersion:      ProtocolVersion,
		MinimumClientVersion: MinimumClientVersion,
	})
}

// requireGET refuses anything but a GET.
func requireGET(r *http.Request) *errs.Error {
	if r.Method != http.MethodGet {
		return errs.WithDetails(errs.CodeRequestInvalid,
			errors.New("wrong method"), map[string]any{"expectedMethod": http.MethodGet})
	}
	return nil
}
