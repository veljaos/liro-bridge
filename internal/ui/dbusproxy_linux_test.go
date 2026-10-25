//go:build linux

package ui

import (
	"errors"
	"os"
	"testing"
	"testing/fstest"
	"time"
)

func TestTheStatLineIsReadPastACommandNameWithParentheses(t *testing.T) {
	line := "4242 (a (b) c) S 17 4242 4242 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 1 0 987654 1000 10 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0"
	p, ok := parseStat(4242, line)
	if !ok {
		t.Fatal("not parsed")
	}
	if p.comm != "a (b) c" || p.ppid != 17 || p.start != 987654 {
		t.Fatalf("got %+v", p)
	}
}

func chain(start uint64, proxy int, web bool) sandboxChain {
	return sandboxChain{root: procInfo{start: start}, proxy: proxy, web: web}
}

// The pairing rule on its own: each live web process keeps the latest
// proxy started no later than it; every other proxy is an orphan.
func TestAProxyWhoseWebProcessHasGoneIsTheOnlyOneReleased(t *testing.T) {
	cases := []struct {
		name   string
		chains []sandboxChain
		want   []int
	}{
		{"one window open", []sandboxChain{chain(10, 101, false), chain(11, 0, true)}, nil},
		{"one window closed", []sandboxChain{chain(10, 101, false)}, []int{101}},
		{"first of two closed", []sandboxChain{chain(10, 101, false), chain(20, 201, false), chain(21, 0, true)}, []int{101}},
		{"second of two closed", []sandboxChain{chain(10, 101, false), chain(11, 0, true), chain(20, 201, false)}, []int{201}},
		{"three left, two windows open", []sandboxChain{chain(5, 51, false), chain(10, 101, false), chain(11, 0, true), chain(20, 201, false), chain(20, 0, true)}, []int{51}},
		{"nothing", nil, nil},
	}
	for _, c := range cases {
		got := orphanedProxies(c.chains)
		if len(got) != len(c.want) {
			t.Errorf("%s: orphans %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: orphans %v, want %v", c.name, got, c.want)
			}
		}
	}
}

func dbusProxiesUnderThisProcess() int {
	n := 0
	self := os.Getpid()
	procs := readProcs()
	var walk func(int)
	children := map[int][]int{}
	for pid, p := range procs {
		children[p.ppid] = append(children[p.ppid], pid)
	}
	walk = func(p int) {
		for _, c := range children[p] {
			if procs[c].comm == "xdg-dbus-proxy" {
				n++
			}
			walk(c)
		}
	}
	walk(self)
	return n
}

// TestClosingAWindowLeavesNoDBusProxyBehind is D23 ([[D-460]]): every
// window closed left a bwrap → bwrap → xdg-dbus-proxy chain for the life
// of the agent. It needs what the agent needs — a display, and a
// bubblewrap that can make a sandbox — and skips without either, which
// is every CI runner this project has: measured where it can be, under
// Xvfb, and said so.
func TestClosingAWindowLeavesNoDBusProxyBehind(t *testing.T) {
	requireWebKitCanStart(t)
	if err := theUIThread.start(); err != nil {
		if errors.Is(err, ErrNoDisplay) {
			t.Skip("no windowing system on this machine: ", err)
		}
		t.Fatal(err)
	}
	if os.Getenv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS") != "" {
		t.Skip("the sandbox is off, so WebKit starts no D-Bus proxy and there is nothing to count")
	}
	assets := fstest.MapFS{"p.html": &fstest.MapFile{Data: []byte(`<!doctype html><title>d23</title><p>d23</p>`)}}
	before := dbusProxiesUnderThisProcess()
	for i := 1; i <= 3; i++ {
		w, err := NewWindow(Options{Title: "d23", Width: 200, Height: 120, Assets: assets, VirtualHost: "d23.invalid", StartPage: "/p.html"})
		if err != nil {
			t.Fatalf("NewWindow %d: %v", i, err)
		}
		open := dbusProxiesUnderThisProcess()
		if open <= before {
			_ = w.Close()
			t.Skipf("window %d started no D-Bus proxy (%d before, %d with it open): nothing here can show D23", i, before, open)
		}
		_ = w.Close()
		deadline := time.Now().Add(10 * time.Second)
		for dbusProxiesUnderThisProcess() > before {
			if time.Now().After(deadline) {
				t.Fatalf("window %d closed 10 s ago and %d D-Bus proxies are still running under the agent (%d before any window)",
					i, dbusProxiesUnderThisProcess(), before)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}
