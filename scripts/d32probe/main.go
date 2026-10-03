//go:build linux

// Command d32probe separates the things that happen at a process's first
// window, for open-items D32: on Fedora 44 an agent's first web window
// paints and every later one is white, and the first window is also where
// the process initialises GTK, opens the display, is refused by the Settings
// portal, sets up GDK's renderer, registers liro:// on WebKit's default
// context and creates the context's first view — and it is the only window
// not preceded by a TerminateWebProcess (D-420).
//
// It opens windows through internal/ui's NewWindow and Close — the agent's
// own host, not a GTK window built to resemble it (D-350, D-352) — and the
// native PIN dialog through CollectPIN. Whether a window painted is read by
// a person: after each window opens, the probe asks in the terminal, and the
// person types what they see. Nothing here reads the screen (D-420: an
// accessibility state is not a reading of it).
//
//	go build -o ~/d32probe ./scripts/d32probe
//	~/d32probe -seq "w w"
//
// The sequence is steps separated by spaces, run in order:
//
//	w  open a web window, ask, then close it with Close (TerminateWebProcess, destroy)
//	o  open a web window, ask, and leave it open
//	c  close every window o left open, oldest first
//	p  show the native PIN dialog (no web view); the person presses Cancel
//
// It makes itself non-dumpable as the agent does (platform.ForbidCoreDumps),
// so the Settings portal refuses it as it refuses the agent (B29); -dumpable
// leaves the flag set, and the portal should then answer. Environment
// variables a run varies (GSK_RENDERER and the like) are given on the command
// line and recorded in the report.
//
// **On Ubuntu this binary is not /usr/bin/liro-bridge**, so the package's
// AppArmor profile does not cover it, and WebKit's sandbox fails to start its
// web process (D-324). It runs there only with the sandbox off, which is a
// difference from the agent and is recorded in the report. Fedora has no such
// restriction.
//
// The report goes to ./d32probe-report-<UTC time>.txt as well as the terminal.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing/fstest"
	"time"

	"github.com/veljaos/liro-bridge/internal/platform"
	"github.com/veljaos/liro-bridge/internal/ui"
)

var report io.Writer = os.Stdout

func say(format string, args ...any) {
	fmt.Fprintf(report, time.Now().Format("15:04:05.000")+"  "+format+"\n", args...)
}

const page = `<!doctype html>
<meta charset="utf-8">
<style>
  html, body { margin: 0; height: 100%; }
  body { background: #1d4ed8; color: #fff; font: bold 40px sans-serif;
         display: flex; flex-direction: column; align-items: center; justify-content: center; }
  #clock { font-size: 64px; margin-top: 16px; }
</style>
<div>d32probe · RUN</div>
<div>window N</div>
<div id="clock"></div>
<script>
  function tick() { document.getElementById("clock").textContent = new Date().toLocaleTimeString(); }
  tick(); setInterval(tick, 1000);
</script>
`

func main() {
	seq := flag.String("seq", "w w", "steps: w (open, ask, close), o (open, ask, keep), c (close kept), p (PIN dialog)")
	dumpable := flag.Bool("dumpable", false, "do not clear the dumpable flag (the portal should then answer)")
	label := flag.String("label", "", "a name for this run, shown in each window and the report")
	flag.Parse()

	f, err := os.Create("d32probe-report-" + time.Now().UTC().Format("20060102-150405") + ".txt")
	if err == nil {
		report = io.MultiWriter(os.Stdout, f)
		defer func() { _ = f.Close() }()
	}

	say("d32probe, pid %d, run %q, sequence %q", os.Getpid(), *label, *seq)
	if *dumpable {
		say("dumpable flag left set (-dumpable): the Settings portal should answer this process")
	} else if err := platform.ForbidCoreDumps(); err != nil {
		say("ForbidCoreDumps failed: %v — the run is not the agent's condition", err)
	} else {
		say("non-dumpable, as the agent (platform.ForbidCoreDumps)")
	}
	for _, kv := range os.Environ() {
		for _, p := range []string{"GSK_", "GDK_", "WEBKIT_", "GTK_", "XDG_SESSION_TYPE=", "WAYLAND_DISPLAY=", "DISPLAY=", "LIBGL_", "MESA_"} {
			if strings.HasPrefix(kv, p) {
				say("env %s", kv)
			}
		}
	}

	in := bufio.NewReader(os.Stdin)
	var kept []ui.Window
	n := 0

	for i, step := range strings.Fields(*seq) {
		say("--- step %d: %s", i+1, step)
		switch step {
		case "w", "o":
			n++
			w, err := open(*label, n)
			if err != nil {
				say("window %d: NewWindow failed: %v", n, err)
				descendants()
				continue
			}
			say("window %d: NewWindow returned (the page's load finished)", n)
			descendants()
			ask(in, fmt.Sprintf("window %d", n))
			if step == "w" {
				closeOne(w, n)
			} else {
				kept = append(kept, w)
			}
		case "c":
			for j, w := range kept {
				closeOne(w, j+1)
			}
			kept = nil
		case "p":
			say("PIN dialog: opening — press its Cancel button; type nothing into it")
			dst := make([]byte, 64)
			_, ok, err := ui.CollectPIN(0, ui.PINPrompt{
				Title:   "d32probe",
				Heading: "d32probe — not a PIN",
				Subject: "Press Cancel. Type nothing here.",
				Label:   "Nothing",
				Hint:    "",
				OK:      "OK",
				Cancel:  "Cancel",
				Entered: "%d",
			}, 32, dst)
			say("PIN dialog: returned ok=%v err=%v", ok, err)
			ask(in, "the PIN dialog (as it was before you pressed Cancel)")
		default:
			say("unknown step %q, skipped", step)
		}
	}
	for j, w := range kept {
		closeOne(w, j+1)
	}
	say("--- done")
	descendants()
}

func open(label string, n int) (ui.Window, error) {
	body := strings.Replace(page, "RUN", htmlEscape(label), 1)
	body = strings.Replace(body, "window N", "window "+strconv.Itoa(n), 1)
	return ui.NewWindow(ui.Options{
		Title:       fmt.Sprintf("d32probe — window %d", n),
		Width:       560,
		Height:      420,
		Assets:      fstest.MapFS{"index.html": {Data: []byte(body)}},
		VirtualHost: "d32probe",
		StartPage:   "index.html",
		OnClosed:    func() { say("window %d: closed by the person (close-request)", n) },
	})
}

func closeOne(w ui.Window, n int) {
	err := w.Close()
	say("window %d: Close returned %v", n, err)
	time.Sleep(time.Second)
	descendants()
}

// ask records what the person saw. It does not interpret it: anything other
// than p or w is kept as written.
func ask(in *bufio.Reader, what string) {
	fmt.Printf("\n    Look at %s. Painted (p), white (w), or describe it, then Enter: ", what)
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)
	switch line {
	case "p":
		line = "painted"
	case "w":
		line = "white"
	}
	say("%s: the person read %q", what, line)
}

// descendants lists this process's descendants by name, so a run records
// which web, network and sandbox processes were alive at each step (D23).
func descendants() {
	parent := map[int]int{}
	name := map[int]string{}
	dirs, _ := filepath.Glob("/proc/[0-9]*")
	for _, d := range dirs {
		pid, err := strconv.Atoi(filepath.Base(d))
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(d + "/stat")
		if err != nil {
			continue
		}
		s := string(stat)
		lp, rp := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
		if lp < 0 || rp < lp {
			continue
		}
		fields := strings.Fields(s[rp+1:])
		if len(fields) < 2 {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		parent[pid] = ppid
		name[pid] = s[lp+1 : rp]
	}
	self := os.Getpid()
	var out []string
	for pid := range parent {
		for p := parent[pid]; p > 1; p = parent[p] {
			if p == self {
				out = append(out, fmt.Sprintf("%d %s (parent %d)", pid, name[pid], parent[pid]))
				break
			}
		}
	}
	sort.Strings(out)
	say("descendants: %d %v", len(out), out)
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
