package audit

import (
	"encoding/hex"

	"github.com/veljaos/liro-bridge/internal/errs"
)

func hexEncode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return hex.EncodeToString(b)
}

func hexDecode(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return hex.DecodeString(s)
}

func errCode(s string) errs.Code {
	return errs.Code(s)
}
