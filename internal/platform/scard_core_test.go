package platform

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type listResult struct {
	names []string
	err   error
}

// fakeScardConn stands in for the real winscard.dll calls, so the
// re-establish/retry and status-mapping logic in pcscService can be
// exercised without hardware (F1 §2.5).
type fakeScardConn struct {
	establishN   int
	establishErr error

	listSeq []listResult // consumed front-to-back, one per establishContext

	statusFn func(names []string) ([]ReaderState, error)

	released int
}

func (f *fakeScardConn) establishContext() (scardContext, error) {
	f.establishN++
	if f.establishErr != nil {
		return 0, f.establishErr
	}
	return scardContext(f.establishN), nil
}

func (f *fakeScardConn) releaseContext(scardContext) error {
	f.released++
	return nil
}

func (f *fakeScardConn) listReaders(scardContext) ([]string, error) {
	if len(f.listSeq) == 0 {
		return nil, errors.New("fake: no more listReaders results queued")
	}
	r := f.listSeq[0]
	f.listSeq = f.listSeq[1:]
	return r.names, r.err
}

func (f *fakeScardConn) statusChange(_ scardContext, names []string) ([]ReaderState, error) {
	if f.statusFn == nil {
		return nil, errors.New("fake: statusChange not configured")
	}
	return f.statusFn(names)
}

func TestReadersNoReadersAvailableIsNotAnError(t *testing.T) {
	fake := &fakeScardConn{listSeq: []listResult{{err: errNoReadersAvailable}}}
	svc := &pcscService{conn: fake}

	got, err := svc.Readers(context.Background())
	if err != nil {
		t.Fatalf("Readers() error = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("Readers() = %#v, want empty slice", got)
	}
	if fake.released != 1 {
		t.Fatalf("context released %d times, want 1", fake.released)
	}
}

func TestReadersMapsNoServiceToSentinel(t *testing.T) {
	fake := &fakeScardConn{listSeq: []listResult{{err: ErrSmartCardServiceDown}}}
	svc := &pcscService{conn: fake}

	_, err := svc.Readers(context.Background())
	if !errors.Is(err, ErrSmartCardServiceDown) {
		t.Fatalf("Readers() error = %v, want ErrSmartCardServiceDown", err)
	}
}

func TestReadersInvalidHandleRetriesExactlyOnceThenSucceeds(t *testing.T) {
	fake := &fakeScardConn{
		listSeq: []listResult{
			{err: errContextInvalid},
			{names: []string{"Reader A"}},
		},
		statusFn: func(names []string) ([]ReaderState, error) {
			return []ReaderState{{Name: names[0], CardPresent: true}}, nil
		},
	}
	svc := &pcscService{conn: fake}

	got, err := svc.Readers(context.Background())
	if err != nil {
		t.Fatalf("Readers() error = %v, want nil", err)
	}
	if len(got) != 1 || got[0].Name != "Reader A" {
		t.Fatalf("Readers() = %#v", got)
	}
	if fake.establishN != 2 {
		t.Fatalf("establishContext called %d times, want exactly 2 (one retry)", fake.establishN)
	}
	if fake.released != 2 {
		t.Fatalf("context released %d times, want 2", fake.released)
	}
}

// TestReadersInvalidHandleNeverLoops is the test that makes the "do not
// loop" rule in F1 §2.3 fail if violated: the fake would happily return
// errContextInvalid forever, so a bug that keeps retrying would hang this
// test (or exhaust the queue) instead of returning after one retry.
func TestReadersInvalidHandleNeverLoops(t *testing.T) {
	fake := &fakeScardConn{
		listSeq: []listResult{
			{err: errContextInvalid},
			{err: errContextInvalid},
			{err: errContextInvalid},
		},
	}
	svc := &pcscService{conn: fake}

	_, err := svc.Readers(context.Background())
	if !errors.Is(err, errContextInvalid) {
		t.Fatalf("Readers() error = %v, want errContextInvalid surfaced after the single retry", err)
	}
	if fake.establishN != 2 {
		t.Fatalf("establishContext called %d times, want exactly 2 (initial attempt + one retry, no more)", fake.establishN)
	}
}

func TestReadersReleasesContextOnEstablishSuccessButListError(t *testing.T) {
	fake := &fakeScardConn{listSeq: []listResult{{err: errors.New("boom")}}}
	svc := &pcscService{conn: fake}

	_, err := svc.Readers(context.Background())
	if err == nil {
		t.Fatal("Readers() error = nil, want the underlying error surfaced")
	}
	if fake.released != 1 {
		t.Fatalf("context released %d times, want 1 (every path releases)", fake.released)
	}
}

func TestAnyCardPresentTrueWhenOneReaderHasCard(t *testing.T) {
	fake := &fakeScardConn{
		listSeq: []listResult{{names: []string{"A", "B"}}},
		statusFn: func(names []string) ([]ReaderState, error) {
			return []ReaderState{
				{Name: names[0], CardPresent: false},
				{Name: names[1], CardPresent: true},
			}, nil
		},
	}
	svc := &pcscService{conn: fake}

	present, err := svc.AnyCardPresent(context.Background())
	if err != nil {
		t.Fatalf("AnyCardPresent() error = %v", err)
	}
	if !present {
		t.Fatal("AnyCardPresent() = false, want true")
	}
}

func TestAnyCardPresentFalseWhenNoReaderHasCard(t *testing.T) {
	fake := &fakeScardConn{
		listSeq: []listResult{{names: []string{"A"}}},
		statusFn: func(names []string) ([]ReaderState, error) {
			return []ReaderState{{Name: names[0], CardPresent: false}}, nil
		},
	}
	svc := &pcscService{conn: fake}

	present, err := svc.AnyCardPresent(context.Background())
	if err != nil {
		t.Fatalf("AnyCardPresent() error = %v", err)
	}
	if present {
		t.Fatal("AnyCardPresent() = true, want false")
	}
}

func TestMapReturnCode(t *testing.T) {
	cases := []struct {
		name string
		code uint32
		want error
	}{
		{"success", scardSSuccess, nil},
		{"no readers available", scardENoReadersAvailable, errNoReadersAvailable},
		{"no service", scardENoService, ErrSmartCardServiceDown},
		{"invalid handle", scardEInvalidHandle, errContextInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mapReturnCode("SomeCall", c.code)
			if c.want == nil {
				if got != nil {
					t.Fatalf("mapReturnCode(%s) = %v, want nil", c.name, got)
				}
				return
			}
			if !errors.Is(got, c.want) {
				t.Fatalf("mapReturnCode(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}

	got := mapReturnCode("SomeCall", 0xDEADBEEF)
	if got == nil || !strings.Contains(got.Error(), "SomeCall") || !strings.Contains(got.Error(), "DEADBEEF") {
		t.Fatalf("mapReturnCode(unknown) = %v, want it to name the call and the code", got)
	}
}
