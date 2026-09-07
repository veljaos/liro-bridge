package api

import (
	"net/http"
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// TestOnlyINTERNALIsAServerError is the rule the first real-client run
// had to correct.
//
// A job the person did not answer came back as HTTP 500, because every
// code without an explicit case fell through to one. That is a lie
// twice over: it tells an integrator the agent is broken, and it tells
// their monitoring the same. 500 belongs to INTERNAL, which is what
// INTERNAL means (SPEC §7).
//
// It walks errs.AllCodes rather than a list kept here, so a code added
// later cannot quietly reintroduce the default (D-158's method).
func TestOnlyINTERNALIsAServerError(t *testing.T) {
	for _, code := range errs.AllCodes() {
		status := statusFor(code)
		if code == errs.CodeInternal {
			if status != http.StatusInternalServerError {
				t.Errorf("%s maps to %d, want 500", code, status)
			}
			continue
		}
		if status >= 500 {
			t.Errorf("%s maps to %d; only INTERNAL is a server error", code, status)
		}
		if status < 400 {
			t.Errorf("%s maps to %d, which is not a refusal at all", code, status)
		}
	}
}

// TestTheCodesAPersonDecidedAreForbidden: a refusal and a window
// nobody answered are the same kind of answer — not authorised — and
// neither is a fault of the request or of the agent.
func TestTheCodesAPersonDecidedAreForbidden(t *testing.T) {
	for _, code := range []errs.Code{errs.CodeConsentDenied, errs.CodeConsentTimeout, errs.CodePairingDenied} {
		if got := statusFor(code); got != http.StatusForbidden {
			t.Errorf("%s maps to %d, want 403", code, got)
		}
	}
}

// TestASigningConditionIsUnprocessable covers the default: the card,
// the PIN, the certificate, the document.
func TestASigningConditionIsUnprocessable(t *testing.T) {
	for _, code := range []errs.Code{
		errs.CodeCardNotPresent, errs.CodePINLocked, errs.CodeCertNotFound,
		errs.CodePDFInvalid, errs.CodeSignFailed, errs.CodeTSAUnavailable,
	} {
		if got := statusFor(code); got != http.StatusUnprocessableEntity {
			t.Errorf("%s maps to %d, want 422", code, got)
		}
	}
}
