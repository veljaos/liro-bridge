package windowscng

import (
	"context"
	"errors"
	"testing"

	"github.com/veljaos/liro-bridge/internal/keysource"
)

func TestSourceNameIsWindowsCNG(t *testing.T) {
	if got := (Source{}).Name(); got != "windows-cng" {
		t.Fatalf("Name() = %q, want windows-cng", got)
	}
}

func TestSourceOpenUsesInjectedConn(t *testing.T) {
	conn := &fakeConn{der: []byte("cert"), key: 7}
	src := Source{conn: conn}
	sess, err := src.Open(context.Background(), keysource.Thumbprint("ABC"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if sess.Certificate().Thumbprint != "ABC" {
		t.Fatalf("Certificate().Thumbprint = %q, want ABC", sess.Certificate().Thumbprint)
	}
}

func TestSourceOpenRejectsCancelledContext(t *testing.T) {
	src := Source{conn: &fakeConn{der: []byte("cert"), key: 1}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := src.Open(ctx, keysource.Thumbprint("ABC")); err == nil {
		t.Fatal("Open on a cancelled context must fail")
	}
}

// TestSourcePresenceReflectsPerCertificateResult is Task 2's discriminator:
// two certificates, only one of which reports its card present, must not
// share one answer — this is exactly the bug the task describes (one
// card inserted marking every hardware-backed certificate as available).
func TestSourcePresenceReflectsPerCertificateResult(t *testing.T) {
	present := Source{conn: &fakeConn{presencePresent: true}}
	absent := Source{conn: &fakeConn{presencePresent: false}}

	ok, err := present.Presence(context.Background(), keysource.Thumbprint("MUP"))
	if err != nil || !ok {
		t.Fatalf("Presence = %v, %v; want true, nil", ok, err)
	}
	ok, err = absent.Presence(context.Background(), keysource.Thumbprint("HALCOM"))
	if err != nil || ok {
		t.Fatalf("Presence = %v, %v; want false, nil", ok, err)
	}
}

func TestSourcePresencePropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	src := Source{conn: &fakeConn{presenceErr: wantErr}}
	if _, err := src.Presence(context.Background(), keysource.Thumbprint("ABC")); !errors.Is(err, wantErr) {
		t.Fatalf("Presence error = %v, want %v", err, wantErr)
	}
}

func TestSourcePresenceRejectsCancelledContext(t *testing.T) {
	src := Source{conn: &fakeConn{presencePresent: true}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := src.Presence(ctx, keysource.Thumbprint("ABC")); err == nil {
		t.Fatal("Presence on a cancelled context must fail")
	}
}
