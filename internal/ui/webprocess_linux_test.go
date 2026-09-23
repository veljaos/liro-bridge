//go:build linux

package ui

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// webProcessDescendants counts this process's descendants that are WebKit
// web processes. bwrap sits between them and us, so it walks ancestry
// rather than reading one level of children.
func webProcessDescendants(t *testing.T) int {
	t.Helper()
	parent := map[int]int{}
	cmdline := map[int]string{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		// The command name is in parentheses and may contain spaces; the
		// fields after the last ')' are fixed.
		rest := string(stat[strings.LastIndexByte(string(stat), ')')+2:])
		fields := strings.Fields(rest)
		if len(fields) < 2 {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		parent[pid] = ppid
		cl, _ := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		cmdline[pid] = string(cl)
	}
	self := os.Getpid()
	n := 0
	for pid, cl := range cmdline {
		if !strings.HasPrefix(cl, "/usr/lib") || !strings.Contains(cl, "WebKitWebProcess") {
			continue
		}
		for p := parent[pid]; p > 1; p = parent[p] {
			if p == self {
				n++
				break
			}
		}
	}
	return n
}

// TestAClosedWindowTakesItsWebProcessWithIt is D-355's leak: every window the
// agent opened left a WebKit web process running after it closed. Before the
// fix this test's own diagnostic found the process alive after Close and
// after two forced garbage collections; the assertion is deliberately made
// without a collection, because an idle agent may not run one for hours.
//
// It needs a web process to start at all, which on Ubuntu 24.04 means the
// test binary must be at a path an AppArmor profile names (D-324); elsewhere
// it skips with newTestWindow's reason.
func TestAClosedWindowTakesItsWebProcessWithIt(t *testing.T) {
	before := webProcessDescendants(t)
	w := newTestWindow(t, Options{})
	during := webProcessDescendants(t)
	if during <= before {
		t.Fatal("opening a window started no web process this test can see, so it cannot measure one ending")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	end := time.Now().Add(5 * time.Second)
	for time.Now().Before(end) {
		if webProcessDescendants(t) <= before {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("5 s after Close, %d web process(es) opened for this window are still running", webProcessDescendants(t)-before)
}
