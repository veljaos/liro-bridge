//go:build linux

package ui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/chooser"
)

// self is this test binary, which TestMain turns into a fake helper when
// it is started as one.
func self(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

const testBus = "unix:path=/nonexistent/liro-test-bus"

// What the helper is given is D-409's constraint, read from inside the
// child: its command line is the binary and the subcommand, its environment
// is the session bus address and nothing else, its working directory is /,
// and the request arrives on stdin whole.
func TestTheHelperIsGivenNothingButTheBusAndItsRequest(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", testBus)
	// Something the parent has that the helper must not: the control that
	// this test could see an inherited variable if one were passed.
	t.Setenv("LIRO_TEST_SECRET", "must-not-cross")

	req := chooser.Request{
		Kind:          chooser.KindFolder,
		Title:         "report",
		InitialFolder: "/home/someone/Dokumenti",
		Parent:        "wayland:abc",
	}
	r, err := spawnChooser(self(t), req, nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var rep fakeReport
	if err := json.Unmarshal([]byte(r.Detail), &rep); err != nil {
		t.Fatalf("the fake helper's report did not decode: %v (%q)", err, r.Detail)
	}
	if want := []string{chooser.Subcommand}; !reflect.DeepEqual(rep.Args, want) {
		t.Errorf("command line = %q, want %q", rep.Args, want)
	}
	if want := []string{"DBUS_SESSION_BUS_ADDRESS=" + testBus}; !reflect.DeepEqual(rep.Env, want) {
		t.Errorf("environment = %q, want only %q", rep.Env, want)
	}
	if rep.Cwd != "/" {
		t.Errorf("working directory = %q, want /", rep.Cwd)
	}
	if !reflect.DeepEqual(rep.Request, req) {
		t.Errorf("request as received = %+v, want %+v", rep.Request, req)
	}
	if !reflect.DeepEqual(r.Paths, []string{"/chosen.pdf"}) {
		t.Errorf("paths = %q", r.Paths)
	}
}

// With no DBUS_SESSION_BUS_ADDRESS, the systemd user bus's socket, if there
// is one; with neither, the chooser fails rather than guessing.
func TestTheBusAddressFallsBackToTheRuntimeSocketOnly(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if _, err := sessionBusAddress(); err == nil {
		t.Error("an address was found with no variable and no socket")
	}
	if _, err := spawnChooser(self(t), chooser.Request{Kind: chooser.KindFiles, Title: "report"}, nil, time.Minute); err == nil {
		t.Error("a helper was run with no bus to give it")
	}
}

// A helper that never answers is killed at the parent's watchdog, and is
// gone afterwards — checked by its exact PID.
func TestAHelperThatNeverAnswersIsKilledAtTheWatchdog(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", testBus)
	pidFile := filepath.Join(t.TempDir(), "pid")
	_, err := spawnChooser(self(t), chooser.Request{Kind: chooser.KindFiles, Title: "hang:" + pidFile}, nil, 500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("err = %v, want the watchdog", err)
	}
	assertGone(t, pidFile)
}

// The window closing is a cancel: the helper is told by its stdin ending
// and answers.
func TestTheWindowClosingStopsTheHelper(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", testBus)
	closed := make(chan struct{})
	time.AfterFunc(200*time.Millisecond, func() { close(closed) })
	r, err := spawnChooser(self(t), chooser.Request{Kind: chooser.KindFiles, Title: "wait-stdin"}, closed, time.Minute)
	if err != nil || r.Outcome != chooser.OutcomeCancelled {
		t.Fatalf("result, err = %+v, %v; want a cancel", r, err)
	}
}

// A helper that does not stop when its window closes is killed after
// StopGrace — the process does not outlive its window (D-408).
func TestAHelperThatIgnoresTheWindowClosingIsKilled(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", testBus)
	pidFile := filepath.Join(t.TempDir(), "pid")
	closed := make(chan struct{})
	time.AfterFunc(200*time.Millisecond, func() { close(closed) })
	r, err := spawnChooser(self(t), chooser.Request{Kind: chooser.KindFiles, Title: "hang:" + pidFile}, closed, time.Minute)
	if err != nil || r.Outcome != chooser.OutcomeCancelled {
		t.Fatalf("result, err = %+v, %v; want a cancel", r, err)
	}
	assertGone(t, pidFile)
}

func TestAnAnswerThatIsNotAResultIsAnError(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", testBus)
	if _, err := spawnChooser(self(t), chooser.Request{Kind: chooser.KindFiles, Title: "garbage"}, nil, time.Minute); err == nil {
		t.Fatal("a line that is not a Result was accepted")
	}
}

// With no helper binary set — every test binary, and a program whose main
// did not set one — a chooser fails and says so, and nothing is spawned.
func TestNoHelperBinaryIsAFailureNotASpawn(t *testing.T) {
	if _, err := spawnChooser("", chooser.Request{Kind: chooser.KindFiles}, nil, time.Minute); err == nil {
		t.Fatal("a chooser ran with no helper binary")
	}
}

// assertGone reads the PID the fake helper wrote and checks that no such
// process exists: spawnChooser both ended and reaped it.
func assertGone(t *testing.T, pidFile string) {
	t.Helper()
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("the fake helper wrote no PID: %v", err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil || pid <= 0 {
		t.Fatalf("PID file holds %q", b)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Errorf("helper %d is still there after spawnChooser returned (kill 0: %v)", pid, err)
	}
}
