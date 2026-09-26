//go:build linux

package platform

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"testing"
)

// TestPCSCDIsFoundWhereTheClientLibraryLooks is the three states a vendor
// module can find pcscd in, each made with a real socket rather than a
// fake of one: listening, never installed, and a socket file left behind
// with nothing listening — which pcsc-lite treats as no service too.
func TestPCSCDIsFoundWhereTheClientLibraryLooks(t *testing.T) {
	dir := t.TempDir()

	listening := filepath.Join(dir, "listening.comm")
	l, err := net.Listen("unix", listening)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	stale := filepath.Join(dir, "stale.comm")
	sl, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	sl.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = sl.Close()

	for _, tc := range []struct {
		name, path string
		down       bool
	}{
		{"pcscd listening", listening, false},
		{"no socket at all", filepath.Join(dir, "absent.comm"), true},
		{"a socket file nothing listens on", stale, true},
		// pcsc-lite uses the empty path rather than its default (D-358).
		{"the variable set and empty", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(pcscdSocketEnv, tc.path)
			err := CardServiceCheck()(context.Background())
			if got := errors.Is(err, ErrSmartCardServiceDown); got != tc.down {
				t.Fatalf("service down = %v (err %v), want %v", got, err, tc.down)
			}
		})
	}
}
