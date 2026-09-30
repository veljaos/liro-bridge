//go:build linux

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/chooser"
)

// TestOnlyTheChooserHelperKeepsItsDumpableFlag is the property the portal
// checks, read the way the portal reads it (D-408): whether the process's
// /proc/PID/root can be opened by another process of the same user. The
// installed binary, started as the chooser helper, must pass that check and
// still have a core limit of zero; a process that ran ForbidCoreDumps, as
// every other process of this binary does, must fail it — which is also
// the control that this instrument can see a refusal at all.
func TestOnlyTheChooserHelperKeepsItsDumpableFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := build(t, t.TempDir(), "liro-bridge", nil)

	helper := exec.Command(bin, chooser.Subcommand)
	helper.Env = []string{"DBUS_SESSION_BUS_ADDRESS=unix:path=/nonexistent/liro-test-bus"}
	helper.Dir = "/"
	helperPID := startWaiting(t, helper)

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	control := exec.Command(self)
	control.Env = append(os.Environ(), forbidCoreDumpsChild+"=1")
	controlPID := startWaiting(t, control)

	// The helper sets its limit and then waits for its request; the
	// control sets both and waits. Read each only once it has done what
	// it does first, so neither reading is of a process still starting.
	waitFor(t, "the helper's core limit to be zero", func() bool { return coreLimitZero(helperPID) })
	waitFor(t, "the control's core limit to be zero", func() bool { return coreLimitZero(controlPID) })

	if _, err := os.Readlink(fmt.Sprintf("/proc/%d/root", helperPID)); err != nil {
		t.Errorf("the helper's /proc/PID/root cannot be read (%v): the portal would refuse it as it refuses the agent", err)
	}
	waitFor(t, "the control to be refused", func() bool {
		_, err := os.Readlink(fmt.Sprintf("/proc/%d/root", controlPID))
		return errors.Is(err, os.ErrPermission)
	})
}

// startWaiting starts cmd with its stdin held open, so that it waits, and
// ends it at the test's end by its exact PID.
func startWaiting(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	})
	return cmd.Process.Pid
}

func coreLimitZero(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/limits", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Max core file size") {
			f := strings.Fields(strings.TrimPrefix(line, "Max core file size"))
			return len(f) >= 2 && f[0] == "0" && f[1] == "0"
		}
	}
	return false
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("waited 10 s for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// The helper is told apart by its whole command line and nothing less.
func TestTheHelperIsRecognisedByItsWholeCommandLineOnly(t *testing.T) {
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{"/usr/bin/liro-bridge", chooser.Subcommand}, true},
		{[]string{"/usr/bin/liro-bridge", chooser.Subcommand, "x"}, false},
		{[]string{"/usr/bin/liro-bridge", "open", chooser.Subcommand}, false},
		{[]string{"/usr/bin/liro-bridge", "tray"}, false},
		{[]string{"/usr/bin/liro-bridge"}, false},
		{[]string{chooser.Subcommand}, false},
	} {
		if got := chooserHelperRequested(c.args); got != c.want {
			t.Errorf("chooserHelperRequested(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}

// main's first statement dispatches the helper and exits, and
// ForbidCoreDumps comes after it: the process that skips the flag cannot
// fall through into anything else (D-410).
func TestMainDispatchesTheHelperFirstAndExits(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body []ast.Stmt
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "main" {
			body = fn.Body.List
		}
	}
	if len(body) < 2 {
		t.Fatal("main has fewer than two statements; this test cannot see what it checks")
	}
	first, ok := body[0].(*ast.IfStmt)
	if !ok || !calls(first.Cond, "chooserHelperRequested") {
		t.Fatal("main's first statement is not the chooser helper's dispatch")
	}
	if len(first.Body.List) != 1 || !calls(first.Body.List[0], "Exit") || !calls(first.Body.List[0], "runChooserHelper") {
		t.Error("the helper's dispatch is not os.Exit(runChooserHelper()) alone: the process that skipped the flag could fall through")
	}
	forbid := -1
	for i, s := range body {
		if calls(s, "ForbidCoreDumps") {
			forbid = i
			break
		}
	}
	if forbid < 1 {
		t.Errorf("ForbidCoreDumps is at statement %d of main; it must come after the dispatch and nowhere be skipped", forbid)
	}
}

// calls reports whether n contains a call to a function or method named name.
func calls(n ast.Node, name string) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			found = found || fn.Name == name
		case *ast.SelectorExpr:
			found = found || fn.Sel.Name == name
		}
		return true
	})
	return found
}
