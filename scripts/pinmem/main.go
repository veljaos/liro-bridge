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
// written to ./pinmem-report-<UTC time>.txt as well as printed.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
		say("    at %#x in %s, %s; in front: %q", h.addr, h.name, lockedAt(h.addr), inFront(h.addr))
	}
}

// lockedAt reads /proc/self/smaps for the mapping holding addr and reports
// its Locked size: whether a copy sits in locked memory is a reading, not a
// fit (D-382).
func lockedAt(addr uintptr) string {
	b, err := os.ReadFile("/proc/self/smaps")
	if err != nil {
		return "smaps unreadable"
	}
	inside := false
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 0 && strings.Contains(f[0], "-") && !strings.HasSuffix(f[0], ":") {
			bounds := strings.SplitN(f[0], "-", 2)
			s, err1 := strconv.ParseUint(bounds[0], 16, 64)
			e, err2 := strconv.ParseUint(bounds[1], 16, 64)
			inside = err1 == nil && err2 == nil && uintptr(s) <= addr && addr < uintptr(e)
			continue
		}
		if inside && len(f) >= 2 && f[0] == "Locked:" {
			return "mapping Locked " + f[1] + " kB"
		}
	}
	return "mapping not found in smaps"
}

// inFront is the 32 bytes before addr as printable characters, dots for the
// rest — a structure's header in front of a copy is often what names it.
// Nothing is masked: the needle is a test string the program itself shows,
// never a PIN.
func inFront(addr uintptr) string {
	mem, err := os.Open("/proc/self/mem")
	if err != nil {
		return ""
	}
	defer func() { _ = mem.Close() }()
	var b [32]byte
	n, _ := mem.ReadAt(b[:], int64(addr-32))
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		c := b[i]
		switch {
		case c >= 0x20 && c < 0x7f:
			out[i] = c
		default:
			out[i] = '.'
		}
	}
	clear(b[:])
	return string(out)
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

// ibusCounts is what crossed IBus's input-context interface while the dialog
// was up: calls by method name, key events split by press and release, and
// how many distinct senders made them. Never a key value (D-380).
type ibusCounts struct {
	byMember          map[string]int
	presses, releases int
	unparsed          int
	senders           map[string]bool
	note              string
	// who numbers the senders in the order they appeared. IBus's bus will
	// not say which process a sender is; ibusClients says whether this
	// process could be one (D-382).
	who       map[string]string
	perSender map[string]map[string]int
}

// ibusClients lists the processes holding a connection to ibus-daemon's
// socket, from the kernel's socket table: IBus's own bus refuses to say which
// process owns a connection (GetConnectionUnixProcessID: "does not support"),
// so a sender on it cannot be named — but whether THIS process is connected
// at all can be read here, and a process with no connection sent nothing
// (D-382).
func ibusClients() (clients []string, self bool) {
	addr, err := exec.Command("ibus", "address").Output()
	if err != nil {
		return []string{"ibus address unavailable"}, false
	}
	path := strings.TrimPrefix(strings.TrimSpace(string(addr)), "unix:path=")
	path, _, _ = strings.Cut(path, ",")
	out, err := exec.Command("ss", "-xp").Output()
	if err != nil {
		return []string{"ss: " + err.Error()}, false
	}
	type row struct{ local, peer, users string }
	var rows []row
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 8 {
			continue
		}
		users := ""
		if len(f) >= 9 {
			users = f[8]
		}
		rows = append(rows, row{f[5], f[7], users})
		if f[4] == path && len(f) >= 9 {
			rows[len(rows)-1].local = "daemon:" + f[5]
		}
	}
	mine := fmt.Sprintf("pid=%d,", os.Getpid())
	for _, d := range rows {
		if !strings.HasPrefix(d.local, "daemon:") {
			continue
		}
		for _, c := range rows {
			if c.local == d.peer {
				clients = append(clients, c.users)
				if strings.Contains(c.users, mine) {
					self = true
				}
			}
		}
	}
	return clients, self
}

// a11yBus is the accessibility bus's address, from the session bus.
func a11yBus() string {
	out, err := exec.Command("gdbus", "call", "--session", "--dest", "org.a11y.Bus",
		"--object-path", "/org/a11y/bus", "--method", "org.a11y.Bus.GetAddress").Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	v = strings.TrimPrefix(v, "('")
	v, _, _ = strings.Cut(v, "',")
	return v
}

// watchTextChanged records AT-SPI TextChanged signals on the accessibility
// bus into a file, from a separate dbus-monitor process. The payload of such a
// signal is the inserted or deleted text, so reading it while scanning would
// put copies of the needle into this process's own heap and the scans would
// count them; the file is read only after the last scan, then removed
// (D-382).
func watchTextChanged(bus string) (stop func() string) {
	if bus == "" {
		return func() string { return "" }
	}
	f, err := os.CreateTemp("", "pinmem-a11y-*.txt")
	if err != nil {
		return func() string { return "" }
	}
	cmd := exec.Command("dbus-monitor", "--address", bus,
		"type='signal',interface='org.a11y.atspi.Event.Object',member='TextChanged'")
	cmd.Stdout, cmd.Stderr = f, f
	if err := cmd.Start(); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return func() string { return "" }
	}
	return func() string {
		_ = cmd.Process.Kill() // this child, by its own PID
		_ = cmd.Wait()
		_ = f.Close()
		return f.Name()
	}
}

// textChangedReport reads the monitor's file after the last scan: each
// TextChanged signal's sender, resolved to a process while this one still
// holds its connection, its detail (insert or delete), and whether its
// payload is the needle — compared, never printed. The file is removed.
func textChangedReport(bus, file string, needle []byte) {
	if file == "" {
		say("accessibility bus: not watched (no address, or dbus-monitor did not start)")
		return
	}
	defer func() { _ = os.Remove(file) }()
	b, err := os.ReadFile(file)
	if err != nil {
		say("accessibility bus: %v", err)
		return
	}
	defer clear(b)
	type key struct{ sender, detail string }
	total := map[key]int{}
	carries := map[key]int{}
	var cur key
	inSignal := false
	for _, l := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.Contains(l, "member=TextChanged"):
			inSignal = true
			cur = key{}
			for _, f := range strings.Fields(l) {
				if v, ok := strings.CutPrefix(f, "sender="); ok {
					cur.sender = v
				}
			}
		case inSignal && cur.detail == "" && strings.HasPrefix(t, "string \""):
			cur.detail = strings.Trim(strings.TrimPrefix(t, "string "), "\"")
			total[cur]++
		case inSignal && strings.HasPrefix(t, "variant") && strings.Contains(t, "string \""):
			_, v, _ := strings.Cut(t, "string \"")
			v = strings.TrimSuffix(v, "\"")
			if v == string(needle) {
				carries[cur]++
			}
			inSignal = false
		}
	}
	if len(total) == 0 {
		say("accessibility bus: no TextChanged signals while the dialog was up")
		return
	}
	who := map[string]string{}
	for k, n := range total {
		if _, ok := who[k.sender]; !ok {
			who[k.sender] = resolveOn(bus, k.sender)
		}
		say("accessibility bus: %d TextChanged %q from %s (%s); payload is the needle in %d", n, k.detail, k.sender, who[k.sender], carries[k])
	}
}

// resolveOn asks a real dbus-daemon which process owns a unique name.
func resolveOn(bus, sender string) string {
	out, err := exec.Command("dbus-send", "--bus="+bus, "--dest=org.freedesktop.DBus", "--print-reply",
		"/org/freedesktop/DBus", "org.freedesktop.DBus.GetConnectionUnixProcessID", "string:"+sender).Output()
	if err != nil {
		return "unresolved"
	}
	f := strings.Fields(string(out))
	if len(f) == 0 {
		return "unresolved"
	}
	pid := f[len(f)-1]
	comm, _ := os.ReadFile("/proc/" + pid + "/comm")
	self := ""
	if pid == strconv.Itoa(os.Getpid()) {
		self = ", THIS PROCESS"
	}
	return "pid " + pid + " " + strings.TrimSpace(string(comm)) + self
}

// ibusReleaseMask is IBUS_RELEASE_MASK, bit 30 of ProcessKeyEvent's state.
const ibusReleaseMask = 1 << 30

// watchIBus monitors every method call on IBus's InputContext interface while
// it runs. dbus-monitor prints a header line naming sender and member, then
// one indented line per argument; ProcessKeyEvent's third argument is the
// state, whose release bit is all that is read of it. If IBus is not running,
// or its bus refuses a monitor, it says so and the counts are not evidence.
func watchIBus() (stop func() ibusCounts) {
	addr, err := exec.Command("ibus", "address").Output()
	if err != nil || len(bytes.TrimSpace(addr)) == 0 {
		return func() ibusCounts { return ibusCounts{note: "IBus address not available (ibus not running?)"} }
	}
	cmd := exec.Command("dbus-monitor", "--address", string(bytes.TrimSpace(addr)),
		"type='method_call',interface='org.freedesktop.IBus.InputContext'")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return func() ibusCounts { return ibusCounts{note: "dbus-monitor: " + err.Error()} }
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return func() ibusCounts { return ibusCounts{note: "dbus-monitor did not start: " + err.Error()} }
	}
	c := ibusCounts{byMember: map[string]int{}, senders: map[string]bool{}, who: map[string]string{}, perSender: map[string]map[string]int{}}
	var refused bool
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(out)
		member := ""
		var args []string
		flush := func() {
			if member == "ProcessKeyEvent" {
				if len(args) >= 3 {
					if state, err := strconv.ParseUint(args[2], 10, 64); err == nil {
						if state&ibusReleaseMask != 0 {
							c.releases++
						} else {
							c.presses++
						}
					} else {
						c.unparsed++
					}
				} else {
					c.unparsed++
				}
			}
			member, args = "", nil
		}
		for sc.Scan() {
			l := sc.Text()
			mu.Lock()
			switch {
			case strings.Contains(l, "Failed") || strings.Contains(l, "AccessDenied"):
				refused = true
			case strings.Contains(l, "member="):
				flush()
				sender := ""
				for _, f := range strings.FieldsFunc(l, func(r rune) bool { return r == ' ' || r == ';' }) {
					if v, ok := strings.CutPrefix(f, "member="); ok {
						member = v
						c.byMember[v]++
					}
					if v, ok := strings.CutPrefix(f, "sender="); ok {
						sender = v
						c.senders[v] = true
					}
				}
				if sender != "" {
					if _, known := c.who[sender]; !known {
						c.who[sender] = fmt.Sprintf("sender %d", len(c.who)+1)
						c.perSender[sender] = map[string]int{}
					}
					c.perSender[sender][member]++
				}
			case strings.HasPrefix(strings.TrimSpace(l), "uint32 "):
				args = append(args, strings.TrimPrefix(strings.TrimSpace(l), "uint32 "))
			}
			mu.Unlock()
		}
		mu.Lock()
		flush()
		mu.Unlock()
	}()
	return func() ibusCounts {
		_ = cmd.Process.Kill() // this child, by its own PID
		_ = cmd.Wait()
		wg.Wait()
		c.note = fmt.Sprintf("dbus-monitor on IBus's bus, pid %d", cmd.Process.Pid)
		if refused {
			c.note = "IBus's bus refused the monitor — the counts are not evidence"
		}
		if c.unparsed > 0 {
			c.note += fmt.Sprintf("; %d key events whose state could not be read — the press/release split is not evidence", c.unparsed)
		}
		return c
	}
}

func main() {
	// One file per run: a fixed name let a second run overwrite the first
	// run's report before it was read (D-380).
	f, err := os.Create("pinmem-report-" + time.Now().UTC().Format("20060102-150405") + ".txt")
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
	bus := a11yBus()
	stopA11y := watchTextChanged(bus)

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
	clients, self := ibusClients()
	say("IBus's clients with the dialog open: %v", clients)
	say("  this process connected to ibus-daemon: %v", self)

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
	ibus := stopIBus()
	a11yFile := stopA11y()

	say("")
	switch {
	case a.err != nil:
		say("CollectPIN: %v — nothing below is a measurement", a.err)
		textChangedReport(bus, a11yFile, needle.b[:needleLen])
		return
	case !a.ok:
		say("cancelled — nothing below is a measurement")
		textChangedReport(bus, a11yFile, needle.b[:needleLen])
		return
	case a.n != needleLen || !bytes.Equal(dst.b[:a.n], needle.b[:needleLen]):
		say("INVALID: the dialog returned %d bytes that are not the needle (a keyboard layout?). Every scan above would read zero and look clean; none of it is evidence.", a.n)
		clear(dst.b)
		textChangedReport(bus, a11yFile, needle.b[:needleLen])
		return
	}
	say("returned: the needle, byte for byte (%d bytes)", a.n)

	describe("after OK, the returned copy still held:", scan(needle, scratch, skip))
	clear(dst.b)
	describe("after wiping the returned copy:", scan(needle, scratch, skip))
	say("IM modules mapped: %v", imModules())
	say("IBus InputContext calls while the dialog was up, by method: %v", ibus.byMember)
	say("  of which key events: %d presses, %d releases; from %d distinct sender(s)", ibus.presses, ibus.releases, len(ibus.senders))
	clients, self = ibusClients()
	say("IBus's clients at the end: %v", clients)
	say("  this process connected to ibus-daemon: %v", self)
	for sender, calls := range ibus.perSender {
		say("  sender %s (%s): %v", sender, ibus.who[sender], calls)
	}
	say("  (%s)", ibus.note)

	// Last, after every scan: reading the monitor's file puts the payloads
	// into this process.
	textChangedReport(bus, a11yFile, needle.b[:needleLen])
	clear(needle.b)
}

// selftest is the instrument's own control, with no dialog and no person:
// one copy of the needle planted in ordinary heap memory must be found
// exactly once, and none once it is wiped. Without it a zero from the real
// run could mean "no copies" or "cannot see copies" (D-304, question 1).
func selftest(needle, scratch page, skip []page) {
	describe("selftest, nothing planted:", scan(needle, scratch, skip))
	// Planted through a package variable, so it lives on the heap and the
	// wipe below cannot be dropped: a local slice nothing reads afterwards
	// sat on the goroutine's stack, and the compiler removed its clear as a
	// dead store — the selftest failed on its own wipe, not on the scan
	// (D-382; SPEC §6.5.1 clause 2's elided loop, in the instrument).
	selftestPlanted = make([]byte, needleLen)
	planted := selftestPlanted
	copy(planted, needle.b[:needleLen])
	hits := scan(needle, scratch, skip)
	describe("selftest, one copy planted on the heap:", hits)
	clear(planted)
	after := scan(needle, scratch, skip)
	describe("selftest, the planted copy wiped:", after)
	// The connection check's own control: a plain connection to IBus's
	// socket from this process must be seen, and not once it is closed.
	_, before := ibusClients()
	connected := false
	if addr, err := exec.Command("ibus", "address").Output(); err == nil {
		path := strings.TrimPrefix(strings.TrimSpace(string(addr)), "unix:path=")
		path, _, _ = strings.Cut(path, ",")
		if conn, err := net.Dial("unix", path); err == nil {
			_, connected = ibusClients()
			_ = conn.Close()
		}
	}
	time.Sleep(200 * time.Millisecond)
	_, closed := ibusClients()
	say("selftest, IBus connection seen: before %v, while connected %v, after closing %v", before, connected, closed)
	if len(hits) == 1 && len(after) == 0 && !before && connected && !closed {
		say("selftest: OK — finds one planted copy and none after wiping, and sees its own IBus connection and not after")
	} else {
		say("selftest: FAILED — this instrument cannot be believed")
	}
	runtimeKeepAlive(planted)
}

func runtimeKeepAlive(b []byte) { runtime.KeepAlive(b) }

// selftestPlanted holds selftest's planted copy; see selftest.
var selftestPlanted []byte

func uintptrOf(b []byte) uintptr { return uintptr(unsafe.Pointer(&b[0])) }
