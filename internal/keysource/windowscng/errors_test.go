package windowscng

import (
	"testing"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// TestMapStatusCoversEverySpecifiedCode proves every status row in F2
// §2.4 maps to its documented code — this is the failing test the
// review discipline asks for: change any one of these mappings and this
// test goes red.
func TestMapStatusCoversEverySpecifiedCode(t *testing.T) {
	cases := []struct {
		name   string
		status uint32
		want   errs.Code
	}{
		{"SCARD_W_CANCELLED_BY_USER", 0x8010006E, errs.CodeConsentDenied},
		{"SCARD_W_WRONG_CHV", 0x8010006B, errs.CodePINIncorrect},
		{"SCARD_W_CHV_BLOCKED", 0x8010006C, errs.CodePINLocked},
		{"SCARD_W_REMOVED_CARD", 0x80100069, errs.CodeCardNotPresent},
		{"NTE_BAD_KEYSET", 0x80090016, errs.CodeCertNotFound},
		{"NTE_NO_KEY", 0x8009000D, errs.CodeCertNotUsable},
		{"anything else", 0xDEADBEEF, errs.CodeSignFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mapStatus("NCryptSignHash", c.status)
			if got.Code != c.want {
				t.Fatalf("mapStatus(0x%08X).Code = %q, want %q", c.status, got.Code, c.want)
			}
		})
	}
}

// TestMapStatusUnclassifiedCarriesHexInDetails proves the fallback case
// never turns the raw status into a human-readable message field — only
// structured Details cross an API boundary (SPEC §7).
func TestMapStatusUnclassifiedCarriesHexInDetails(t *testing.T) {
	got := mapStatus("NCryptSignHash", 0xDEADBEEF)
	if got.Details["status"] != "0xDEADBEEF" {
		t.Fatalf("Details[status] = %v, want 0xDEADBEEF", got.Details["status"])
	}
}
