//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/ui"
)

// The window layer has four recorded lifetime defects — D-099's
// intermittent hang, D-101's two (a Go object handed to WebView2 as a
// bare address, and a controller released twice), D-114's drop target,
// D-129's window created underneath its owner — and every one of them
// surfaced from *opening and closing* windows, not from what was drawn
// in them. This is the measurement that would show the next one: open
// each window a hundred times, close it a hundred times, and watch the
// process's own handle, GDI, USER and thread counts across the run.
//
// It is off by default. Seven hundred window creations is roughly six
// minutes, which is not a thing to add to every `go test ./...`; set
//
//	LIRO_WINDOW_CYCLES=100
//
// to run it. Any positive number works, and a small one is a useful
// smoke test.
//
// The counters are the process's own. runtime.MemStats cannot see a
// leaked HWND, an HDC or a WebView2 controller, and those are exactly
// what this layer leaks when it leaks anything.

// windowCycles reads how many open/close cycles to run per window, or
// zero when the test should skip.
func windowCycles(t *testing.T) int {
	t.Helper()
	s := os.Getenv("LIRO_WINDOW_CYCLES")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		t.Fatalf("LIRO_WINDOW_CYCLES=%q: want a positive integer", s)
	}
	return n
}

// lifetimeSample is what one reading of the process's resources looks
// like. Handles, GDI and USER objects come from Windows; the rest from
// the Go runtime.
type lifetimeSample struct {
	cycle      int
	handles    uint32
	gdi        uint32
	user       uint32
	threads    int
	goroutines int
	heapMB     float64
}

func takeLifetimeSample(cycle int) lifetimeSample {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return lifetimeSample{
		cycle:      cycle,
		handles:    processHandleCount(),
		gdi:        processGDIObjects(),
		user:       processUserObjects(),
		threads:    processThreadCount(),
		goroutines: runtime.NumGoroutine(),
		heapMB:     float64(ms.HeapAlloc) / (1 << 20),
	}
}

func (s lifetimeSample) String() string {
	return fmt.Sprintf("cycle %4d  handles %5d  gdi %4d  user %4d  threads %4d  goroutines %4d  heap %6.2f MB",
		s.cycle, s.handles, s.gdi, s.user, s.threads, s.goroutines, s.heapMB)
}

// windowUnderTest names one window and how to open it, at the size and
// start page the product itself uses.
type windowUnderTest struct {
	name string
	opts func(messages chan ui.Message) ui.Options
}

func lifetimeWindows() []windowUnderTest {
	c := i18n.Load("sr-Latn")
	base := func(title, page string, w, h int) func(chan ui.Message) ui.Options {
		return func(messages chan ui.Message) ui.Options {
			return ui.Options{
				Title:       title,
				Width:       w,
				Height:      h,
				Assets:      assetsFS,
				VirtualHost: liroVirtualHost,
				StartPage:   page,
				OnMessage:   func(m ui.Message) { messages <- m },
				OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
			}
		}
	}
	return []windowUnderTest{
		{"main (documents step)", base(c.T("main.title"), "/pages/main.html", stepDocumentsWidth, stepDocumentsHeight)},
		{"consent (certificate step)", base(c.T("consent.window_title"), "/pages/consent.html", stepCertificateWidth, stepCertificateHeight)},
		{"method (stamp step)", base(c.T("stampwindow.title"), "/pages/stamp.html", stampWindowWidth, stepMethodHeight)},
		{"stamp picker (placement)", base(c.T("place.title"), "/pages/place.html", placeWindowWidth, placeWindowHeight)},
		{"settings", base(c.T("settings.window_title"), "/pages/settings.html", 520, 880)},
		{"certificates", base(c.T("certswindow.title"), "/pages/certificates.html", 460, 640)},
		{"audit log", base(c.T("auditwindow.title"), "/pages/auditlog.html", 460, 520)},
	}
}

// TestEveryWindowSurvivesAHundredOpenAndCloseCycles is the endurance
// measurement FTEST §7 asks for, reshaped around what the owner said:
// the tray is not left running for hours, so what matters is not a long
// idle but many short lives.
func TestEveryWindowSurvivesAHundredOpenAndCloseCycles(t *testing.T) {
	cycles := windowCycles(t)
	if cycles == 0 {
		t.Skip("set LIRO_WINDOW_CYCLES=100 to run the window lifetime measurement")
	}

	// Give the layer one window before the baseline, so the baseline is
	// a warm process rather than a cold one: the first WebView2
	// environment in a process costs handles and threads that are not a
	// per-cycle cost and would otherwise be counted as one.
	warmUp(t)

	for _, w := range lifetimeWindows() {
		t.Run(w.name, func(t *testing.T) {
			runtime.GC()
			before := takeLifetimeSample(0)
			t.Log("before ", before)

			var worst time.Duration
			var total time.Duration
			for i := 1; i <= cycles; i++ {
				start := time.Now()
				messages := make(chan ui.Message, 8)
				win, err := ui.NewWindow(w.opts(messages))
				if err != nil {
					t.Fatalf("cycle %d: NewWindow: %v", i, err)
				}
				if err := win.Close(); err != nil {
					t.Fatalf("cycle %d: Close: %v", i, err)
				}
				d := time.Since(start)
				total += d
				if d > worst {
					worst = d
				}
				if i%25 == 0 || i == cycles {
					t.Log("       ", takeLifetimeSample(i))
				}
			}

			runtime.GC()
			// A closed window's WebView2 browser process group outlives
			// the window by a moment; the handles it holds come back
			// when it goes. Give it the same grace the product does
			// before reading the counters that decide whether anything
			// leaked.
			time.Sleep(2 * time.Second)
			runtime.GC()
			after := takeLifetimeSample(cycles)
			t.Log("after  ", after)
			t.Logf("        %d cycles, mean %s per open+close, worst %s",
				cycles, (total / time.Duration(cycles)).Round(time.Millisecond), worst.Round(time.Millisecond))

			// What counts as a leak: a per-cycle cost. A fixed cost
			// paid once — a new thread pool, a cached environment — is
			// not one, so the budget is a small constant plus a
			// fraction of a handle per cycle rather than zero growth.
			checkNoPerCycleGrowth(t, "handles", int(before.handles), int(after.handles), cycles, 64)
			checkNoPerCycleGrowth(t, "GDI objects", int(before.gdi), int(after.gdi), cycles, 16)
			checkNoPerCycleGrowth(t, "USER objects", int(before.user), int(after.user), cycles, 16)
			checkNoPerCycleGrowth(t, "threads", before.threads, after.threads, cycles, 16)
			checkNoPerCycleGrowth(t, "goroutines", before.goroutines, after.goroutines, cycles, 8)
		})
	}
}

// checkNoPerCycleGrowth fails when growth is proportional to the number
// of cycles rather than bounded by a constant. One handle per window
// that is never given back is a tray process that dies in an afternoon;
// sixty-four handles once is a thread pool.
func checkNoPerCycleGrowth(t *testing.T, what string, before, after, cycles, constantBudget int) {
	t.Helper()
	growth := after - before
	if growth <= constantBudget {
		return
	}
	perCycle := float64(growth) / float64(cycles)
	t.Errorf("%s grew by %d across %d open/close cycles (%.2f per cycle, budget %d fixed): "+
		"growth this size is per-cycle, which is a leak",
		what, growth, cycles, perCycle, constantBudget)
}

// warmUp opens and closes one window so the process has paid the
// one-time costs of the window layer before anything is counted.
func warmUp(t *testing.T) {
	t.Helper()
	messages := make(chan ui.Message, 8)
	win, err := ui.NewWindow(ui.Options{
		Title:       "warm-up",
		Width:       400,
		Height:      300,
		Assets:      assetsFS,
		VirtualHost: liroVirtualHost,
		StartPage:   "/pages/settings.html",
		OnMessage:   func(m ui.Message) { messages <- m },
		OnClosed:    func() { messages <- ui.Message{Type: ui.MessageTypeCancel} },
	})
	if err != nil {
		t.Fatalf("warm-up NewWindow: %v", err)
	}
	if err := win.Close(); err != nil {
		t.Fatalf("warm-up Close: %v", err)
	}
	time.Sleep(2 * time.Second)
}
