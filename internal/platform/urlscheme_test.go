package platform

import "testing"

func TestLinkCommandQuotesTheExecutableAndTheLink(t *testing.T) {
	got := LinkCommand(`C:\Program Files\Liro\liro-bridge.exe`)
	want := `"C:\Program Files\Liro\liro-bridge.exe" open-link "%1"`
	if got != want {
		t.Fatalf("LinkCommand = %q, want %q", got, want)
	}
}
