//go:build windows

package main

// The drop delivery chain, end to end, for the half of it a test can
// reach.
//
// D-123 is right that no test here can drag a file: which window
// Windows hands a drop to is the part that had to be measured with the
// owner's own hand, and it is not reproducible from code. What *is*
// reproducible is everything after that — the HDROP being read, the
// paths reaching the callback, the callback reaching the window's loop,
// and the queue and the rendered list agreeing about what arrived.
//
// This test exists because that chain changed: callbacks no longer run
// on the window's message-loop thread (D-129), and the drop path goes
// through the same queue as everything else. A regression there would
// be silent — a window that looks like a drop target and does nothing —
// which is the exact failure this project has already shipped twice.

import (
	"context"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/veljaos/liro-bridge/internal/config"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

var (
	kernel32Test    = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalAlloc = kernel32Test.NewProc("GlobalAlloc")
	procGlobalLock  = kernel32Test.NewProc("GlobalLock")
	procGlobalUnlck = kernel32Test.NewProc("GlobalUnlock")
)

// dropFilesHeader mirrors DROPFILES, the header an HDROP's memory
// begins with. pFiles is the offset from the start of the block to the
// double-NUL-terminated list of paths; fWide says they are UTF-16.
type dropFilesHeader struct {
	pFiles uint32
	x, y   int32
	fNC    int32
	fWide  int32
}

// makeHDROP builds the handle the shell delivers with WM_DROPFILES,
// with the same layout DragQueryFileW reads.
func makeHDROP(t *testing.T, paths []string) uintptr {
	t.Helper()
	var chars []uint16
	for _, p := range paths {
		u, err := windows.UTF16FromString(p)
		if err != nil {
			t.Fatalf("encoding %q: %v", p, err)
		}
		chars = append(chars, u...)
	}
	chars = append(chars, 0) // the second NUL that ends the list

	headerSize := unsafe.Sizeof(dropFilesHeader{})
	const gmemMoveable, gmemZeroInit = 0x0002, 0x0040
	h, _, err := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit,
		headerSize+uintptr(len(chars)*2))
	if h == 0 {
		t.Fatalf("GlobalAlloc: %v", err)
	}
	base, _, _ := procGlobalLock.Call(h)
	if base == 0 {
		t.Fatal("GlobalLock returned nothing")
	}
	hdr := (*dropFilesHeader)(unsafe.Pointer(base))
	hdr.pFiles = uint32(headerSize)
	hdr.fWide = 1
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(base+headerSize)), len(chars)), chars)
	_, _, _ = procGlobalUnlck.Call(h)
	return h
}

// TestADropReachesTheListItLandsIn drops four documents on a real main
// window, then drops two of them again, and reads the result off the
// page — not out of the queue, which is the layer a rendering bug hides
// behind.
func TestADropReachesTheListItLandsIn(t *testing.T) {
	c := i18n.Load("sr-Latn")
	dir := t.TempDir()
	paths := []string{
		writeTestPDF(t, dir, "TEST 1.pdf", 10),
		writeTestPDF(t, dir, "TEST 2.pdf", 10),
		writeTestPDF(t, dir, "TEST 3.pdf", 10),
		writeTestPDF(t, dir, "TEST 4.pdf", 10),
	}

	m := &mainWindow{
		messages: make(chan ui.Message, 16),
		dropped:  make(chan []string, 16),
		closed:   make(chan struct{}),
		c:        c,
		locale:   "sr-Latn",
		cfg:      config.Default(),
	}
	win, err := ui.NewWindow(ui.Options{
		Title:          c.T("main.title"),
		Width:          mainWindowWidth,
		Height:         mainWindowHeight,
		Assets:         assetsFS,
		VirtualHost:    liroVirtualHost,
		StartPage:      "/pages/main.html",
		OnMessage:      func(msg ui.Message) { m.messages <- msg },
		OnFilesDropped: func(p []string) { m.dropped <- p },
		OnClosed:       func() { close(m.closed) },
	})
	if err != nil {
		t.Fatalf("NewWindow(main): %v", err)
	}
	defer func() { _ = win.Close() }()
	m.win = win
	if err := win.PostJSON(m.filesPayload("init")); err != nil {
		t.Fatalf("PostJSON(init): %v", err)
	}

	// The loop runs where it runs in production, on its own goroutine.
	// Its own context, not t.Context(): a test's context is cancelled
	// after the deferred functions have run, so waiting for the loop in
	// a defer that relies on it would wait forever.
	ctx, stopLoop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.loop(ctx); close(done) }()
	defer func() {
		stopLoop()
		<-done
	}()

	// The shell's own message, carrying the shell's own handle shape.
	const wmDropFiles = 0x0233
	drop := func(p ...string) {
		t.Helper()
		if r, _, err := procPostMessageT.Call(win.Handle(), wmDropFiles, makeHDROP(t, p), 0); r == 0 {
			t.Fatalf("PostMessage(WM_DROPFILES): %v", err)
		}
	}
	rows := func() float64 {
		return evalNumber(t, win, "document.querySelectorAll('#file-list .file-row').length")
	}
	waitForRows := func(want float64) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			if rows() == want {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("the list shows %v rows, want %v", rows(), want)
	}

	drop(paths...)
	waitForRows(4)

	// The same two again: refused, said out loud, and the list unchanged.
	drop(paths[3])
	drop(paths[2])
	waitForRows(4)
	if m.queue.Len() != 4 {
		t.Fatalf("the queue holds %d documents after four were dropped and two repeated", m.queue.Len())
	}
	notices := evalText(t, win, "document.getElementById('notices').textContent")
	if notices == "" {
		t.Fatal("a repeated document was refused without the window saying so")
	}
	list := evalText(t, win, "document.getElementById('file-list').textContent")
	for _, name := range []string{"TEST 1.pdf", "TEST 2.pdf", "TEST 3.pdf", "TEST 4.pdf"} {
		if strings.Count(list, name) != 1 {
			t.Fatalf("the list does not show %q exactly once: %q", name, list)
		}
	}
}
