package signing

import (
	"context"
	"errors"

	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/keysource"
)

// fakeKeySession is a programmable keysource.Session used across this
// package's tests — mirroring the fakeStore/fakeConn pattern used
// elsewhere in this project to test orchestration logic without real
// hardware.
type fakeKeySession struct {
	cert keysource.Certificate

	// signResults, if set, is consumed one entry per SignDigest call,
	// in order — each entry is either a signature or an error. When
	// exhausted, further calls succeed with a placeholder signature.
	signResults []signResult

	// delays, if set, is consumed in parallel with signResults via a
	// fake clock advanced by the test — see withFakeClock in
	// timing_test.go. Sign itself does not sleep; tests advance a fake
	// now() function instead, so these tests run in milliseconds, not
	// in real seconds.
	calls  int
	closed bool
}

type signResult struct {
	sig []byte
	err error
}

func (f *fakeKeySession) SignDigest(_ context.Context, _ keysource.DigestAlgorithm, _ []byte) ([]byte, error) {
	i := f.calls
	f.calls++
	if i < len(f.signResults) {
		return f.signResults[i].sig, f.signResults[i].err
	}
	return []byte("sig"), nil
}

func (f *fakeKeySession) Certificate() keysource.Certificate { return f.cert }
func (f *fakeKeySession) Chain() [][]byte                    { return nil }
func (f *fakeKeySession) Close() error {
	f.closed = true
	return nil
}

// fakeKeySource opens the same fakeKeySession every time, unless
// openErr is set.
type fakeKeySource struct {
	session *fakeKeySession
	openErr error
	opens   int
}

func (f *fakeKeySource) Name() string { return "fake" }

func (f *fakeKeySource) List(context.Context) ([]keysource.Certificate, error) {
	return []keysource.Certificate{f.session.cert}, nil
}

func (f *fakeKeySource) Open(context.Context, keysource.Thumbprint) (keysource.Session, error) {
	f.opens++
	if f.openErr != nil {
		return nil, f.openErr
	}
	return f.session, nil
}

var errFakeOpen = errors.New("fake open failure")

// codeErr builds an *errs.Error for a given code, for tests that need a
// specific failure classification from a fake backend.
func codeErr(code errs.Code) error {
	return errs.New(code, errors.New("fake failure"))
}
