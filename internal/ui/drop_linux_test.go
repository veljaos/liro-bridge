//go:build linux

package ui

import (
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/veljaos/liro-bridge/internal/chooser"
)

// A drop is read as text/uri-list (D-410), so what reaches the window is
// what this parser and chooser.LocalPaths make of the text a file manager
// writes: one URI per line, CRLF, comments and blank lines ignored,
// percent-encoding undone, and anything that is not a file on this disk
// left out — a location the desktop can browse is not a path a caller can
// open, and an empty string must never reach one as a path. No display
// needed.
func TestADroppedURIListBecomesLocalPathsAndNothingElse(t *testing.T) {
	// The shape D-409 read from Files, extended with what RFC 2483 allows.
	text := "# dragged from Files\r\n" +
		"file:///home/someone/ugovor.pdf\r\n" +
		"\r\n" +
		"https://example.invalid/faktura.pdf\r\n" +
		"file:///tmp/%C4%8Cita%C4%8D%20kartica/ra%C4%8Dun.pdf\r\n" +
		"file://localhost/home/someone/folder\n" +
		"file://other-host/share/x.pdf\r\n" +
		"file://\r\n"
	uris := parseURIList(text)
	if len(uris) != 6 {
		t.Fatalf("parseURIList kept %d lines, want 6 (no comment, no blank): %q", len(uris), uris)
	}
	got, skipped := chooser.LocalPaths(uris)
	want := []string{"/home/someone/ugovor.pdf", "/tmp/Čitač kartica/račun.pdf", "/home/someone/folder"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
	if skipped != 3 {
		t.Errorf("skipped = %d, want 3 (https, another host, an empty file URI)", skipped)
	}
}

func TestAnEmptyDropIsNoPaths(t *testing.T) {
	for _, text := range []string{"", "\r\n", "# only a comment\r\n"} {
		if got, _ := chooser.LocalPaths(parseURIList(text)); len(got) != 0 {
			t.Errorf("%q gave %q, want nothing", text, got)
		}
	}
}

const dropTargetChildEnv = "LIRO_TEST_DROP_TARGET_CHILD"

// dropTargetAnswer is the line the child prints; the parent believes
// nothing else, so a child that died before reaching it cannot pass.
const dropTargetAnswer = "drop target accepts text/uri-list after the collector: "

// The window's drop target must still hold its formats after Go's
// collector has run (D-412). dev.13 gave gtk_drop_target_async_new formats
// the collector then freed, and the agent died on the first drag, 18
// minutes after its window opened: no test drags (D-094), and none forced
// the collector. This one does, in a child process:
//
//   - the target is built by newDropTarget, the window's own function, so
//     the formats' last Go reference ends when it returns;
//   - three collections are forced, each waited for until a finalizer of
//     the child's own has run, so the formats' finalizer has had its turn;
//   - then the target is asked whether it accepts text/uri-list.
//
// MALLOC_PERTURB_ overwrites freed memory, so freed formats read as
// garbage rather than as what they held, and glibc's per-thread cache is
// switched off for the child because a free into it is not overwritten.
// Both read in D-413, with dev.13's constructor put back: with the cache
// off the child died reading a pointer of 0xa5a5…, the perturbation byte;
// with it on it died too, but the formats' count still read 1 and the
// pointer was a different value each run — most likely the cache's own
// bookkeeping: red by luck of the layout, not by the overwrite.
//
// A child, because what it looks for is a crash in C, which would take
// every other test in the binary with it.
func TestTheDropTargetKeepsItsFormatsAfterTheCollectorRuns(t *testing.T) {
	if os.Getenv(dropTargetChildEnv) == "1" {
		os.Exit(dropTargetChild())
	}
	cmd := exec.Command(os.Args[0],
		"-test.run=^TestTheDropTargetKeepsItsFormatsAfterTheCollectorRuns$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(), dropTargetChildEnv+"=1",
		"MALLOC_PERTURB_=165", "GLIBC_TUNABLES=glibc.malloc.tcache_count=0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 3 {
		t.Skip("no display for GTK")
	}
	if err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), dropTargetAnswer+"true") {
		t.Fatalf("the child did not answer true:\n%s", out)
	}
	t.Logf("child:\n%s", out)
}

// dropTargetChild exits 0 if the target still accepts text/uri-list, 1 if
// it does not, 3 with no display, 4 if the collector could not be waited
// for.
func dropTargetChild() int {
	if err := theUIThread.start(); err != nil {
		return 3
	}
	var target *gtk.DropTargetAsync
	if err := theUIThread.do(func() { target = newDropTarget(func([]string) {}) }); err != nil {
		return 3
	}
	if !collectAndFinalize(3) {
		fmt.Println("a forced collection's finalizer did not run within 5 s")
		return 4
	}
	var accepted bool
	_ = theUIThread.do(func() {
		// This wrapper's finalizer would drop one of GTK's references
		// (newDropTarget's comment). The child exits before another
		// collection, and nothing reads the target after this.
		formats := target.Formats()
		accepted = formats != nil && formats.ContainMIMEType(uriListMIME)
		runtime.KeepAlive(formats)
	})
	runtime.KeepAlive(target)
	fmt.Println(dropTargetAnswer + fmt.Sprint(accepted))
	if !accepted {
		return 1
	}
	return 0
}

// finalizerSentinel is large enough to stay out of the tiny allocator,
// whose objects' finalizers may never run.
type finalizerSentinel struct{ _ [64]byte }

// collectAndFinalize forces n collections, waiting after each until a
// finalizer set before it has run. Finalizers run one at a time on one
// goroutine, batch after batch, so once the last sentinel's has run,
// everything queued by the earlier collections has run too.
func collectAndFinalize(n int) bool {
	for range n {
		done := make(chan struct{})
		runtime.SetFinalizer(new(finalizerSentinel), func(*finalizerSentinel) { close(done) })
		runtime.GC()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			return false
		}
	}
	return true
}
