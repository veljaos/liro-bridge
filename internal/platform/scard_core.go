package platform

import (
	"context"
	"errors"
	"fmt"
)

// Return codes from winscard.dll (and PC/SC generally) that this package
// treats specially. Values from the WinSCard header; see F1 §2.3.
const (
	scardSSuccess            uint32 = 0x00000000
	scardENoReadersAvailable uint32 = 0x8010002E
	scardENoService          uint32 = 0x8010001D
	scardEInvalidHandle      uint32 = 0x80100003
)

// errNoReadersAvailable means the machine has no smart card reader
// attached. It is not surfaced as an error from Readers — F1 §2.3:
// SCARD_E_NO_READERS_AVAILABLE means "no reader", not a failure.
var errNoReadersAvailable = errors.New("no smart card readers available")

// errContextInvalid means the established context has gone stale (service
// restart, reader unplug). readers() re-establishes the context and
// retries exactly once on this error, never more.
var errContextInvalid = errors.New("smart card context is no longer valid")

// ErrSmartCardServiceDown means the OS smart card service is not running.
// Unlike an absent reader, the fix is to start a service, not to plug in
// hardware (F1 §2.3, errs.CodeSmartCardServiceDown).
var ErrSmartCardServiceDown = errors.New("smart card service is not running")

// mapReturnCode converts a raw SCard return code into the sentinel errors
// above, or nil on success. op names the failing call, used only for the
// generic, unclassified-error case.
func mapReturnCode(op string, code uint32) error {
	switch code {
	case scardSSuccess:
		return nil
	case scardENoReadersAvailable:
		return errNoReadersAvailable
	case scardENoService:
		return ErrSmartCardServiceDown
	case scardEInvalidHandle:
		return errContextInvalid
	default:
		return fmt.Errorf("%s: SCard error 0x%08X", op, code)
	}
}

// scardContext is an opaque handle returned by scardConn.establishContext.
type scardContext uintptr

// scardConn is the thin interface over the OS smart card API that the
// retry and status-mapping logic in pcscService is tested against. The
// Windows implementation backs it with winscard.dll (smartcard_windows.go);
// tests back it with a fake, since the DLL calls themselves cannot be
// unit-tested (F1 §2.5).
type scardConn interface {
	establishContext() (scardContext, error)
	releaseContext(scardContext) error
	listReaders(scardContext) ([]string, error)
	statusChange(scardContext, []string) ([]ReaderState, error)
}

// pcscService implements SmartCardService over a scardConn, with the
// re-establish-and-retry-once behaviour required by F1 §2.3.
type pcscService struct {
	conn scardConn
}

// Readers implements SmartCardService.
func (s *pcscService) Readers(_ context.Context) ([]ReaderState, error) {
	return s.readers(1)
}

// readers does the actual work. retriesLeft bounds re-establishment on a
// stale context to exactly one attempt — never an unbounded loop.
func (s *pcscService) readers(retriesLeft int) ([]ReaderState, error) {
	hctx, err := s.conn.establishContext()
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.conn.releaseContext(hctx) }()

	names, err := s.conn.listReaders(hctx)
	if err != nil {
		switch {
		case errors.Is(err, errNoReadersAvailable):
			return []ReaderState{}, nil
		case errors.Is(err, errContextInvalid) && retriesLeft > 0:
			return s.readers(retriesLeft - 1)
		default:
			return nil, err
		}
	}
	if len(names) == 0 {
		return []ReaderState{}, nil
	}

	states, err := s.conn.statusChange(hctx, names)
	if err != nil {
		if errors.Is(err, errContextInvalid) && retriesLeft > 0 {
			return s.readers(retriesLeft - 1)
		}
		return nil, err
	}
	return states, nil
}

// AnyCardPresent implements SmartCardService.
func (s *pcscService) AnyCardPresent(ctx context.Context) (bool, error) {
	return anyCardPresent(ctx, s)
}
