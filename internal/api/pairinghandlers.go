package api

import (
	"errors"
	"net/http"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// pairRequestBody is POST /v2/pair/request.
type pairRequestBody struct {
	ApplicationName string `json:"applicationName"`
	Origin          string `json:"origin"`
}

// pairRequestResponse is what that call returns.
//
// There is no code in it, and there never will be. The whole mechanism
// rests on the six digits travelling through a person who read them off
// the agent's own window; a code in this response would let an
// application pair itself with nobody watching (F7 §2.1).
type pairRequestResponse struct {
	RequestID        string `json:"requestId"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
}

// pairConfirmBody is POST /v2/pair/confirm.
type pairConfirmBody struct {
	RequestID string `json:"requestId"`
	Code      string `json:"code"`
	Origin    string `json:"origin"`
}

// pairConfirmResponse carries the device secret. It is the only
// response in this protocol that ever does, and it is sent once.
type pairConfirmResponse struct {
	AppID           string `json:"appId"`
	DeviceSecret    string `json:"deviceSecret"`
	ApplicationName string `json:"applicationName"`
	Origin          string `json:"origin"`
}

// handlePairRequest begins a pairing (F7 §2.1).
func (s *Server) handlePairRequest(w http.ResponseWriter, r *http.Request) {
	if e := requirePOST(r); e != nil {
		fail(w, e)
		return
	}
	var body pairRequestBody
	if _, e := readJSONBody(r, maxPairingBody, &body); e != nil {
		fail(w, e)
		return
	}
	if e := checkDeclaredOrigin(r, body.Origin); e != nil {
		fail(w, e)
		return
	}

	id, ttl, e := s.flow.Request(body.ApplicationName, body.Origin)
	if e != nil {
		fail(w, e)
		return
	}
	writeJSON(w, http.StatusOK, pairRequestResponse{
		RequestID:        id,
		ExpiresInSeconds: int(ttl.Seconds()),
	})
}

// handlePairConfirm completes a pairing and issues the device secret
// (F7 §2.1).
func (s *Server) handlePairConfirm(w http.ResponseWriter, r *http.Request) {
	if e := requirePOST(r); e != nil {
		fail(w, e)
		return
	}
	var body pairConfirmBody
	if _, e := readJSONBody(r, maxPairingBody, &body); e != nil {
		fail(w, e)
		return
	}
	if e := checkDeclaredOrigin(r, body.Origin); e != nil {
		fail(w, e)
		return
	}

	result, e := s.flow.Confirm(body.RequestID, body.Code, body.Origin)
	if e != nil {
		fail(w, e)
		return
	}
	writeJSON(w, http.StatusOK, pairConfirmResponse{
		AppID:           result.Pairing.AppID,
		DeviceSecret:    EncodeDeviceSecret(result.DeviceSecret),
		ApplicationName: result.Pairing.Name,
		Origin:          result.Pairing.Origin,
	})
}

// checkDeclaredOrigin reconciles the origin an application declared in
// its body with the Origin header, where there is one.
//
// A program on this machine declares its own origin, and can declare
// anything; what the value buys is that the person sees it at pairing
// time and that it is bound thereafter. A browser is the one caller
// that cannot lie — the header is set by the browser itself — so where
// the header exists it is authoritative and the two must agree.
//
// This is honest about its reach rather than pretending to more: the
// protocol's actual defence against a secret reaching a browser is that
// a browser cannot make one of these calls at all (see
// withProtocolRules), not that it would be caught if it did.
func checkDeclaredOrigin(r *http.Request, declared string) *errs.Error {
	header := r.Header.Get("Origin")
	if header == "" || header == declared {
		return nil
	}
	return errs.WithDetails(errs.CodeRequestInvalid,
		errors.New("the Origin header and the declared origin disagree"),
		map[string]any{"field": "origin"})
}
