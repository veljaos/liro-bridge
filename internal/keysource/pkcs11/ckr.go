package pkcs11

import (
	"errors"
	"fmt"
)

// ckr is a PKCS#11 return value (CK_RV).
//
// The names are PKCS#11 v2.40 §A. Every one of them is printed with its raw
// hexadecimal value beside it, and a value this table does not know says so
// rather than printing a plausible neighbour — because a lookup table that is
// wrong is worse than none: it does not look like a gap. That is not a
// hypothetical here. An earlier table in this project's own PKCS#11 work had
// 0x10 labelled CKR_DEVICE_ERROR when it is CKR_ATTRIBUTE_READ_ONLY, which
// made 0xE1 print as "?" — and 0xE1 is CKR_TOKEN_NOT_RECOGNIZED, the single
// most informative answer a Serbian card reader gives.
type ckr uint32

const (
	ckrOK                          ckr = 0x0000
	ckrCancel                      ckr = 0x0001
	ckrHostMemory                  ckr = 0x0002
	ckrSlotIDInvalid               ckr = 0x0003
	ckrGeneralError                ckr = 0x0005
	ckrFunctionFailed              ckr = 0x0006
	ckrArgumentsBad                ckr = 0x0007
	ckrAttributeReadOnly           ckr = 0x0010
	ckrAttributeSensitive          ckr = 0x0011
	ckrAttributeTypeInvalid        ckr = 0x0012
	ckrAttributeValueInvalid       ckr = 0x0013
	ckrDataInvalid                 ckr = 0x0020
	ckrDataLenRange                ckr = 0x0021
	ckrDeviceError                 ckr = 0x0030
	ckrDeviceMemory                ckr = 0x0031
	ckrDeviceRemoved               ckr = 0x0032
	ckrFunctionCanceled            ckr = 0x0050
	ckrFunctionNotSupported        ckr = 0x0054
	ckrKeyHandleInvalid            ckr = 0x0060
	ckrKeyTypeInconsistent         ckr = 0x0063
	ckrKeyFunctionNotPermitted     ckr = 0x0068
	ckrMechanismInvalid            ckr = 0x0070
	ckrMechanismParamInvalid       ckr = 0x0071
	ckrObjectHandleInvalid         ckr = 0x0082
	ckrOperationActive             ckr = 0x0090
	ckrOperationNotInitialized     ckr = 0x0091
	ckrPINIncorrect                ckr = 0x00A0
	ckrPINInvalid                  ckr = 0x00A1
	ckrPINLenRange                 ckr = 0x00A2
	ckrPINExpired                  ckr = 0x00A3
	ckrPINLocked                   ckr = 0x00A4
	ckrSessionClosed               ckr = 0x00B0
	ckrSessionCount                ckr = 0x00B1
	ckrSessionHandleInvalid        ckr = 0x00B3
	ckrSessionParallelNotSupported ckr = 0x00B4
	ckrSessionReadOnly             ckr = 0x00B5
	ckrSignatureInvalid            ckr = 0x00C0
	ckrSignatureLenRange           ckr = 0x00C1
	ckrTemplateIncomplete          ckr = 0x00D0
	ckrTemplateInconsistent        ckr = 0x00D1
	ckrTokenNotPresent             ckr = 0x00E0
	ckrTokenNotRecognized          ckr = 0x00E1
	ckrTokenWriteProtected         ckr = 0x00E2
	ckrUserAlreadyLoggedIn         ckr = 0x0100
	ckrUserNotLoggedIn             ckr = 0x0101
	ckrUserPINNotInitialized       ckr = 0x0102
	ckrUserTypeInvalid             ckr = 0x0103
	ckrBufferTooSmall              ckr = 0x0150
	ckrCryptokiNotInitialized      ckr = 0x0190
	ckrCryptokiAlreadyInitialized  ckr = 0x0191
	ckrFunctionRejected            ckr = 0x0200

	ckrVendorDefined ckr = 0x80000000
)

var ckrNames = map[ckr]string{
	ckrOK:                          "CKR_OK",
	ckrCancel:                      "CKR_CANCEL",
	ckrHostMemory:                  "CKR_HOST_MEMORY",
	ckrSlotIDInvalid:               "CKR_SLOT_ID_INVALID",
	ckrGeneralError:                "CKR_GENERAL_ERROR",
	ckrFunctionFailed:              "CKR_FUNCTION_FAILED",
	ckrArgumentsBad:                "CKR_ARGUMENTS_BAD",
	ckrAttributeReadOnly:           "CKR_ATTRIBUTE_READ_ONLY",
	ckrAttributeSensitive:          "CKR_ATTRIBUTE_SENSITIVE",
	ckrAttributeTypeInvalid:        "CKR_ATTRIBUTE_TYPE_INVALID",
	ckrAttributeValueInvalid:       "CKR_ATTRIBUTE_VALUE_INVALID",
	ckrDataInvalid:                 "CKR_DATA_INVALID",
	ckrDataLenRange:                "CKR_DATA_LEN_RANGE",
	ckrDeviceError:                 "CKR_DEVICE_ERROR",
	ckrDeviceMemory:                "CKR_DEVICE_MEMORY",
	ckrDeviceRemoved:               "CKR_DEVICE_REMOVED",
	ckrFunctionCanceled:            "CKR_FUNCTION_CANCELED",
	ckrFunctionNotSupported:        "CKR_FUNCTION_NOT_SUPPORTED",
	ckrKeyHandleInvalid:            "CKR_KEY_HANDLE_INVALID",
	ckrKeyTypeInconsistent:         "CKR_KEY_TYPE_INCONSISTENT",
	ckrKeyFunctionNotPermitted:     "CKR_KEY_FUNCTION_NOT_PERMITTED",
	ckrMechanismInvalid:            "CKR_MECHANISM_INVALID",
	ckrMechanismParamInvalid:       "CKR_MECHANISM_PARAM_INVALID",
	ckrObjectHandleInvalid:         "CKR_OBJECT_HANDLE_INVALID",
	ckrOperationActive:             "CKR_OPERATION_ACTIVE",
	ckrOperationNotInitialized:     "CKR_OPERATION_NOT_INITIALIZED",
	ckrPINIncorrect:                "CKR_PIN_INCORRECT",
	ckrPINInvalid:                  "CKR_PIN_INVALID",
	ckrPINLenRange:                 "CKR_PIN_LEN_RANGE",
	ckrPINExpired:                  "CKR_PIN_EXPIRED",
	ckrPINLocked:                   "CKR_PIN_LOCKED",
	ckrSessionClosed:               "CKR_SESSION_CLOSED",
	ckrSessionCount:                "CKR_SESSION_COUNT",
	ckrSessionHandleInvalid:        "CKR_SESSION_HANDLE_INVALID",
	ckrSessionParallelNotSupported: "CKR_SESSION_PARALLEL_NOT_SUPPORTED",
	ckrSessionReadOnly:             "CKR_SESSION_READ_ONLY",
	ckrSignatureInvalid:            "CKR_SIGNATURE_INVALID",
	ckrSignatureLenRange:           "CKR_SIGNATURE_LEN_RANGE",
	ckrTemplateIncomplete:          "CKR_TEMPLATE_INCOMPLETE",
	ckrTemplateInconsistent:        "CKR_TEMPLATE_INCONSISTENT",
	ckrTokenNotPresent:             "CKR_TOKEN_NOT_PRESENT",
	ckrTokenNotRecognized:          "CKR_TOKEN_NOT_RECOGNIZED",
	ckrTokenWriteProtected:         "CKR_TOKEN_WRITE_PROTECTED",
	ckrUserAlreadyLoggedIn:         "CKR_USER_ALREADY_LOGGED_IN",
	ckrUserNotLoggedIn:             "CKR_USER_NOT_LOGGED_IN",
	ckrUserPINNotInitialized:       "CKR_USER_PIN_NOT_INITIALIZED",
	ckrUserTypeInvalid:             "CKR_USER_TYPE_INVALID",
	ckrBufferTooSmall:              "CKR_BUFFER_TOO_SMALL",
	ckrCryptokiNotInitialized:      "CKR_CRYPTOKI_NOT_INITIALIZED",
	ckrCryptokiAlreadyInitialized:  "CKR_CRYPTOKI_ALREADY_INITIALIZED",
	ckrFunctionRejected:            "CKR_FUNCTION_REJECTED",
}

func (r ckr) String() string {
	if name, ok := ckrNames[r]; ok {
		return fmt.Sprintf("%s (0x%X)", name, uint32(r))
	}
	if r&ckrVendorDefined != 0 {
		return fmt.Sprintf("CKR_VENDOR_DEFINED+0x%X (0x%X)", uint32(r&^ckrVendorDefined), uint32(r))
	}
	return fmt.Sprintf("unknown CK_RV (0x%X)", uint32(r))
}

// ckrError is a failed PKCS#11 call, carrying the function that failed so that
// a log line says which one rather than only what it returned.
type ckrError struct {
	fn string
	rv ckr
}

func (e *ckrError) Error() string { return e.fn + ": " + e.rv.String() }

// asCKR reports the CK_RV inside err, if there is one.
func asCKR(err error) (ckr, bool) {
	var e *ckrError
	if errors.As(err, &e) {
		return e.rv, true
	}
	return 0, false
}
