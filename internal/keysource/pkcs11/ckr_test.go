package pkcs11

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// A CK_RV table that is wrong is worse than none, because it does not look
// like a gap — it looks like an answer. This project has already paid for one:
// an earlier table in its PKCS#11 work had 0x10 labelled CKR_DEVICE_ERROR when
// it is CKR_ATTRIBUTE_READ_ONLY, which made 0xE1 print as "?" — and 0xE1 is
// CKR_TOKEN_NOT_RECOGNIZED, the answer that tells this layer a card is not
// this module's rather than that something failed.
//
// So the values are pinned against PKCS#11 v2.40 §A here rather than trusted,
// and the two that were wrong before are first.
func TestTheReturnValueTableIsRight(t *testing.T) {
	for _, c := range []struct {
		rv   ckr
		want string
	}{
		{0x0010, "CKR_ATTRIBUTE_READ_ONLY"},  // was wrong once; not CKR_DEVICE_ERROR
		{0x00E1, "CKR_TOKEN_NOT_RECOGNIZED"}, // the consequence of that being wrong
		{0x0000, "CKR_OK"},
		{0x0007, "CKR_ARGUMENTS_BAD"},
		{0x0030, "CKR_DEVICE_ERROR"},
		{0x0050, "CKR_FUNCTION_CANCELED"},
		{0x00A0, "CKR_PIN_INCORRECT"},
		{0x00A2, "CKR_PIN_LEN_RANGE"},
		{0x00A4, "CKR_PIN_LOCKED"},
		{0x00E0, "CKR_TOKEN_NOT_PRESENT"},
		{0x0101, "CKR_USER_NOT_LOGGED_IN"},
		{0x0190, "CKR_CRYPTOKI_NOT_INITIALIZED"},
	} {
		got := c.rv.String()
		if !strings.HasPrefix(got, c.want+" ") {
			t.Errorf("ckr(0x%X).String() = %q, want it to name %s", uint32(c.rv), got, c.want)
		}
		if !strings.Contains(got, fmt.Sprintf("0x%X", uint32(c.rv))) {
			t.Errorf("ckr(0x%X).String() = %q, want the raw value in it too — a name "+
				"without the number cannot be checked against the specification",
				uint32(c.rv), got)
		}
	}
}

// TestNoTwoReturnValuesShareAName catches a transposition, which is how the
// earlier table went wrong: two entries with one value, or one name used
// twice, is a silent aliasing rather than a compile error.
func TestNoTwoReturnValuesShareAName(t *testing.T) {
	seen := map[string]ckr{}
	for rv, name := range ckrNames {
		if other, dup := seen[name]; dup {
			t.Errorf("%s is the name of both 0x%X and 0x%X", name, uint32(other), uint32(rv))
		}
		seen[name] = rv
	}
}

func TestAnUnknownReturnValueSaysSoRatherThanGuessing(t *testing.T) {
	got := ckr(0x0FFF).String()
	if !strings.Contains(got, "unknown") {
		t.Errorf("ckr(0x0FFF).String() = %q, want it to say it does not know — a table "+
			"that answers for a value it has never seen is the defect this test exists for", got)
	}
	if !strings.Contains(got, "0xFFF") {
		t.Errorf("ckr(0x0FFF).String() = %q, want the raw value", got)
	}

	vendor := ckr(0x80000042).String()
	if !strings.Contains(vendor, "VENDOR_DEFINED") || !strings.Contains(vendor, "0x42") {
		t.Errorf("a vendor-defined value rendered as %q, want it named as vendor-defined "+
			"with its offset", vendor)
	}
}

func TestACallFailureCarriesTheFunctionThatFailed(t *testing.T) {
	err := error(&ckrError{"C_GetTokenInfo", ckrTokenNotRecognized})
	if !strings.Contains(err.Error(), "C_GetTokenInfo") {
		t.Errorf("%q does not name the function that failed", err)
	}

	rv, ok := asCKR(fmt.Errorf("reading the token: %w", err))
	if !ok || rv != ckrTokenNotRecognized {
		t.Errorf("asCKR through a wrapped error = (%v, %v), want (CKR_TOKEN_NOT_RECOGNIZED, true)", rv, ok)
	}
	if _, ok := asCKR(errors.New("something else")); ok {
		t.Error("asCKR found a CK_RV in an error that has none")
	}
}
