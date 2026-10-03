package main

import (
	"context"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/errs"
	"github.com/veljaos/liro-bridge/internal/jobs"
)

// TestTheCallersAnswerIsACopyTakenOnce is D33's hand-over: what the
// caller is given at the end of a run is fixed there, and nothing the
// window does while the report is on screen reaches it.
func TestTheCallersAnswerIsACopyTakenOnce(t *testing.T) {
	b := &remoteBatch{
		outcomes:  []api.SignOutcome{{Signature: []byte{1}}, {Code: errs.CodeSignFailed}},
		delivered: make(chan struct{}),
	}

	b.deliver()
	select {
	case <-b.delivered:
	default:
		t.Fatal("deliver did not close delivered, so the caller would go on waiting for the window")
	}

	b.outcomes[0] = api.SignOutcome{Code: errs.CodeConsentDenied}
	b.code = errs.CodeConsentDenied
	b.deliver() // a second call must neither panic on the closed channel nor take a second copy

	if len(b.final.Outcomes) != 2 || len(b.final.Outcomes[0].Signature) != 1 || b.final.Code != "" {
		t.Fatalf("the delivered answer changed after it was delivered: %+v", b.final)
	}
}

// TestDeliverWithoutAChannelDoesNotPanic covers a batch built without
// one, as some tests build it.
func TestDeliverWithoutAChannelDoesNotPanic(t *testing.T) {
	(&remoteBatch{}).deliver()
}

// TestSignAnswersAtTheRunsEndAndKeepsTheWindowSlot is D33 from the API's
// side: Sign returns the moment the flow has an answer, while its window
// is still up, and a second request's window does not open until that
// one has gone — the report a person is reading is still a window, and
// the one-window rule (protocolSigner) is about windows.
func TestSignAnswersAtTheRunsEndAndKeepsTheWindowSlot(t *testing.T) {
	first := make(chan struct{})
	flows := make(chan string, 2)

	original := protocolFlow
	t.Cleanup(func() { protocolFlow = original })
	protocolFlow = func(_ context.Context, _ config.Config, _ string, req api.SignRequest, _ *jobs.Job) (api.SignResult, <-chan struct{}) {
		flows <- req.Application
		if req.Application == "first" {
			return api.SignResult{Outcomes: []api.SignOutcome{{Signature: []byte{1}}}}, first
		}
		gone := make(chan struct{})
		close(gone)
		return api.SignResult{}, gone
	}

	s := newProtocolSigner(config.Default())

	answered := make(chan api.SignResult, 1)
	go func() {
		r, _ := s.Sign(context.Background(), api.SignRequest{Application: "first"}, &jobs.Job{ID: "a"})
		answered <- r
	}()
	select {
	case r := <-answered:
		if r.Signed() != 1 {
			t.Fatalf("Sign answered %+v, want the one signature the flow returned", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Sign did not return while the first window was still up: the caller is waiting for the window again")
	}
	<-flows

	second := make(chan struct{})
	go func() {
		_, _ = s.Sign(context.Background(), api.SignRequest{Application: "second"}, &jobs.Job{ID: "b"})
		close(second)
	}()
	select {
	case app := <-flows:
		t.Fatalf("the %s request's window opened while the first window was still up", app)
	case <-time.After(200 * time.Millisecond):
	}

	close(first)
	select {
	case <-second:
	case <-time.After(5 * time.Second):
		t.Fatal("the second request never ran after the first window went: the slot was not given back")
	}
}
