//go:build windows

package ui

import (
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestOnlyWindowsWithNoTargetYetAreRegisteredAgain(t *testing.T) {
	registered := []*dropTarget{{hwnd: 0x10}, {hwnd: 0x20}, nil}
	current := []uintptr{0x10, 0x20, 0x30, 0x40, 0x30}

	got := newDropTargetCandidates(registered, current)

	want := []uintptr{0x30, 0x40}
	if len(got) != len(want) {
		t.Fatalf("newDropTargetCandidates = %#x, want %#x", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("newDropTargetCandidates = %#x, want %#x", got, want)
		}
	}
}

func TestAWindowAlreadyRegisteredIsNeverRegisteredTwice(t *testing.T) {
	registered := []*dropTarget{{hwnd: 0xAA}, {hwnd: 0xBB}}
	if got := newDropTargetCandidates(registered, []uintptr{0xAA, 0xBB}); len(got) != 0 {
		t.Fatalf("newDropTargetCandidates = %#x, want none", got)
	}
}

func TestWhetherADropWouldReachThisWindow(t *testing.T) {
	const ourPID = uint32(4242)
	ours := []uintptr{0x100, 0x200, 0x300}

	cases := []struct {
		name     string
		under    uintptr
		underPID uint32
		want     coverState
	}{
		{"the frame itself", 0x100, ourPID, coverNone},
		// Chrome_RenderWidgetHostHWND, which every measured drop has
		// arrived at, is owned by the msedgewebview2 process — so a
		// window of ours carrying somebody else's process id is the
		// normal case and must not read as covered.
		{"a browser window underneath it, owned by another process", 0x300, 9999, coverNone},
		{"a second window of this same agent", 0x900, ourPID, coverByOwnProcess},
		{"somebody else's window", 0x900, 777, coverByOtherProcess},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyCover(c.under, c.underPID, ours, ourPID); got != c.want {
				t.Fatalf("classifyCover = %v, want %v", got, c.want)
			}
		})
	}
}

// The one that needed the watch to exist: a window the browser creates
// after the registration snapshot gets a drop target anyway.
//
// It stands in for the compositor's own window — measured to carry no
// target on a fresh window and one after any navigation — using a child
// window this test creates itself, because a test cannot make WebView2
// produce its compositor window on demand. The code path and the timer
// are the same ones.
//
// Two things about how it is written, both of which cost a run to
// learn:
//
// The child is created ON THE UI THREAD, through invoke. Created from
// the test's own goroutine it would belong to that thread, and
// destroying the parent then has to destroy a child owned by a thread
// that is itself blocked inside w.Close() waiting for the UI thread —
// which deadlocks the one UI thread this process has and hangs every
// window test after it. Measured: a 25-minute timeout with the UI
// thread parked in wndProc's WM_CLOSE. Nothing in the product does
// this; WebView2's own children belong to the thread that hosts them.
//
// The target is read from outside with GetPropW rather than from
// w.dropTargets, which belongs to the UI thread and would be a data
// race under -race, which is where this suite actually runs.
func TestAWindowThatAppearsAfterTheSnapshotStillGetsADropTarget(t *testing.T) {
	w, err := NewWindow(Options{
		Title:          "liro-bridge drop watch test",
		Width:          300,
		Height:         200,
		OnFilesDropped: func([]string) {},
	})
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	win, ok := w.(*window)
	if !ok {
		t.Fatalf("NewWindow returned a %T", w)
	}

	// No cleanup destroying it: the parent takes its children with it,
	// and destroying it from here is the deadlock described above.
	made := make(chan uintptr, 1)
	win.invoke(func() { made <- createChildWindowHere(win.hwnd) })
	var child uintptr
	select {
	case child = <-made:
	case <-time.After(30 * time.Second):
		t.Fatal("the UI thread never ran the closure that creates the child window")
	}
	if child == 0 {
		t.Fatal("CreateWindowExW for the test child returned 0")
	}

	// Before the watch has been round: the snapshot could not have
	// covered a window that did not exist when it was taken. This is
	// the half that makes the assertion below mean something.
	if hasDropTarget(child) {
		t.Fatal("a window created after the registration already had a drop target; the test proves nothing")
	}

	// Observing the property, with a ceiling that exists only to turn a
	// watch that never runs into a failure instead of a hang (D-201).
	deadline := time.Now().Add(30 * time.Second)
	for !hasDropTarget(child) {
		if time.Now().After(deadline) {
			t.Fatal("a window created after the registration never got a drop target: the drop watch did not pick it up")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

var (
	procGetPropW    = user32DLL.NewProc("GetPropW")
	oleDropTargetNm = windows.StringToUTF16Ptr("OleDropTargetInterface")
	staticClassName = windows.StringToUTF16Ptr("STATIC")
	emptyWindowText = windows.StringToUTF16Ptr("")
)

// hasDropTarget asks the window itself, the way RegisterDragDrop
// records it and the way this project's own out-of-process probe reads
// it, rather than asking this package's bookkeeping.
func hasDropTarget(hwnd uintptr) bool {
	r, _, _ := procGetPropW.Call(hwnd, uintptr(unsafe.Pointer(oleDropTargetNm)))
	return r != 0
}

// createChildWindowHere must be called on the thread that owns parent.
func createChildWindowHere(parent uintptr) uintptr {
	const (
		wsChild   = 0x40000000
		wsVisible = 0x10000000
	)
	h, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(staticClassName)),
		uintptr(unsafe.Pointer(emptyWindowText)),
		wsChild|wsVisible,
		0, 0, 10, 10,
		parent,
		0,
		0, // hInstance may be 0 for a system class such as STATIC
		0,
	)
	return h
}
