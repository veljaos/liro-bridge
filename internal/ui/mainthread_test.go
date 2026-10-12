package ui

import (
	"testing"
	"time"
)

// A link delivered before anyone asked for links is kept, and handed
// over when the handler is set; one after it goes straight through.
func TestALinkDeliveredBeforeTheHandlerIsKept(t *testing.T) {
	t.Cleanup(func() { SetURLHandler(nil) })
	urls.Lock()
	urls.handler, urls.pending = nil, nil
	urls.Unlock()

	deliverURL("liro-bridge://sign?job=https://a.example/1")
	got := make(chan string, 2)
	SetURLHandler(func(u string) { got <- u })
	deliverURL("liro-bridge://sign?job=https://b.example/2")
	for _, want := range []string{"liro-bridge://sign?job=https://a.example/1", "liro-bridge://sign?job=https://b.example/2"} {
		select {
		case u := <-got:
			if u != want {
				t.Fatalf("got %q, want %q", u, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%q never arrived", want)
		}
	}
}
