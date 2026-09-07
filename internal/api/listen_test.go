package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestListenTakesTheFirstFreePortInTheRange(t *testing.T) {
	// Occupy the first two ports of a range so the third is the first
	// free one — which is what a second and a third user session on one
	// machine actually produce (SPEC §14.1).
	base := freePortRangeStart(t, 3)
	first, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, strconv.Itoa(base)))
	if err != nil {
		t.Skipf("could not occupy port %d: %v", base, err)
	}
	defer func() { _ = first.Close() }()
	second, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, strconv.Itoa(base+1)))
	if err != nil {
		t.Skipf("could not occupy port %d: %v", base+1, err)
	}
	defer func() { _ = second.Close() }()

	ln, port, err := Listen(base, base+2)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	if port != base+2 {
		t.Fatalf("took port %d, want %d — the first two are occupied", port, base+2)
	}
}

func TestListenBindsLoopbackAndNothingElse(t *testing.T) {
	ln, port, err := Listen(0, 0) // falls back to SPEC §14's own range
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	if port < DefaultPortRangeStart || port > DefaultPortRangeEnd {
		t.Fatalf("port %d is outside %d-%d", port, DefaultPortRangeStart, DefaultPortRangeEnd)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("the listener's address is %T, want *net.TCPAddr", ln.Addr())
	}
	if !addr.IP.IsLoopback() {
		t.Fatalf("bound %s, which is not loopback", addr.IP)
	}
	// Not merely "a loopback address": the specific one, because
	// 0.0.0.0 would also report a loopback connection to a client
	// connecting over 127.0.0.1 and would additionally be reachable
	// from the network.
	if addr.IP.String() != loopbackHost {
		t.Fatalf("bound %s, want %s", addr.IP, loopbackHost)
	}
}

func TestListenRefusesWhenEveryPortInTheRangeIsTaken(t *testing.T) {
	base := freePortRangeStart(t, 2)
	var held []net.Listener
	for p := base; p <= base+1; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, strconv.Itoa(p)))
		if err != nil {
			t.Skipf("could not occupy port %d: %v", p, err)
		}
		held = append(held, ln)
	}
	defer func() {
		for _, ln := range held {
			_ = ln.Close()
		}
	}()

	if _, _, err := Listen(base, base+1); err == nil {
		t.Fatal("Listen succeeded with every port in the range taken")
	}
}

// TestNothingInThisPackageBindsAnywhereButLoopback reads the package's
// own source rather than its behaviour.
//
// SPEC §6.1 does not say "listens on loopback by default"; it says
// binding to a non-loopback interface must be impossible. A test that
// only checks what Listen returns proves nothing about a second call
// somebody adds next year — so this one asserts the property that
// actually makes it impossible: there is exactly one place in the
// package that binds a socket, and the host it binds is the constant.
//
// This is the method D-025 used for "no PIN field anywhere" and D-158
// for "AllCodes is complete": when the property is about what is
// declared rather than about what one call returns, the syntax tree is
// what can see it.
func TestNothingInThisPackageBindsAnywhereButLoopback(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}

	binds := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "net" {
				return true
			}
			if sel.Sel.Name != "Listen" && sel.Sel.Name != "ListenPacket" {
				return true
			}
			binds++
			if name != "listen.go" {
				t.Errorf("%s binds a socket; only listen.go may", name)
				return true
			}
			if !mentionsLoopbackHost(call) {
				t.Errorf("%s:%d binds a socket without going through loopbackHost",
					name, fset.Position(call.Pos()).Line)
			}
			return true
		})
	}
	if binds != 1 {
		t.Fatalf("the package binds a socket in %d places, want exactly 1", binds)
	}
}

// mentionsLoopbackHost reports whether the call's arguments reach the
// loopbackHost constant somewhere.
func mentionsLoopbackHost(call *ast.CallExpr) bool {
	found := false
	for _, arg := range call.Args {
		ast.Inspect(arg, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name == "loopbackHost" {
				found = true
			}
			return true
		})
	}
	return found
}

// freePortRangeStart finds a run of n consecutive free ports, so a test
// that occupies some of them is not fighting whatever else is on this
// machine — including another agent, which is exactly the thing the
// port range exists for.
func freePortRangeStart(t *testing.T, n int) int {
	t.Helper()
	for base := 41000; base < 45000; base += n {
		free := true
		var held []net.Listener
		for p := base; p < base+n; p++ {
			ln, err := net.Listen("tcp", net.JoinHostPort(loopbackHost, strconv.Itoa(p)))
			if err != nil {
				free = false
				break
			}
			held = append(held, ln)
		}
		for _, ln := range held {
			_ = ln.Close()
		}
		if free {
			return base
		}
	}
	t.Skipf("no run of %d free ports found", n)
	return 0
}
