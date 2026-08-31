package errs

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestSerialisationOmitsCause proves that the internal cause never appears
// in the JSON sent across an API boundary. This is the test that stops
// someone helpfully adding a Message field in six months.
func TestSerialisationOmitsCause(t *testing.T) {
	cause := errors.New("smart card reader driver returned SCARD_E_NO_SMARTCARD for reader \"ACS ACR38\"")
	err := New(CodeCardNotPresent, cause)

	b, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("Marshal: %v", marshalErr)
	}

	if strings.Contains(string(b), "SCARD_E_NO_SMARTCARD") || strings.Contains(string(b), "ACS ACR38") {
		t.Fatalf("serialised error leaked the cause: %s", b)
	}

	var decoded struct {
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.Code != string(CodeCardNotPresent) {
		t.Fatalf("Code = %q, want %q", decoded.Code, CodeCardNotPresent)
	}
	if decoded.Details != nil {
		t.Fatalf("Details = %v, want nil (no details were set)", decoded.Details)
	}
}

func TestWithDetailsSerialisesDetails(t *testing.T) {
	err := WithDetails(CodeNoReader, errors.New("PC/SC context init failed"), map[string]any{
		"reader": "Generic Smart Card Reader",
	})

	b, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatalf("Marshal: %v", marshalErr)
	}
	if strings.Contains(string(b), "PC/SC") {
		t.Fatalf("serialised error leaked the cause: %s", b)
	}
	if !strings.Contains(string(b), "Generic Smart Card Reader") {
		t.Fatalf("serialised error is missing details: %s", b)
	}
}

func TestUnwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	err := New(CodeInternal, sentinel)
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is did not find the wrapped cause")
	}
}

func TestErrorStringIncludesCause(t *testing.T) {
	cause := errors.New("boom")
	err := New(CodeInternal, cause)
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Error() = %q, want it to include the cause for logs", err.Error())
	}
}
