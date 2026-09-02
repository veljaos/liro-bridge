package windowscng

import (
	"context"
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
