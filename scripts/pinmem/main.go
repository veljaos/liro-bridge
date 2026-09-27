//go:build linux

// Command pinmem measures where what a person types into this program's own
// PIN dialog ends up in the dialog's process (open-items B1 and B2; D-380).
//
// It opens the real dialog — internal/ui's CollectPIN, the one the agent
// shows — not a GTK window built to resemble it. That is the whole difference
// from the probe of D-350, which measured GTK driven by code and was read as
// though it had measured this program's dialog; D-351 and D-352 had to
// unpick that. And it lives in the repository, because the first pinmem lived
// in /tmp and went with a reboot, so B2 cost its rebuild twice (D-380).
//
//	go build -o ~/pinmem ./scripts/pinmem && ~/pinmem
//
// What it does, in order:
//
//  1. Makes a random 20-character needle, in a page of its own, and shows it.
//     The person types THAT — never a PIN. Random, so a match is not a
//     coincidence (D-352: a string a person chooses collides with static
//     tables); shown by the program, so a baseline can be taken with the
//     same needle before anything is typed, which D-352's interactive mode
//     could not do.
//  2. Scans before the dialog, and with the dialog open and empty: the
//     baselines. Both should be zero; if not, nothing after them means
//     anything.
//  3. Scans every two seconds while the dialog is up, printing the count and
//     VmLck. The person types the needle, waits until the count has settled,
//     and only then presses OK: CollectPIN clears its entry the moment it
//     copies out, so the widget's own copy can only be seen while it is up.
//  4. After CollectPIN returns: checks that what came back is the needle
//     byte for byte — a keyboard layout that typed something else would make
//     every scan read zero and look clean — then scans with the returned
//     copy still held, wipes it, and scans once more.
//  5. Reports which input-method modules are mapped, and whether IBus's own
//     bus carried key events while the dialog was up (a count, never the
//     keys): a keystroke that reaches another process has crossed a process
//     boundary SPEC §6.5.1 clause 2 does not grant.
//
// The needle, the scan buffer and the returned copy live in mmap'd pages the
// scan skips; the instrument must not find itself (D-350). The report is
// written to ./pinmem-report.txt as well as printed.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/veljaos/liro-bridge/internal/ui"
)

const needleLen = 20

// alphabet has no y or z (they swap on a Serbian Latin layout), and no
// characters that read alike (0/o, 1/l/i).
const alphabet = "abcdefghjkmnpqrstuvwx23456789"

var report io.Writer = os.Stdout

func say(format string, a ...any) { _, _ = fmt.Fprintf(report, format+"\n", a...) }

// page is an anonymous mapping outside the Go heap, so its contents are
// copied nowhere by the runtime and its address range can be skipped.
type page struct{ b []byte }

func newPage(size int) page {
	b, err := unix.Mmap(-1, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	if err != nil {
		panic(err)
	}
	return page{b}
}

func (p page) start() uintptr { return uintptrOf(p.b) }
func (p page) end() uintptr   { return p.start() + uintptr(len(p.b)) }

type mapping struct {
	start, end uintptr
	name       string
}

func writableMappings() []mapping {
	f, err := os.Open("/proc/self/maps")
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()
	var out []mapping
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 5 || !strings.HasPrefix(fields[1], "rw") {
			continue
		}
		bounds := strings.SplitN(fields[0], "-", 2)
		s, _ := strconv.ParseUint(bounds[0], 16, 64)
		e, _ := strconv.ParseUint(bounds[1], 16, 64)
		name := "[anonymous]"
		if len(fields) >= 6 {
			name = strings.Join(fields[5:], " ")
		}
		out = append(out, mapping{uintptr(s), uintptr(e), name})
	}
	return out
}

type hit struct {
	addr uintptr
	name string
}

// ranges is each writable mapping with the instrument's own pages cut out of
// it exactly. The kernel may merge those pages into a larger anonymous
// mapping, so skipping by mapping — or by chunk — could skip memory that is
// not the instrument's, which is what this must never be blind to.
func ranges(skip []page) []mapping {
	var out []mapping
	for _, m := range writableMappings() {
		parts := []mapping{m}
		for _, p := range skip {
			var next []mapping
			for _, r := range parts {
				if p.end() <= r.start || p.start() >= r.end {
					next = append(next, r)
					continue
				}
				if r.start < p.start() {
					next = append(next, mapping{r.start, p.start(), r.name})
				}
				if p.end() < r.end {
					next = append(next, mapping{p.end(), r.end, r.name})
				}
			}
			parts = next
		}
		out = append(out, parts...)
	}
	return out
}

// scan counts occurrences of the needle in every writable range that is not
// the instrument's own. It reads through /proc/self/mem into scratch, one
// chunk at a time with an overlap so a match across a boundary is not lost,
// and zeroes scratch after each so a copy never outlives the look.
func scan(needle, scratch page, skip []page) []hit {
	mem, err := os.Open("/proc/self/mem")
	if err != nil {
		panic(err)
	}
	defer func() { _ = mem.Close() }()
	n := needle.b[:needleLen]
	seen := map[uintptr]bool{}
	var hits []hit
	chunk := len(scratch.b)
	for _, r := range ranges(skip) {
		for off := r.start; off < r.end; off += uintptr(chunk - needleLen) {
			size := chunk
			if rest := int(r.end - off); rest < size {
				size = rest
			}
			got, _ := mem.ReadAt(scratch.b[:size], int64(off))
			buf := scratch.b[:got]
			for i := 0; ; {
				j := bytes.Index(buf[i:], n)
				if j < 0 {
					break
				}
				at := off + uintptr(i+j)
				if !seen[at] {
					seen[at] = true
					hits = append(hits, hit{at, r.name})
				}
				i += j + 1
			}
			clear(scratch.b)
			if size < chunk {
				break
			}
		}
	}
	return hits
}

func vmLck() string {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "?"
	}
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "VmLck:") {
			return strings.Join(strings.Fields(l)[1:], " ")
		}
	}
	return "?"
}

func describe(label string, hits []hit) {
	say("%-44s %d copies, VmLck=%s", label, len(hits), vmLck())
	for _, h := range hits {
		say("    at %#x in %s", h.addr, h.name)
	}
}

// imModules lists the input-method modules and IBus libraries mapped into
// this process — a fact about the process, not a guess from what is installed.
func imModules() []string {
	b, _ := os.ReadFile("/proc/self/maps")
	seen := map[string]bool{}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 6 {
			continue
		}
		if strings.Contains(f[5], "immodules") || strings.Contains(f[5], "libim-") || strings.Contains(f[5], "libibus") {
			if !seen[f[5]] {
				seen[f[5]] = true
				out = append(out, f[5])
			}
		}
	}
	return out
}

// watchIBus counts ProcessKeyEvent calls on IBus's own bus while it runs. It
// prints no message contents. If IBus is not running, or its bus refuses a
// monitor, it says so and counts nothing — which is then not evidence of
// anything.
func watchIBus() (stop func() (count int64, note string)) {
	addr, err := exec.Command("ibus", "address").Output()
	if err != nil || len(bytes.TrimSpace(addr)) == 0 {
		return func() (int64, string) { return 0, "IBus address not available (ibus not running?)" }
	}
	cmd := exec.Command("dbus-monitor", "--address", string(bytes.TrimSpace(addr)),
		"type='method_call',interface='org.freedesktop.IBus.InputContext',member='ProcessKeyEvent'")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return func() (int64, string) { return 0, "dbus-monitor: " + err.Error() }
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return func() (int64, string) { return 0, "dbus-monitor did not start: " + err.Error() }
	}
	var count atomic.Int64
	var refused atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			l := sc.Text()
			if strings.Contains(l, "member=ProcessKeyEvent") {
				count.Add(1)
			}
			if strings.Contains(l, "Failed") || strings.Contains(l, "AccessDenied") {
				refused.Store(true)
			}
		}
	}()
	return func() (int64, string) {
		_ = cmd.Process.Kill() // this child, by its own PID
		_ = cmd.Wait()
		wg.Wait()
		if refused.Load() {
			return count.Load(), "IBus's bus refused the monitor — the count is not evidence"
		}
		return count.Load(), fmt.Sprintf("dbus-monitor on IBus's bus, pid %d", cmd.Process.Pid)
	}
}

func main() {
	f, err := os.Create("pinmem-report.txt")
	if err == nil {
		report = io.MultiWriter(os.Stdout, f)
		defer func() { _ = f.Close() }()
	}

	needle := newPage(4096)
	scratch := newPage(1 << 20)
	dst := newPage(4096)
	skip := []page{needle, scratch, dst}

	if _, err := rand.Read(needle.b[:needleLen]); err != nil {
		panic(err)
	}
	for i := 0; i < needleLen; i++ {
		needle.b[i] = alphabet[int(needle.b[i])%len(alphabet)]
	}

	if len(os.Args) > 1 && os.Args[1] == "selftest" {
		selftest(needle, scratch, skip)
		return
	}

	say("pinmem — the real PIN dialog (internal/ui.CollectPIN), pid %d", os.Getpid())
	say("GTK_IM_MODULE=%q", os.Getenv("GTK_IM_MODULE"))
	say("")
	fmt.Print("Type exactly this into the dialog — NOT a PIN:\n\n    ")
	_, _ = unix.Write(1, needle.b[:needleLen]) // straight from its page; no formatted copy
	fmt.Print("\n\nthen wait until two lines in a row show the same count, and only then press OK.\n\n")

	describe("before the dialog (baseline):", scan(needle, scratch, skip))

	prompt := ui.PINPrompt{
		Title:   "pinmem",
		Heading: "Liro Bridge — pinmem (a measurement, not a PIN)",
		Subject: "Type the 20 characters shown in the terminal. Never a PIN.",
		Label:   "Test string",
		Hint:    "20 characters",
		OK:      "OK",
		Cancel:  "Cancel",
	}

	stopIBus := watchIBus()

	type answer struct {
		n   int
		ok  bool
		err error
	}
	done := make(chan answer, 1)
	go func() {
		n, ok, err := ui.CollectPIN(0, prompt, 32, dst.b)
		done <- answer{n, ok, err}
	}()

	time.Sleep(3 * time.Second)
	describe("dialog open, nothing typed (baseline):", scan(needle, scratch, skip))
	say("IM modules mapped: %v", imModules())

	var a answer
	tick := time.NewTicker(2 * time.Second)
wait:
	for {
		select {
		case a = <-done:
			break wait
		case <-tick.C:
			describe("while the dialog is up:", scan(needle, scratch, skip))
		}
	}
	tick.Stop()
	keys, note := stopIBus()

	say("")
	switch {
	case a.err != nil:
		say("CollectPIN: %v — nothing below is a measurement", a.err)
		return
	case !a.ok:
		say("cancelled — nothing below is a measurement")
		return
	case a.n != needleLen || !bytes.Equal(dst.b[:a.n], needle.b[:needleLen]):
		say("INVALID: the dialog returned %d bytes that are not the needle (a keyboard layout?). Every scan above would read zero and look clean; none of it is evidence.", a.n)
		clear(dst.b)
		return
	}
	say("returned: the needle, byte for byte (%d bytes)", a.n)

	describe("after OK, the returned copy still held:", scan(needle, scratch, skip))
	clear(dst.b)
	describe("after wiping the returned copy:", scan(needle, scratch, skip))
	say("IM modules mapped: %v", imModules())
	say("IBus ProcessKeyEvent calls while the dialog was up: %d (%s)", keys, note)

	clear(needle.b)
}

// selftest is the instrument's own control, with no dialog and no person:
// one copy of the needle planted in ordinary heap memory must be found
// exactly once, and none once it is wiped. Without it a zero from the real
// run could mean "no copies" or "cannot see copies" (D-304, question 1).
func selftest(needle, scratch page, skip []page) {
	describe("selftest, nothing planted:", scan(needle, scratch, skip))
	planted := make([]byte, needleLen)
	copy(planted, needle.b[:needleLen])
	hits := scan(needle, scratch, skip)
	describe("selftest, one copy planted on the heap:", hits)
	clear(planted)
	after := scan(needle, scratch, skip)
	describe("selftest, the planted copy wiped:", after)
	if len(hits) == 1 && len(after) == 0 {
		say("selftest: OK — finds one planted copy, and none after wiping")
	} else {
		say("selftest: FAILED — this instrument cannot be believed")
	}
	runtimeKeepAlive(planted)
}

func runtimeKeepAlive(b []byte) { runtime.KeepAlive(b) }

func uintptrOf(b []byte) uintptr { return uintptr(unsafe.Pointer(&b[0])) }
