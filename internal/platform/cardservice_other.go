//go:build !linux

package platform

import "context"

// CardServiceCheck is nil where the reader listing already says whether
// the smart card service is running: on Windows, SCardEstablishContext
// answers SCARD_E_NO_SERVICE itself (F1 §2.3). See cardservice_linux.go
// for why Linux needs the separate question.
func CardServiceCheck() func(context.Context) error { return nil }
