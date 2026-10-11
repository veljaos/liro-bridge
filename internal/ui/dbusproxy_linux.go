//go:build linux

package ui

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// D23: every window left a bwrap → bwrap → xdg-dbus-proxy chain behind,
// for the life of the agent ([[D-460]]).
//
// WebKitGTK starts one xdg-dbus-proxy for every sandboxed web process,
// under bubblewrap, and keeps the read end of a pipe whose write end the
// proxy holds; the proxy exits when that read end is closed. The read end
// belongs to the web process's ProcessLauncher, which lives as long as
// its WebProcessProxy, which lives as long as the page's WebPageProxy,
// which lives until the WebKitWebView is finalized (WebKit 2.52.6,
// XDGDBusProxy.cpp, AuxiliaryProcessProxy.cpp, read at the tag). And the
// view is never finalized: the binding keeps a toggle reference on every
// object it has wrapped, and on this build no box it made was ever
// finalized — not a window's view, not a bare view, not a hundred bare
// labels, through repeated forced collections (measured, D-460). Ending
// the web process (D-355) ends the web process; it does not end its
// launcher, so the proxy stays.
//
// So the agent ends what WebKit no longer needs: once a window's web
// process has gone, a proxy chain with no web process left to serve is
// found, and this process's end of its pipe is replaced with /dev/null —
// the number stays valid, so WebKit's own eventual close of it closes
// /dev/null and nothing else, and the proxy sees its pipe close and
// exits, as it does when the agent exits. Measured under Xvfb on
// WebKitGTK 2.52.6: the chain gone within a second of the release, and
// the next window opened and drawn as before.

// procInfo is the part of /proc/PID/stat this needs.
type procInfo struct {
	pid, ppid int
	comm      string
	start     uint64 // starttime, clock ticks since boot
}

func readProcs() map[int]procInfo {
	out := map[int]procInfo{}
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		p, ok := parseStat(pid, string(b))
		if ok {
			out[pid] = p
		}
	}
	return out
}

// parseStat reads comm, ppid and starttime out of a /proc/PID/stat line.
// comm is between the first '(' and the last ')' because it may itself
// contain both.
func parseStat(pid int, s string) (procInfo, bool) {
	open, end := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || end < open || end+2 > len(s) {
		return procInfo{}, false
	}
	fields := strings.Fields(s[end+2:])
	// After comm: state(0) ppid(1) ... starttime is field 22 of the whole
	// line, so index 19 here.
	if len(fields) < 20 {
		return procInfo{}, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return procInfo{}, false
	}
	start, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return procInfo{}, false
	}
	return procInfo{pid: pid, ppid: ppid, comm: s[open+1 : end], start: start}, true
}

// sandboxChain is one bubblewrap tree started directly by this process.
type sandboxChain struct {
	root  procInfo
	proxy int  // the xdg-dbus-proxy inside it, or 0
	web   bool // whether a WebKitWebProcess is inside it
}

// sandboxChains classifies every bwrap that is a direct child of self.
func sandboxChains(self int, procs map[int]procInfo) []sandboxChain {
	children := map[int][]int{}
	for pid, p := range procs {
		children[p.ppid] = append(children[p.ppid], pid)
	}
	var out []sandboxChain
	for _, c := range children[self] {
		root := procs[c]
		if root.comm != "bwrap" {
			continue
		}
		ch := sandboxChain{root: root}
		stack := append([]int(nil), children[c]...)
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch procs[p].comm {
			case "xdg-dbus-proxy":
				ch.proxy = p
			case "WebKitWebProces": // comm is cut at 15 bytes
				ch.web = true
			}
			stack = append(stack, children[p]...)
		}
		out = append(out, ch)
	}
	return out
}

// orphanedProxies is the pairing: WebKit starts a web process's proxy
// immediately before the web process itself (bubblewrapSpawn calls
// XDGDBusProxy::launch first), so each live web process's proxy is the
// latest proxy chain started no later than it, not already taken. Every
// proxy left over serves a web process that has gone. Pure, so it can be
// tested without a sandbox.
func orphanedProxies(chains []sandboxChain) []int {
	var proxies, webs []sandboxChain
	for _, c := range chains {
		switch {
		case c.proxy != 0:
			proxies = append(proxies, c)
		case c.web:
			webs = append(webs, c)
		}
	}
	sort.Slice(proxies, func(i, j int) bool { return proxies[i].root.start < proxies[j].root.start })
	sort.Slice(webs, func(i, j int) bool { return webs[i].root.start < webs[j].root.start })
	taken := make([]bool, len(proxies))
	for _, w := range webs {
		best := -1
		for i, p := range proxies {
			if taken[i] || p.root.start > w.root.start {
				continue
			}
			best = i
		}
		if best >= 0 {
			taken[best] = true
		}
	}
	var out []int
	for i, p := range proxies {
		if !taken[i] {
			out = append(out, p.proxy)
		}
	}
	return out
}

// pipeFDs maps each pipe a process holds to the descriptors it holds it on.
func pipeFDs(pid int) map[string][]int {
	out := map[string][]int{}
	dir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	for _, e := range ents {
		link, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil || !strings.HasPrefix(link, "pipe:") {
			continue
		}
		fd, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		out[link] = append(out[link], fd)
	}
	return out
}

// releaseOrphanedDBusProxies ends every proxy chain whose web process has
// gone, and reports how many it released. On the UI thread only: WebKit
// closes its descriptors there too, so nothing can close the descriptor
// this reads and reuse its number before the replacement lands on it.
func releaseOrphanedDBusProxies() int {
	self := os.Getpid()
	orphans := orphanedProxies(sandboxChains(self, readProcs()))
	if len(orphans) == 0 {
		return 0
	}
	mine := pipeFDs(self)
	// A pipe every proxy shares with us — the standard output and error
	// they inherited — is not a proxy's own and is never touched.
	shared := map[string]int{}
	allProxies := map[int]map[string][]int{}
	for _, c := range sandboxChains(self, readProcs()) {
		if c.proxy == 0 {
			continue
		}
		fds := pipeFDs(c.proxy)
		allProxies[c.proxy] = fds
		for link := range fds {
			shared[link]++
		}
	}
	released := 0
	for _, proxy := range orphans {
		for link := range allProxies[proxy] {
			ours := mine[link]
			if shared[link] != 1 || len(ours) != 1 || ours[0] <= 2 {
				continue
			}
			if replaceWithDevNull(ours[0]) {
				released++
				slog.Info("ui: ended the D-Bus proxy of a web process that has gone", "proxy", proxy)
			}
		}
	}
	return released
}

func replaceWithDevNull(fd int) bool {
	null, err := syscall.Open("/dev/null", syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer func() { _ = syscall.Close(null) }()
	return syscall.Dup3(null, fd, syscall.O_CLOEXEC) == nil
}

// releaseProxiesAfterClose waits, off the UI thread, for the closed
// window's web process to be gone — TerminateWebProcess asks, and the
// process takes a moment — and then releases what it left. Bounded: a
// web process that has not gone in five seconds is left to the next
// window's close to find.
func releaseProxiesAfterClose() {
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if len(orphanedProxies(sandboxChains(os.Getpid(), readProcs()))) > 0 {
				_ = theUIThread.do(func() { releaseOrphanedDBusProxies() })
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}
